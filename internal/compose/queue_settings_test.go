package compose

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/tasks"
)

const queueSecretVar = "COMPOSE_TEST_GH_TOKEN"

// allFieldsQueueFile declares one queue with every setting compose expresses.
func allFieldsQueueFile() File {
	return File{Version: 1,
		Agents: map[string]AgentSpec{"dev-lead": {Image: "basic:latest"}, "dev-developer": {Image: "basic:latest"}},
		TaskQueues: map[string]TaskQueueSpec{"DEV": {
			Name: "Development", Description: "Product work", Owners: []string{"dev-lead"}, ResponsibleAgent: "dev-lead",
			Pools:    map[string][]string{"developers": {"dev-developer"}, "customers": {"dev-lead"}},
			Workflow: "development:0.1.0",
			Triggers: []TriggerSpec{{Pattern: "plugin:telegram", CorrelationKey: "chat_id", Action: "create_task"}},
			Env:      map[string]string{"CUSTOMER_AGENT_POOL": "customers", "AUTO_MERGE": "false"},
			Secrets:  map[string]SecretSpec{"GH_TOKEN": {FromEnv: queueSecretVar}},
		}},
	}
}

func newAllFieldsCaller() *workflowComposeCaller {
	fc := newWorkflowComposeCaller()
	fc.publish("unused/development", "development", "0.1.0", digestA, "0.1.0")
	return fc
}

func TestParseTaskQueueAllFields(t *testing.T) {
	const y = `
version: 1
agents:
  dev-lead: {image: basic:latest}
  dev-developer: {image: basic:latest}
task_queues:
  DEV:
    name: Development
    description: Product work
    owners: [dev-lead]
    responsible_agent: dev-lead
    pools:
      developers: [dev-developer]
      customers: [dev-lead]
    workflow: development:0.1.0
    triggers:
      - pattern: plugin:telegram
        correlation_key: chat_id
        action: create_task
    env:
      CUSTOMER_AGENT_POOL: customers
      AUTO_MERGE: "false"
    secrets:
      GH_TOKEN: {from_env: COMPOSE_TEST_GH_TOKEN}
`
	f, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	got, want := f.TaskQueues["DEV"], allFieldsQueueFile().TaskQueues["DEV"]
	if got.Description != want.Description || !slices.Equal(got.Owners, want.Owners) || got.ResponsibleAgent != want.ResponsibleAgent ||
		!slices.Equal(got.Triggers, want.Triggers) || got.Env["AUTO_MERGE"] != "false" || got.Secrets["GH_TOKEN"] != want.Secrets["GH_TOKEN"] {
		t.Fatalf("parsed queue = %+v", got)
	}
	if _, err := Parse([]byte("version: 1\ntask_queues:\n  DEV:\n    name: D\n    secrets:\n      K: {value: x}\n")); err == nil {
		t.Fatal("Parse accepted an unknown secret field")
	}
}

func TestValidateTaskQueueSettings(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*TaskQueueSpec)
		want string
	}{
		{"responsible agent undeclared", func(q *TaskQueueSpec) { q.ResponsibleAgent = "ghost" }, `responsible_agent references undeclared agent "ghost"`},
		{"pool agent undeclared", func(q *TaskQueueSpec) { q.Pools["developers"] = []string{"ghost"} }, `references undeclared agent "ghost"`},
		{"secret without from_env", func(q *TaskQueueSpec) { q.Secrets["GH_TOKEN"] = SecretSpec{} }, `secret "GH_TOKEN" needs from_env`},
		{"secret from_env invalid", func(q *TaskQueueSpec) { q.Secrets["GH_TOKEN"] = SecretSpec{FromEnv: "1BAD"} }, `secret "GH_TOKEN" from_env "1BAD"`},
		{"env key invalid", func(q *TaskQueueSpec) { q.Env["bad-key"] = "x" }, `env key "bad-key"`},
		{"env key reserved", func(q *TaskQueueSpec) { q.Env["TARIBOY_X"] = "x" }, `env key "TARIBOY_X"`},
		{"secret key reserved", func(q *TaskQueueSpec) { q.Secrets["TARIBOY_X"] = SecretSpec{FromEnv: "V"} }, `secret key "TARIBOY_X"`},
		{"key in env and secrets", func(q *TaskQueueSpec) { q.Env["GH_TOKEN"] = "x" }, `"GH_TOKEN" is declared in env and secrets`},
		{"customer pool as secret", func(q *TaskQueueSpec) {
			delete(q.Env, "CUSTOMER_AGENT_POOL")
			q.Secrets["CUSTOMER_AGENT_POOL"] = SecretSpec{FromEnv: "V"}
		}, `CUSTOMER_AGENT_POOL is a plain env var`},
		{"trigger pattern invalid", func(q *TaskQueueSpec) { q.Triggers[0].Pattern = "telegram" }, `trigger pattern "telegram"`},
		{"trigger action invalid", func(q *TaskQueueSpec) { q.Triggers[0].Action = "close_task" }, `trigger action "close_task"`},
		{"trigger repeated", func(q *TaskQueueSpec) { q.Triggers = append(q.Triggers, q.Triggers[0]) }, `trigger plugin:telegram create_task correlation_key=chat_id is declared twice`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := allFieldsQueueFile()
			q := f.TaskQueues["DEV"]
			tc.edit(&q)
			f.TaskQueues["DEV"] = q
			err := f.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `"DEV"`) {
				t.Fatalf("Validate = %v; want %q", err, tc.want)
			}
		})
	}
	f := allFieldsQueueFile()
	q := f.TaskQueues["DEV"]
	q.Triggers = append(q.Triggers, TriggerSpec{Pattern: "plugin:*"})
	f.TaskQueues["DEV"] = q
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate rejected an empty trigger action: %v", err)
	}
}

func TestUpCreatesQueueWithAllFields(t *testing.T) {
	t.Setenv(queueSecretVar, "secret-value-create")
	fc := newAllFieldsCaller()
	upNoBuild(t, NewRunner(fc, "", io.Discard), allFieldsQueueFile())

	create := bodyFor(fc.fakeCaller, "POST /api/task-queues").(map[string]any)
	if create["description"] != "Product work" || !slices.Equal(create["owners"].([]string), []string{"dev-lead"}) {
		t.Fatalf("create body = %v", create)
	}
	q := fc.queues["DEV"]
	if q.Description != "Product work" || !slices.Equal(q.Owners, []string{"dev-lead"}) || q.ResponsibleAgent != "dev-lead" {
		t.Fatalf("queue = %+v", q)
	}
	if fc.env["DEV"]["AUTO_MERGE"] != "false" || fc.env["DEV"]["CUSTOMER_AGENT_POOL"] != "customers" {
		t.Fatalf("env = %v", fc.env["DEV"])
	}
	if fc.secrets["DEV"]["GH_TOKEN"] != "secret-value-create" {
		t.Fatalf("secret was not set: %v", fc.secrets["DEV"])
	}
	if len(fc.triggers["DEV"]) != 1 || fc.triggers["DEV"][0].Pattern != "plugin:telegram" || fc.triggers["DEV"][0].CorrelationKey != "chat_id" {
		t.Fatalf("triggers = %+v", fc.triggers["DEV"])
	}
	if fc.bound["DEV"].Digest != digestA {
		t.Fatalf("bound = %+v", fc.bound["DEV"])
	}
}

func TestUpUpdatesChangedQueueFields(t *testing.T) {
	t.Setenv(queueSecretVar, "secret-value-update")
	fc := newAllFieldsCaller()
	r := NewRunner(fc, "", io.Discard)
	upNoBuild(t, r, allFieldsQueueFile())

	f := allFieldsQueueFile()
	q := f.TaskQueues["DEV"]
	q.Name, q.Description, q.Owners, q.ResponsibleAgent = "Dev", "New work", []string{"dev-developer", "dev-lead"}, "dev-developer"
	q.Env = map[string]string{"CUSTOMER_AGENT_POOL": "developers", "AUTO_MERGE": "true", "LOG_LEVEL": "debug"}
	q.Triggers = append(q.Triggers, TriggerSpec{Pattern: "plugin:slack", Action: "create_task"})
	f.TaskQueues["DEV"] = q
	before := len(fc.calls)
	revision := fc.queues["DEV"].Revision
	upNoBuild(t, r, f)

	patches := 0
	for i := before; i < len(fc.calls); i++ {
		if fc.calls[i] != "PATCH /api/task-queues/DEV" {
			continue
		}
		patches++
		body := fc.bodies[i].(map[string]any)
		if patches == 1 && (body["revision"] != revision || body["name"] != "Dev" || body["description"] != "New work") {
			t.Fatalf("first patch body = %v (revision %d)", body, revision)
		}
		if patches == 2 && (body["revision"] != revision+1 || body["responsible_agent"] != "dev-developer") {
			t.Fatalf("responsible_agent patch body = %v; want revision %d", body, revision+1)
		}
	}
	if patches != 2 {
		t.Fatalf("PATCH calls = %d; want 2 in %v", patches, fc.calls[before:])
	}
	got := fc.queues["DEV"]
	if got.Name != "Dev" || got.Description != "New work" || !slices.Equal(got.Owners, []string{"dev-developer", "dev-lead"}) || got.ResponsibleAgent != "dev-developer" {
		t.Fatalf("queue = %+v", got)
	}
	for _, key := range []string{"AUTO_MERGE", "LOG_LEVEL", "CUSTOMER_AGENT_POOL"} {
		if n := countCallsSince(fc.calls, before, "PUT /api/task-queues/DEV/env/"+key); n != 1 {
			t.Fatalf("PUT env %s calls = %d", key, n)
		}
	}
	if fc.env["DEV"]["CUSTOMER_AGENT_POOL"] != "developers" || fc.env["DEV"]["AUTO_MERGE"] != "true" {
		t.Fatalf("env = %v", fc.env["DEV"])
	}
	if n := countCallsSince(fc.calls, before, "POST /api/task-queues/DEV/workflow-triggers"); n != 1 || len(fc.triggers["DEV"]) != 2 {
		t.Fatalf("trigger creates = %d, triggers = %+v", n, fc.triggers["DEV"])
	}
}

// The daemon trims the description, so a YAML block scalar with a final
// newline must not drift.
func TestQueueDescriptionComparesTrimmed(t *testing.T) {
	f := taskQueueComposeFile()
	q := f.TaskQueues["DEV"]
	q.Description = "Product work\n"
	f.TaskQueues["DEV"] = q
	fc := newTaskQueueComposeCaller()
	fc.agents["dev"] = map[string]any{"name": "dev", "state": "running", "image": "basic:latest"}
	fc.queues["DEV"] = tasks.Queue{Prefix: "DEV", Name: "Development", Description: "Product work", Revision: 1}
	fc.pools["DEV"] = map[string]tasks.AgentPool{"developers": {Queue: "DEV", Name: "developers", Agents: []string{"dev"}, Revision: 1}}
	upNoBuild(t, NewRunner(fc, "", io.Discard), f)
	if n := countCalls(fc.fakeCaller, "PATCH /api/task-queues/DEV"); n != 0 {
		t.Fatalf("PATCH calls = %d; want 0", n)
	}
}

func TestUpLeavesQueueFieldsTheFileOmits(t *testing.T) {
	fc := newTaskQueueComposeCaller()
	fc.agents["dev"] = map[string]any{"name": "dev", "state": "running", "image": "basic:latest"}
	fc.queues["DEV"] = tasks.Queue{Prefix: "DEV", Name: "Development", Description: "kept", Owners: []string{"dev"}, ResponsibleAgent: "dev", Revision: 3}
	fc.pools["DEV"] = map[string]tasks.AgentPool{"developers": {Queue: "DEV", Name: "developers", Agents: []string{"dev"}, Revision: 1}}
	upNoBuild(t, NewRunner(fc, "", io.Discard), taskQueueComposeFile())
	for _, call := range fc.calls {
		if strings.HasPrefix(call, "PATCH /api/task-queues/DEV") && !strings.Contains(call, "/pools/") {
			t.Fatalf("up patched a queue whose declared fields match: %v", fc.calls)
		}
	}
}

// secretCase sets the live state of one row of the secret table.
type secretCase struct {
	name      string
	varSet    bool
	hasSecret bool
	global    bool
	status    string // line status prints for the secret
	drift     bool
	outcome   Outcome
	puts      int
}

func TestQueueSecretOutcomes(t *testing.T) {
	for _, tc := range []secretCase{
		{name: "var and secret", varSet: true, hasSecret: true, status: "secret GH_TOKEN ok (value not compared)", outcome: OutcomeOK, puts: 1},
		{name: "var without secret", varSet: true, status: "secret GH_TOKEN MISSING", drift: true, outcome: OutcomeChanged, puts: 1},
		// The file names the variable, so its value wins over the daemon environment.
		{name: "var without secret, daemon environment", varSet: true, global: true, status: "secret GH_TOKEN MISSING", drift: true, outcome: OutcomeChanged, puts: 1},
		{name: "secret without var", hasSecret: true, status: "secret GH_TOKEN ok (" + queueSecretVar + " not set, kept)", outcome: OutcomeOK},
		{name: "daemon environment", global: true, status: "secret GH_TOKEN ok (daemon environment)", outcome: OutcomeOK},
		{name: "neither", status: "secret GH_TOKEN MISSING (" + queueSecretVar + " not set)", drift: true, outcome: OutcomeFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.varSet {
				t.Setenv(queueSecretVar, "secret-value-table")
			} else {
				t.Setenv(queueSecretVar, "")
			}
			f := File{Version: 1, TaskQueues: map[string]TaskQueueSpec{"DEV": {Name: "Development",
				Secrets: map[string]SecretSpec{"GH_TOKEN": {FromEnv: queueSecretVar}}}}}
			fc := newTaskQueueComposeCaller()
			fc.queues["DEV"] = tasks.Queue{Prefix: "DEV", Name: "Development", Revision: 1}
			if tc.hasSecret {
				fc.secrets["DEV"] = map[string]string{"GH_TOKEN": "old"}
			}
			if tc.global {
				fc.globalEnv["DEV"] = []string{"GH_TOKEN"}
			}
			var out strings.Builder
			r := NewRunner(fc, "", &out)
			if err := r.Status(f); err != nil {
				t.Fatal(err)
			}
			wantDrift := "drift: 0"
			if tc.drift {
				wantDrift = "drift: 1"
			}
			if !strings.Contains(out.String(), tc.status) || !strings.Contains(out.String(), wantDrift) {
				t.Fatalf("status output:\n%s\nwant %q and %q", out.String(), tc.status, wantDrift)
			}
			plan, err := r.Plan(f)
			if err != nil {
				t.Fatal(err)
			}
			rep := r.Apply(plan)
			if got := entryOutcome(rep, "queue:DEV"); got != tc.outcome {
				t.Fatalf("outcome = %s; want %s (%v)", got, tc.outcome, rep.Entries)
			}
			if n := countCalls(fc.fakeCaller, "PUT /api/task-queues/DEV/secrets/GH_TOKEN"); n != tc.puts {
				t.Fatalf("secret PUT calls = %d; want %d", n, tc.puts)
			}
			if tc.outcome == OutcomeFailed && !strings.Contains(rep.Err().Error(), "ttasks queue secret set DEV GH_TOKEN") {
				t.Fatalf("error = %v; want the ttasks command", rep.Err())
			}
		})
	}
}

func entryOutcome(rep Report, id string) Outcome {
	for _, e := range rep.Entries {
		if e.Resource == id {
			return e.Outcome
		}
	}
	return ""
}

// A missing secret fails the queue and skips its workflow binding and
// triggers, so the daemon never binds a workflow without its secrets.
func TestMissingQueueSecretSkipsBindingAndTriggers(t *testing.T) {
	t.Setenv(queueSecretVar, "")
	fc := newAllFieldsCaller()
	r := NewRunner(fc, "", io.Discard)
	r.skipBuild = true
	if err := r.Up(allFieldsQueueFile()); err == nil {
		t.Fatal("up succeeded without the secret")
	}
	for _, call := range []string{"PUT /api/task-queues/DEV/workflow", "POST /api/task-queues/DEV/workflow-triggers"} {
		if callIndex(fc.calls, call) >= 0 {
			t.Fatalf("%s ran without the secret: %v", call, fc.calls)
		}
	}
}

func TestQueueApplyOrder(t *testing.T) {
	t.Setenv(queueSecretVar, "secret-value-order")
	fc := newAllFieldsCaller()
	upNoBuild(t, NewRunner(fc, "", io.Discard), allFieldsQueueFile())
	at := func(call string) int {
		i := callIndex(fc.calls, call)
		if i < 0 {
			t.Fatalf("missing call %s in %v", call, fc.calls)
		}
		return i
	}
	bind := at("PUT /api/task-queues/DEV/workflow")
	for _, before := range []string{"PUT /api/task-queues/DEV/env/AUTO_MERGE", "PUT /api/task-queues/DEV/secrets/GH_TOKEN",
		"PUT /api/task-queues/DEV/env/CUSTOMER_AGENT_POOL", "PATCH /api/task-queues/DEV/pools/customers"} {
		if at(before) > bind {
			t.Fatalf("%s ran after the workflow binding: %v", before, fc.calls)
		}
	}
	if at("PUT /api/task-queues/DEV/env/AUTO_MERGE") > at("POST /api/agents") {
		t.Fatalf("plain env was not set in the queues phase: %v", fc.calls)
	}
	lastAgent := -1
	for i, c := range fc.calls {
		if c == "POST /api/agents" {
			lastAgent = i
		}
	}
	for _, pool := range []string{"customers", "developers"} {
		if at("PATCH /api/task-queues/DEV/pools/"+pool) < lastAgent {
			t.Fatalf("pool %s bound before its agents were created: %v", pool, fc.calls)
		}
	}
	customerPool := at("PUT /api/task-queues/DEV/env/CUSTOMER_AGENT_POOL")
	if customerPool < at("PATCH /api/task-queues/DEV/pools/customers") {
		t.Fatalf("CUSTOMER_AGENT_POOL set before the pools: %v", fc.calls)
	}
	if at("POST /api/task-queues/DEV/workflow-triggers") < bind {
		t.Fatalf("trigger created before the workflow binding: %v", fc.calls)
	}
}

func TestQueueSecretValueNeverPrinted(t *testing.T) {
	const value = "s3cr3t-VALUE-7f1e9a"
	t.Setenv(queueSecretVar, value)
	check := func(what, text string) {
		t.Helper()
		if strings.Contains(text, value) {
			t.Fatalf("%s printed the secret value:\n%s", what, text)
		}
	}
	fc := newAllFieldsCaller()
	var out strings.Builder
	r := NewRunner(fc, "", &out)
	r.skipBuild = true
	f := allFieldsQueueFile()

	if err := r.DryRun(f); err != nil {
		t.Fatal(err)
	}
	check("dry-run on create", out.String())
	if err := r.Up(f); err != nil {
		t.Fatal(err)
	}
	check("up", out.String())
	for _, run := range []func(File) error{r.Status, r.DryRun} {
		out.Reset()
		if err := run(f); err != nil {
			t.Fatal(err)
		}
		check("status and dry-run", out.String())
	}

	fc.fail = map[string]error{"PUT /api/task-queues/DEV/secrets/GH_TOKEN": fmt.Errorf("transport echoed body {\"value\":%q}", value)}
	out.Reset()
	err := r.Up(f)
	if err == nil {
		t.Fatal("up succeeded although the secret write failed")
	}
	check("failed up", out.String())
	check("up error", err.Error())
	var rep Report
	plan, _ := r.Plan(f)
	rep = r.Apply(plan)
	check("apply error", rep.Err().Error())
	if !strings.Contains(rep.Err().Error(), "[redacted]") {
		t.Fatalf("error = %v; want the value replaced", rep.Err())
	}
}

func TestDryRunDoesNotPrintSecretRefresh(t *testing.T) {
	t.Setenv(queueSecretVar, "secret-value-dry")
	fc := newAllFieldsCaller()
	r := NewRunner(fc, "", io.Discard)
	upNoBuild(t, r, allFieldsQueueFile())
	var out strings.Builder
	r.out = &out
	if err := r.DryRun(allFieldsQueueFile()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "phase queues") {
		t.Fatalf("dry-run shows a converged secret:\n%s", out.String())
	}

	delete(fc.secrets["DEV"], "GH_TOKEN")
	out.Reset()
	if err := r.DryRun(allFieldsQueueFile()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "secret GH_TOKEN") {
		t.Fatalf("dry-run does not show the secret it sets:\n%s", out.String())
	}
}

func TestQueueAllFieldsIdempotent(t *testing.T) {
	t.Setenv(queueSecretVar, "secret-value-idem")
	fc := newAllFieldsCaller()
	r := NewRunner(fc, "", io.Discard)
	upNoBuild(t, r, allFieldsQueueFile())

	before := len(fc.calls)
	var out strings.Builder
	r.out = &out
	upNoBuild(t, r, allFieldsQueueFile())
	if !strings.Contains(out.String(), " 0 changed") {
		t.Fatalf("second up report:\n%s", out.String())
	}
	for _, call := range fc.calls[before:] {
		if strings.HasPrefix(call, "GET ") || call == "PUT /api/task-queues/DEV/secrets/GH_TOKEN" {
			continue
		}
		if strings.Contains(call, "/api/task-queues") {
			t.Fatalf("second up mutated the queue: %s", call)
		}
	}
	out.Reset()
	if err := r.Status(allFieldsQueueFile()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "drift: 0") {
		t.Fatalf("status after up:\n%s", out.String())
	}
}

func TestQueueItemsTheFileOmitsAreNotDeclared(t *testing.T) {
	t.Setenv(queueSecretVar, "secret-value-undeclared")
	fc := newAllFieldsCaller()
	r := NewRunner(fc, "", io.Discard)
	upNoBuild(t, r, allFieldsQueueFile())
	fc.pools["DEV"]["old-pool"] = tasks.AgentPool{Queue: "DEV", Name: "old-pool", Agents: []string{"dev-lead"}, Revision: 1}
	fc.env["DEV"]["LOG_LEVEL"] = "debug"
	fc.secrets["DEV"]["OLD_TOKEN"] = "x"
	fc.triggers["DEV"] = append(fc.triggers["DEV"], tasks.QueueWorkflowTrigger{ID: 99, Queue: "DEV", Pattern: "plugin:slack", Action: "create_task", Enabled: true})

	want := []string{"pool old-pool not declared", "env LOG_LEVEL not declared", "secret OLD_TOKEN not declared", "trigger plugin:slack create_task not declared"}
	for name, run := range map[string]func(File) error{"status": r.Status, "dry-run": r.DryRun} {
		var out strings.Builder
		r.out = &out
		if err := run(allFieldsQueueFile()); err != nil {
			t.Fatal(err)
		}
		for _, line := range want {
			if !strings.Contains(out.String(), line) {
				t.Fatalf("%s output lacks %q:\n%s", name, line, out.String())
			}
		}
		if name == "status" && !strings.Contains(out.String(), "drift: 0") {
			t.Fatalf("not declared items count as drift:\n%s", out.String())
		}
	}
	before := len(fc.calls)
	upNoBuild(t, r, allFieldsQueueFile())
	for _, call := range fc.calls[before:] {
		if strings.HasPrefix(call, "DELETE ") {
			t.Fatalf("up removed an item the file omits: %s", call)
		}
	}
}
