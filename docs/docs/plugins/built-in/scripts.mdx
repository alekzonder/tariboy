---
title: scripts
description: Run asynchronous one-shot or fixed-interval local commands with durable per-run logs.
sidebar:
  label: scripts
  icon: scroll-text
---

`scripts` runs local shell commands without keeping the current agent iteration
open. It is an optional capability included in `basic:latest`. It does not
publish arbitrary channel messages; the separate [`schedule`](/docs/plugins/built-in/schedule)
plugin owns timed bus messages and is unchanged.

## Capability surface

Run exactly once:

```bash
tools script run health --description "Check service health" -- curl -fsS http://localhost:8080/health
```

Run immediately and then 60 seconds after each completion:

```bash
tools script schedule poll --every 60 --description "Poll the queue" -- ./bin/poll-queue
```

Runs of one recurring script never overlap. The fixed delay starts at the
previous run's actual finish time, and success or failure does not stop the
schedule.

Inspect and control definitions and runs separately:

```bash
tools script ls
tools script runs scr-agent-...
tools script logs srun-agent-...
tools script rerun scr-agent-...        # completed one-shot definitions only
tools script cancel scr-agent-...       # stop a definition and its active run
tools script cancel srun-agent-...      # stop only this run
tools script rm scr-agent-...           # inactive definitions only
```

Both creation commands require `--`. Everything after it is preserved as the
`sh -c` command, including tokens that look like flags. Commands run in the
agent's effective working directory. An absolute file path from the
[`workdir`](/docs/plugins/built-in/workdir) prompt can be used after `--` when
the effective CWD is elsewhere.

## Manage scripts from Desktop

Open the agent's **Scripts** tab to create and inspect that agent's local
scripts.

1. Choose **Run once** for one queued command, or **Schedule** for a recurring
   command. Enter a name, description, and command. A scheduled script also
   needs a positive interval in seconds; **Quiet exit** is optional and accepts
   an exit code from `0` through `255`.
2. Submit **Run once** or **Schedule**. A scheduled script starts immediately,
   then waits for its interval after each run finishes. Runs for one definition
   do not overlap.
3. Expand a script in the **Scripts** list to see its runs. Expand a run to
   inspect its status, exit code, timestamps, duration, log path, and inline
   log. Use **Copy path** to copy the log's absolute path or **Download log**
   to save the complete log.
4. Use **Cancel** on an active definition to stop future runs and request
   cancellation of its active run. Use **Cancel run** on an active run when
   only that attempt should stop. After a one-shot definition is completed,
   **Rerun** queues another attempt.
5. Remove only an inactive definition. Desktop asks for confirmation because
   removal permanently deletes the definition and all of its run history.

If a command has begun but is not terminal yet, wait for its final run status
before removing the definition. A cancellation request remains in progress
until the command's process exits.

## Results and exit codes

Every run has its own durable status, timestamps, exit code, and owner-only log
file. Combined stdout and stderr remain in that file. The concise
`script.result` inbox message contains the script ID, run ID, name, mode,
status, optional exit code, and absolute log path; it never embeds command
output.

By default:

- exit `0` records `succeeded` and publishes a result;
- every nonzero exit, including `2`, records `failed` and publishes a result;
- timeout, cancellation, and daemon-restart interruption have distinct
  statuses without invented exit codes.

A recurring script can suppress one expected numeric result explicitly:

```bash
tools script schedule poll --every 60 --quiet-exit 2 -- ./bin/poll-queue
```

The matching run is still stored with its exit code and log, but creates no
message and does not wake the agent. `--quiet-exit` is unavailable for
one-shot runs, and no exit code is quiet by default.

## Restart and lifecycle

Pending runs remain claimable after restart. A run that was already running is
recorded as `interrupted` and is not blindly repeated, because the command may
have completed an external side effect before the daemon lost process state.
A recurring definition schedules its next attempt after recovery; an
interrupted one-shot becomes completed and can be rerun explicitly.

Cancellation is durable and idempotent. A running attempt is shown as
`cancelling` until its process group actually exits, and it continues to occupy
the script's active slot during that window. A daemon restart preserves the
request and recovers that attempt as `cancelled`, not `interrupted`.

Removing an active definition, or one whose cancelled process has not exited
yet, returns `409 script_active`; cancel it first and wait for its run to become
terminal.
Removal deletes its run metadata and log files. A terminal result already
committed to the durable outbox remains deliverable even if its inactive
definition is removed before publication. Purging an agent removes all of its
script data.

## Prompt integration

```yaml Tariboyfile.yaml
plugins:
  - name: scripts
  - name: workdir
prompts:
  - file: $CURRENT_VERSION_STORE/skills/workdir/prompt.md
  - runtime: workdir
  - file: $CURRENT_VERSION_STORE/skills/scripts/prompt.md
```

The prompt tells agents to queue a durable run once, finish the current
iteration, and inspect the later result instead of invoking the creation
command repeatedly.

## Security boundary

Scripts execute with the agent's environment and filesystem permissions. The
capability does not create a sandbox, elevate privileges, or make an unsafe
shell command safe. Treat command bodies as executable input and avoid placing
credentials directly in them. Log reads and downloads are owner-scoped and
confined to the agent's scripts directory. The daemon opens the recorded file
relative to a pinned directory descriptor and refuses symlinks, hard links,
and non-regular files.

## Related reference

- [Agent tools: durable scripts](/docs/binaries/agent-tools#durable-scripts)
- [Channels: schedules and scripts](/docs/reference/channels#schedules-and-scripts)
- [Security and controls](/docs/security-controls)
