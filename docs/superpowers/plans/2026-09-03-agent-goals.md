# Agent Goals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `tariboyd` persist one sticky Native Task goal per agent, wake the agent through its existing inbox until an approved release condition, and expose Goal and task PR/wait state through CLI, compose, prompts, and Desktop UI.

**Architecture:** Replace the runtime behavior of the existing `internal/taskreminder` package with a per-agent goal reconciler backed by additive agent columns and the authoritative Native Task tables. Keep `bus.Publish -> delivery -> loop.WakeMessage` as the only execution path; schema-v2 images opt into the daemon-rendered goal through `runtime: goal`.

**Tech Stack:** Go 1.26, SQLite, React 19, TypeScript 6, Python 3 agent-skill scripts, Vitest/Testing Library, Playwright, Tauri WebDriver, Starlight MDX.

**Spec:** `docs/superpowers/specs/2026-09-03-agent-goals-design.md`

## Global Constraints

- `goal_enabled` defaults to `true`; `goal_wait_customer_timeout_s` defaults to `300` and is a positive whole number.
- `current_goal_task_key` is daemon-owned and read-only outside the goal reconciler.
- Keep a selected task sticky until Done/Cancelled, a non-empty structured PR URL, expired customer wait, loss of assignment/visibility, deletion, or Goal disablement.
- Candidate order is priority P0..P3, then `in_progress` before `open`, then `created_at`, then task key.
- Only absolute `http`/`https` PR URLs without credentials are accepted; empty clears the field.
- Reuse ordinary `task.goal` inbox publication and `WakeMessage`; never start an iteration from the reconciler.
- Images without `runtime: goal` receive no goal text; task content remains untrusted task input.
- Remove the global Task reminders runtime and UI, but retain migration history and the inert `task_reminders` table.
- Every daemon/agent test uses isolated state and never reaches live `~/.tariboy`, `~/.tariboyd`, or `127.0.0.1:9990`.
- Do not bump the product version or stage ignored Desktop output.
- Do not merge the branch to `main` until the customer explicitly approves the completed implementation on TARI-43.

---

### Task 1: Migrate and validate Native Task goal-release fields

**Files:**
- Create: `internal/store/migrations/0037_agent_goals.sql`
- Modify: `internal/tasks/model.go`
- Modify: `internal/tasks/store.go`
- Modify: `internal/tasks/workflow.go`
- Modify: `internal/tasks/service.go`
- Test: `internal/tasks/service_test.go`
- Test: `internal/tasks/workflow_test.go`

**Interfaces:**
- Produces: `tasks.StatusWaitCustomer = "wait_customer"`; `Task.PullRequest string`; `UpdateTaskInput.PullRequest *string`; `NormalizePullRequest(string) (string, error)`.
- Consumes: existing optimistic `UpdateTask`, `taskSelect`, `scanTask`, `appendEventTx`, and workflow task mutation paths.

- [ ] **Step 1: Write failing migration/model tests**

```go
func TestAgentGoalsMigrationPreservesTasksAndAddsReleaseFields(t *testing.T) {
	base := migratedStore(t)
	var status, pullRequest string
	err := base.DB.QueryRow(`SELECT status, pull_request FROM tasks WHERE task_key='TEST-1'`).Scan(&status, &pullRequest)
	if err != nil || status != StatusOpen || pullRequest != "" { t.Fatalf("migration = %q %q %v", status, pullRequest, err) }
}

func TestNormalizePullRequest(t *testing.T) {
	for _, bad := range []string{"github.com/o/r/pull/1", "ftp://example.test/1", "https://u:p@example.test/1"} {
		if _, err := NormalizePullRequest(bad); ErrorCode(err) != "invalid_pull_request" { t.Fatalf("%q: %v", bad, err) }
	}
	got, err := NormalizePullRequest(" HTTPS://Example.test/pull/1 ")
	if err != nil || got != "https://example.test/pull/1" { t.Fatalf("got %q, %v", got, err) }
}
```

- [ ] **Step 2: Run the focused tests and verify the expected failure**

Run: `go test ./internal/tasks -run 'TestAgentGoalsMigration|TestNormalizePullRequest' -count=1`

Expected: FAIL because `pull_request`, `StatusWaitCustomer`, and `NormalizePullRequest` do not exist.

- [ ] **Step 3: Add the minimal schema and model implementation**

Rebuild `tasks` in `0037_agent_goals.sql` with the existing columns, foreign keys, workflow columns, indexes, and this changed fragment:

```sql
status TEXT NOT NULL DEFAULT 'open'
  CHECK (status IN ('open','in_progress','wait_customer','done','cancelled')),
pull_request TEXT NOT NULL DEFAULT '',
workflow_version_id INTEGER REFERENCES task_workflow_versions(id) ON DELETE RESTRICT,
workflow_status TEXT,
workflow_revision INTEGER
```

Also add the per-agent columns in the same migration so new and upgraded databases share one atomic feature boundary:

```sql
ALTER TABLE agents ADD COLUMN goal_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agents ADD COLUMN goal_wait_customer_timeout_s INTEGER NOT NULL DEFAULT 300 CHECK(goal_wait_customer_timeout_s > 0);
ALTER TABLE agents ADD COLUMN current_goal_task_key TEXT NOT NULL DEFAULT '';
```

Implement URL validation with `net/url` and no dependency:

```go
func NormalizePullRequest(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" { return "", nil }
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) || u.Host == "" || u.User != nil {
		return "", domainError(http.StatusBadRequest, "invalid_pull_request", "pull request must be an absolute http or https URL without credentials")
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	return u.String(), nil
}
```

Thread `pull_request` through `taskSelect`, `scanTask`, task INSERT/UPDATE statements, normal and workflow event payloads, and add `StatusWaitCustomer` to `validStatus`.

- [ ] **Step 4: Prove task updates preserve revision and workflow behavior**

Add table cases showing that setting/clearing `pull_request` increments revision, emits `task.updated` with `pull_request`, rejects invalid URLs without mutation, accepts `wait_customer`, and leaves workflow-owned lifecycle constraints unchanged.

Run: `go test ./internal/tasks -run 'Test.*(PullRequest|WaitCustomer|AgentGoalsMigration)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrations/0037_agent_goals.sql internal/tasks
git commit -m "feat: add task goal release fields"
```

### Task 2: Make customer waits transition task status atomically

**Files:**
- Modify: `internal/tasks/comments.go`
- Test: `internal/tasks/comments_test.go`

**Interfaces:**
- Consumes: `Service.AddComment`, `openWaitsForPrincipal`, `upsertWait`, `appendEventTx`.
- Produces: automatic `in_progress -> wait_customer -> in_progress` transitions in the same comment transaction.

- [ ] **Step 1: Write failing transaction tests**

```go
func TestAssignedAgentCustomerQuestionTransitionsToWaitCustomer(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusInProgress)
	result, err := svc.AddComment(context.Background(), AgentActor("worker"), task.Key,
		AddCommentInput{Body: "@user:customer choose one", IdempotencyKey: "ask-1"})
	if err != nil || len(result.CreatedWaits) != 1 { t.Fatal(err) }
	assertTaskStatus(t, svc, task.Key, StatusWaitCustomer)
}

func TestFinalCustomerAnswerReturnsWaitCustomerToInProgress(t *testing.T) {
	svc, task := assignedTask(t, "worker", StatusInProgress)
	_, _ = svc.AddComment(context.Background(), AgentActor("worker"), task.Key,
		AddCommentInput{Body: "@user:customer first", IdempotencyKey: "ask-1"})
	_, err := svc.AddComment(context.Background(), CustomerActor("customer"), task.Key,
		AddCommentInput{Body: "answered", IdempotencyKey: "answer-1"})
	if err != nil { t.Fatal(err) }
	assertTaskStatus(t, svc, task.Key, StatusInProgress)
}
```

Use table rows `{actor: agent:other, want: in_progress}`, `{mention: agent:reviewer, want: in_progress}`, `{start: done, want: done}`, `{start: cancelled, want: cancelled}`, and `{manualBeforeAnswer: open, want: open}` to cover the non-transition paths.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/tasks -run 'Test.*Customer.*(Transitions|Answer|Wait)' -count=1`

Expected: FAIL because `AddComment` only increments revision and never changes status.

- [ ] **Step 3: Implement both transitions inside `AddComment`**

After resolving/creating waits and before the event append, derive exactly one status change:

```go
assignedAgentQuestion := task.Assignee == actor.Principal && !actor.IsCustomer &&
	containsPrincipal(created, task.Customer) && task.Status != StatusDone && task.Status != StatusCancelled
lastCustomerWaitResolved := task.Status == StatusWaitCustomer &&
	!hasOpenWait(ctx, tx, task.ID, task.Customer)
switch {
case assignedAgentQuestion:
	task.Status = StatusWaitCustomer
case lastCustomerWaitResolved:
	task.Status = StatusInProgress
}
```

Update status and revision in one `UPDATE`, append `task.updated` only when status changed, then append the existing `task.comment_added`; keep notification outbox rows in the same transaction.

- [ ] **Step 4: Run the focused tests**

Run: `go test ./internal/tasks -run 'Test.*(Comment|WaitCustomer|CustomerAnswer)' -count=1`

Expected: PASS, including idempotent comment replay and multiple-wait cases.

- [ ] **Step 5: Commit**

```bash
git add internal/tasks/comments.go internal/tasks/comments_test.go
git commit -m "feat: transition tasks around customer waits"
```

### Task 3: Persist per-agent Goal settings and ownership

**Files:**
- Modify: `internal/agent/agent.go`
- Test: `internal/agent/agent_test.go`

**Interfaces:**
- Produces: `Agent.GoalEnabled bool`, `Agent.GoalWaitCustomerTimeoutS int`, `Agent.CurrentGoalTaskKey string`; `Store.SetCurrentGoal(name, key string) error`; `Store.Update` clears the key when disabling Goal.
- Consumes: migration `0037_agent_goals.sql` from Task 1.

- [ ] **Step 1: Write failing store tests**

```go
func TestGoalDefaultsAndDisableClearsSelection(t *testing.T) {
	s := newAgentStore(t)
	mustCreateAgent(t, s, Agent{Name: "worker"})
	ag, _ := s.Get("worker")
	if !ag.GoalEnabled || ag.GoalWaitCustomerTimeoutS != 300 || ag.CurrentGoalTaskKey != "" { t.Fatalf("goal defaults: %#v", ag) }
	if err := s.SetCurrentGoal("worker", "TARI-43"); err != nil { t.Fatal(err) }
	ag.GoalEnabled = false
	if err := s.Update(ag); err != nil { t.Fatal(err) }
	ag, _ = s.Get("worker")
	if ag.CurrentGoalTaskKey != "" { t.Fatalf("stale goal %q", ag.CurrentGoalTaskKey) }
}
```

Use timeout inputs `0` and `-1`; both must return `invalid_goal_wait_customer_timeout` and leave the stored value at `300`.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/agent -run 'TestGoal' -count=1`

Expected: FAIL because agent goal fields and setter do not exist.

- [ ] **Step 3: Extend existing CRUD without a side table**

Add the three fields to every agent SELECT/scan, INSERT, UPDATE, clone source, and JSON projection. Default zero-valued create input to enabled/300 at the command boundary; validate positive timeout. Use a guarded setter:

```go
func (s *Store) SetCurrentGoal(name, key string) error {
	res, err := s.db.Exec(`UPDATE agents SET current_goal_task_key=? WHERE name=? AND goal_enabled=1`, key, name)
	if err != nil { return err }
	return affected(res)
}
```

Make `Update` assign `current_goal_task_key = CASE WHEN ? THEN current_goal_task_key ELSE '' END` so disabling cannot strand ownership.

- [ ] **Step 4: Run agent store tests**

Run: `go test ./internal/agent -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/agent.go internal/agent/agent_test.go
git commit -m "feat: persist per-agent goal settings"
```

### Task 4: Replace reminder selection with sticky goal selection

**Files:**
- Delete: `internal/taskreminder/config.go`
- Delete: `internal/taskreminder/config_test.go`
- Modify: `internal/taskreminder/store.go`
- Modify: `internal/taskreminder/store_test.go`

**Interfaces:**
- Produces: `Goal{Agent, TaskKey, Revision, Reason, Waiting bool}`; `Store.ReconcileAgent(agent string, now time.Time) (Goal, error)`; `Store.Current(agent string, now time.Time) (tasks.Task, bool, error)`.
- Consumes: `agents.goal_*`, `agents.current_goal_task_key`, tasks, waits, iterations, direct assignee principal.

- [ ] **Step 1: Replace reminder tests with failing policy tests**

Use a fixed clock and table-driven database fixtures covering all order dimensions and release conditions:

```go
func TestReconcileAgentSelectsAndKeepsStickyGoal(t *testing.T) {
	s := goalStore(t, time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC))
	seedTask(t, s, "T-1", "agent:worker", "P1", "open", "2026-09-01T00:00:00Z")
	seedTask(t, s, "T-2", "agent:worker", "P1", "in_progress", "2026-09-02T00:00:00Z")
	goal, err := s.ReconcileAgent("worker", now)
	if err != nil || goal.TaskKey != "T-2" || goal.Reason != "selected" { t.Fatalf("goal=%#v err=%v", goal, err) }
	seedTask(t, s, "T-0", "agent:worker", "P0", "in_progress", "2026-08-01T00:00:00Z")
	goal, _ = s.ReconcileAgent("worker", now)
	if goal.TaskKey != "T-2" { t.Fatalf("sticky goal changed to %q", goal.TaskKey) }
}
```

Use table rows for release `{status: done}`, `{status: cancelled}`, `{pullRequest: https://example.test/pull/1}`, `{deleteTask: true}`, `{assignee: agent:other}`, `{goalEnabled: false}`, and `{loopEnabled: false}`. Use fixed wait ages `299s` and `300s`, a `wait_customer` row without an unresolved wait, an answered wait returning to `in_progress`, and equal candidates `T-1`/`T-2`; assert the exact selected or empty key for every row.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/taskreminder -run 'TestReconcileAgent|TestCurrent' -count=1`

Expected: FAIL because the package still implements idle reminder generations.

- [ ] **Step 3: Implement selection as one transaction**

Within `ReconcileAgent`, load the agent flags/timeout/current key, validate the sticky task, and otherwise select one candidate with SQL ordering:

```sql
SELECT t.task_key, t.revision
FROM tasks t
WHERE t.assignee='agent:' || ?
  AND t.pull_request=''
  AND t.status IN ('in_progress','open')
ORDER BY CASE t.priority WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 WHEN 'P2' THEN 2 ELSE 3 END,
         CASE t.status WHEN 'in_progress' THEN 0 ELSE 1 END,
         t.created_at, t.task_key
LIMIT 1
```

Use the oldest unresolved wait for `task.customer` to evaluate timeout, update `current_goal_task_key` before commit, and return an empty Goal when no candidate exists. `Current` reuses the same validation transaction and returns the selected `tasks.Task` without publishing.

- [ ] **Step 4: Run all selection tests**

Run: `go test ./internal/taskreminder -count=1`

Expected: PASS with no reads or writes to `daemon_config.task_reminder` or `task_reminders`.

- [ ] **Step 5: Commit**

```bash
git add -A internal/taskreminder
git commit -m "feat: select sticky agent goals"
```

### Task 5: Publish idempotent goal wakes on recovery and iteration completion

**Files:**
- Modify: `internal/taskreminder/reconciler.go`
- Modify: `internal/taskreminder/reconciler_test.go`
- Modify: `internal/tasks/service.go`
- Modify: `internal/loop/engine.go`
- Modify: `internal/loop/manager.go`
- Modify: `internal/daemon/daemon.go`
- Test: `internal/loop/engine_test.go`
- Test: `internal/daemon/daemon_test.go`

**Interfaces:**
- Produces: `Reconciler.Signal()`; `Reconciler.IterationCompleted(agent, iterationID string)`; coalesced signal channel; `task.goal` messages.
- Consumes: Task 4 `ReconcileAgent`, `bus.Publish`, existing publish hook, manager/engine terminal outcome path.

- [ ] **Step 1: Write failing delivery and trigger tests**

```go
func TestReconcilerPublishesGoalThroughInbox(t *testing.T) {
	r, bus := seededGoalReconciler(t)
	if err := r.Reconcile(context.Background(), "", ""); err != nil { t.Fatal(err) }
	msg := onlyMessage(t, bus)
	if msg.Type != "task.goal" || msg.Channel != "agent:worker:inbox" || msg.Data["reason"] != "selected" { t.Fatalf("%#v", msg) }
}
```

Use named cases `startup_duplicate`, `recovery_duplicate`, `terminal_iteration_once`, `waiting_customer`, `no_candidate`, `disabled_agent`, and `first_publish_fails`. Assert exact message counts `[1,1,1,0,0,0,1]`; the terminal case uses iteration `iter-7` and reason `iteration_completed`, while the failure case still publishes the second agent.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/taskreminder ./internal/loop ./internal/daemon -run 'Test.*Goal' -count=1`

Expected: FAIL because only minute-based reminder reconciliation exists.

- [ ] **Step 3: Implement coalesced triggers and stable idempotency**

Use one buffered channel and the existing minute ticker:

```go
func (r *Reconciler) Signal() { select { case r.signals <- struct{}{}: default: } }

func goalMessage(goal Goal, iterationID string) bus.Message {
	reason := goal.Reason
	return bus.Message{
		IdempotencyKey: fmt.Sprintf("task-goal:%s:%s:%d:%s", goal.Agent, goal.TaskKey, goal.Revision, iterationID),
		Channel: bus.InboxChannel(goal.Agent), Source: "tasks", Type: "task.goal",
		Data: map[string]any{"task_key": goal.TaskKey, "reason": reason},
	}
}
```

`Run` performs startup reconciliation, then selects on context, ticker, and `signals`. Task service mutations call the coalesced signal callback. Engine calls the terminal callback after the iteration row is finalized, passing the stable iteration ID. Agent enable/loop/Goal mutations signal through manager configuration. Keep publish errors joined per scan and logged without terminating `Run`.

- [ ] **Step 4: Run focused and package tests**

Run: `go test ./internal/taskreminder ./internal/tasks ./internal/loop ./internal/daemon -count=1`

Expected: PASS; existing bus hook tests still prove `WakeMessage` is downstream of publication.

- [ ] **Step 5: Commit**

```bash
git add internal/taskreminder internal/tasks/service.go internal/loop internal/daemon/daemon.go internal/daemon/daemon_test.go
git commit -m "feat: reconcile and wake agent goals"
```

### Task 6: Render the authoritative goal in schema-v2 prompts

**Files:**
- Modify: `internal/imagefile/v2.go`
- Modify: `internal/imagefile/v2_test.go`
- Modify: `internal/loop/prompt_v2.go`
- Modify: `internal/loop/prompt_v2_test.go`
- Modify: `internal/loop/runner.go`
- Modify: `internal/commands/promptapi.go`
- Modify: `internal/commands/promptapi_test.go`
- Modify: `internal/builtinimages/source/Tariboyfile.yaml`
- Modify: `store/images/tariboy-developer/Tariboyfile.yaml`
- Test: `internal/builtinimages/builtinimages_test.go`

**Interfaces:**
- Produces: `RuntimePromptValues.Goal string`; `FormatRuntimeGoal(tasks.Task) string`; accepted singleton `runtime: goal` mapped to skill `tasks`.
- Consumes: Task 4 `Store.Current`, schema-v2 renderer, basic/developer image sources.

- [ ] **Step 1: Write failing placeholder and renderer tests**

```go
func TestRenderPromptTemplateGoal(t *testing.T) {
	template := runtimeTemplate(t, "goal")
	got, err := RenderPromptTemplate(template, t.TempDir(), RuntimePromptValues{Goal: "# Agent Goal\n\nkey: TARI-43"})
	if err != nil { t.Fatal(err) }
	want := "Use the `tasks` skill for this runtime data.\n\n# Agent Goal\n\nkey: TARI-43\n"
	if got != want { t.Fatalf("got %q", got) }
}
```

Use validation inputs `[runtime: goal]` (success) and `[runtime: goal, runtime: goal]` (`duplicate runtime placeholder "goal"`); render an empty Goal as no output and description `line one\nline two` as two literal prompt lines. Parse both shipped YAML files and assert one `PromptEntry{Runtime:"goal"}` in each.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/imagefile ./internal/loop ./internal/commands ./internal/builtinimages -run 'Test.*Goal' -count=1`

Expected: FAIL because `goal` is not a known runtime placeholder.

- [ ] **Step 3: Add the runtime value at the existing render seam**

```go
func FormatRuntimeGoal(task tasks.Task) string {
	return fmt.Sprintf("# Agent Goal\n\nkey: %s\ntitle: %s\npriority: %s\nstatus: %s\ndescription: %s",
		task.Key, task.Title, task.Priority, task.Status, task.Description)
}
```

Add `goal` to `runtimePromptNames`, the runtime value map, and `runtimeSkills` as `tasks`. At iteration preparation and prompt preview, load `Current(agent, now)`; pass an empty Goal when disabled or absent. Place `runtime: goal` after identity in both image templates. Do not add a plugin or a new prompt file.

- [ ] **Step 4: Run image/prompt tests**

Run: `go test ./internal/imagefile ./internal/image ./internal/loop ./internal/commands ./internal/builtinimages -count=1`

Expected: PASS, including unchanged images that omit the placeholder.

- [ ] **Step 5: Commit**

```bash
git add internal/imagefile internal/loop internal/commands/promptapi.go internal/commands/promptapi_test.go internal/builtinimages store/images/tariboy-developer/Tariboyfile.yaml
git commit -m "feat: render agent goals in image prompts"
```

### Task 7: Expose task PR and wait status through API, CLI, and agent tools

**Files:**
- Modify: `internal/commands/tasks.go`
- Modify: `internal/commands/tasks_test.go`
- Modify: `internal/commands/tasks_openapi.go`
- Modify: `store/skills/tasks/scripts/tasks.py`
- Modify: `store/skills/test_store_skills.py`

**Interfaces:**
- Produces: `PATCH /api/tasks/{key}` parameter `pull_request`; `tasks update KEY --pull-request URL`; status accepts `wait_customer`; task schema exposes `pull_request`.
- Consumes: Task 1 `UpdateTaskInput.PullRequest` and task JSON.

- [ ] **Step 1: Write failing command/script tests**

```go
func TestTaskUpdateAcceptsPullRequestAndWaitCustomer(t *testing.T) {
	params := registry.Params{"key":"TARI-43", "revision":int64(2), "pull_request":"https://github.com/o/r/pull/7", "status":"wait_customer"}
	got := captureTaskUpdate(t, params)
	if got.PullRequest == nil || *got.PullRequest != "https://github.com/o/r/pull/7" || got.Status == nil || *got.Status != StatusWaitCustomer {
		t.Fatalf("update input = %#v", got)
	}
}
```

```python
def test_tasks_update_pull_request(call):
    run_tasks("update", "TARI-43", "--revision", "2", "--pull-request", "https://github.com/o/r/pull/7")
    assert call.body["pull_request"] == "https://github.com/o/r/pull/7"
```

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/commands -run 'TestTask.*(PullRequest|WaitCustomer)' -count=1 && python3 -m unittest store.skills.test_store_skills`

Expected: FAIL because the command descriptors and parser reject/omit the new values.

- [ ] **Step 3: Thread the existing update field**

Add `PullRequest: optionalStringParam(p, "pull_request")` to `tasks.update`, update the command/OpenAPI schemas, and let `tasks.py` copy `--pull-request` into the PATCH body. Extend only the existing status choices/help text; do not add a separate PR command.

- [ ] **Step 4: Run command and Store skill tests**

Run: `go test ./internal/commands -run 'TestTask' -count=1 && python3 -m unittest store.skills.test_store_skills`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/tasks.go internal/commands/tasks_test.go internal/commands/tasks_openapi.go store/skills/tasks/scripts/tasks.py store/skills/test_store_skills.py
git commit -m "feat: expose task pull requests"
```

### Task 8: Expose per-agent Goal settings through API, CLI, clone, and compose

**Files:**
- Modify: `internal/commands/agents.go`
- Modify: `internal/commands/agents_test.go`
- Modify: `internal/commands/registrycmd.go`
- Modify: `internal/registry/registry.go`
- Modify: `internal/compose/file.go`
- Modify: `internal/compose/reconcile.go`
- Modify: `internal/compose/file_test.go`
- Modify: `internal/compose/reconcile_test.go`
- Modify: `ui/src/lib/types.ts`
- Modify: `ui/src/pages/terminals/agentCreateDraft.ts`
- Modify: `ui/src/pages/terminals/agentCreateDraft.test.ts`
- Modify: `ui/src/pages/terminals/CreateAgentDialog.tsx`
- Modify: `ui/src/pages/terminals/CreateAgentDialog.test.tsx`

**Interfaces:**
- Produces: agent create/update fields `goal_enabled`, `goal_wait_customer_timeout_s`; read-only `current_goal_task_key`; compose `GoalSpec{Enabled *bool, WaitCustomerTimeout string}`.
- Consumes: Task 3 agent store fields and existing duration parsing/config mutation paths.

- [ ] **Step 1: Write failing round-trip tests**

```go
func TestAgentGoalSettingsRoundTrip(t *testing.T) {
	result := runAgentCreate(t, map[string]any{"name":"worker", "goal_enabled":false, "goal_wait_customer_timeout_s":120})
	if result["goal_enabled"] != false || result["goal_wait_customer_timeout_s"] != 120 { t.Fatalf("%#v", result) }
}

func TestComposeGoalDuration(t *testing.T) {
	spec := parseCompose(t, "agents:\n  worker:\n    image: basic:latest\n    goal:\n      enabled: true\n      wait_customer_timeout: 5m\n")
	if spec.Agents["worker"].Goal.WaitCustomerTimeout != "5m" { t.Fatalf("%#v", spec) }
}
```

Add invalid/zero duration, omitted-field preservation, inspect/list, update disable clearing, and clone-copy cases. Add UI draft tests proving complete cross-host clone copies enabled/timeout but never writes current key.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/commands ./internal/compose -run 'Test.*Goal' -count=1 && cd ui && npm test -- --run src/pages/terminals/agentCreateDraft.test.ts src/pages/terminals/CreateAgentDialog.test.tsx`

Expected: FAIL because Goal fields are not on public agent configuration.

- [ ] **Step 3: Extend existing agent configuration paths**

```go
type GoalSpec struct {
	Enabled *bool `yaml:"enabled"`
	WaitCustomerTimeout string `yaml:"wait_customer_timeout"`
}
```

Parse the duration with existing compose duration helpers, require whole positive seconds, and preserve current values when fields are omitted. Add explicit create/update CLI arguments and JSON projection fields. Extend `AgentView` and create draft/default/clone payloads; default new agents to enabled/300 and copy editable fields during clone.

- [ ] **Step 4: Run backend and create/clone UI tests**

Run: `go test ./internal/agent ./internal/commands ./internal/compose -count=1 && cd ui && npm test -- --run src/pages/terminals/agentCreateDraft.test.ts src/pages/terminals/CreateAgentDialog.test.tsx`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/commands internal/registry internal/compose ui/src/lib/types.ts ui/src/pages/terminals
git commit -m "feat: configure goals for agents"
```

### Task 9: Replace Task reminders UI with Goal and task PR controls

**Files:**
- Delete: `ui/src/pages/settings/TaskReminderSettings.tsx`
- Delete: `ui/src/pages/settings/TaskReminderSettings.test.tsx`
- Modify: `ui/src/lib/api.ts`
- Modify: `ui/src/lib/api.test.ts`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/pages/settings/SettingsPage.tsx`
- Modify: `ui/src/pages/settings/SettingsPage.test.tsx`
- Modify: `ui/src/pages/AgentSettings.tsx`
- Modify: `ui/src/pages/AgentSettings.test.tsx`
- Modify: `ui/src/lib/tasks.ts`
- Modify: `ui/src/lib/tasks.test.ts`
- Modify: `ui/src/pages/tasks/TaskDetail.tsx`
- Modify: `ui/src/pages/tasks/TasksWorkspace.tsx`
- Modify: `ui/src/pages/tasks/TasksWorkspace.test.tsx`

**Interfaces:**
- Produces: per-agent Goal section using existing `useSectionDraft`; `Wait customer` option; editable Pull request URL sent via existing optimistic save.
- Consumes: Tasks 7-8 API fields and explicit host routing.

- [ ] **Step 1: Write failing Goal section and task detail tests**

```tsx
it("saves Goal settings as one explicit-host section", async () => {
  renderAgentSettings({ goal_enabled: true, goal_wait_customer_timeout_s: 300, current_goal_task_key: "TARI-43" });
  await user.click(screen.getByRole("switch", { name: "Enable Goal" }));
  await user.clear(screen.getByLabelText("Wait customer timeout seconds"));
  await user.type(screen.getByLabelText("Wait customer timeout seconds"), "120");
  await user.click(screen.getByRole("button", { name: "Save Goal settings" }));
  expect(agentPost).toHaveBeenCalledWith("worker", "goal-enabled", { enabled: false });
  expect(agentPost).toHaveBeenCalledWith("worker", "goal-wait-customer-timeout", { seconds: 120 });
});
```

Use UI cases with timeout inputs `0`, `1.5`, and `120`; discard from `120` back to `300`; fail the second sequential save and assert only that draft remains dirty; render key `TARI-43` in a disabled input; use target `{baseUrl:"https://remote.test",token:"secret"}`; select `wait_customer`; save then clear `https://example.test/pull/7`; and assert every PATCH retains the loaded revision.

- [ ] **Step 2: Run and observe failure**

Run: `cd ui && npm test -- --run src/pages/AgentSettings.test.tsx src/pages/tasks/TasksWorkspace.test.tsx src/pages/settings/SettingsPage.test.tsx`

Expected: FAIL because the global reminder route exists and per-agent Goal fields do not.

- [ ] **Step 3: Reuse the existing section and optimistic task save patterns**

Add two `FieldSpec`s to a `GOAL_FIELDS` section, use the existing dirty/discard/sequential-save machinery, and show `current_goal_task_key || "No current goal"` read-only. Remove Task reminders navigation, route, config parsing helpers, and API tests. Extend `TaskStatus`, `Task`, update input, `TaskDetail` local state, and `TasksWorkspace` save body with `pull_request`.

- [ ] **Step 4: Run UI unit, type, lint, and branding checks**

Run: `cd ui && npm test -- --run src/pages/AgentSettings.test.tsx src/pages/tasks/TasksWorkspace.test.tsx src/pages/settings/SettingsPage.test.tsx src/lib/tasks.test.ts && npx tsc -b && npm run lint && npm run branding:check`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A ui/src
git commit -m "feat: configure goals in desktop"
```

### Task 10: Prove production Desktop behavior

**Files:**
- Modify: `ui/tests/tasks-e2e.pw.ts`
- Modify: `ui/tests/desktop/tasks-priority.pw.ts`
- Create: `ui/tests/desktop/agent-goal.pw.ts`

**Interfaces:**
- Consumes: production Task and Agent Configuration components plus real isolated daemon fixtures.
- Produces: browser and `tauri-driver` coverage required for shared UI changes.

- [ ] **Step 1: Add failing production-flow assertions**

Extend the browser Task flow to set `Wait customer`, save/clear a PR URL, reload, and assert both persist. Add a Desktop WebDriver scenario that creates `worker` and `TARI-1` through the isolated daemon HTTP API, opens Agent Configuration, disables/re-enables Goal, changes timeout to `120`, verifies the current goal key, then reopens the task and verifies its structured PR field.

```ts
test("configures an agent goal and task release fields", async ({ desktop }) => {
  await waitForMainWindow(desktop);
  await desktop.execute(`
    const status = await window.__TAURI_INTERNALS__.invoke("daemon_status");
    const call = async (method, path, body) => fetch(status.base_url + path, {
      method, headers: {"content-type":"application/json"}, body: JSON.stringify(body)
    }).then((response) => response.json()).then((envelope) => envelope.result);
    await call("POST", "/api/agents", {name:"worker", image:"basic:latest", harness:"stub", loop:false});
    await call("POST", "/api/task-queues", {prefix:"TARI", name:"Goals"});
    await call("POST", "/api/tasks", {queue:"TARI", title:"Goal one", assignee:"agent:worker"});
    window.location.hash = "#/servers/local/agents/worker/configuration";
  `);
  await expect.poll(() => desktop.execute("return document.body.innerText")).toContain("Current goal");
});
```

- [ ] **Step 2: Run focused browser tests and observe failure before UI implementation is complete**

Run: `cd ui && npm run test:tasks-browser -- --grep "release fields"`

Expected: FAIL until Task 9's controls and API round-trip are present.

- [ ] **Step 3: Run the real production browser and Desktop checks**

Run: `cd ui && npm run test:tasks-browser`

Run: `. "$HOME/.cargo/env" && make desktop-e2e-build && make desktop-e2e DESKTOP_E2E_ARGS="tests/desktop/agent-goal.pw.ts tests/desktop/tasks-priority.pw.ts"`

Expected: PASS using fixture-owned base/runtime/listener state; no live daemon interaction.

- [ ] **Step 4: Commit**

```bash
git add ui/tests/tasks-e2e.pw.ts ui/tests/desktop/tasks-priority.pw.ts ui/tests/desktop/agent-goal.pw.ts
git commit -m "test: cover agent goals in desktop"
```

### Task 11: Update current documentation and remove reminder claims

**Files:**
- Modify: `README.md`
- Modify: `docs/docs/tasks.mdx`
- Modify: `docs/docs/architecture/index.mdx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/architecture/iteration-loop.mdx`
- Modify: `docs/docs/architecture/messaging.mdx`
- Modify: `docs/docs/reference/channels.md`
- Modify: `docs/docs/architecture/web-ui.mdx`
- Modify: `docs/docs/images/index.mdx`
- Modify: `docs/docs/images-and-groups/index.mdx`
- Modify: `docs/docs/binaries/operator-cli.mdx`
- Modify: `docs/docs/binaries/agent-tools.mdx`
- Modify: `docs/docs/binaries/compose.mdx`

**Interfaces:**
- Consumes: completed behavior from Tasks 1-10.
- Produces: one current product contract with no stale global reminder/default-off/idle-threshold claims.

- [ ] **Step 1: Update documentation with exact behavior**

Document: per-agent default-on settings; sticky ordering/release rules; structured `pull_request`; automatic customer wait transitions; `task.goal` delivery; `runtime: goal`; Goal CLI/compose/UI fields; basic/developer image inclusion; and the fact that disabled agent/Autopilot state queues no new goal wake. Remove Task reminder routes, daemon config examples, and idle-generation language.

- [ ] **Step 2: Search for stale literals and fix every product claim**

Run: `rg -n "Task reminders|task reminder|task_reminder|task\.reminder|idle_threshold_s" README.md docs/docs ui/src internal --glob '!internal/store/migrations/0036_task_reminders.sql' --glob '!docs/superpowers/**'`

Expected: only intentional historical/code compatibility references remain; no active product documentation or UI route claims the removed policy.

- [ ] **Step 3: Run documentation gates**

Run: `cd docs && npm run doctor && npm run build`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/docs
git commit -m "docs: describe agent goals"
```

### Task 12: Complete branch verification and hold for merge approval

The first `make full-check` run exposed the tmux supervisor lifecycle defect
described in the spec. Complete Task 13 before rerunning this task from Step 1;
the earlier failed run is diagnostic evidence, not final verification.

**Files:**
- Inspect: all changes from `main...HEAD`
- Do not modify version pins or generated Desktop output.

**Interfaces:**
- Consumes: all prior tasks.
- Produces: a verified local branch ready for customer review, not a merge.

- [ ] **Step 1: Run the complete relevant suite**

Run: `make full-check`

Expected: all summary rows PASS, including `check`, build, core E2E, full smoke, browser suites, and Linux `desktop-e2e` on this host.

- [ ] **Step 2: Run final repository hygiene checks**

Run: `git diff --check main...HEAD`

Run: `git status --short --branch`

Run: `git diff --stat main...HEAD && git diff main...HEAD`

Expected: no whitespace errors, only TARI-43 files, no version bump, no ignored Desktop artifacts staged.

- [ ] **Step 3: Request code review and resolve findings**

Use `superpowers:requesting-code-review`; resolve every Critical and Important finding, then rerun only verification invalidated by the changed state.

- [ ] **Step 4: Record branch readiness and ask for merge approval on TARI-43**

Post the commits, full-check result, review result, and branch name. Ask the customer to review the local branch and explicitly approve or reject merging it into `main`. Record the resulting task-question ID as the only wait object.

- [ ] **Step 5: After explicit approval only, finish local integration**

Use `superpowers:finishing-a-development-branch`: merge `tari-43-agent-goals` into local `main`, run the distinct post-merge `make full-check`, remove the worktree and local branch, post the consolidated final Native Task comment, run `tasks done TARI-43`, and remove TARI-43 from durable context. If approval is not present, do none of these actions.

### Task 13: Make the existing shim supervisor observe real child exit

**Files:**
- Create: `internal/shim/child_exit_linux.go`
- Create: `internal/shim/child_exit_darwin.go`
- Modify: `internal/shim/shim.go`
- Modify: `internal/shim/shim_test.go`
- Modify: `docs/docs/architecture/shim.mdx`

**Interfaces:**
- Produces: `waitChildExit(pid int) error`, implemented with Linux
  `waitid(P_PID, pid, WEXITED|WNOWAIT)` and Darwin
  `kqueue` `EVFILT_PROC|NOTE_EXIT`; neither implementation reaps the child.
- Consumes: the existing `RunTmuxSupervisor`, owned foreground process group,
  transient tmux status file, and `golang.org/x/sys/unix` dependency.

- [ ] **Step 1: Write the failing stop/continue regression test**

Start `RunTmuxSupervisor` around a harness that records its PID, traps `TERM`,
and exits `23` only after a release file appears. Send `SIGSTOP` and `SIGCONT`
to the harness leader, assert that the supervisor remains running and writes no
status, create the release file, then assert successful completion and exact
status `23`.

```go
func TestRunTmuxSupervisorIgnoresStoppedHarness(t *testing.T) {
	// Start the supervised harness and read its PID marker.
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil { t.Fatal(err) }
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil { t.Fatal(err) }
	select {
	case err := <-done:
		t.Fatalf("supervisor treated stop/continue as exit: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Fatalf("status written before exit: %v", err)
	}
	if err := os.WriteFile(releasePath, nil, 0o600); err != nil { t.Fatal(err) }
	if err := <-done; err != nil { t.Fatal(err) }
	if got := readSupervisorStatus(t, statusPath); got != 23 { t.Fatalf("status = %d, want 23", got) }
}
```

- [ ] **Step 2: Run the regression test and observe the current failure**

Run: `go test ./internal/shim -run TestRunTmuxSupervisorIgnoresStoppedHarness -count=1`

Expected: FAIL because the first `SIGCHLD` notification currently triggers
group termination and records `137` instead of waiting for real exit `23`.

- [ ] **Step 3: Add focused lifecycle and failure tests**

Keep the existing HUP and natural-status coverage, and add table-driven cases
for: natural leader exit with a lingering descendant; HUP with a
`TERM`-cooperative leader; HUP with a `TERM`-ignoring leader that requires
`SIGKILL`; observer failure; group-signal failure; reap failure; and expiry of
each bounded wait. Inject only the three syscall seams used by the supervisor.
Every operational failure must return promptly and leave the status path
absent.

```go
var (
	waitTmuxChildExit = waitChildExit
	killTmuxGroup = syscall.Kill
	reapTmuxChild = syscall.Wait4
)
```

- [ ] **Step 4: Implement the two native observers**

In `child_exit_linux.go`, retry `unix.Waitid` on `EINTR` and use only
`unix.WEXITED|unix.WNOWAIT`, so stop/continue events are excluded and the
leader remains unreaped. In `child_exit_darwin.go`, create one kqueue, register
the direct PID with `EVFILT_PROC`, `EV_ADD|EV_ONESHOT`, and `NOTE_EXIT`, wait
through `EINTR`, validate the returned event, and close the queue on return.
Do not add a goroutine, polling loop, helper executable, or dependency in these
platform files.

- [ ] **Step 5: Replace signal-driven exit with bounded shim-owned teardown**

`RunTmuxSupervisor` listens only for `SIGHUP` and runs `waitChildExit` in its
single observer goroutine. Natural exit performs group `SIGKILL` before the
sole blocking reap. HUP performs `SIGTERM`, waits two seconds for confirmed
exit, escalates to `SIGKILL`, and waits at most two more seconds. Treat `ESRCH`
as success. Join operational errors, attempt best-effort final `SIGKILL`, and
write the status file only when exit observation, cleanup, and the exact
`Wait4(pid, ...)` reap all succeed.

- [ ] **Step 6: Run focused tests and platform compilation**

Run: `go test ./internal/shim -count=1`

Run: `GOOS=darwin GOARCH=arm64 go test ./internal/shim -run '^$'`

Expected: PASS; the Linux suite proves the lifecycle behavior and the Darwin
compile proves the kqueue backend remains buildable.

- [ ] **Step 7: Update the shim architecture contract**

Replace the first-`SIGCHLD` wording with the real-exit observer, bounded
`TERM`→`KILL` teardown, kill-before-reap ordering, `ESRCH` handling, and
fail-closed missing-status behavior. Keep the document explicit that this is
the existing shim process and no helper process is started.

- [ ] **Step 8: Run the isolated Desktop regression**

Run: `. "$HOME/.cargo/env" && make desktop-e2e DESKTOP_E2E_ARGS="tests/desktop/customer-question-notification.pw.ts"`

Expected: PASS with fixture-owned base/runtime/listener state and no surviving
harness descendant after agent kill.

- [ ] **Step 9: Commit**

```bash
git add internal/shim/child_exit_linux.go internal/shim/child_exit_darwin.go internal/shim/shim.go internal/shim/shim_test.go docs/docs/architecture/shim.mdx
git commit -m "fix(shim): observe harness exit safely"
```

After this task passes its review gate, return to Task 12 Step 1. Do not merge
the branch without the separate explicit customer approval recorded on
TARI-43.
