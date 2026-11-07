package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/tasks"
)

type attributionTaskReader struct {
	tasks map[string]tasks.Task
	err   map[string]error
	seen  []tasks.Actor
	reads []string
}

func (*attributionTaskReader) CustomerLogin() string { return "customer" }

type attributionProxy struct {
	key, task, epic string
	updated         int
	changed         bool
}

func (p *attributionProxy) UpdateTask(key, task, epic string) int {
	p.key, p.task, p.epic = key, task, epic
	return p.updated
}

func (p *attributionProxy) SetTaskIfEmpty(key, task, epic string) (int, bool) {
	p.key, p.task, p.epic = key, task, epic
	return p.updated, p.changed
}

func (r *attributionTaskReader) GetTask(_ context.Context, actor tasks.Actor, key string) (tasks.TaskDetail, error) {
	r.seen = append(r.seen, actor)
	r.reads = append(r.reads, actor.Principal+" "+key)
	if err := r.err[key]; err != nil {
		return tasks.TaskDetail{}, err
	}
	task, ok := r.tasks[key]
	if !ok {
		return tasks.TaskDetail{}, errors.New("task not found")
	}
	return tasks.TaskDetail{Task: task}, nil
}

// TestResolveNativeTaskAttributionReadsGoalAsAgentAndAncestorsAsCustomer: the
// goal is read with the agent's rights, its ancestors with the customer's, and
// the root once more as the agent to learn whether the agent may see it.
func TestResolveNativeTaskAttributionReadsGoalAsAgentAndAncestorsAsCustomer(t *testing.T) {
	t.Parallel()
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{
		"SUPER-1": {Key: "SUPER-1"},
		"SUPER-2": {Key: "SUPER-2", ParentKey: "SUPER-1"},
		"SUPER-3": {Key: "SUPER-3", ParentKey: "SUPER-2"},
	}}

	tests := []struct {
		name      string
		key       string
		wantTask  string
		wantEpic  string
		wantReads []string
	}{
		{name: "root", key: "SUPER-1", wantTask: "SUPER-1", wantEpic: "SUPER-1",
			wantReads: []string{"agent:lively-quokka SUPER-1"}},
		{name: "child", key: "SUPER-2", wantTask: "SUPER-2", wantEpic: "SUPER-1",
			wantReads: []string{"agent:lively-quokka SUPER-2", "user:customer SUPER-1", "agent:lively-quokka SUPER-1"}},
		{name: "grandchild", key: "SUPER-3", wantTask: "SUPER-3", wantEpic: "SUPER-1",
			wantReads: []string{"agent:lively-quokka SUPER-3", "user:customer SUPER-2", "user:customer SUPER-1", "agent:lively-quokka SUPER-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader.reads = nil
			got, err := resolveNativeTaskAttribution(context.Background(), reader, "lively-quokka", tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if got.TaskID != tt.wantTask || got.EpicID != tt.wantEpic || !got.EpicVisible || got.Partial != nil {
				t.Fatalf("attribution = %+v, want %q/%q visible", got, tt.wantTask, tt.wantEpic)
			}
			if strings.Join(reader.reads, ",") != strings.Join(tt.wantReads, ",") {
				t.Fatalf("reads = %v, want %v", reader.reads, tt.wantReads)
			}
		})
	}
}

// TestResolveNativeTaskAttributionStopsAtUnreadableAncestor: an ancestor the
// customer cannot read either ends the walk; the last read ancestor becomes the
// root and the cause is kept in Partial, not returned as an error.
func TestResolveNativeTaskAttributionStopsAtUnreadableAncestor(t *testing.T) {
	t.Parallel()
	cause := errors.New("storage down")
	reader := &attributionTaskReader{
		tasks: map[string]tasks.Task{
			"SUPER-2": {Key: "SUPER-2", ParentKey: "SUPER-1"},
			"SUPER-3": {Key: "SUPER-3", ParentKey: "SUPER-2"},
		},
		err: map[string]error{"SUPER-1": cause},
	}
	got, err := resolveNativeTaskAttribution(context.Background(), reader, "worker", "SUPER-3")
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != "SUPER-3" || got.EpicID != "SUPER-2" || !errors.Is(got.Partial, cause) {
		t.Fatalf("attribution = %+v, want SUPER-3/SUPER-2 with partial %v", got, cause)
	}
}

func TestResolveNativeTaskAttributionPropagatesLookupError(t *testing.T) {
	t.Parallel()
	want := errors.New("native task unavailable")
	reader := &attributionTaskReader{err: map[string]error{"SUPER-9": want}}
	_, err := resolveNativeTaskAttribution(context.Background(), reader, "worker", "SUPER-9")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestResolveNativeTaskAttributionRejectsParentCycle(t *testing.T) {
	t.Parallel()
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{
		"SUPER-1": {Key: "SUPER-1", ParentKey: "SUPER-2"},
		"SUPER-2": {Key: "SUPER-2", ParentKey: "SUPER-1"},
	}}
	_, err := resolveNativeTaskAttribution(context.Background(), reader, "worker", "SUPER-1")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error = %v, want parent cycle error", err)
	}
}

func TestResolveNativeTaskAttributionUsesRealServiceAuthorization(t *testing.T) {
	t.Parallel()
	state := storetest.Open(t)
	service := tasks.NewService(state.DB, "customer", func() time.Time {
		return time.Date(2026, 8, 3, 6, 0, 0, 0, time.UTC)
	})
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	if _, err := service.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: "SUPER", Name: "Super"}); err != nil {
		t.Fatal(err)
	}
	root, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "SUPER", Title: "root", Assignee: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{ParentKey: root.Key, Title: "child"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolveNativeTaskAttribution(ctx, service, "worker", child.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != child.Key || got.EpicID != root.Key || !got.EpicVisible {
		t.Fatalf("attribution=%+v, want %q/%q visible", got, child.Key, root.Key)
	}
	if _, err := resolveNativeTaskAttribution(ctx, service, "outsider", child.Key); tasks.ErrorCode(err) != "not_found" {
		t.Fatalf("outsider error=%v code=%q, want not_found", err, tasks.ErrorCode(err))
	}
}

// crossQueueGoal is a task tree whose root and parent live in queue ROOTQ and
// whose leaf, assigned to agent, lives in queue WORKQ: the agent sees only the
// leaf.
type crossQueueGoal struct {
	service            *tasks.Service
	root, parent, leaf tasks.Task
}

func newCrossQueueGoal(t *testing.T, agentName string) crossQueueGoal {
	t.Helper()
	state := storetest.Open(t)
	service := tasks.NewService(state.DB, "customer", func() time.Time {
		return time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)
	})
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	for _, prefix := range []string{"ROOTQ", "WORKQ"} {
		if _, err := service.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: prefix, Name: prefix}); err != nil {
			t.Fatal(err)
		}
	}
	root, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "ROOTQ", Title: "project"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{ParentKey: root.Key, Title: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{ParentKey: parent.Key, Queue: "WORKQ",
		Title: "order", Assignee: agentName})
	if err != nil {
		t.Fatal(err)
	}
	agentActor := tasks.AgentActor(agentName)
	if _, err := service.GetTask(ctx, agentActor, parent.Key); tasks.ErrorCode(err) != "not_found" {
		t.Fatalf("agent reads its parent in another queue: %v", err)
	}
	if _, err := service.GetTask(ctx, agentActor, root.Key); tasks.ErrorCode(err) != "not_found" {
		t.Fatalf("agent reads the root in another queue: %v", err)
	}
	return crossQueueGoal{service: service, root: root, parent: parent, leaf: leaf}
}

func TestResolveNativeTaskAttributionReachesRootBehindHiddenParent(t *testing.T) {
	t.Parallel()
	tree := newCrossQueueGoal(t, "worker")
	got, err := resolveNativeTaskAttribution(context.Background(), tree.service, "worker", tree.leaf.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.TaskID != tree.leaf.Key || got.EpicID != tree.root.Key || got.EpicVisible || got.Partial != nil {
		t.Fatalf("attribution=%+v, want %q/%q hidden", got, tree.leaf.Key, tree.root.Key)
	}
	if _, err := resolveNativeTaskAttribution(context.Background(), tree.service, "outsider", tree.leaf.Key); tasks.ErrorCode(err) != "not_found" {
		t.Fatalf("outsider error=%v, want not_found", err)
	}
}

// TestSetGoalHidesARootTheAgentCannotSee: the proxy gets the real root, and
// no field of the answer carries its key or the hidden parent's.
func TestSetGoalHidesARootTheAgentCannotSee(t *testing.T) {
	t.Parallel()
	tree := newCrossQueueGoal(t, "worker")
	proxy := &attributionProxy{updated: 1, changed: true}
	selected := ""
	result, err := setGoal(context.Background(), nil, tree.service, func(_, key string, activate func() (func(), error)) error {
		if _, err := activate(); err != nil {
			return err
		}
		selected = key
		return nil
	}, proxy, "iter-1", "worker", tree.leaf.Key)
	if err != nil {
		t.Fatal(err)
	}
	if selected != tree.leaf.Key || proxy.task != tree.leaf.Key || proxy.epic != tree.root.Key {
		t.Fatalf("selected=%q proxy=%+v, want %s/%s", selected, proxy, tree.leaf.Key, tree.root.Key)
	}
	if result["task_id"] != tree.leaf.Key || result["epic_id"] != "" || result["updated"] != 1 {
		t.Fatalf("result=%+v", result)
	}
	answer, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{tree.root.Key, tree.parent.Key} {
		if strings.Contains(string(answer), hidden) {
			t.Fatalf("answer %s names hidden task %s", answer, hidden)
		}
	}
}

// TestToolsFacadeSetGoalWarnsOnPartialAttribution: an unreadable ancestor does
// not fail goal set; the daemon log names the agent, goal, root and cause.
func TestToolsFacadeSetGoalWarnsOnPartialAttribution(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	proxy := &facadeGoalProxy{attributionProxy: attributionProxy{updated: 1, changed: true}}
	f := toolsFacade{agent: "worker", iterations: fixedIteration("iter-1"), goals: noGoalRecorder{}, cfg: &ManagerConfig{
		Log:   slog.New(slog.NewTextHandler(&logs, nil)),
		Proxy: proxy,
		Tasks: &partialTaskControl{attributionTaskReader: attributionTaskReader{
			tasks: map[string]tasks.Task{"SUPER-2": {Key: "SUPER-2", ParentKey: "SUPER-1"}},
			err:   map[string]error{"SUPER-1": errors.New("storage down")},
		}},
		SetGoal: func(_, _ string, activate func() (func(), error)) error {
			_, err := activate()
			return err
		},
	}}
	result, err := f.setGoal("SUPER-2")
	if err != nil {
		t.Fatal(err)
	}
	if result["task_id"] != "SUPER-2" || proxy.epic != "SUPER-2" {
		t.Fatalf("result=%v proxy=%+v", result, proxy.attributionProxy)
	}
	assertPartialAttributionWarning(t, logs.String(), "worker", "SUPER-2", "SUPER-2", "storage down")
}

func assertPartialAttributionWarning(t *testing.T, logs, agentName, goal, root, cause string) {
	t.Helper()
	for _, want := range []string{"level=WARN", "agent=" + agentName, "goal=" + goal, "root=" + root, "err=\"" + cause + "\""} {
		if !strings.Contains(logs, want) {
			t.Fatalf("log %q lacks %q", logs, want)
		}
	}
}

type facadeGoalProxy struct {
	attributionProxy
}

func (*facadeGoalProxy) ProxyBaseURL() string                           { return "" }
func (*facadeGoalProxy) MintToken(_, _, _, _, _ string) (string, error) { return "", nil }
func (*facadeGoalProxy) RevokeToken(string)                             {}
func (*facadeGoalProxy) RevokeIteration(string)                         {}

type noGoalRecorder struct{}

func (noGoalRecorder) noteSelectedGoal(string, string, string)     {}
func (noGoalRecorder) iterationTask(string, string) (string, bool) { return "", false }

// partialTaskControl satisfies registry.TaskControl for the facade; only
// GetTask and CustomerLogin are used by goal set.
type partialTaskControl struct {
	registry.TaskControl
	attributionTaskReader
}

func (p *partialTaskControl) CustomerLogin() string { return p.attributionTaskReader.CustomerLogin() }
func (p *partialTaskControl) GetTask(ctx context.Context, actor tasks.Actor, key string) (tasks.TaskDetail, error) {
	return p.attributionTaskReader.GetTask(ctx, actor, key)
}

func TestSetGoalUpdatesSelectionAndLiveProxyAttribution(t *testing.T) {
	t.Parallel()
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{
		"T-1": {Key: "T-1"},
		"T-2": {Key: "T-2", ParentKey: "T-1"},
	}}
	proxy := &attributionProxy{updated: 1, changed: true}
	selected := ""
	result, err := setGoal(context.Background(), nil, reader, func(agent, key string, activate func() (func(), error)) error {
		if agent != "worker" {
			t.Fatalf("agent=%q", agent)
		}
		if _, err := activate(); err != nil {
			return err
		}
		selected = key
		return nil
	}, proxy, "iter-1", "worker", "T-2")
	if err != nil {
		t.Fatal(err)
	}
	if selected != "T-2" || proxy.key != "iter-1" || proxy.task != "T-2" || proxy.epic != "T-1" {
		t.Fatalf("selected=%q proxy=%+v", selected, proxy)
	}
	if result["task_id"] != "T-2" || result["epic_id"] != "T-1" || result["updated"] != 1 {
		t.Fatalf("result=%v", result)
	}
}

// TestSetGoalRefusesATaskReadOnlyAsAFormerHolder: a workflow holder row lets
// an agent read its former task, never select it. A task the agent reads as
// the ancestor of its own work resolves as it always did.
func TestSetGoalRefusesATaskReadOnlyAsAFormerHolder(t *testing.T) {
	t.Parallel()
	state := storetest.Open(t)
	service := tasks.NewService(state.DB, "customer", func() time.Time {
		return time.Date(2026, 10, 2, 6, 0, 0, 0, time.UTC)
	})
	ctx := context.Background()
	customer := tasks.CustomerActor("customer")
	if _, err := service.CreateQueue(ctx, customer, tasks.CreateQueueInput{Prefix: "SUPER", Name: "Super"}); err != nil {
		t.Fatal(err)
	}
	root, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "SUPER", Title: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{ParentKey: root.Key, Title: "child", Assignee: "worker"}); err != nil {
		t.Fatal(err)
	}
	former, err := service.CreateTask(ctx, customer, tasks.CreateTaskInput{Queue: "SUPER", Title: "former"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.DB.Exec(`INSERT INTO task_workflow_holders(task_id, pool, agent, dispatched_at) VALUES (?, 'devs', 'worker', 'now')`,
		former.ID); err != nil {
		t.Fatal(err)
	}
	if detail, err := service.GetTask(ctx, tasks.AgentActor("worker"), former.Key); err != nil || detail.Task.Access != "context" {
		t.Fatalf("former holder read = %q, %v", detail.Task.Access, err)
	}

	var selected []string
	selectGoal := func(_, key string, _ func() (func(), error)) error {
		selected = append(selected, key)
		return nil
	}
	proxy := &attributionProxy{updated: 1, changed: true}
	if _, err := setGoal(ctx, nil, service, selectGoal, proxy, "iter-1", "worker", former.Key); err == nil {
		t.Fatal("a former holder selected its task as the goal")
	}
	if _, err := setGoal(ctx, nil, service, selectGoal, proxy, "iter-1", "worker", root.Key); err != nil {
		t.Fatalf("ancestor context reader: %v", err)
	}
	if len(selected) != 1 || selected[0] != root.Key {
		t.Fatalf("selected = %v; want only %s", selected, root.Key)
	}
}

func TestSetGoalDoesNotPersistWhenLiveLeaseCannotBeUpdated(t *testing.T) {
	t.Parallel()
	reader := &attributionTaskReader{tasks: map[string]tasks.Task{"T-1": {Key: "T-1"}}}
	proxy := &attributionProxy{}
	selected := false
	_, err := setGoal(context.Background(), nil, reader, func(_, _ string, activate func() (func(), error)) error {
		if _, err := activate(); err != nil {
			return err
		}
		selected = true
		return nil
	}, proxy, "iter-1", "worker", "T-1")
	if err == nil || selected {
		t.Fatalf("err=%v selected=%t", err, selected)
	}
}
