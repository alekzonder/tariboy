# Agent Goals Design

## Goal

Make `tariboyd` choose and retain one Native Task as each agent's current goal,
show that goal explicitly in schema-v2 iteration prompts, and continue nudging
the agent through its ordinary inbox until the task reaches an approved release
condition. The mode is configured per agent and enabled by default.

This supersedes the daemon-global, default-off Task reminders policy. It reuses
the existing durable `Publish -> delivery -> WakeMessage` path rather than
adding another execution mechanism.

## Ownership and invariants

`tariboyd` is the only owner of goal selection. SQLite remains authoritative
for agent settings, the selected task key, Native Task state, and message
idempotency. Images opt into seeing the current value by declaring
`runtime: goal`; images that omit the placeholder receive no goal text.

For each agent:

- `goal_enabled` defaults to `true`.
- `goal_wait_customer_timeout_s` defaults to `300` and must be a positive
  integral number of seconds.
- `current_goal_task_key` is the persisted sticky selection. It is not an
  operator-editable setting.
- Disabling Goal clears `current_goal_task_key`. Re-enabling it selects from
  current task state instead of reviving a stale selection.
- Master-disabled agents and agents with disabled Autopilot loops neither
  select nor receive goal wake messages.

No global policy service or second task scheduler is introduced.

## Native Task model

### Status and pull request

Add `wait_customer` to the existing task statuses:

1. `open`
2. `in_progress`
3. `wait_customer`
4. `done`
5. `cancelled`

Add `pull_request` as an optional string field on `Task`. A non-empty value is
a structured task attribute, not text inferred from comments. The write path
accepts only an absolute `http` or `https` URL without embedded credentials and
stores its normalized string form. An empty string clears it. The field flows
through task create/read/update JSON, `tasks update`, task events, and Task
Detail UI. It is available to flexible and workflow-managed tasks under their
existing mutation and revision rules.

The daemon never queries GitHub to discover PRs and never parses task comments
for URLs. The development workflow records the canonical PR URL through the
task update surface immediately after successful PR creation.

### Customer-question transitions

The existing comment-and-wait transaction owns automatic status changes:

- When the task's assigned agent creates a wait for that task's customer, the
  same transaction changes a non-terminal flexible task to `wait_customer`,
  increments its revision, and emits the normal `task.updated` event.
- Customer answers continue resolving waits in the comment transaction.
- When no unresolved wait for the task customer remains, the same transaction
  changes `wait_customer` back to `in_progress`.
- An intervening manual status change is preserved: an answer only performs
  the automatic return when the current status is still `wait_customer`.
- Questions by another principal and waits for another principal do not change
  task status.

The grace boundary uses the oldest unresolved wait for the task customer. A
manually selected `wait_customer` status with no unresolved customer wait is
not sticky and is excluded from goal candidates.

## Selection and release policy

The goal reconciler first validates the persisted key against authoritative
task state. A selected goal remains sticky despite later priority changes and
despite another task becoming more urgent. It is released only when one of
these conditions becomes true:

- task status is `done` or `cancelled`;
- `pull_request` is non-empty;
- task status is `wait_customer` and its oldest unresolved customer wait is at
  least `goal_wait_customer_timeout_s` old;
- the task no longer exists, is no longer assigned to the agent, or is no
  longer visible to it;
- Goal is disabled for the agent.

An unanswered customer wait remains the current goal before its grace period
expires, but does not generate a continuation wake while waiting. After expiry
the task is excluded from candidate selection until its customer answers and
the task returns to `in_progress`.

When no valid sticky goal remains, eligible tasks are those assigned directly
to the agent with empty `pull_request` and status `in_progress` or `open`.
Flexible and workflow-managed tasks use the same task-level ordering. Sort
ascending by:

1. priority (`P0`, `P1`, `P2`, `P3`);
2. status (`in_progress` before `open`);
3. `created_at`;
4. task key as the deterministic tie-breaker.

The first row becomes `current_goal_task_key`. If there is no eligible row,
the key remains empty and no goal message is published.

## Reconciliation and delivery

Evolve `internal/taskreminder` into the per-agent goal reconciler rather than
running both mechanisms. Reconcile at daemon startup, on the existing bounded
one-minute recovery cadence, after relevant task or agent mutations, and
immediately after an agent iteration reaches a terminal outcome. Triggers are
coalesced; the periodic scan remains crash recovery rather than the primary
continuation timer.

For a valid goal that is not actively waiting for the customer, publish one
ordinary `task.goal` message to `agent:<name>:inbox`. The message contains the
task key and the reason (`selected` or `iteration_completed`). Its idempotency
key is derived from the agent, task key and revision, and latest terminal
iteration identity. This gives one wake per unchanged selection boundary and
one wake after each completed iteration, while daemon restart or repeated scans
cannot resend the same generation.

Publishing remains strictly:

```text
goal reconciliation -> bus.Publish -> inbox delivery -> WakeMessage
```

The reconciler never starts an iteration directly. The loop engine still
serializes execution and requires an enabled loop with a pending delivery. A
task mutation racing a publication is safe because prompt rendering and the
agent's task tools read authoritative state again.

Failures are logged per agent and retried through later triggers or the
recovery cadence. One agent's failure does not block another. The old
`task_reminder` configuration and its UI are removed; the migration history and
legacy reminder table remain in place but have no runtime writer.

## Prompt runtime

Add `goal` to schema-v2 runtime placeholder validation and rendering. Before
each iteration, the runner loads `current_goal_task_key`, verifies it through
the same selection policy, and renders a non-empty block containing:

```text
# Agent Goal

key: TARI-43
title: Agent goal
priority: P1
status: in_progress
description: ...
```

The runtime is preceded by an instruction to use the packaged `tasks` skill.
Task text is treated as task input, not as daemon instructions or lifecycle
authority. An empty or disabled goal renders no text. The canonical `basic`
and `tariboy-developer` image templates declare `runtime: goal` near identity
and task tooling; other images remain unchanged unless their source explicitly
adds it.

The inbox message is only the durable wake hint. `[runtime: goal]` is the
daemon-authoritative task context delivered to the harness.

## API, CLI, compose, and UI

Agent create, inspect, list, update, and clone projections include
`goal_enabled` and `goal_wait_customer_timeout_s`; inspect/list also expose
`current_goal_task_key` read-only. The operator CLI accepts explicit Goal
settings on create and update. Compose adds a terse optional block:

```yaml
agents:
  worker:
    goal:
      enabled: true
      wait_customer_timeout: 300s
```

Omitted compose fields retain daemon defaults or current values. Duration
parsing follows existing compose duration conventions and persists whole
seconds.

Agent Configuration replaces the server-global Task reminders screen with a
Goal section containing an Enabled switch and Wait customer timeout seconds.
It uses the existing per-section dirty draft, discard, sequential save, error,
and explicit-host routing behavior. The selected goal key is shown read-only
when present. Agent creation and clone carry the editable Goal settings through
their existing complete configuration payloads.

Task Detail adds `Wait customer` to its status control and an editable Pull
request URL field. All writes retain optimistic task revisions and the selected
host target.

## Persistence and migration

Use additive migrations followed by the repository's established SQLite table
rebuild where the task status check constraint requires it. Existing task rows
retain their values and receive an empty `pull_request`. Existing agent rows
receive Goal enabled, a 300-second timeout, and an empty current key. The
current key references tasks defensively in application logic so task cleanup
or migration cannot strand daemon startup.

Do not repurpose the reminder fingerprint as goal ownership. Existing reminder
rows are inert historical state after the old worker is removed.

## Documentation and safety

Update current task, loop, messaging, image, CLI/compose, and UI documentation
in the same change. Remove claims that Task reminders are global, default off,
or driven by idle time. Document the structured PR contract and automatic
customer-wait transitions.

All daemon and agent tests use isolated `TARIBOY_BASE_DIR` and
`TARIBOY_RUNTIME_DIR` values with an isolated or disabled HTTP listener. Never
touch the live daemon or live data. Generated Desktop output remains unstaged;
rebuild committed Store UI only if shared source changes affect it.

## Shim supervisor lifecycle

Final Desktop verification exposed a pre-existing process-group leak in the
tmux harness cleanup path. The hidden supervisor remains a mode of the existing
shim process; no daemon service, helper process, protocol, dependency, or
operator setting is added.

The shim starts the harness as the direct leader of an owned foreground process
group and keeps that leader unreaped until cleanup is complete. A platform
observer reports only actual child exit: Linux uses
`waitid(P_PID, pid, WEXITED|WNOWAIT)` and macOS uses
`kqueue` `EVFILT_PROC|NOTE_EXIT`. `SIGSTOP`, `SIGCONT`, and unrelated or forged
`SIGCHLD` notifications do not end supervision or produce an exit status.

On natural exit, the shim first sends `SIGKILL` to the still-pinned process
group to remove lingering descendants, then reaps the direct leader exactly
once and atomically records its real exit status. On tmux `SIGHUP`, the shim
sends `SIGTERM` to the group and waits up to two seconds for confirmed leader
exit; if needed it sends `SIGKILL` and waits a further two seconds. It then uses
the same kill-before-reap path, preserving a cooperative harness exit code or
recording signal exit `137` after forced termination. `ESRCH` means the group
is already empty and is not an error.

Observer, signal, reap, or deadline failures are bounded and return an error.
They do not write the transient status file, so the outer shim retains its
existing fail-closed result of unknown failure (`-1`) instead of inventing
success from an uninitialized wait status. Error paths make a best-effort
`SIGKILL` cleanup but never block indefinitely waiting for an unconfirmed
child state.

## Test strategy

Use focused TDD slices for:

- task migration, `wait_customer` validation, safe pull-request URL mutation,
  event revisions, and API/CLI/UI round trips;
- agent migration/defaults, update validation, clone, compose, and inspect/list
  projections;
- assigned-agent question and final-customer-answer transactions, including
  multiple waits and intervening manual status changes;
- deterministic candidate ordering, sticky selection, every release condition,
  wait grace and answer re-entry, disabled settings, and missing tasks;
- startup/recovery/task-mutation/iteration-completion reconciliation, ordinary
  inbox delivery, idempotency, and per-agent failure isolation;
- `runtime: goal` placeholder validation, empty rendering, task rendering, and
  the `basic` and `tariboy-developer` image declarations;
- React Configuration and Task Detail behavior, production Desktop Playwright,
  and `tauri-driver` coverage required for UI changes.
- shim stop/continue handling, natural exit with lingering descendants,
  cooperative and forced tmux teardown, bounded observer/signal/reap failures,
  Linux Desktop cleanup, and Darwin compilation of the native exit observer.

Final branch verification is `make full-check`, plus focused Rust tests only if
the native host changes, followed by `git diff --check` and complete diff
inspection. Per the customer's integration constraint, the branch is then
left unmerged for review; local merge and distinct post-merge verification run
only after explicit approval recorded on TARI-43.
