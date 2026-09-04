# Goal Delivery Cooldown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop repeated unprocessed `task.goal` inbox deliveries for the same agent while retaining prompt-visible goal guidance and a UI-configurable 60-second default cooldown.

**Architecture:** Extend the existing per-agent Goal state with a positive delivery-cooldown setting and daemon-owned last-delivery timestamp. The existing reconciler remains the sole publisher: it checks pending `task.goal` deliveries and the timestamp before publishing, records a successful publication atomically through the existing store transaction, and leaves message ack/redelivery unchanged. The runtime Goal block stays daemon-owned and is rendered for every Goal-capable image, with concise instructions for active work, customer waits, and PR handoff.

**Tech Stack:** Go 1.26, SQLite migrations, React/TypeScript, Vitest, existing isolated daemon tests.

**Spec:** `docs/superpowers/specs/2026-09-03-agent-goals-design.md` (extended by approved TARI-57 task comments 981–983).

## Global Constraints

- `goal_delivery_cooldown_s` defaults to `60`, must be a positive whole number, and is editable beside Goal settings.
- Do not publish another `task.goal` while a prior `task.goal` delivery for the agent remains unprocessed, regardless of elapsed cooldown.
- Preserve `Publish -> delivery -> WakeMessage`; do not add a scheduler, polling loop, or direct iteration start.
- Goal-capable images always receive the runtime Goal guidance; task title and description remain untrusted input.
- A task with a non-empty PR transitions to `wait_customer`; customer-answer and PR states are explained in runtime guidance.
- Use isolated test state only; do not bump the product version or generate/stage desktop output.

---

### Task 1: Persist the agent delivery cooldown

**Files:**
- Create: `internal/store/migrations/0039_goal_delivery_cooldown.sql`
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/agent_test.go`
- Modify: `internal/commands/agents.go`
- Modify: `internal/commands/agents_test.go`
- Modify: `internal/compose/file.go`
- Modify: `internal/compose/reconcile.go`
- Test: `internal/compose/reconcile_test.go`

**Interfaces:**
- Produces: `Agent.GoalDeliveryCooldownS int`, `Agent.LastGoalDeliveryAt string`, and existing create/update/compose projections for `goal_delivery_cooldown_s`.
- Consumes: existing positive Goal timeout validation and daemon-owned current-goal storage conventions.

- [ ] **Step 1: Write failing persistence and public-API tests**

Add cases proving a migrated agent has cooldown `60` and empty timestamp; create, update, clone, inspect/list, and compose accept a positive whole-second cooldown; zero/negative values leave stored state unchanged.

- [ ] **Step 2: Run the focused tests and observe failure**

Run: `go test ./internal/agent ./internal/commands ./internal/compose -run 'Test.*Goal.*Cooldown' -count=1`

Expected: FAIL because the new field and migration do not exist.

- [ ] **Step 3: Implement the narrow persisted setting**

Add the two agent columns in the next migration. Thread `GoalDeliveryCooldownS` through existing agent scans, inserts, updates, command parameters, and compose reconciliation. Keep `LastGoalDeliveryAt` read-only and updated only by goal publication code.

- [ ] **Step 4: Verify the focused tests pass**

Run: `go test ./internal/agent ./internal/commands ./internal/compose -run 'Test.*Goal.*Cooldown' -count=1`

Expected: PASS.

### Task 2: Gate goal publication at the shared reconciler

**Files:**
- Modify: `internal/taskreminder/store.go`
- Modify: `internal/taskreminder/store_test.go`
- Modify: `internal/taskreminder/reconciler.go`
- Modify: `internal/taskreminder/reconciler_test.go`
- Modify: `internal/tasks/service.go`
- Test: `internal/tasks/service_test.go`

**Interfaces:**
- Produces: a reconciler decision that publishes only when no unprocessed `task.goal` delivery exists and `last_goal_delivery_at + goal_delivery_cooldown_s <= now`.
- Consumes: the existing durable deliveries table, `goalMessage`, and injected clock.

- [ ] **Step 1: Write failing reconciler tests**

Add isolated cases that call `Reconcile` twice with a still-pending first delivery (one message), mark that delivery processed then reconcile before 60 seconds (still one), advance the injected clock to 60 seconds (second eligible delivery), and prove an unrelated processed message does not block goal delivery. Add a task-service case proving setting a non-empty PR URL changes an assigned nonterminal task to `wait_customer` in the same optimistic update.

- [ ] **Step 2: Run and observe the expected failure**

Run: `go test ./internal/taskreminder -run 'TestReconciler.*(Cooldown|Unprocessed)' -count=1`

Expected: FAIL because the current idempotency generation allows repeated selected/iteration-completed deliveries.

- [ ] **Step 3: Implement the smallest shared guard**

Query the existing delivery/message rows for a pending `task.goal` on the agent inbox. Make the store return a publish-eligible decision and persist the timestamp only after `bus.Publish` succeeds; keep existing idempotency and retry behavior. In the existing task update transaction, set `wait_customer` when an assigned agent records a non-empty PR URL. Do not add a second queue or timer.

- [ ] **Step 4: Verify focused and package tests**

Run: `go test ./internal/taskreminder -count=1`

Expected: PASS.

### Task 3: Make runtime Goal guidance unconditional for Goal-capable images

**Files:**
- Modify: `internal/loop/runner.go`
- Modify: `internal/loop/prompt_v2.go`
- Test: `internal/loop/prompt_v2_test.go`
- Test: `internal/loop/runner_test.go`
- Modify: `internal/commands/promptapi.go`
- Test: `internal/commands/promptapi_test.go`

**Interfaces:**
- Produces: a consistent runtime Goal block explaining active execution, `wait_customer`, customer answers, and PR handoff.
- Consumes: `taskreminder.Store.Current` and the existing task skill capability check.

- [ ] **Step 1: Write failing prompt tests**

Add cases for a Goal-capable schema-v2 image without a `runtime: goal` entry and assert the rendered prompt contains the Goal heading and action rules. Assert the block says to wait for a customer answer when status is `wait_customer` and to set/status the task `wait_customer` after recording a PR.

- [ ] **Step 2: Run and observe failure**

Run: `go test ./internal/loop ./internal/commands -run 'Test.*Goal.*(Capable|Guidance|Prompt)' -count=1`

Expected: FAIL because rendering currently depends on a template `runtime: goal` entry and contains only task data.

- [ ] **Step 3: Render one daemon-owned guidance block**

Use the existing Goal/task capability signal rather than inventing image metadata. Remove only the template-entry guard that suppresses Goal runtime for capable images, preserve empty-goal handling, and prepend fixed lifecycle instructions rather than interpolating task text as instructions.

- [ ] **Step 4: Verify prompt tests pass**

Run: `go test ./internal/loop ./internal/commands -run 'Test.*Goal.*(Capable|Guidance|Prompt)' -count=1`

Expected: PASS.

### Task 4: Expose the setting beside Goal in Desktop and document the revised contract

**Files:**
- Modify: `ui/src/lib/types.ts`
- Modify: `ui/src/pages/AgentSettings.tsx`
- Modify: `ui/src/pages/AgentSettings.test.tsx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/architecture/iteration-loop.mdx`
- Modify: `docs/docs/architecture/messaging.mdx`
- Modify: `docs/docs/architecture/web-ui.mdx`
- Modify: `docs/docs/reference/channels.md`

**Interfaces:**
- Produces: a positive integer `Goal delivery cooldown seconds` field saved through the existing Goal section and current documentation of cooldown/unprocessed-delivery and PR/customer-wait behavior.

- [ ] **Step 1: Write the failing UI test**

Extend `AgentSettings.test.tsx` to change the new field to `120`, save Goal settings, and assert the explicit-host request carries `{seconds: 120}` to the existing/new Goal cooldown endpoint; reject `0` and decimal input before POST.

- [ ] **Step 2: Run and observe failure**

Run: `cd ui && npm test -- --run src/pages/AgentSettings.test.tsx`

Expected: FAIL because the UI has no cooldown field or endpoint call.

- [ ] **Step 3: Reuse the Goal section and update product docs**

Add one `FieldSpec` beside the existing wait-customer timeout and no new state-management abstraction. State that the default is one minute, duplicate unprocessed goals are suppressed, goal runtime is always present for capable images, and PR handoff moves the task to `wait_customer`.

- [ ] **Step 4: Verify UI and documentation**

Run: `cd ui && npm test -- --run src/pages/AgentSettings.test.tsx && npx tsc -b && npm run lint && npm run branding:check`

Run: `cd docs && npm run doctor && npm run build`

Expected: PASS.

### Task 5: Integrate and verify

- [ ] **Step 1: Run changed-area verification**

Run: `make check`

Expected: PASS.

- [ ] **Step 2: Inspect the final change**

Run: `git diff --check && git diff --stat && git diff`

Expected: no whitespace errors and only the planned Goal delivery, UI, and documentation files.

- [ ] **Step 3: Commit and use the PR workflow**

Run: `git add <planned files> && git commit -m "fix: throttle unprocessed goal delivery"`

Run the recorded PR-mode ensure/monitor workflow; do not merge locally.
