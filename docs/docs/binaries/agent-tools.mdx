---
title: Agent capability scripts
description: Run packaged skill-local scripts inside an agent for identity, status, loop control, context, messaging, schedules, and durable work.
sidebar:
  label: Capability scripts
  icon: wrench
---

Run **inside** an agent from the applicable packaged skill directory. Each
`scripts/*.sh` launcher executes the skill's Python 3 standard-library client.
Only the legacy `tasks` and `i-am-done` also have PATH compatibility shims. The scripts
talk to the identity-bound per-agent socket selected by
`$TARIBOY_TOOLS_SOCKET`; the daemon derives the agent identity server-side.
For non-bare iterations, `python3` is resolved to an absolute path from the
final agent environment before the harness starts. An explicit agent `PATH`
must therefore include it.

| Tool | Purpose |
| --- | --- |
| `scripts/whoami.sh` | agent, cwd, current iteration |
| `scripts/status.sh [set …]` | read / set the one-line status |
| `scripts/loop.sh done` | finish this iteration (`i-am-done`) |
| `scripts/context.sh get` / `set <text>` | read / overwrite durable working memory |
| `scripts/goal.sh set <TASK-KEY>` | select an assigned active Goal for this iteration |
| `tmail inbox`/`sent`/`archive`/`show` | read the mailbox |
| `tmail send --to P …` / `tmail reply ID TEXT` | send a letter, answer one; `--instant` sends an [instant letter](/docs/reference/mail#instant-letters) |
| `tmail instant [--format text\|json]` | for a harness hook: print the instant letters that wait for this iteration (`POST /tools/mail/instant`) |
| `scripts/schedule.sh add`/`ls`/`cancel` | agent-owned scheduled wake-ups |
| `tscripts run`/`schedule`/`rerun`/`ls`/`runs`/`logs`/`cancel`/`rm` | local asynchronous commands and run history |
| `tariboy assessment add --file PATH` / `ls [--history]` / `show ID` | record an [assessment](/docs/plugins/built-in/assessments) (needs `assessments`); read assessments (open to every agent). `ls` returns the latest assessment of each iteration and task; `--history` returns every assessment |
| `GET /tools/group/info`, `status`, `observe/{member}`, `POST /tools/group/loop` | group coordination; group sends are `tmail send --to group:<g>` or `agent:<member>` |

`tariboy-tasks` is installed globally with the `ttasks` alias. Packaged daemons
also expose it through a shared runtime `bin/ttasks` symlink on their baseline
`PATH`, so fresh Desktop agents can use it before optional CLI installation.
An explicit agent `PATH` override must retain that directory or another
installed `ttasks`. A non-empty
`TARIBOY_TOOLS_SOCKET` selects its identity-bound agent mode and a missing or
unreachable socket fails closed; without the variable it uses the host Unix
daemon socket as the customer actor. Images that enable `plugins: [{name:
tasks}]` also get the bare `tasks` legacy compatibility shim. See [Native
Tasks](/docs/tasks) for the command flow.

`tariboy-scripts` is installed globally with the `tscripts` alias, and the
daemon links it as runtime `bin/tscripts` in the same way as `ttasks` and
`tmail`. It replaces `scripts.py` of the `scripts` skill and prints the same
output, errors and exit codes. It works only in agent mode: without
`TARIBOY_TOOLS_SOCKET` it exits `2`. The skill launcher `scripts/scripts.sh`
stays a thin launcher, `exec tscripts "$@"`, like `tasks.sh` and `mail.sh`.
`--json`, or `TARIBOY_TOOLS_JSON=1`, prints the result as compact JSON.
`tscripts --version` prints the client build. Exit codes:

- `0`: the command succeeded.
- `1`: the daemon refused the request. The client prints the message of the daemon.
- `2`: the command line is wrong, `TARIBOY_TOOLS_SOCKET` is not set, or the socket is not reachable.

On a task that follows a queue's [workflow](/docs/task-workflows) the daemon
assigns the work, and `ttasks done`, `ttasks update --status`, and
`ttasks ready --claim` do not apply. The agent reads `ttasks workflow get KEY`,
sets declared artifacts with `ttasks artifacts set|ls|show`, and leaves its
status with `ttasks advance KEY --outcome NAME --from STATUS`, where `--from`
makes a retried advance fail with `status_changed` instead of applying twice.
`advance` waits for the transition's check scripts and exits non-zero on a
rejection or failure, naming the next command; `--no-wait` returns the pending
request at once. `ttasks workflow runs KEY` and `ttasks workflow log KEY RUN`
show what the scripts did. `ttasks workflow move`, `ttasks cancel`, `ttasks requeue`, and the
`ttasks queue` commands are operator-only. Workflow scripts themselves never
receive the tools socket.

`tariboyd` normally selects and persists one sticky assigned task per enabled
agent, then stamps its key and top-level root onto the AI-proxy lease before the
harness starts. If an iteration starts without a task, the agent may create or
claim one through `ttasks` and run `scripts/goal.sh set <TASK-KEY>`. The task
must be active and assigned to that agent. Subsequent requests use the new
task/root attribution; earlier requests remain unattributed. A second call with
a different key is rejected.

Task priority is one of `P0` (Critical), `P1` (High), `P2` (Normal), or `P3`
(Low). New tasks default to `P2`; set or change it explicitly with:

```bash
ttasks create --queue OPS --title "Restore service" --priority P0
ttasks update OPS-12 --priority P1
```

`ttasks show`, list commands, and `--json` output include `priority`. Roots and
nested siblings are returned in priority order (`P0` through `P3`), then by
their manual position within the same priority bucket.

## Unknown flags are rejected

A flag that no command branch reads is a hard error: the script prints
`unknown flag …` and exits `2`. The check
runs after the request is built but before it is sent, so nothing reaches the
daemon. Older builds dropped an unread flag silently and exited `0`, which is how
`ttasks update --priority` could look like it worked while changing nothing.

`tscripts run` and `tscripts schedule` have a `--` separator: flags are parsed up to it, and
everything after it is taken verbatim for the local command — that is why
`-- curl --silent URL` hands `--silent` to `curl`. No other command has one, so
elsewhere an argument that starts with `--` cannot be passed through: it is
always read as a flag, and the token after it as its value. Use the
`--flag=--value` form there, which keeps the value inside a single token.

## Durable scripts

Scripts run shell commands in the agent's workdir without blocking its current
iteration. Like an iteration, each run first sources the global and agent shell
scripts, so their exports, such as tokens, reach the command. Queue one run, or
create a fixed-delay recurring definition:

```bash
tscripts run health --description "Check service health" -- curl -fsS http://localhost:8080/health
tscripts schedule poll --every 60 -- ./bin/poll-queue
tscripts ls
tscripts runs scr-agent-...
tscripts logs srun-agent-...
tscripts rerun scr-agent-...        # also resumes a stopped schedule
tscripts cancel scr-agent-...       # definition, including its active run
tscripts cancel srun-agent-...      # this run only
tscripts rm scr-agent-...
```

Each run's combined stdout and stderr is kept in its own agent scripts log. Exit
`0` records success; every other exit, including `2`, records failure. Both
send a `script.result` letter from `system:scripts` so the next iteration can act. The letter carries
script/run IDs, name, mode, status, optional exit code, and the absolute
`log_path`, without embedding stdout or stderr. Read that file when the result
details are needed.

The one exception is the quiet exit code `111`, available to the script as
`TARIBOY_QUIET_EXIT`: a recurring run that exits with it publishes nothing and
keeps its schedule. The run and log remain visible. Recurring runs start once
immediately, wait the configured delay after completion, and never overlap. The
`--quiet-exit CODE` flag is deprecated; see
[scripts](/docs/plugins/built-in/scripts#results-and-exit-codes).
