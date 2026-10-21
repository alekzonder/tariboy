package compose

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// planTaskQueues converges queues and their agent pools through the typed
// native Tasks REST commands, whose daemon handlers are backed by
// registry.TaskControl. Compose never reaches into task persistence directly.
func (p *planner) planTaskQueues(f File) error {
	if len(f.TaskQueues) == 0 {
		return nil
	}
	queues, err := p.r.listTaskQueues()
	if err != nil {
		return fmt.Errorf("task queues: %w", err)
	}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		want := f.TaskQueues[prefix]
		id := "queue:" + prefix
		queue, exists := queues[prefix]
		pools := map[string]tasks.AgentPool{}
		create := -1
		if !exists {
			p.status(id, true, "task queue %-10s MISSING (want name=%s)", prefix, want.Name)
			create = p.add(Action{Resource: id, Kind: "create", Note: "creating task queue " + prefix, Fail: "create task queue " + prefix,
				Call: Call{"POST", "/api/task-queues", map[string]any{"prefix": prefix, "name": want.Name}}})
		} else {
			p.resource(id).queue = prefix
			if queue.Name != want.Name {
				p.status(id, true, "task queue %-10s name drift: have=%s want=%s", prefix, queue.Name, want.Name)
				p.add(Action{Resource: id, Kind: "rename", Note: "updating task queue " + prefix, Fail: "update task queue " + prefix,
					Call: Call{"PATCH", "/api/task-queues/" + prefix, map[string]any{"name": want.Name, "revision": queue.Revision}}})
			}
			if pools, err = p.r.listTaskPools(prefix); err != nil {
				return fmt.Errorf("task queue %s pools: %w", prefix, err)
			}
		}
		for _, poolName := range sortedKeys(want.Pools) {
			agents := normalizeAgentNames(want.Pools[poolName])
			current, found := pools[poolName]
			switch {
			case found && slices.Equal(current.Agents, agents):
				p.status(id, false, "task queue %-10s pool %s ok", prefix, poolName)
				continue
			case found:
				p.status(id, true, "task queue %-10s pool %s drift: have=%v want=%v", prefix, poolName, current.Agents, agents)
			case exists:
				p.status(id, true, "task queue %-10s pool %s MISSING (want=%v)", prefix, poolName, agents)
			}
			p.add(Action{Resource: id, Kind: "pool", Note: fmt.Sprintf("binding task queue %s pool %s", prefix, poolName),
				Fail: fmt.Sprintf("bind pool %s/%s", prefix, poolName),
				Call: Call{"PATCH", "/api/task-queues/" + prefix + "/pools/" + poolName, map[string]any{"agents": agents, "revision": current.Revision,
					"idempotency_key": composeIdempotency("pool", prefix, poolName, current.Revision, agents)}}}, create)
		}
	}
	return nil
}

// planQueueWorkflows builds the Store workflows the file names, then binds
// each queue to the digest it resolves to. The binds follow the queue and
// pool actions, since a bind needs the pools the workflow names, and depend
// on them. A queue with no workflow value keeps whatever binding it has;
// Compose never clears one.
func (p *planner) planQueueWorkflows(f File) error {
	built := map[string]int{}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		ref, declared, err := f.TaskQueues[prefix].WorkflowRef()
		if err != nil {
			return fmt.Errorf("task queue %s workflow: %w", prefix, err)
		}
		if _, done := built[ref.Selector()]; declared && ref.Store != "" && !done {
			built[ref.Selector()] = p.add(Action{Resource: "workflow:" + ref.Selector(), Kind: "build", Note: "building workflow " + ref.Selector(),
				Fail: "build workflow " + ref.Selector(), Call: Call{"POST", "/api/workflow-images/build", map[string]any{"source": ref.Selector()}}})
		}
	}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		ref, declared, _ := f.TaskQueues[prefix].WorkflowRef()
		if !declared {
			continue
		}
		id := "queue:" + prefix
		build := -1
		if ref.Store != "" {
			build = built[ref.Selector()]
		}
		p.add(Action{Resource: id, Kind: "workflow", run: p.r.bindWorkflow(prefix, ref, build)}, append(p.actionsOf(id), build)...)
	}
	return nil
}

// actionsOf returns the indexes of the actions planned so far for a resource.
func (p *planner) actionsOf(id string) []int {
	var out []int
	for i, a := range p.plan.Actions {
		if a.Resource == id {
			out = append(out, i)
		}
	}
	return out
}

// bindWorkflow resolves and binds the workflow of one queue at apply time. A
// Store workflow without a tag binds the digest its build (action build)
// returned; every other form resolves NAME:TAG after the builds and binds by
// the tag, so the queue follows it.
func (r *Runner) bindWorkflow(prefix string, ref WorkflowRef, build int) func([]json.RawMessage) (bool, error) {
	return func(results []json.RawMessage) (bool, error) {
		var img workflowImage
		if build >= 0 {
			if err := json.Unmarshal(results[build], &img); err != nil {
				return false, err
			}
		}
		bindRef, tag := img.Name+":"+img.Digest, ""
		if ref.Store == "" || ref.Tag != "" {
			bindRef = ref.Ref()
			tag = bindRef[strings.LastIndex(bindRef, ":")+1:]
			var found bool
			var err error
			if img, found, err = r.inspectWorkflowImage(ref.Ref()); err != nil {
				return false, fmt.Errorf("resolve workflow %s: %w", ref.Ref(), err)
			}
			if !found && ref.Store != "" {
				return false, fmt.Errorf("task queue %s workflow: %s does not exist after building %s", prefix, ref.Ref(), ref.Selector())
			}
			if !found {
				return false, fmt.Errorf("task queue %s workflow: %s is not built; build it or add the Store prefix (STORE/%s)", prefix, ref.Ref(), ref.Ref())
			}
		}
		current, bound, err := r.queueWorkflow(prefix)
		if err != nil {
			return false, fmt.Errorf("read workflow of task queue %s: %w", prefix, err)
		}
		if bound && current.Digest == img.Digest && current.Tag == tag {
			r.logf("task queue %s workflow %s:%s unchanged", prefix, img.Name, img.Version)
			return false, nil
		}
		revision := int64(0)
		if bound {
			revision = current.Revision
		}
		r.logf("binding task queue %s workflow %s:%s", prefix, img.Name, img.Version)
		if _, err := r.call.Call("PUT", "/api/task-queues/"+prefix+"/workflow", map[string]any{"ref": bindRef, "revision": revision}); err != nil {
			return false, bindWorkflowError(prefix, ref, err)
		}
		return true, nil
	}
}

// workflowImage is the part of a workflow image the binding step needs.
type workflowImage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// shortDigest is the form a digest takes in status and progress output.
func shortDigest(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

func isAPIError(err error, code string) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

// inspectWorkflowImage resolves NAME:TAG through the daemon; found is false
// when the image or the tag does not exist.
func (r *Runner) inspectWorkflowImage(ref string) (img workflowImage, found bool, err error) {
	name, tag, _ := strings.Cut(ref, ":")
	err = r.get("/api/workflow-images/"+name+"/"+tag, &img)
	if isAPIError(err, "not_found") {
		return workflowImage{}, false, nil
	}
	if err != nil {
		return workflowImage{}, false, err
	}
	return img, true, nil
}

// storeWorkflowVersion reads the workflow_version of a Store workflow source
// as the daemon reads it from the Store's checkout; ok is false when the Store
// or the source cannot be read.
func (r *Runner) storeWorkflowVersion(ref WorkflowRef) (string, bool) {
	var detail struct {
		Workflows []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Error   string `json:"error"`
		} `json:"workflows"`
	}
	if err := r.get("/api/stores/"+ref.Store, &detail); err != nil {
		return "", false
	}
	for _, workflow := range detail.Workflows {
		if workflow.Name == ref.Name && workflow.Error == "" && workflow.Version != "" {
			return workflow.Version, true
		}
	}
	return "", false
}

// queueWorkflow reads the binding of a queue; bound is false when it has none.
func (r *Runner) queueWorkflow(queue string) (tasks.QueueWorkflow, bool, error) {
	var binding tasks.QueueWorkflow
	err := r.get("/api/task-queues/"+queue+"/workflow", &binding)
	if isAPIError(err, "queue_workflow_not_found") {
		return tasks.QueueWorkflow{}, false, nil
	}
	if err != nil {
		return tasks.QueueWorkflow{}, false, err
	}
	return binding, true, nil
}

// bindWorkflowError keeps the daemon's error code and message, and for a
// missing secret names the secrets and how to set one. Compose never handles
// a secret value: the operator sets it with the Tasks CLI.
func bindWorkflowError(queue string, ref WorkflowRef, err error) error {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "workflow_secret_missing" {
		return fmt.Errorf("bind workflow %s to task queue %s: %w", ref.Ref(), queue, err)
	}
	var names []string
	if list, ok := apiErr.Details["secrets"].([]any); ok {
		for _, item := range list {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
	}
	key := "KEY"
	if len(names) > 0 {
		key = names[0]
	}
	return fmt.Errorf("bind workflow %s to task queue %s: %w\nmissing secrets: %s\nset one with the value on standard input, for example:\n  printf '%%s' \"$VALUE\" | ttasks queue secret set %s %s",
		ref.Ref(), queue, err, strings.Join(names, ", "), queue, key)
}

func composeIdempotency(parts ...any) string {
	encoded, err := json.Marshal(parts)
	if err != nil {
		panic(fmt.Sprintf("compose idempotency inputs: %v", err))
	}
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("compose:%x", sum[:])
}

func (r *Runner) listTaskQueues() (map[string]tasks.Queue, error) {
	var env struct {
		Queues []tasks.Queue `json:"queues"`
	}
	if err := r.get("/api/task-queues", &env); err != nil {
		return nil, err
	}
	out := map[string]tasks.Queue{}
	for _, queue := range env.Queues {
		out[queue.Prefix] = queue
	}
	return out, nil
}

func (r *Runner) listTaskPools(queue string) (map[string]tasks.AgentPool, error) {
	var env struct {
		Items []tasks.AgentPool `json:"items"`
	}
	if err := r.get("/api/task-queues/"+queue+"/pools", &env); err != nil {
		return nil, err
	}
	out := map[string]tasks.AgentPool{}
	for _, pool := range env.Items {
		out[pool.Name] = pool
	}
	return out, nil
}
