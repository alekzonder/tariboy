---
title: Agents & the iteration loop
description: An agent runs a series of supervised iterations, driven by a timer or by messages, on top of a whoami / loop / channels core.
sidebar:
  label: Iteration loop
  icon: repeat
---

An **agent** is a configured, named instance of an image with its own working
directory and its own loop. It does not run continuously — it runs a series of
**iterations**. On each iteration the daemon:

1. prepares a prompt — a platform-owned **Task Processing Order** block groups
   one-shot instructions, messages (including awaiting replies), and Goal in
   that order. Schema v2 preserves the declared order of the remaining image
   layers; self-contained legacy schema-v1 artifacts use the same work block;
2. launches the harness (e.g. Claude) under the [shim](/docs/architecture/shim)
   for exactly one pass;
3. records the outcome, iteration logs, AI usage, and audit timeline.

## What drives the loop

The loop is driven three ways, all requiring the agent to be **enabled**:

- **Timer** — `interval_s`: wake every N seconds.
- **Schedule** — an RFC 5545 `RRULE`, set with `tariboy loop schedule`: start
  an iteration at each occurrence, with the trigger `schedule`. See
  [Schedule](#schedule).
- **Message** — a publish that matches one of the agent's subscriptions nudges
  the loop to start an iteration now (only if pending deliveries exist).

Loop policy is per-agent: `interval`, `timeout` (soft), `hard-timeout`,
`on_timeout` and `on_error` (e.g. `restart`). An iteration ends when it signals
completion, exits, or reaches an enforced deadline.

### Schedule

The schedule is an optional `RRULE` with an optional `DTSTART` line before it,
for example:

| Run | Schedule |
| --- | --- |
| Every weekday at 09:30 | `RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=9;BYMINUTE=30` |
| The 15th of each month at 08:00 | `RRULE:FREQ=MONTHLY;BYMONTHDAY=15;BYHOUR=8;BYMINUTE=0` |
| The last day of each month at 23:45 | `RRULE:FREQ=MONTHLY;BYMONTHDAY=-1;BYHOUR=23;BYMINUTE=45` |
| Once, on 20 October 2026 at 09:30 | `DTSTART:20261020T093000` then `RRULE:FREQ=DAILY;COUNT=1` |
| Every 2 hours | `RRULE:FREQ=HOURLY;INTERVAL=2` |

The two lines can be separated by a newline or a space. Schedules run at whole
minutes: the daemon rejects `FREQ=SECONDLY` and any `BYSECOND` other than `0`.
When the daemon saves a schedule, it stores a normalized form: it adds
`BYSECOND=0` and, when there is no `DTSTART`, it adds `DTSTART` as the current
minute. A `DTSTART` without `TZID` or `Z` is in the time zone of the daemon
host, also for a remote host. Name another zone with
`DTSTART;TZID=Europe/Moscow:20261018T000000`. An invalid rule is rejected with
`bad_schedule` and the previous schedule stays.

The engine arms one timer for the nearer of the next `interval_s` firing and
the next occurrence. The two sources work independently; set `interval_s` to 0
to start the agent only on its schedule. An occurrence never starts a second
iteration while one runs, and missed occurrences are not replayed: after an
iteration, or after the daemon or the agent was off, the next start is the
first occurrence after the current time. A finished rule (`COUNT` or `UNTIL`)
stays saved but starts nothing. The agent view reports `schedule` and
`schedule_next_at` (RFC 3339, empty when there is no next start).

### Agent Goals

Goal is per-agent and enabled by default. The reconciler retains one assigned
Native Task selection until it is released: `done` or `cancelled`, lost
assignment or visibility, Goal disabled, a `wait_customer` task whose oldest
unanswered customer wait reaches the agent's positive
`goal_wait_customer_timeout_s` (300 seconds by default), or eligible assigned
work with a strictly higher priority. It orders replacements by priority (`P0`
through `P3`), then `in_progress` before `open`, creation time, and task key.
Equal-priority work does not displace a valid selection. A task's pull request
URL does not affect Goal selection or release.

At startup, relevant task or agent changes, terminal iteration completion, and
the bounded one-minute recovery cadence, the reconciler publishes one durable
`task.goal` inbox message. It follows `Publish -> delivery -> WakeMessage` and
never starts an iteration directly. An unprocessed Goal delivery outside the DLQ
blocks another publication. Dead-lettered deliveries are retained but do not
block a new Goal generation; the positive per-agent delivery cooldown
(60 seconds by default) still blocks rapid repeats. A wait within its grace
period remains the goal but sends no continuation wake. Disabled agents and
agents on `bare:latest` neither select nor receive a goal wake, but preserve a
valid sticky key; only disabling Goal clears it.

Every non-bare agent receives daemon-owned Goal guidance inside **Task Processing
Order**, regardless of image plugins or runtime entries. The guidance has two
forms, chosen by the selected task.

- **Flexible task** (a queue with no workflow): the block tells the agent to work
  through the Native Task workflow, wait for a recorded customer answer while in
  `wait_customer`, and set `wait_customer` after recording a PR while monitoring
  it rather than merging. When selected, it also contains the task key, title,
  priority, status, and description. With no selected task it carries only the
  general guidance.
- **Workflow task** (a task that follows a workflow image): the block states
  that the task follows a workflow and that the agent leaves the status only with
  `ttasks advance`, then lists the task fields, the status instructions from the
  pinned image, the outcomes with their requirements, the artifacts, the last
  rejected or failed transition request of the visit, and the exact `ttasks
  artifacts set` and `ttasks advance --from STATUS` commands. If the workflow
  data cannot be loaded the agent gets the flexible text and a note to run
  `ttasks workflow get KEY`. See [The Goal block](/docs/task-workflows#the-goal-block).

The inbox message is only a durable wake hint. Task title, description, artifact
values, and script messages are untrusted task input, not daemon instructions or
lifecycle authority; the workflow form renders them as fenced, bounded data.
Status instructions are trusted image content.

**Idle counting.** The workflow form feeds the `idle_iterations` limit. The loop
manager is the single place that reports an iteration end: every terminal path of
an iteration (normal finish, runner error, a manual failure with the tmux session
alive, and the adoption and stale-kill recovery paths) calls
`Manager.iterationCompleted`, which first hands the daemon the agent, the
iteration, the Goal task key the iteration was prepared with, and its start and
end times, and only then wakes the Goal reconciler. The daemon counts the
iteration as idle when the agent held the task's pool status when the iteration
began and made no transition request during it, and pauses the task at the
limit; see [Pauses](/docs/task-workflows#pauses).

The block starts with `# Task Processing Order` and a fixed instruction to
process one-shot, then messages, then Goal. Its `## One-shot`, `## Messages`,
and `## Goal` subsections always appear. Missing inputs explicitly say that
there is no one-shot instruction, no incoming messages, or no selected Goal
for this iteration; the agent is told to continue without running commands
merely to search for absent input. Outstanding requests remain nested under
Messages even when no incoming messages exist. Message IDs, threading fields,
subject/data, and the explicit `message processed` instruction are retained.
Bare sessions still receive no prompt.

The operator prompt preview uses the same grouping and Goal selection. It
keeps placeholders for messages and the one-shot supplied at execution time;
viewing the preview does not drain messages or increment delivery attempts.

## Harness environment and preflight

The daemon's own environment is the baseline every iteration starts from, so
agents see what the account sees. Desktop starts the daemon inside the account's
login shell, which supplies that environment directly; a daemon started without a
usable shell instead resolves the account's `PATH` once, before loop startup,
from a bounded interactive-login-shell probe. Per-iteration construction begins
with that daemon environment, applies harness-required values, then explicit
agent environment and secrets. An explicit agent `PATH` is therefore an
intentional override. Non-bare images prepend the agent bin directory after the
override; bare images omit that prefix and otherwise keep the daemon baseline.

Before starting the shim, the runner checks the adapter's actual executable
against this final environment. For a non-bare image it also resolves `python3`
there and supplies that absolute interpreter path to the packaged skill
launchers. A missing or non-executable harness or Python interpreter fails
preparation through the normal `harness_error` path without including
environment values, prompts, secrets, working directories, or user files. This
preflight improves the diagnosis but cannot remove the normal race in which an
executable disappears before process launch.

Immediately before the unchanged harness command is executed, Bash sources
`<base-dir>/global-agent-shell.sh`, then
`<base-dir>/agents/<name>/agent-shell.sh`. Missing or empty files do nothing;
exports and filesystem effects from either script are available to that
iteration. A source-time failure prevents the harness from starting and follows
the existing `harness_error` lifecycle.

The same scripts, in the same order, are sourced before every command the
daemon runs as the agent outside an iteration: each durable script run, where
a source-time failure fails that run, and each `run_as: agent` workflow check,
which starts with the environment the scripts export and fails without running
when they fail.

Every `harness_error` iteration writes an `iteration_finished` audit event whose
`error` field states the cause, which the audit timeline shows next to the
status. A failure before or during launch, such as a configured agent CWD that
does not exist, records the launch error. A harness that exits non-zero records
`harness exited with code N`, followed for non-interactive agents by up to 20
trailing lines (2 KiB) of `harness.stderr.log`; the same lines are already teed
into the audit as `harness_output`.

For a schema-v2 image with skills, the launch gate prepares
`agents/<agent>/image-bridges/<image-digest>/<adapter-contract>/<harness>` before
an image is promoted or an iteration row is created. The bridge contains copied
skill bytes and generated adapter metadata; it never links back to the image or
source tree. Publication is owner-only and atomic. The current adapter contract
version is `2`. A complete bridge is reused across iterations and daemon
restarts, while an incomplete or modified cache is rebuilt. A preparation or
harness-support failure leaves the previous image active and records the
pending-image error. Because no iteration row exists yet, such a failure also
halts the loop and records its cause in the agent's `error_reason`, which the
agent status surfaces as `halt_reason` and the workspace header displays; the
audit timeline records one `iteration_failed` event with the
`image_activation_failed` reason. `Start` or `Restart` clears it and resumes the
loop.

The adapters attach the bridge additively:

| Harness | Projection |
| --- | --- |
| Claude Code | `--plugin-dir <absolute bridge>` |
| Codex CLI | At-most-8,000-character prompt catalog with absolute bridge `SKILL.md` paths |
| OpenCode | `OPENCODE_CONFIG_DIR=<absolute bridge>` with an absolute `skills.paths` entry |
| Cursor CLI | The same bounded prompt catalog as Codex, with absolute bridge `SKILL.md` paths |

Claude Code requires version `2.1.227` or newer. OpenCode is capability-probed
with `opencode debug config`; its effective configuration must report the
generated absolute skill path. Codex needs no image-skill version probe and no
plugin installation: Tariboy prepends escaped name, description, and path
metadata to the shared prompt file, and Codex reads the selected skill on
demand. Tariboy does not change CWD, `HOME`, or `CODEX_HOME`, and it does not
disable normal global or CWD discovery. Schema-v1 images retain their legacy
CWD-relative skill materialization.

Harness adapters select their native execution mode from the persisted
`interactive` setting. Batch Codex iterations use `codex exec --json` with
stdin disconnected so the loop receives machine-readable output and cannot
wait for terminal input. Interactive Codex iterations instead launch the
top-level Codex TUI with the assembled prompt and an open terminal. Both modes
consume the same catalog-prefixed prompt; Codex image skills do not appear in
native `/skills`, `$` completion, or plugin inventory. Claude
similarly omits its print-mode flags. Model, effective environment, working
directory, and AI-proxy routing still apply in both modes. Codex receives the
agent's effort as `-c model_reasoning_effort="<effort>"`, and `opencode run`
receives the agent's model as `--model`; OpenCode has no effort setting.

Both Codex modes take their approval flag from the daemon's codex approval mode
(`codex_approval_mode` in `daemon_config`), read when each iteration starts:
`yolo`, the default, passes `--yolo` (no sandbox, no approval prompts), and
`approve-for-me` passes `--approve-for-me` (the workspace-write sandbox, with
approval requests reviewed automatically by Codex). Change it with
`tariboy harness codex-approval set MODE` or **Settings → General → Codex**; a
running iteration keeps the flag it started with. `--approve-for-me` needs a
Codex CLI that has the flag; an older one fails the launch with an unknown
argument error. See
[Security controls](/docs/security-controls#codex-approval-mode).

The Cursor CLI runs its `agent` executable with the prompt as the last
argument, `--force`, `--endpoint <AI proxy URL>`, and `--model` when the agent
names one. Batch iterations add `-p --output-format stream-json --trust`, and
the loop reads token usage from the `result` event on harness stdout (see
[AI proxy](/docs/architecture/ai-proxy#cursor-harness)). Interactive iterations launch the
Cursor TUI. Like Codex, a prompt too large for one argument fails the launch.
The agent's `effort` setting has no Cursor equivalent and is ignored.

While a timed iteration is running, Agent Overview shows its deadline and can
extend it by one configured soft-timeout period. Each extension is persisted
before the watchdog is updated, so a daemon restart can adopt the running
iteration with the same deadline and extension count. Extensions are unavailable
after the timeout starts firing or for agents with no soft timeout.

Agents signal completion with `scripts/loop.sh done` (surfaced as `i-am-done`).

When a schema-v2 template declares `runtime: workdir`, preparation resolves the
agent's managed `agents/<agent>/workdir` to an absolute path and renders it
separately from the effective CWD in `runtime: identity`. The two paths are the
same for a default-CWD agent and differ when the operator configured an
external repository CWD. Rendering the path grants no new filesystem access;
the harness already runs with the daemon account's permissions.

:::note
Loop policy is set with the `tariboy loop …` subcommands — `interval`,
`schedule`, `timeout`, `hard-timeout`, `on-timeout`, `on-error`, `max-idle`,
`ai-stall-timeout`. See
[the command reference](/docs/reference/commands#operator-commands).
:::

## Image capabilities

Schema-v2 agents receive exactly the plugins declared by their image, including
an empty list. No core union is implicit. The familiar built-in capabilities
are:

- **whoami** — identity: which agent, cwd, current iteration, and client/daemon versions.
- **loop** — iteration control: signal done, start/stop the loop.
- **messages** — publish to and receive from channels.

On top of the core, an image opts into **optional capabilities**:

- `context` — durable working memory,
- `status` — a one-line "what I'm doing",
- `schedule`, `scripts`, `tasks`,
- `image-creator` and any validated, explicitly
  installed external plugin capability.

The built-in `workdir` plugin is instruction-only. The `goal` capability adds
only `scripts/goal.sh set <TASK-KEY>`, allowing an agent that started without a
Goal to select an active task assigned to itself and attribute subsequent AI
requests. It cannot replace a different Goal already attached to that
iteration. A schema-v2 image explicitly packages their skills and composes the
matching runtime value. See
[built-in plugins](/docs/plugins/built-in).

See [Built-in plugins](/docs/plugins/built-in) for the complete capability,
command, prompt, and persistence reference.

Capabilities gate routes and compatibility shims, but do not package skills or
contribute prompt text automatically. Schema-v1 images retain the historical
core union and render the matching skill instructions for compatibility.

Before each iteration's prompt or message preparation, the launch gate first
honors a pending image assignment. Without one, it checks whether the active
ordinary mutable-build ref now resolves to a different digest and stages that
digest as pending. It then validates the new artifact, prepares and verifies
any native skill bridge, reconciles image-owned shims, atomically promotes the
DB assignment, and records the iteration's image ref, source version, digest,
and template hash. The `runtime: identity` value renders that immutable source
version when present. A running iteration is
never interrupted. Backup and staging markers make an interrupted swap
recoverable without changing durable agent state.
Recovery compares the pinned digest, not only the ref. For daemon-managed and
ordinary mutable-build refs, both inspection and staging resolve the exact
retained digest recorded on the agent, including assignments that were pending
when the daemon was upgraded or the ordinary ref was rebuilt.

The agent image read API projects the same selection order under the image
publication gate: explicit pending digest, ordinary mutable active ref, then
current pinned image. Current version metadata always comes from pinned
inspection. This read does not stage or promote an assignment; next is a
preview at read time and can change before the launch gate runs. A changed
digest triggers activation even if the source version stays the same. A failed
inspection is returned explicitly, and the separate pending error preserves
activation failures even without a pending ref.

## Manager internals and lock order

The loop manager (`internal/loop`) keeps one engine per running agent. It is a
facade over these parts:

| Part | Owns |
| --- | --- |
| Agent runtimes (`runtimes.go`) | The in-memory registry: running engines, tools servers bound before an engine starts, iterations being adopted, a Restart pending after adoption, and stale-kill markers. It also holds each agent's lifecycle gate and handoff lock. |
| Script supervisor (`scripts.go`) | Durable script runs: claim, start as the agent, cancel, and record the result. |
| Tools facade (`tools_facade.go`) | The per-agent tools socket API. It reaches the manager only through narrow interfaces, so the API does not depend on a loop engine. |
| Image activation (`image_activation.go`) | Pending image activation, the Reprovision swap, the skill bridge, and crash recovery of an interrupted swap. |
| Iteration watcher (`iteration_watcher.go`) | The one polling loop that waits for `result.json`. It resyncs the hard deadline, enforces the soft deadline and the done grace with at most one Kill, and, for adoption, probes the shim. The runner and adoption both use it. |

The manager takes its locks in one order:

1. The agent's **lifecycle gate**. It serializes the lifecycle of one agent:
   start, Stop, Remove, Reprovision, and a Restart launch after adoption from
   its decision until its iteration row exists. A wait for it ends when the
   daemon shuts down.
2. The image **publication gate**. Image builds, image activation, Run, and
   Reprovision take it.
3. The registry lock. It guards only the registry maps. No database call,
   socket operation, or callback runs while it is held.

A path that holds a later lock never waits for an earlier one. For example,
Reprovision takes the agent's lifecycle gate before the publication gate, the
same order as a Restart launch that activates an image. Run never starts an
agent, so it never takes a lifecycle gate while it holds the publication gate.
A normal iteration launch takes only the publication gate. The agent's handoff
lock (see [the shim](/docs/architecture/shim)) is taken while no gate is held.

Image activation can take long: it unpacks the image, prepares the skill
bridge, and probes the harness. While a Restart launch waits in it, only the
lifecycle operations of that same agent wait for it: Start, Stop, Restart,
Remove, Reprovision, and the agent's own `loop start` and `loop stop`. Kill,
state reads, wakes, other tools calls, and every other agent continue.
