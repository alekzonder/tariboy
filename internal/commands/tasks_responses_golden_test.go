package commands

import (
	"context"
	"encoding/json"
	"flag"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

var updateTasksGolden = flag.Bool("update-tasks-golden", false, "rewrite testdata/tasks_responses.golden.json")

// tasksGoldenVolatile names keys whose values are random per run.
var tasksGoldenVolatile = map[string]bool{"event_id": true}

// tasksGoldenClock is the fixed time of the tasks golden fixture.
var tasksGoldenClock = time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

// claimingStub serves the native Tasks service with the workflow stub in
// front of it, and claims as agent dev-a: the operator route acts as the
// customer, who may not claim.
type claimingStub struct{ *workflowStub }

func (s claimingStub) ClaimTask(ctx context.Context, _ tasks.Actor, key string, revision int64) (tasks.Task, error) {
	return s.TaskControl.ClaimTask(ctx, tasks.AgentActor("dev-a"), key, revision)
}

// tasksGoldenCall is one task command call of the golden fixture.
type tasksGoldenCall struct {
	label, path string
	params      registry.Params
}

// tasksFixture returns a context backed by the native Tasks service on a
// fresh store, with agent dev-a, the workflow stub in front of the service,
// and claims made as dev-a.
func tasksFixture(t *testing.T) (*registry.Ctx, *tasks.Service) {
	t.Helper()
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "dev-a", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	svc := tasks.NewService(c.Store.DB, "customer", func() time.Time { return tasksGoldenClock })
	c.Tasks = claimingStub{&workflowStub{TaskControl: svc}}
	return c, svc
}

// Catches a task command response changing the JSON a client sees: the set
// of keys, their JSON types, and their values must match the recorded
// responses. The native service answers the task, queue, comment, relation,
// event, pool and trigger commands; the workflow stub answers
// the commands that need a workflow engine.
func TestTasksResponsesMatchGolden(t *testing.T) {
	c, svc := tasksFixture(t)
	recorder := newGoldenRecorder(t, tasksGoldenVolatile)
	run := func(call tasksGoldenCall) any {
		t.Helper()
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, value)
		return value
	}
	runAll := func(calls ...tasksGoldenCall) {
		t.Helper()
		for _, call := range calls {
			run(call)
		}
	}
	keyOf := func(value any) string {
		t.Helper()
		var task struct {
			Key string `json:"key"`
		}
		raw, _ := json.Marshal(value)
		if err := json.Unmarshal(raw, &task); err != nil || task.Key == "" {
			t.Fatalf("no task key in %s", raw)
		}
		return task.Key
	}
	// Task keys are random: create each task outside the recorder first, so
	// the replacement exists before any response carries the key.
	create := func(label, alias string, params registry.Params) string {
		t.Helper()
		value, err := h(t, "tasks.create")(c, params)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		key := keyOf(value)
		recorder.replacements = append(recorder.replacements, key, alias)
		recorder.record(label, value)
		return key
	}

	runAll(
		tasksGoldenCall{"tasks.queue.list empty", "tasks.queue.list", registry.Params{}},
		tasksGoldenCall{"tasks.queue.create GLD", "tasks.queue.create", registry.Params{"prefix": "GLD", "name": "Golden", "description": "pinned", "owners": []any{"dev-a"}, "responsible_agent": "dev-a"}},
		tasksGoldenCall{"tasks.queue.create EMP", "tasks.queue.create", registry.Params{"prefix": "EMP", "name": "Empty"}},
		tasksGoldenCall{"tasks.queue.get GLD", "tasks.queue.get", registry.Params{"prefix": "GLD"}},
		tasksGoldenCall{"tasks.queue.update GLD", "tasks.queue.update", registry.Params{"prefix": "GLD", "description": "updated", "owners": "", "revision": float64(1)}},
		tasksGoldenCall{"tasks.queue.list", "tasks.queue.list", registry.Params{}},
		tasksGoldenCall{"tasks.list empty", "tasks.list", registry.Params{"queue": "GLD"}},
	)
	parent := create("tasks.create parent", "GLD-PARENT", registry.Params{"queue": "GLD", "title": "Parent", "description": "the root", "pull_request": "https://example.test/o/r/pull/1", "priority": "P1"})
	child := create("tasks.create child", "GLD-CHILD", registry.Params{"queue": "GLD", "parent_key": parent, "title": "Child"})
	other := create("tasks.create other", "GLD-OTHER", registry.Params{"queue": "GLD", "title": "Other"})

	// rev reads a task's current revision for the next revision-checked call.
	rev := func(key string) float64 {
		t.Helper()
		detail, err := svc.GetTask(t.Context(), tasks.CustomerActor("customer"), key)
		if err != nil {
			t.Fatal(err)
		}
		return float64(detail.Task.Revision)
	}
	run(tasksGoldenCall{"tasks.update child", "tasks.update", registry.Params{"key": child, "title": "Child renamed", "revision": rev(child)}})
	runAll(
		tasksGoldenCall{"tasks.comments.list empty", "tasks.comments.list", registry.Params{"key": parent}},
		tasksGoldenCall{"tasks.comments.add mention", "tasks.comments.add", registry.Params{"key": other, "body": "Please look, @agent:dev-a"}},
	)
	// An agent's question to the customer, answered below.
	if _, err := svc.AddComment(t.Context(), tasks.AgentActor("dev-a"), parent, tasks.AddCommentInput{Body: "Need input from @user:customer"}); err != nil {
		t.Fatal(err)
	}
	runAll(
		tasksGoldenCall{"tasks.comments.add answer", "tasks.comments.add", registry.Params{"key": parent, "body": "Answered", "idempotency_key": "answer"}},
		tasksGoldenCall{"tasks.comments.list", "tasks.comments.list", registry.Params{"key": parent}},
		tasksGoldenCall{"tasks.relations.list empty", "tasks.relations.list", registry.Params{"key": other}},
	)
	run(tasksGoldenCall{"tasks.relations.add", "tasks.relations.add", registry.Params{"key": other, "target_key": parent, "type": "related", "revision": rev(other), "idempotency_key": "rel"}})
	runAll(
		tasksGoldenCall{"tasks.relations.list", "tasks.relations.list", registry.Params{"key": other}},
		tasksGoldenCall{"tasks.get parent", "tasks.get", registry.Params{"key": parent}},
		tasksGoldenCall{"tasks.get other", "tasks.get", registry.Params{"key": other}},
	)
	detail, err := svc.GetTask(t.Context(), tasks.CustomerActor("customer"), other)
	if err != nil || len(detail.Relations) != 1 {
		t.Fatalf("relations = %#v, %v", detail.Relations, err)
	}
	run(tasksGoldenCall{"tasks.relations.delete", "tasks.relations.delete", registry.Params{"key": other, "relation_id": float64(detail.Relations[0].ID), "revision": rev(other), "idempotency_key": "unrel"}})
	runAll(
		tasksGoldenCall{"tasks.events", "tasks.events", registry.Params{"key": parent}},
		tasksGoldenCall{"tasks.events after", "tasks.events", registry.Params{"key": parent, "after": float64(1000)}},
		tasksGoldenCall{"tasks.principals", "tasks.principals", registry.Params{}},
	)
	run(tasksGoldenCall{"tasks.claim other", "tasks.claim", registry.Params{"key": other, "revision": rev(other)}})
	run(tasksGoldenCall{"tasks.move child to root", "tasks.move", registry.Params{"key": child, "parent_key": "", "revision": rev(child)}})
	run(tasksGoldenCall{"tasks.complete child", "tasks.complete", registry.Params{"key": child, "revision": rev(child)}})
	runAll(
		tasksGoldenCall{"tasks.list", "tasks.list", registry.Params{"queue": "GLD"}},
		tasksGoldenCall{"tasks.list page", "tasks.list", registry.Params{"queue": "GLD", "limit": float64(1)}},
	)
	bundle := run(tasksGoldenCall{"tasks.export parent", "tasks.export", registry.Params{"key": parent}})

	// The import runs on a second daemon, which has the queue but not the task.
	target, targetSvc := tasksFixture(t)
	if _, err := targetSvc.CreateQueue(t.Context(), tasks.CustomerActor("customer"), tasks.CreateQueueInput{Prefix: "GLD", Name: "Golden"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(bundle)
	var bundleParam map[string]any
	if err := json.Unmarshal(raw, &bundleParam); err != nil {
		t.Fatal(err)
	}
	imported, err := h(t, "tasks.import")(target, registry.Params{"bundle": bundleParam})
	if err != nil {
		t.Fatalf("tasks.import: %v", err)
	}
	recorder.record("tasks.import parent", imported)

	runAll(
		tasksGoldenCall{"tasks.queue.pool.set", "tasks.queue.pool.set", registry.Params{"queue": "GLD", "pool": "developers", "agents": []any{"dev-a"}, "revision": float64(0), "idempotency_key": "pool"}},
		tasksGoldenCall{"tasks.queue.pool.get", "tasks.queue.pool.get", registry.Params{"queue": "GLD", "pool": "developers"}},
		tasksGoldenCall{"tasks.queue.pool.list", "tasks.queue.pool.list", registry.Params{"queue": "GLD"}},
		tasksGoldenCall{"tasks.queue.pool.list empty", "tasks.queue.pool.list", registry.Params{"queue": "EMP"}},
		tasksGoldenCall{"tasks.queue.trigger.list empty", "tasks.queue.trigger.list", registry.Params{"queue": "GLD"}},
		tasksGoldenCall{"tasks.queue.trigger.create", "tasks.queue.trigger.create", registry.Params{"queue": "GLD", "pattern": "plugin:*", "action": "create_task"}},
		tasksGoldenCall{"tasks.queue.trigger.create correlated", "tasks.queue.trigger.create", registry.Params{"queue": "GLD", "pattern": "plugin:alerts", "correlation_key": "alert-7", "action": "create_task"}},
		tasksGoldenCall{"tasks.queue.trigger.list", "tasks.queue.trigger.list", registry.Params{"queue": "GLD"}},
	)
	triggers, err := svc.ListQueueWorkflowTriggers(t.Context(), tasks.CustomerActor("customer"), "GLD")
	if err != nil || len(triggers) == 0 {
		t.Fatalf("triggers = %#v, %v", triggers, err)
	}
	run(tasksGoldenCall{"tasks.queue.trigger.delete", "tasks.queue.trigger.delete", registry.Params{"queue": "GLD", "id": float64(triggers[0].ID)}})

	// The workflow stub answers these.
	runAll(
		tasksGoldenCall{"tasks.advance", "tasks.advance", registry.Params{"key": "DEV-1", "outcome": "ready", "message": "PR up"}},
		tasksGoldenCall{"tasks.artifacts.set", "tasks.artifacts.set", registry.Params{"key": "DEV-1", "name": "plan", "value": "step 1\n"}},
		tasksGoldenCall{"tasks.artifacts.list", "tasks.artifacts.list", registry.Params{"key": "DEV-1"}},
		tasksGoldenCall{"tasks.artifacts.get", "tasks.artifacts.get", registry.Params{"key": "DEV-1", "name": "plan"}},
		tasksGoldenCall{"tasks.workflow.get", "tasks.workflow.get", registry.Params{"key": "DEV-1"}},
		tasksGoldenCall{"tasks.workflow.requests.get", "tasks.workflow.requests.get", registry.Params{"key": "DEV-1", "id": "7"}},
		tasksGoldenCall{"tasks.workflow.requests.retry", "tasks.workflow.requests.retry", registry.Params{"key": "DEV-1", "id": "7"}},
		tasksGoldenCall{"tasks.workflow.runs.list", "tasks.workflow.runs.list", registry.Params{"key": "DEV-1"}},
		tasksGoldenCall{"tasks.workflow.runs.get", "tasks.workflow.runs.get", registry.Params{"key": "DEV-1", "id": "4"}},
		tasksGoldenCall{"tasks.workflow.runs.log", "tasks.workflow.runs.log", registry.Params{"key": "DEV-1", "id": "4"}},
		tasksGoldenCall{"tasks.workflow.move", "tasks.workflow.move", registry.Params{"key": "DEV-1", "to": "review", "reason": "unstick"}},
		tasksGoldenCall{"tasks.workflow.upgrade", "tasks.workflow.upgrade", registry.Params{"key": "DEV-1", "reason": "new rules"}},
		tasksGoldenCall{"tasks.workflow.resume", "tasks.workflow.resume", registry.Params{"key": "DEV-1", "decision": "release"}},
		tasksGoldenCall{"tasks.workflow.reassign", "tasks.workflow.reassign", registry.Params{"key": "DEV-1", "agent": "dev-2"}},
		tasksGoldenCall{"tasks.cancel", "tasks.cancel", registry.Params{"key": "DEV-1"}},
		tasksGoldenCall{"tasks.requeue", "tasks.requeue", registry.Params{"key": "DEV-1", "queue": "OPS", "revision": float64(3)}},
		tasksGoldenCall{"tasks.queue.workflow.set", "tasks.queue.workflow.set", registry.Params{"queue": "DEV", "ref": "flow:1.0.0", "revision": float64(2)}},
		tasksGoldenCall{"tasks.queue.workflow.get", "tasks.queue.workflow.get", registry.Params{"queue": "DEV"}},
		tasksGoldenCall{"tasks.queue.workflow.clear", "tasks.queue.workflow.clear", registry.Params{"queue": "dev", "revision": float64(3)}},
		tasksGoldenCall{"tasks.queue.secret.set", "tasks.queue.secret.set", registry.Params{"queue": "dev", "key": "GH_TOKEN", "value": "s"}},
		tasksGoldenCall{"tasks.queue.secret.ls", "tasks.queue.secret.ls", registry.Params{"queue": "DEV"}},
		tasksGoldenCall{"tasks.queue.secret.rm", "tasks.queue.secret.rm", registry.Params{"queue": "dev", "key": "GH_TOKEN"}},
		tasksGoldenCall{"tasks.queue.env.set", "tasks.queue.env.set", registry.Params{"queue": "dev", "key": "LOG_LEVEL", "value": "debug"}},
		tasksGoldenCall{"tasks.queue.env.ls", "tasks.queue.env.ls", registry.Params{"queue": "DEV"}},
		tasksGoldenCall{"tasks.queue.env.rm", "tasks.queue.env.rm", registry.Params{"queue": "dev", "key": "LOG_LEVEL"}},
		tasksGoldenCall{"tasks.queue.source.ls", "tasks.queue.source.ls", registry.Params{"queue": "DEV"}},
		tasksGoldenCall{"tasks.queue.source.log", "tasks.queue.source.log", registry.Params{"queue": "DEV", "id": "5", "max_bytes": float64(10)}},
	)

	// tasks.activity merges AI usage with task events. Each task gets its own
	// last activity time, so the rows sort the same way whatever the random keys.
	for i, key := range []string{parent, child, other} {
		if err := aiproxy.NewStore(c.Store, nil).Insert(aiproxy.AIRequest{ID: "r" + key, TS: tasksGoldenClock.Add(time.Duration(i+1) * time.Minute).Format(time.RFC3339Nano),
			Agent: "dev-a", Iteration: "it-1", Model: "m", InputTokens: 10, OutputTokens: 5, CostUSD: 0.25, Status: "ok", TaskID: key}); err != nil {
			t.Fatal(err)
		}
	}
	window := registry.Params{"since": tasksGoldenClock.Add(-time.Hour).Format(time.RFC3339), "until": tasksGoldenClock.Add(time.Hour).Format(time.RFC3339)}
	runAll(
		tasksGoldenCall{"tasks.activity", "tasks.activity", window},
		tasksGoldenCall{"tasks.activity agent", "tasks.activity", registry.Params{"since": window["since"], "until": window["until"], "agent": "dev-a"}},
		tasksGoldenCall{"tasks.activity empty", "tasks.activity", registry.Params{"since": tasksGoldenClock.Add(-48 * time.Hour).Format(time.RFC3339), "until": tasksGoldenClock.Add(-24 * time.Hour).Format(time.RFC3339)}},
	)
	recorder.check("tasks_responses.golden.json", *updateTasksGolden)
}
