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

// queuesPhase creates and updates the task queues through the typed native
// Tasks REST commands, whose daemon handlers are backed by
// registry.TaskControl. Compose never reaches into task persistence directly.
// It sets the name, description, owners, plain env and secrets; pools,
// CUSTOMER_AGENT_POOL, the responsible agent, the workflow binding and the
// triggers follow in the queue bindings phase, once the agents exist.
type queuesPhase struct{}

func (queuesPhase) name() string { return "queues" }

func (queuesPhase) plan(p *planner, f File, live *liveState) error {
	for _, prefix := range sortedKeys(f.TaskQueues) {
		want := f.TaskQueues[prefix]
		id := "queue:" + prefix
		if queue, exists := live.queues[prefix]; exists {
			p.resource(id).queue = prefix
			p.planQueueUpdate(prefix, want, queue)
		} else {
			p.planQueueCreate(prefix, want)
		}
		p.planQueueEnv(prefix, want, live, false)
		p.planQueueSecrets(prefix, want, live)
	}
	return nil
}

func (p *planner) planQueueCreate(prefix string, want TaskQueueSpec) {
	id := "queue:" + prefix
	p.status(id, true, "task queue %-10s MISSING (want name=%s)", prefix, want.Name)
	body := map[string]any{"prefix": prefix, "name": want.Name}
	fields := []Field{{Name: "name", To: want.Name}}
	if desc := strings.TrimSpace(want.Description); desc != "" {
		body["description"] = desc
		fields = append(fields, Field{Name: "description", To: desc})
	}
	if owners := normalizeAgentNames(want.Owners); len(owners) > 0 {
		body["owners"] = owners
		fields = append(fields, Field{Name: "owners", To: "[" + strings.Join(owners, ", ") + "]"})
	}
	p.provide(id, p.add(Action{Resource: id, Kind: "create", Note: "creating task queue " + prefix, Fail: "create task queue " + prefix,
		Fields: fields, Call: Call{"POST", "/api/task-queues", body}}))
}

// planQueueUpdate sends one PATCH with every declared field that differs:
// name, description and owners. An empty description or owners list in the
// file leaves the daemon's value.
func (p *planner) planQueueUpdate(prefix string, want TaskQueueSpec, queue tasks.Queue) {
	id := "queue:" + prefix
	body := map[string]any{"revision": queue.Revision}
	var fields []Field
	if queue.Name != want.Name {
		p.status(id, true, "task queue %-10s name drift: have=%s want=%s", prefix, queue.Name, want.Name)
		body["name"] = want.Name
		fields = append(fields, Field{Name: "name", From: queue.Name, To: want.Name})
	}
	// The daemon trims the description, so the file's value is compared trimmed.
	if desc := strings.TrimSpace(want.Description); desc != "" && queue.Description != desc {
		p.status(id, true, "task queue %-10s description drift: have=%q want=%q", prefix, queue.Description, desc)
		body["description"] = desc
		fields = append(fields, Field{Name: "description", From: queue.Description, To: desc})
	}
	if owners := normalizeAgentNames(want.Owners); len(owners) > 0 && !slices.Equal(normalizeAgentNames(queue.Owners), owners) {
		have, to := "["+strings.Join(queue.Owners, ", ")+"]", "["+strings.Join(owners, ", ")+"]"
		p.status(id, true, "task queue %-10s owners drift: have=%s want=%s", prefix, have, to)
		body["owners"] = owners
		fields = append(fields, Field{Name: "owners", From: have, To: to})
	}
	if len(fields) == 0 {
		return
	}
	p.add(Action{Resource: id, Kind: "update", Note: "updating task queue " + prefix, Fail: "update task queue " + prefix,
		Fields: fields, Call: Call{"PATCH", "/api/task-queues/" + prefix, body}})
}

// bindWorkflow resolves and binds the workflow of one queue at apply time. A
// Store workflow without a tag binds the digest its build (action build)
// returned; every other form resolves NAME:TAG after the builds and binds by
// the tag, so the queue follows it.
func (r *Runner) bindWorkflow(prefix string, ref WorkflowRef, build int) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func(results []json.RawMessage) (json.RawMessage, bool, error) {
		changed, err := r.resolveAndBind(prefix, ref, build, results)
		return nil, changed, err
	}
}

func (r *Runner) resolveAndBind(prefix string, ref WorkflowRef, build int, results []json.RawMessage) (bool, error) {
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
// missing secret names the secrets and how to set one: with the Tasks CLI, or
// through secrets.KEY.from_env in the file.
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
