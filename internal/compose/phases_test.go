package compose

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// phasedFile declares one item of every kind the engine applies: an image, a
// task queue with a pool and a Store workflow, a group with a lead and a
// budget, and two agents on the image in that group.
func phasedFile() File {
	return File{Version: 1,
		Images: map[string]ImageSpec{"dev": {Context: "dev"}},
		Groups: map[string]GroupSpec{"team": {Lead: "lead", Budget: &BudgetSpec{LimitUSD: 9}}},
		Agents: map[string]AgentSpec{
			"lead":   {Image: "dev:latest", Group: "team", Budget: &BudgetSpec{LimitUSD: 5}},
			"worker": {Image: "dev:latest", Group: "team", Timeout: "1m"},
		},
		TaskQueues: map[string]TaskQueueSpec{"DEV": {Name: "Development", Workflow: "official/development",
			Pools: map[string][]string{"developers": {"lead", "worker"}}}},
	}
}

func phasedCaller() *workflowComposeCaller {
	fc := newWorkflowComposeCaller()
	fc.publish("official/development", "development", "0.1.0", digestA, "0.1.0", "latest")
	return fc
}

// mutating returns the calls other than GET, from index start. A workflow
// validate is a POST that changes nothing, so it is not listed.
func mutating(calls []string, start int) []string {
	var out []string
	for _, c := range calls[start:] {
		if !strings.HasPrefix(c, "GET ") && c != "POST /api/workflow-images/validate" {
			out = append(out, c)
		}
	}
	return out
}

// Every action carries the phase that planned it, and the phases follow the
// fixed order.
func TestPlanActionsFollowThePhaseOrder(t *testing.T) {
	r := NewRunner(phasedCaller(), "", io.Discard)
	plan, err := r.Plan(phasedFile())
	if err != nil {
		t.Fatal(err)
	}
	order := map[string]int{}
	for i, ph := range phases {
		order[ph.name()] = i
	}
	last := -1
	seen := map[string]bool{}
	for _, a := range plan.Actions {
		i, ok := order[a.Phase]
		if !ok {
			t.Fatalf("action %s %s has phase %q", a.Resource, a.Kind, a.Phase)
		}
		if i < last {
			t.Fatalf("action %s %s in phase %s follows a later phase", a.Resource, a.Kind, a.Phase)
		}
		last = i
		seen[a.Phase] = true
	}
	for _, want := range []string{"workflows", "images", "queues", "agents", "groups", "queue bindings", "start"} {
		if !seen[want] {
			t.Errorf("no action in phase %s", want)
		}
	}
	want := []string{"stores", "workflows", "images", "queues", "agents", "groups", "queue bindings", "start"}
	var names []string
	for _, ph := range phases {
		names = append(names, ph.name())
	}
	if !slices.Equal(names, want) {
		t.Fatalf("phases = %v, want %v", names, want)
	}
}

// Criterion 1: up issues its mutating calls phase by phase: workflows,
// images, queues, agents (without a group), groups (members, then the lead),
// queue bindings, start.
func TestUpIssuesMutatingCallsPhaseByPhase(t *testing.T) {
	fc := phasedCaller()
	r := NewRunner(fc, "", io.Discard)
	if err := r.Up(phasedFile()); err != nil {
		t.Fatalf("up: %v", err)
	}
	want := []string{
		"POST /api/workflow-images/build",
		"POST /api/images/build",
		"POST /api/task-queues",
		"POST /api/agents", // lead
		"POST /api/budgets",
		"POST /api/agents", // worker
		"POST /api/agents/worker/loop/timeout",
		"POST /api/groups",
		"POST /api/groups/team/assign", // lead
		"POST /api/groups/team/assign", // worker
		"PATCH /api/groups/team/lead",
		"POST /api/budgets",
		"PATCH /api/task-queues/DEV/pools/developers",
		"PUT /api/task-queues/DEV/workflow",
		"POST /api/agents/lead/start",
		"POST /api/agents/worker/start",
	}
	if got := mutating(fc.calls, 0); !slices.Equal(got, want) {
		t.Fatalf("mutating calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i, c := range fc.calls {
		switch c {
		case "POST /api/agents":
			if _, ok := fc.bodies[i].(map[string]any)["group"]; ok {
				t.Fatalf("agent create carries a group: %v", fc.bodies[i])
			}
		case "POST /api/groups":
			if _, ok := fc.bodies[i].(map[string]any)["lead"]; ok {
				t.Fatalf("group create carries a lead: %v", fc.bodies[i])
			}
		}
	}
	if got := fc.groups["team"]["lead"]; got != "lead" {
		t.Fatalf("group lead = %v", got)
	}
	if got := fc.bodies[callIndex(fc.calls, "POST /api/budgets")].(map[string]any)["scope"]; got != "agent:lead" {
		t.Fatalf("first budget scope = %v", got)
	}
}

// Criterion 2: a failed agent create skips its budget, its group membership,
// the lead it is, the pool it is in, the workflow binding that needs that pool
// and its start. The other agent is created, assigned and started.
func TestUpAgentCreateFailureSkipsItsDependentsInLaterPhases(t *testing.T) {
	fc := phasedCaller()
	caller := &failCreateCaller{fakeCaller: fc.fakeCaller, failName: "lead"}
	var out strings.Builder
	r := NewRunner(&routedCaller{first: caller, rest: fc}, "", &out)
	err := r.Up(phasedFile())
	if err == nil || !strings.Contains(actionErrs(err), "create agent lead") {
		t.Fatalf("Up = %v", err)
	}
	for _, c := range []string{"POST /api/agents/lead/start", "PATCH /api/groups/team/lead",
		"PATCH /api/task-queues/DEV/pools/developers", "PUT /api/task-queues/DEV/workflow"} {
		if n := countCalls(fc.fakeCaller, c); n != 0 {
			t.Errorf("%s ran after the create of lead failed", c)
		}
	}
	for i, c := range fc.calls {
		if c == "POST /api/groups/team/assign" && fc.bodies[i].(map[string]any)["agent"] == "lead" {
			t.Errorf("lead was assigned after its create failed")
		}
		if c == "POST /api/budgets" && fc.bodies[i].(map[string]any)["scope"] == "agent:lead" {
			t.Errorf("budget of lead was set after its create failed")
		}
	}
	if fc.agents["worker"]["group"] != "team" || countCalls(fc.fakeCaller, "POST /api/agents/worker/start") != 1 {
		t.Fatalf("worker did not converge: %v", fc.calls)
	}
	got := out.String()
	for _, want := range []string{"failed   agent:lead", "skipped  budget:agent:lead", "skipped  group:team",
		"skipped  queue:DEV", "changed  agent:worker", "changed  budget:group:team"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}

// routedCaller sends agent creates through first and everything else to rest,
// so a create failure can be injected under a richer fake.
type routedCaller struct {
	first Caller
	rest  Caller
}

func (c *routedCaller) Call(method, route string, body any) (json.RawMessage, error) {
	if method == "POST" && route == "/api/agents" {
		return c.first.Call(method, route, body)
	}
	return c.rest.Call(method, route, body)
}

// A failed image build no longer stops up: it skips the agents on that image
// and the actions that need them; agents on other images converge.
func TestUpImageBuildFailureSkipsOnlyAgentsOnThatImage(t *testing.T) {
	fc := newFake()
	fc.fail = map[string]error{"POST /api/images/build": errors.New("bad Tariboyfile")}
	f := File{Version: 1,
		Images: map[string]ImageSpec{"dev": {Context: "dev"}},
		Agents: map[string]AgentSpec{"a": {Image: "dev:latest"}, "b": {Image: "basic:latest"}},
	}
	var out strings.Builder
	err := NewRunner(fc, "", &out).Up(f)
	if err == nil || !strings.Contains(actionErrs(err), "build image dev") {
		t.Fatalf("Up = %v", err)
	}
	if fc.agents["a"] != nil || fc.agents["b"] == nil || countCalls(fc, "POST /api/agents/b/start") != 1 {
		t.Fatalf("calls: %v", fc.calls)
	}
	for _, want := range []string{"failed   image:dev", "skipped  agent:a", "changed  agent:b"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

// Criterion 4: a second up on the converged state makes no mutating call but
// the two builds, and reports every resource ok because the builds return the
// digests already published.
func TestUpSecondRunOnlyRebuildsAndReportsEveryResourceOK(t *testing.T) {
	fc := phasedCaller()
	var out strings.Builder
	r := NewRunner(fc, "", &out)
	f := phasedFile()
	if err := r.Up(f); err != nil {
		t.Fatalf("first up: %v", err)
	}
	before, outBefore := len(fc.calls), out.Len()
	if err := r.Up(f); err != nil {
		t.Fatalf("second up: %v", err)
	}
	want := []string{"POST /api/workflow-images/build", "POST /api/images/build"}
	if got := mutating(fc.calls, before); !slices.Equal(got, want) {
		t.Fatalf("second up mutating calls = %v, want %v", got, want)
	}
	got := out.String()[outBefore:]
	if strings.Contains(got, "changed ") || !strings.Contains(got, " 0 changed, 0 failed, 0 skipped") {
		t.Fatalf("second report:\n%s", got)
	}
	for _, res := range []string{"ok       image:dev", "ok       workflow:official/development"} {
		if !strings.Contains(got, res) {
			t.Errorf("second report lacks %q:\n%s", res, got)
		}
	}
	// A build that publishes another digest is a change.
	fc.buildDigest = map[string]string{"dev": "digest-new"}
	outBefore = out.Len()
	if err := r.Up(f); err != nil {
		t.Fatalf("third up: %v", err)
	}
	if got := out.String()[outBefore:]; !strings.Contains(got, "changed  image:dev") {
		t.Fatalf("third report:\n%s", got)
	}
}

const dryRunYAML = `version: 1
images:
  dev: { context: ./dev }
groups:
  team: { lead: lead }
agents:
  lead:
    image: dev:latest
    group: team
    env: { TOKEN: s3cret-value }
    budget: { limit_usd: 5 }
  ops:
    image: dev:latest
    group: team
    harness: { type: claude }
    env: { REGION: eu-secret-region }
task_queues:
  DEV:
    name: Development
    pools: { developers: [lead, ops] }
`

// Criterion 3: up --dry-run prints the plan by phase from reads only. It does
// not start the daemon, build anything or make a mutating call, and it prints
// the names of env keys but never their values or a create body.
func TestUpDryRunPrintsThePlanFromReadsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFile)
	if err := os.WriteFile(path, []byte(dryRunYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := ensureDaemonUp
	ensureDaemonUp = func(context.Context, io.Writer) error {
		t.Fatal("dry-run started the daemon")
		return nil
	}
	t.Cleanup(func() { ensureDaemonUp = orig })
	fc := newTaskQueueComposeCaller()
	fc.agents["ops"] = map[string]any{"name": "ops", "state": "running", "enabled": true, "image": "dev:latest",
		"harness": "codex", "group": "team", "cwd": dir, "env": map[string]any{"REGION": "us-old-region"}}
	fc.groups["team"] = map[string]any{"name": "team", "lead": "ops"}
	var out, errOut strings.Builder
	if code := Run(context.Background(), fc, Options{Verb: "up", File: path, DryRun: true}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	for _, c := range fc.calls {
		if !strings.HasPrefix(c, "GET ") {
			t.Fatalf("dry-run made the mutating call %s: %v", c, fc.calls)
		}
	}
	got := out.String()
	for _, want := range []string{"phase images", "build    image:dev", "phase queues", "create   queue:DEV",
		"phase agents", "create   agent:lead", "TOKEN", "update   agent:ops", "harness codex -> claude", "REGION",
		"set      budget:agent:lead", "phase groups", "phase queue bindings", "phase start", "start    agent:lead",
		"restart  agent:ops", "nothing applied"} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output lacks %q:\n%s", want, got)
		}
	}
	for _, secret := range []string{"s3cret-value", "eu-secret-region", "us-old-region", "phase stores"} {
		if strings.Contains(got, secret) {
			t.Errorf("dry-run output shows %q:\n%s", secret, got)
		}
	}
}

// undeclaredCaller holds, next to the file's entities, entities the file does
// not declare: some tied to the file (by a group, a pool or a member), some
// unrelated.
func undeclaredCaller() *taskQueueComposeCaller {
	fc := newTaskQueueComposeCaller()
	agent := func(name, group string) map[string]any {
		return map[string]any{"name": name, "state": "running", "enabled": true, "image": "dev:latest", "harness": "claude", "group": group}
	}
	fc.agents["lead"] = agent("lead", "team")
	fc.agents["worker"] = agent("worker", "legacy") // file says team
	fc.agents["old-bot"] = agent("old-bot", "team")
	fc.agents["pooled"] = agent("pooled", "")
	fc.agents["stranger"] = agent("stranger", "other")
	fc.groups["team"] = map[string]any{"name": "team", "lead": "lead"}
	fc.groups["legacy"] = map[string]any{"name": "legacy", "lead": ""}
	fc.groups["other"] = map[string]any{"name": "other", "lead": "stranger"}
	fc.queues["DEV"] = tasks.Queue{Prefix: "DEV", Name: "Development", Revision: 1}
	fc.queues["OLD"] = tasks.Queue{Prefix: "OLD", Name: "Old", Revision: 1}
	fc.queues["ZZZ"] = tasks.Queue{Prefix: "ZZZ", Name: "Unrelated", Revision: 1}
	fc.pools["DEV"] = map[string]tasks.AgentPool{"developers": {Queue: "DEV", Name: "developers", Agents: []string{"lead", "pooled"}, Revision: 1}}
	fc.pools["OLD"] = map[string]tasks.AgentPool{"dev": {Queue: "OLD", Name: "dev", Agents: []string{"lead"}, Revision: 1}}
	fc.pools["ZZZ"] = map[string]tasks.AgentPool{"any": {Queue: "ZZZ", Name: "any", Agents: []string{"stranger"}, Revision: 1}}
	return fc
}

func undeclaredFile() File {
	return File{Version: 1,
		Groups: map[string]GroupSpec{"team": {Lead: "lead"}},
		Agents: map[string]AgentSpec{"lead": {Image: "dev:latest", Group: "team"}, "worker": {Image: "dev:latest", Group: "team"}},
		TaskQueues: map[string]TaskQueueSpec{"DEV": {Name: "Development",
			Pools: map[string][]string{"developers": {"lead"}}}},
	}
}

func undeclaredLines(out string) []string {
	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "not declared (") && !strings.Contains(line, "workflow") {
			lines = append(lines, strings.Join(strings.Fields(line), " "))
		}
	}
	return lines
}

// Criterion 5: status and up --dry-run list the entities on the daemon that
// are tied to the file but not declared in it. They do not count as drift, and
// up never changes or removes them.
func TestStatusAndDryRunListUndeclaredEntities(t *testing.T) {
	want := []string{
		"agent old-bot not declared (group=team state=running)",
		"agent pooled not declared (pool DEV/developers state=running)",
		"group legacy not declared (member worker)",
		"task queue OLD not declared (pool dev has lead)",
	}
	fc := undeclaredCaller()
	var status strings.Builder
	if err := NewRunner(fc, "", &status).Status(undeclaredFile()); err != nil {
		t.Fatal(err)
	}
	if got := undeclaredLines(status.String()); !reflect.DeepEqual(got, want) {
		t.Fatalf("status not declared lines:\n%s\nwant:\n%s\noutput:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"), status.String())
	}
	// worker's group and the DEV pool drift; nothing undeclared counts.
	if !strings.Contains(status.String(), "drift: 2\n") {
		t.Fatalf("status drift:\n%s", status.String())
	}

	var dry strings.Builder
	if err := NewRunner(fc, "", &dry).DryRun(undeclaredFile()); err != nil {
		t.Fatal(err)
	}
	if got := undeclaredLines(dry.String()); !reflect.DeepEqual(got, want) {
		t.Fatalf("dry-run not declared lines:\n%s\noutput:\n%s", strings.Join(got, "\n"), dry.String())
	}

	before := len(fc.calls)
	r := NewRunner(fc, "", io.Discard)
	r.skipBuild = true
	if err := r.Up(undeclaredFile()); err != nil {
		t.Fatal(err)
	}
	for _, c := range mutating(fc.calls, before) {
		for _, name := range []string{"old-bot", "pooled", "legacy", "OLD", "stranger", "other", "ZZZ"} {
			if strings.Contains(c, name) {
				t.Fatalf("up touched undeclared %s: %s", name, c)
			}
		}
	}
	if fc.agents["old-bot"] == nil || fc.groups["legacy"] == nil || fc.queues["OLD"].Prefix == "" {
		t.Fatal("up removed an undeclared entity")
	}
}

// A new agent starts only once it joined its group: a failed assign skips the
// start, so the agent never runs outside its group.
func TestUpFailedAssignSkipsTheStartOfANewAgent(t *testing.T) {
	fc := newFake()
	fc.fail = map[string]error{"POST /api/groups/team/assign": errors.New("assign failed")}
	f := File{Version: 1,
		Groups: map[string]GroupSpec{"team": {}},
		Agents: map[string]AgentSpec{"a": {Image: "basic:latest", Group: "team"}, "b": {Image: "basic:latest"}},
	}
	r := NewRunner(fc, "", io.Discard)
	r.skipBuild = true
	if err := r.Up(f); err == nil || !strings.Contains(actionErrs(err), "assign agent a") {
		t.Fatalf("Up = %v", err)
	}
	if n := countCalls(fc, "POST /api/agents/a/start"); n != 0 {
		t.Fatalf("a started outside its group: %v", fc.calls)
	}
	if n := countCalls(fc, "POST /api/agents/b/start"); n != 1 {
		t.Fatalf("b did not start: %v", fc.calls)
	}
}

// With no task queue in the file, a failed queue list only leaves out the
// undeclared queues; status still reports.
func TestStatusWithoutQueuesToleratesAFailedQueueList(t *testing.T) {
	fc := newFake()
	fc.fail = map[string]error{"GET /api/task-queues": errors.New("tasks unavailable")}
	var out strings.Builder
	if err := NewRunner(fc, "", &out).Status(File{Version: 1, Agents: map[string]AgentSpec{"a": {Image: "basic:latest"}}}); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "drift: 1") {
		t.Fatalf("status:\n%s", out.String())
	}
}
