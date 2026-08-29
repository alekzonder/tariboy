# Automatic Judge Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add daemon-authoritative JSON configuration that drives complete scheduled Judge cycles, Native Task reporting, approved improvement tasks, CLI control, and a raw JSON UI editor.

**Architecture:** Store one revisioned Judge automation document and compile it into the existing durable schedule subsystem. A schedule wake lets the configured lead begin a durable cycle; the current Judge runner and bus wake paths execute the analyses and summary. Native Tasks remain the operator/customer workflow, and approved proposals create idempotent `IMPROVE` tasks in the same approval transaction.

**Tech Stack:** Go, SQLite, existing schedule/bus/Judge/Tasks services, React/TypeScript, existing CLI registry, Vitest, Playwright, and Tauri WebDriver.

**Spec:** `docs/superpowers/specs/2026-08-29-judge-automation-design.md`

## Global Constraints

- Do not add a scheduler, cron parser, workflow engine, editor dependency, or client-side business validator.
- Agent names and image refs come only from the stored JSON document.
- Queue prefixes are exactly `JUDGE` and `IMPROVE`; customer identity is exactly `user:${USER}` using the daemon environment.
- Use exactly two configured Judge workers.
- The daemon owns parsing, validation, canonicalization, revisions, apply, one-shot creation, and task side effects.
- CLI and UI are thin clients of the same daemon routes.
- Existing Judge evidence, approval hashes, loopback, redaction, and safe image activation boundaries remain intact.
- All daemon/e2e tests use isolated `TARIBOY_BASE_DIR` and `TARIBOY_RUNTIME_DIR`; never test against the live daemon.
- Set release version `0.44.0` only with `scripts/set-version.sh 0.44.0` after behavior and verification are complete.

---

## File Structure

- `internal/store/migrations/0038_judge_automation.sql` — singleton config revisions, cycles, task/run links, and approval-task idempotency.
- `internal/judge/automation.go` — config types, strict parsing, validation diagnostics, canonical projection, apply, run-once, and cycle lifecycle.
- `internal/judge/automation_test.go` — real SQLite tests for validation, reconciliation, idempotency, and cycle state.
- `internal/judge/model.go`, `select.go`, `store.go` — exact image-ref and unprocessed selectors.
- `internal/judge/service.go`, `runner.go` — authorized cycle begin and terminal callbacks.
- `internal/tasks/admin.go`, task storage helpers — transaction-aware queue/task creation reused by automation and approvals.
- `internal/improvement/store.go`, `service.go` — atomic plan approval plus `IMPROVE` task creation.
- `internal/commands/judge.go`, `daemon.go`, `internal/registry/registry.go` — operator HTTP/CLI registry routes.
- `internal/toolscli/toolscli.go` — lead-only `tools judge automation begin` wrapper.
- `ui/src/lib/judge.ts`, `ui/src/pages/JudgeRunsPage.tsx` and tests — raw JSON editor and daemon diagnostics.
- `store/images/llm-as-judge/*` — schema-v2 Judge image with schedule and Tasks capabilities and automatic lead/worker procedures.
- Product docs under `docs/docs/` — current operator/API/UI behavior.

---

### Task 1: Persist and validate the automation document

**Files:**
- Create: `internal/store/migrations/0038_judge_automation.sql`
- Create: `internal/judge/automation.go`
- Create: `internal/judge/automation_test.go`
- Modify: `internal/judge/model.go`
- Modify: `internal/judge/select.go`
- Modify: `internal/judge/store_test.go`

**Interfaces:**
- Produces `AutomationConfig`, `AutomationRevision`, `ValidationDiagnostic`, `ParseAutomation(raw []byte)`, and `ValidateAutomation(ctx, config)`.
- Extends `Selector` with `ImageRefs []string` and `OnlyUnprocessed bool`.

- [ ] **Step 1: Write failing strict-validation tests**

Create table-driven tests that submit literal JSON and assert JSON Pointer diagnostics for an unknown field, empty `USER`, duplicate agents/images, worker count other than two, overlapping Judge/target agents, missing agents/images, unsupported Judge capabilities, and invalid cron. Add one valid document whose canonical output is stable across whitespace and object-key order.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/judge -run 'Automation|SelectorImage|OnlyUnprocessed'`

Expected: compile failures for the missing automation types and selector fields.

- [ ] **Step 3: Implement strict parsing and storage**

Use `json.Decoder.DisallowUnknownFields`, existing cron parsing, agent/image stores, `os.Getenv("USER")`, and canonical `encoding/json`. Migration `0038` stores immutable revisions plus one active revision pointer; no schema library is added.

- [ ] **Step 4: Implement exact selector filters**

Add `i.image_ref IN (...)` and, when requested, `NOT EXISTS` over `judge_targets JOIN judge_runs` where status is not `cancelled`. Apply these predicates before ordering and limit so a three-target one-shot actually selects three eligible rows.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/judge`

Commit: `feat: validate judge automation configuration`

### Task 2: Apply config through existing queues and schedules

**Files:**
- Modify: `internal/judge/automation.go`
- Modify: `internal/judge/automation_test.go`
- Modify: `internal/schedule/store.go`
- Modify: `internal/schedule/store_test.go`
- Modify: `internal/tasks/admin.go`
- Modify: `internal/tasks/admin_test.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/daemon/daemon.go`

**Interfaces:**
- Produces `AutomationService.Get`, `Validate`, `Apply`, and `RunOnce`.
- Produces transaction-aware helpers to upsert `JUDGE`/`IMPROVE` queues and replace the one automation-owned schedule.

- [ ] **Step 1: Write failing apply tests**

Against a real temporary store, assert invalid apply changes nothing; valid apply creates both queues, one recurring schedule addressed to the configured lead, a canonical revision, and an exact revision-only message. Reapplying the same document is idempotent. Changing lead/spec replaces the schedule and queue owner without leaving duplicates.

- [ ] **Step 2: Write failing one-shot tests**

Assert `RunOnce(ctx, 3)` rejects a missing/disabled config and otherwise adds an ordinary due one-shot schedule whose message contains the active revision and `limit: 3`. Assert the recurring schedule is unchanged.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/judge ./internal/schedule ./internal/tasks -run 'Automation|QueueUpsert|ScheduleReplace'`

- [ ] **Step 4: Implement the minimum transaction-aware reconciliation**

Reuse the existing `schedule.add` calculation and Native Task queue rules inside one database transaction. Do not add a timer. Ensure configured Judge agents exist, have the configured image pending/current at a safe boundary, remain enabled, and are wakeable by inbox delivery.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/judge ./internal/schedule ./internal/tasks ./internal/agent ./internal/daemon`

Commit: `feat: apply judge automation with native schedules`

### Task 3: Execute and report a complete automatic cycle

**Files:**
- Modify: `internal/store/migrations/0038_judge_automation.sql`
- Modify: `internal/judge/automation.go`
- Modify: `internal/judge/automation_test.go`
- Modify: `internal/judge/service.go`
- Modify: `internal/judge/service_test.go`
- Modify: `internal/judge/runner.go`
- Modify: `internal/judge/runner_test.go`
- Modify: `internal/toolscli/toolscli.go`
- Modify: `internal/toolscli/toolscli_test.go`
- Modify: `internal/daemon/daemon.go`

**Interfaces:**
- Produces lead action `tools judge automation begin --revision R --delivery ID [--limit N]`.
- Produces durable `AutomationCycle{Revision, DeliveryID, TaskKey, RunID, Status}` and terminal task notification callbacks.

- [ ] **Step 1: Write failing begin/idempotency tests**

Assert only the configured active lead iteration can begin the referenced revision. The first call creates one assigned `JUDGE-*` task and one run; repeating the same delivery returns the same cycle. A simultaneous active cycle creates a separate skipped task but no second run. Zero eligible targets creates and completes a zero-target task.

- [ ] **Step 2: Write failing completion/failure tests**

Submit a summary and assert the linked task receives the bounded conclusion, proposal IDs, `@user:${USER}`, and completion. Inject snapshot/assignment/summary terminal failures and assert daemon fallback records the failure and creates the same dynamic customer notification even if no lead follow-up occurs.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/judge ./internal/toolscli -run 'AutomationCycle|AutomationBegin|CycleTask'`

- [ ] **Step 4: Implement cycle orchestration around the existing runner**

Create the task before selection, call the existing run creation/enqueue path, persist the run link, and add a narrow terminal callback from Judge service/runner into automation. Do not duplicate evidence, leases, consensus, or summary logic. Keep agent tools as authenticated thin calls whose identity comes from the tools socket.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/judge ./internal/toolscli ./internal/daemon`

Commit: `feat: run judge cycles without manual agent starts`

### Task 4: Create improvement tasks atomically after approval

**Files:**
- Modify: `internal/store/migrations/0038_judge_automation.sql`
- Modify: `internal/improvement/model.go`
- Modify: `internal/improvement/store.go`
- Modify: `internal/improvement/store_test.go`
- Modify: `internal/improvement/service.go`
- Modify: `internal/improvement/service_test.go`
- Modify: `internal/tasks/store.go`
- Modify: `internal/tasks/store_test.go`

**Interfaces:**
- Extends the plan approval result with `TaskKeys []string`.
- Creates one `IMPROVE-*` task per exact approved proposal revision using idempotency `improvement:<proposal-id>:<revision-hash>`.

- [ ] **Step 1: Write failing approval-task tests**

Approve a proposal and assert approval, status transition, and a fully populated `IMPROVE-*` task commit together. Retry the decision and assert no duplicate. Reject and assert no task. Approve separate proposals for different repositories/release units and assert separate tasks. Assert linked upstream/downstream proposals create task relations and the downstream blocker.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/improvement ./internal/tasks -run 'Approval.*Task|ImproveQueue'`

- [ ] **Step 3: Implement transaction-safe task materialization**

Insert the task from the canonical proposal document inside `DecidePlan`'s existing transaction. Include Judge task/run, repository, base commit, file allowlist, intent, acceptance, citations, risk, rollback, revision, and hash. Return created/existing keys from service and operator API; publish the approval event only after commit.

- [ ] **Step 4: Verify GREEN and commit**

Run: `go test ./internal/improvement ./internal/tasks ./internal/commands`

Commit: `feat: create approved improvement tasks`

### Task 5: Expose daemon API, thin CLI, and raw JSON UI

**Files:**
- Modify: `internal/registry/registry.go`
- Modify: `internal/commands/judge.go`
- Modify: `internal/commands/judge_test.go`
- Modify: `internal/commands/daemon.go`
- Modify: `internal/cli/judge.go` or the existing Judge CLI registration file
- Modify: corresponding CLI tests
- Modify: `ui/src/lib/judge.ts`
- Modify: `ui/src/pages/JudgeRunsPage.tsx`
- Modify: `ui/src/pages/JudgeRunsPage.test.tsx`

**Interfaces:**
- HTTP: `GET /api/judge-automation`, `POST /api/judge-automation/validate`, `PUT /api/judge-automation`, `POST /api/judge-automation/run-once`.
- CLI: `tariboy judge automation get|validate|apply|run-once`.

- [ ] **Step 1: Write failing route and CLI tests**

Assert raw JSON reaches `tariboyd` unchanged, daemon diagnostics and canonical JSON return unchanged, and CLI performs no local semantic validation. Assert run-once forwards only the optional positive limit.

- [ ] **Step 2: Write failing UI tests**

Render the Judge page against an explicit API target. Assert a labelled textarea loads canonical JSON, edits remain unsaved, Validate renders daemon JSON Pointer errors, Apply sends raw text and replaces it with canonical output, Reset restores the last applied text, and no browser storage is used.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/commands ./internal/cli -run 'JudgeAutomation'`

Run: `cd ui && npm test -- --run src/pages/JudgeRunsPage.test.tsx`

- [ ] **Step 4: Implement the thin surfaces**

Reuse the command registry and explicit-daemon UI API helpers. Use a native `textarea`; do not add Monaco, JSON schema, or client business rules. Apply must still revalidate in the daemon.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/commands ./internal/cli`

Run: `cd ui && npm test -- --run src/pages/JudgeRunsPage.test.tsx && npm run typecheck && npm run lint`

Commit: `feat: configure judge automation in desktop`

### Task 6: Rebuild the Judge image, document, release, and prove automation

**Files:**
- Modify: `store/images/llm-as-judge/Tariboyfile.yaml`
- Modify: `store/images/llm-as-judge/instructions.md`
- Modify: `store/images/llm-as-judge/skills/llm-as-judge/SKILL.md`
- Modify: `docs/docs/plugins/built-in/llm-as-judge.mdx`
- Modify: `docs/docs/reference/commands.md`
- Modify: `docs/docs/reference/channels.md`
- Modify: `docs/docs/tasks.mdx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/architecture/web-ui.mdx`
- Modify: `docs/docs/security-controls.mdx`
- Modify: release-version files only through `scripts/set-version.sh 0.44.0`
- Add or modify isolated Judge/UI e2e coverage under existing e2e locations.

**Interfaces:**
- Judge image lead consumes schedule deliveries and calls `automation begin`; workers drain assignments; lead submits summaries and structured proposals.
- Built binaries report `0.44.0`.

- [ ] **Step 1: Write failing image/e2e contracts**

Assert the image declares `schedule`, `tasks`, `current-task`, messages, loop, and Judge capabilities; mandatory lead/worker lifecycle rules are unconditional prompt layers; the isolated daemon test applies arbitrary agent/image names, fires a one-shot limit of three, and reaches a terminal run plus linked tasks without lifecycle calls.

- [ ] **Step 2: Verify RED, then update the image minimally**

Run the focused image and Judge e2e tests and confirm failure before changing the manifest/instructions. Add only the capabilities and procedures required by the approved design.

- [ ] **Step 3: Update current product documentation**

Document the JSON schema, daemon validation, schedule reuse, queues, dynamic `USER`, selection semantics, full cycle, proposal/task split, CLI, UI, failure behavior, and operator workflow.

- [ ] **Step 4: Run repository and Desktop verification**

Run: `make check`

Run the repository's production Desktop Playwright suite and `make desktop-e2e`, then run `make full-check` because this change reaches daemon e2e and Desktop behavior. Run `git diff --check` and inspect the complete diff.

- [ ] **Step 5: Cut and verify the requested minor release**

Run: `scripts/set-version.sh 0.44.0`

Run: `make build`

Assert `./bin/tariboy version` and `./bin/tariboy --version` both report `0.44.0`.

- [ ] **Step 6: Update installed daemon/tools and run the live automatic smoke**

Install the built daemon and tools through the documented installer, preserving live data. Build the new immutable Judge image, apply the approved JSON configuration, and invoke `tariboy judge automation run-once --limit 3`. Do not start, restart, prompt, or otherwise intervene in Judge agents after the one-shot is created. Observe schedule delivery, one `JUDGE-*` task, exactly three selected iterations, both workers, terminal summary/proposals, and customer notification. If any stage fails, stop observation and report the persisted failure without manually repairing the run.

- [ ] **Step 7: Commit**

Commit: `chore: release 0.44.0`

---

## Final Self-Review Checklist

- [ ] No agent name, image ref, repository, or customer login is hard-coded outside examples/tests.
- [ ] `apply` does not start a review; `run-once` and recurring fires use ordinary schedules.
- [ ] Both queues exist immediately after apply.
- [ ] Selection filters before limit and cannot duplicate non-cancelled work.
- [ ] Every fire creates a Judge task, including zero-target, overlap, and failure outcomes.
- [ ] Approval and improvement task creation are one transaction and idempotent.
- [ ] UI and CLI share daemon validation and canonicalization.
- [ ] Judge agents receive no new source mutation or approval authority.
- [ ] Focused RED→GREEN evidence exists for every behavior change.
- [ ] `make check`, required Desktop/e2e gates, `make full-check`, and `git diff --check` pass.
- [ ] Installed daemon/tools and the three-target automatic smoke are verified without agent intervention.
