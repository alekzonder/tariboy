# Message Queue Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fast, per-agent physical clearing of pending messages, configurable message limits, a 100-message default, and daemon-authoritative queue saturation status.

**Architecture:** Extend the existing bus, command registry, status projection, and React section-draft patterns. One SQLite transaction deletes only the selected agent's pending deliveries and newly orphaned messages; no new persistence layer, dependency, migration, or lock is introduced.

**Tech Stack:** Go, SQLite, registry-generated CLI/HTTP, React, TypeScript, Vitest, Testing Library, MDX.

**Spec:** `docs/superpowers/specs/2026-09-08-message-queue-controls-design.md`

## Global Constraints

- Clear only `acked_at IS NULL AND dlq = 0` deliveries owned by the selected agent.
- Preserve Archive, DLQ, shared messages, and every other agent's deliveries.
- Keep overflow-to-DLQ and five-attempt retry behavior unchanged.
- Default `messages_max_queue` to 100 only for newly created agents; preserve persisted and cloned values.
- Reuse existing registry, AgentSettings section-draft, and alert-dialog patterns; add no dependency or migration.
- Use isolated test state; never touch the live daemon, base/runtime directories, or `127.0.0.1:9990`.
- Final verification is exactly `make check` and `git diff --check`; do not run `make full-check`.
- Do not bump the product version or commit generated Desktop/store UI output.

---

### Task 1: Atomic bus clear and pending count

**Files:**
- Modify: `internal/bus/messages.go`
- Modify: `internal/bus/store.go`
- Test: `internal/bus/messages_test.go`

**Interfaces:**
- Produces: `type ClearPendingResult struct { DeletedDeliveries int64; DeletedMessages int64 }`
- Produces: `func (b *Bus) ClearPending(agent string) (ClearPendingResult, error)`
- Produces: `func (b *Bus) PendingCount(agent string) (int, error)`
- Reuses: `bus.ErrNotFound`, `Bus.emitAudit`, and the pending predicate already used by queue limiting.

- [ ] **Step 1: Write failing persistence tests**

Create one table-driven test around the existing bus fixture. Publish messages that yield: a pending delivery unique to `alice`, one message delivered to `alice` through two subscriptions, one shared with `bob`, one processed for `alice`, one DLQ for `alice`, and one unrelated orphan message. Assert:

```go
got, err := b.ClearPending("alice")
if err != nil { t.Fatal(err) }
if got.DeletedDeliveries != 3 || got.DeletedMessages != 2 {
	t.Fatalf("clear result = %+v", got)
}
assertInboxIDs(t, b, "alice", "pending", nil)
assertInboxIDs(t, b, "alice", "processed", []string{processed.ID})
assertInboxIDs(t, b, "alice", "dlq", []string{dead.ID})
assertInboxIDs(t, b, "bob", "pending", []string{shared.ID})
```

Then call `ClearPending("alice")` again and require zero counters. Require `ErrNotFound` for an unknown agent. Query the fixture DB to prove the shared and unrelated-orphan rows remain while only captured, newly orphaned message rows disappear.

- [ ] **Step 2: Run the clear tests and observe RED**

Run: `go test ./internal/bus -run 'TestClearPending' -count=1`

Expected: FAIL because `ClearPending` and `ClearPendingResult` do not exist.

- [ ] **Step 3: Implement the minimum transaction**

Implement `ClearPending` with one transaction:

```sql
SELECT 1 FROM agents WHERE name = ?;
CREATE TEMP TABLE is unnecessary;
SELECT DISTINCT d.message_id ... WHERE s.agent = ? AND d.acked_at IS NULL AND d.dlq = 0;
DELETE FROM deliveries WHERE rowid IN (
  SELECT d.rowid FROM deliveries d JOIN subscriptions s ON s.id=d.subscription_id
  WHERE s.agent=? AND d.acked_at IS NULL AND d.dlq=0
);
DELETE FROM messages
WHERE id IN (<captured ids>) AND NOT EXISTS (
  SELECT 1 FROM deliveries WHERE deliveries.message_id=messages.id
);
```

Keep captured IDs transaction-local using a prepared placeholder list or a temporary in-memory Go slice; do not delete unrelated pre-existing orphan rows. Read `RowsAffected`, commit, then emit one `message_queue_cleared` audit event containing both counters.

- [ ] **Step 4: Add the shared pending-count helper and tests**

Move the existing `COUNT(DISTINCT d.message_id)` query behind one helper usable from both `queueLimitReached` and `Bus.PendingCount`. Test zero, duplicate-subscription deduplication, DLQ exclusion, processed exclusion, and unknown-agent `ErrNotFound`.

- [ ] **Step 5: Run the bus package**

Run: `go test ./internal/bus -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the vertical slice**

```bash
git add internal/bus/messages.go internal/bus/store.go internal/bus/messages_test.go
git commit -m "feat(bus): clear pending agent messages"
```

### Task 2: Registry API, CLI, configuration commands, and defaults

**Files:**
- Modify: `internal/commands/channel.go`
- Modify: `internal/commands/config.go`
- Modify: `internal/commands/daemon.go`
- Modify: `internal/commands/agents.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/loop/manager.go`
- Modify: `internal/store/migrations/0003_bus.sql`
- Modify: `internal/commands/*_test.go` files that already cover channel/config/registry contracts
- Modify: `internal/agent/agent_test.go`
- Modify: `internal/loop/manager_test.go`

**Interfaces:**
- Consumes: `Bus.ClearPending(name)` and its exact counters.
- Produces: registry command `agent.inbox.clear`, HTTP `POST /api/agents/{name}/inbox/clear`, generated CLI `tariboy agent inbox clear NAME`.
- Produces: narrow commands/routes `agent.messages.batch` and `agent.messages.max-queue` under `/api/agents/{name}/messages/{field}`.
- Produces: positive-value setters on the existing agent store; no generic update endpoint.

- [ ] **Step 1: Write failing registry and HTTP tests**

Register the daemon commands in the existing command fixture and assert:

```go
cmd, ok := reg.Command("agent.inbox.clear")
if !ok || cmd.HTTP.Method != http.MethodPost || cmd.HTTP.Path != "/api/agents/{name}/inbox/clear" {
	t.Fatalf("clear command = %#v, %v", cmd, ok)
}
```

Exercise the handler for success counters, empty queue, and unknown agent `not_found`. Assert CLI resolution accepts `agent inbox clear alice` and renders the standard result envelope.

- [ ] **Step 2: Run the command tests and observe RED**

Run: `go test ./internal/commands ./internal/cli -run 'InboxClear|MessageSettings' -count=1`

Expected: FAIL because the commands are not registered.

- [ ] **Step 3: Add the registry commands**

Add the clear command beside `agent.inbox.processed`, mapping `bus.ErrNotFound` to `api.UserError{Code: "not_found", Msg: "agent not found"}` and returning:

```go
map[string]any{
	"deleted_deliveries": result.DeletedDeliveries,
	"deleted_messages": result.DeletedMessages,
}
```

Add two positive-integer field commands beside existing configuration setters. Each loads the agent, rejects values `< 1` with `bad_messages_batch` or `bad_messages_max_queue`, changes only its owned field through the agent store, and returns the canonical field value.

- [ ] **Step 4: Write failing default and setter tests**

Assert an omitted value creates an agent with `MessagesMaxQueue == 100`, an explicit value is preserved, clone-facing inspect still returns its persisted value, and both setters reject zero without changing the row.

- [ ] **Step 5: Change only new-agent defaults**

Change the fresh schema default, agent-store zero fallback, manager omitted-value fallback, and registry create default from 1000 to 100. Do not add a migration that updates existing rows. Keep `MessagesBatch` at 10.

- [ ] **Step 6: Run affected backend packages**

Run: `go test ./internal/commands ./internal/cli ./internal/agent ./internal/loop -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the vertical slice**

```bash
git add internal/commands internal/agent internal/loop internal/store/migrations/0003_bus.sql
git commit -m "feat(cli): add message queue controls"
```

### Task 3: Daemon-authoritative queue saturation status

**Files:**
- Modify: `internal/commands/agents.go`
- Test: the existing `internal/commands` agent-status test file
- Modify: `ui/src/lib/types.ts`
- Modify: `ui/src/pages/agents/AgentWorkspace.tsx`
- Test: the existing AgentWorkspace React test file

**Interfaces:**
- Consumes: `Bus.PendingCount(name)` and `Agent.MessagesMaxQueue`.
- Produces status JSON fields: `messages_pending: number`, `messages_max_queue: number`, `messages_queue_full: boolean`.
- Produces matching optional TypeScript fields on `AgentStatus` for compatibility with older daemons.

- [ ] **Step 1: Write failing backend status tests**

For pending counts below, equal to, and above the configured limit, assert the status result includes exact counts and `messages_queue_full == (pending >= limit)`. Make the bus count return an error and assert the status command returns that error rather than a false normal state.

- [ ] **Step 2: Run the status tests and observe RED**

Run: `go test ./internal/commands -run 'AgentStatus.*MessageQueue' -count=1`

Expected: FAIL because the status fields are absent.

- [ ] **Step 3: Extend the status projection**

In `agent.status.show`, load the agent once, call `PendingCount`, and add:

```go
result["messages_pending"] = pending
result["messages_max_queue"] = a.MessagesMaxQueue
result["messages_queue_full"] = pending >= a.MessagesMaxQueue
```

Return the count error unchanged.

- [ ] **Step 4: Write the failing header test**

Render the existing workspace fixture with `messages_queue_full: true`, pending 100, limit 100 and assert `Message queue full: 100 / 100` uses destructive styling. Render false/omitted and assert the line is absent.

- [ ] **Step 5: Add the typed header indicator**

Extend `AgentStatus` with optional additive fields and render the line beside the existing `Out of budget` projection. Do not calculate queue fullness in React.

- [ ] **Step 6: Run focused status tests**

Run: `go test ./internal/commands -count=1`

Run: `cd ui && npm test -- AgentWorkspace`

Expected: PASS.

- [ ] **Step 7: Commit the vertical slice**

```bash
git add internal/commands/agents.go internal/commands/*_test.go ui/src/lib/types.ts ui/src/pages/agents/AgentWorkspace.tsx ui/src/pages/agents/*AgentWorkspace*.test.tsx
git commit -m "feat(ui): show full message queues"
```

### Task 4: Queue clear and Messages & Channels configuration UI

**Files:**
- Modify: `ui/src/lib/api.ts`
- Modify: `ui/src/pages/AgentMessages.tsx`
- Test: `ui/src/pages/AgentMessages.test.tsx`
- Modify: `ui/src/pages/AgentSettings.tsx`
- Test: the existing `ui/src/pages/AgentSettings*.test.tsx` file
- Modify: `ui/src/pages/terminals/agentCreateDraft.ts`
- Test: `ui/src/pages/terminals/agentCreateDraft.test.ts`

**Interfaces:**
- Consumes: `POST /api/agents/{name}/inbox/clear` and the two message-setting routes from Task 2.
- Produces: `agentInboxClear(name): Promise<{ deleted_deliveries: number; deleted_messages: number }>`.
- Reuses: `AlertDialog`, `SectionField`, and `useSectionDraft`.

- [ ] **Step 1: Replace the old bulk-action tests with failing clear tests**

Assert the Queue action is named `Clear queue`, opens an in-app destructive confirmation, contains `cannot be recovered` and `Archive and DLQ are not changed`, sends exactly one bulk request, blocks duplicate submit/dismiss while pending, reloads Queue on success, and reports both server counters. Reject the request and assert current rows remain visible and retry is possible.

- [ ] **Step 2: Run the message UI test and observe RED**

Run: `cd ui && npm test -- AgentMessages`

Expected: FAIL because the page still pages and processes messages one by one.

- [ ] **Step 3: Implement the single-request destructive dialog**

Delete the client-side pagination loop and result-input requirement for the bulk action. Keep individual Mark processed and Reply unchanged. Use the existing alert-dialog components, `busy` guard, authoritative reload, and toast infrastructure.

- [ ] **Step 4: Write failing AgentSettings tests**

Render an agent with `messages_batch: 10` and `messages_max_queue: 100`. Assert a `Messages & Channels` section shows the two positive whole-number controls, dirty/discard behavior, stable changed-only save order, partial-failure preservation, and server-canonical reconciliation.

- [ ] **Step 5: Add the section by reusing the draft machinery**

Define `MESSAGE_FIELDS` with keys `messages-batch` and `messages-max-queue`, `minimum: 1`, positive-integer validation, and submit paths from Task 2. Pass it through `useSectionDraft` and the same section renderer used for Loop/Runtime/Goal. Do not create another form abstraction.

- [ ] **Step 6: Change and test the Desktop create default**

Change only `newAgentDraft().messagesMaxQueue` to `"100"`. Assert `newAgentDraft` uses 100 and `cloneAgentDraft` continues copying the source value verbatim.

- [ ] **Step 7: Run focused UI tests**

Run: `cd ui && npm test -- AgentMessages AgentSettings agentCreateDraft`

Expected: PASS.

- [ ] **Step 8: Commit the vertical slice**

```bash
git add ui/src/lib/api.ts ui/src/pages/AgentMessages.tsx ui/src/pages/AgentMessages.test.tsx ui/src/pages/AgentSettings.tsx ui/src/pages/AgentSettings*.test.tsx ui/src/pages/terminals/agentCreateDraft.ts ui/src/pages/terminals/agentCreateDraft.test.ts
git commit -m "feat(ui): manage message queues"
```

### Task 5: Product documentation and final verification

**Files:**
- Modify: `docs/docs/architecture/messaging.mdx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/architecture/web-ui.mdx`
- Modify: `docs/docs/reference/channels.md`

**Interfaces:**
- Documents: clear scope and irreversibility, CLI/HTTP names, counters, unchanged DLQ behavior, Configuration fields, default 100, and derived queue-full status.

- [ ] **Step 1: Update current product documentation**

Replace the old `Mark all processed` bulk description with `Clear queue`; document `tariboy agent inbox clear NAME`, physical pending-only deletion, Archive/DLQ/shared-message preservation, and idempotent counters. Update message settings/defaults and state/UI saturation projections. Do not copy internal plan prose or add a release note/version change.

- [ ] **Step 2: Install worktree-local Node dependencies if still absent**

Run: `cd ui && npm ci`

Run: `cd docs && npm ci`

Expected: both exit 0 and use committed lockfiles.

- [ ] **Step 3: Run the complete allowed branch verification once**

Run: `make check`

Expected: backend-check and frontend-check both PASS. Do not run `make full-check`.

- [ ] **Step 4: Inspect formatting and the complete diff**

Run: `git diff --check`

Run: `git status --short`

Run: `git diff --stat main...HEAD && git diff main...HEAD`

Expected: no whitespace errors, no generated build output, no unrelated edits, and no version change.

- [ ] **Step 5: Commit documentation or final corrections**

```bash
git add docs/docs/architecture/messaging.mdx docs/docs/architecture/state-model.mdx docs/docs/architecture/web-ui.mdx docs/docs/reference/channels.md
git commit -m "docs: describe message queue controls"
```

- [ ] **Step 6: Continue the recorded PR workflow**

Invoke `requesting-code-review`, resolve Critical/Important findings, invoke `verification-before-completion` using the unchanged successful `make check`, then invoke `finishing-a-development-branch` and `github-pr-workflow`. Push this branch, ensure exactly one PR, start exactly one durable monitor, set the Native Task to Wait customer, and wait for merge; never merge the PR from the agent.
