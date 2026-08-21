# Task Reminder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let operators opt in to a durable inbox reminder for enabled Autopilot agents that have assigned open Native Tasks and have been idle longer than a configured seconds threshold.

**Architecture:** Add a typed `task_reminder` daemon-config policy and a daemon-owned reconciler package. It derives candidates from SQLite task and iteration state, records a persisted generation fingerprint, and publishes `task.reminder` through `bus.Publish`; the existing bus hook and `loop.WakeMessage` remain the sole wake/execution path.

**Tech Stack:** Go 1.26, SQLite, React 19, TypeScript 6, Vitest/Testing Library, Starlight MDX.

**Spec:** `outside/superpowers/specs/2026-08-21-task-reminder-design.md`

## Global Constraints

- Default-off policy: absent `task_reminder` means `{enabled:false,idle_threshold_s:300}`.
- `idle_threshold_s` is a positive integral number of seconds.
- Apply to all interval modes, including `interval_s=0`.
- Publish only ordinary `task.reminder` messages to `agent:<name>:inbox`; never invoke an iteration directly.
- Require master enabled and loop enabled state; persist a per-agent assignment/activity generation to prevent flood and restart resend.
- Native Task state remains authoritative; a reminder does not claim, assign, or transition a task.
- Test daemons use isolated base/runtime directories and an isolated or disabled listener.
- Do not bump the product version or stage generated Desktop output.

---

### Task 1: Type and validate the daemon reminder policy

**Files:**
- Create: `internal/taskreminder/config.go`
- Create: `internal/taskreminder/config_test.go`
- Modify: `internal/commands/daemon.go`
- Modify: `internal/commands/daemon_test.go`

**Interfaces:** `taskreminder.Policy{Enabled bool, IdleThresholdS int}`, `DefaultPolicy`, and `ParsePolicy(raw string) (Policy, error)`; `daemon.config.set` accepts normalized `task_reminder` JSON.

- [ ] Write failing tests for absent defaults, valid normalized JSON, and malformed JSON, non-boolean enabled, non-integral threshold, and threshold below one returning `bad_task_reminder` without overwriting the old value.
- [ ] Run `go test ./internal/taskreminder ./internal/commands -run 'Test.*TaskReminder' -count=1` and observe failure because the typed policy does not exist.
- [ ] Implement strict parsing, normalized JSON encoding, and special-key validation in `daemon.config.set`; preserve generic string behavior for unrelated keys.
- [ ] Re-run the focused tests and commit with `feat: add task reminder policy`.

### Task 2: Persist generations and select eligible agents

**Files:**
- Create: `internal/store/migrations/00xx_task_reminders.sql`
- Create: `internal/taskreminder/store.go`
- Create: `internal/taskreminder/store_test.go`

**Interfaces:** `Store.Eligible(policy Policy, now time.Time) ([]Candidate, error)` and `Store.MarkSent(candidate Candidate, sentAt time.Time) error`; a candidate includes agent, sorted task keys, activity boundary, and deterministic fingerprint.

- [ ] Write failing store tests: enabled agents with both zero and positive intervals qualify only after threshold; disabled/master-off agents and done tasks do not; `MarkSent` suppresses an unchanged task/activity generation.
- [ ] Run `go test ./internal/taskreminder -run 'TestStoreEligible|TestStoreMarkSent' -count=1` and observe failure because no generation table or query exists.
- [ ] Add `task_reminders(agent PRIMARY KEY, fingerprint, activity_at, sent_at)` migration. Query open assigned legacy task rows for enabled agents with loop enabled; group/sort keys; use latest terminal iteration or task/agent fallback as the durable activity boundary; exclude the persisted equal fingerprint. Upsert only after delivery succeeds.
- [ ] Re-run focused tests and commit with `feat: persist task reminder generations`.

### Task 3: Reconcile through the ordinary inbox channel

**Files:**
- Create: `internal/taskreminder/reconciler.go`
- Create: `internal/taskreminder/reconciler_test.go`
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/daemon_test.go`

**Interfaces:** `Reconciler.Reconcile(context.Context) error`, `Reconciler.Run(context.Context)`, `bus.Bus.Publish`, and existing daemon context cancellation/publish hook.

- [ ] Write failing fake-clock/real-isolated-bus tests: disabled policy publishes nothing; eligible candidate publishes one `task.reminder` to `agent:worker:inbox` with reason `assigned-work-idle`, threshold, and sorted task keys; duplicate scan/restart boundary does not publish again.
- [ ] Run `go test ./internal/taskreminder -run 'TestReconciler' -count=1` and observe failure because no reconciler exists.
- [ ] On each bounded scan read policy, publish with a fingerprint-derived idempotency key, and mark sent only after successful publish. Run an initial scan and cancellable ticker; log scan errors and continue. Wire worker startup after bus-hook setup and await it before SQLite shutdown. Do not add a loop-manager dependency.
- [ ] Run `go test ./internal/taskreminder ./internal/daemon -run 'TestReconciler|Test.*TaskReminder' -count=1`, then commit with `feat: reconcile idle assigned task reminders`.

### Task 4: Expose Task reminders in Configuration

**Files:**
- Create: `ui/src/pages/settings/TaskReminderSettings.tsx`
- Create: `ui/src/pages/settings/TaskReminderSettings.test.tsx`
- Modify: `ui/src/lib/api.ts`
- Modify: `ui/src/pages/settings/SettingsPage.tsx`
- Modify: `ui/src/App.tsx`

**Interfaces:** current target-aware `GET`/`POST /api/daemon/config`; server-local and server-scoped task-reminder settings routes.

- [ ] Write failing UI tests for absent-key default off/300, saved `{enabled:true,idle_threshold_s:120}`, zero/non-integral feedback, preserved request-error draft, and explicit-host request targeting.
- [ ] Run `cd ui && npm test -- --run src/pages/settings/TaskReminderSettings.test.tsx` and observe failure because no page exists.
- [ ] Add typed config helpers, General Settings navigation, accessible toggle/seconds controls, positive-integer client validation, response errors, save feedback, and local/server-scoped routes using current host routing patterns.
- [ ] Run focused UI test plus `cd ui && npx tsc -b`; commit with `feat: configure task reminders`.

### Task 5: Document and verify the feature

**Files:**
- Modify: `docs/docs/architecture/messaging.mdx`
- Modify: `docs/docs/architecture/iteration-loop.mdx`
- Modify: `docs/docs/reference/channels.md`

**Interfaces:** completed policy, reconciler, and UI behavior.

- [ ] Document defaults and seconds units, all eligibility conditions, generation reset/dedupe, and the ordinary `Publish -> delivery -> WakeMessage` boundary. Do not claim disabled loops wake or that reminders claim tasks.
- [ ] Run `go test ./internal/taskreminder ./internal/commands ./internal/daemon -count=1`, then `cd docs && npm run doctor && npm run build`.
- [ ] Commit documentation with `docs: describe task reminders`.
- [ ] Run `make check`, `git diff --check`, and inspect `git diff --stat main...HEAD`; expect passing checks, no generated Desktop output, and no version change.
