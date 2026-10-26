---
title: schedule
description: Create agent-owned one-shot or cron schedules that send a letter to the agent or another principal.
sidebar:
  label: schedule
  icon: calendar-clock
---

`schedule` lets an agent arrange future letters without keeping an iteration
open. It is optional and is not included in the official Store's `basic` source.

## Capability surface

```bash
scripts/schedule.sh add --kind oneshot --spec 2026-08-20T10:00:00Z
scripts/schedule.sh add --kind cron --spec "*/15 * * * *" \
  --message '{"text":"check the queue"}'
scripts/schedule.sh ls
scripts/schedule.sh cancel <schedule-id>
```

`kind` is `oneshot` or `cron`. The schedule parser validates `spec`; a malformed
time or cron expression is rejected before a schedule is created. `--message`
must be valid JSON.

## Delivery behavior

A firing sends a letter from `system:schedule`, so it follows the same durable
mail and loop-wake behavior as other letters. The agent route
`POST /tools/schedule/add` takes `to`, a recipient principal; without it the
schedule writes to the calling agent. A skill that still passes `--channel`
keeps working: `chat:service:<agent>` and `agent:<agent>:inbox` map to
`agent:<agent>`, and any other channel to the calling agent. The message
template may set `subject`, `text`, `kind` (default `schedule.fire`), `data` and
`notice`; the letter is a request unless `notice` is set. A one-shot disables itself
after firing. A cron schedule computes its next fire time and remains enabled
until cancelled.

Schedules are agent-owned. Listing and cancellation are scoped to the calling
agent; an agent cannot use an arbitrary name to manage another agent's records.

## State and lifecycle

The daemon persists the schedule ID, owner, kind, specification, recipient,
message template, next firing time, and enabled state in SQLite. Daemon restart
therefore does not discard future wake-ups. Removing an agent's durable data
removes its schedule records.

The scheduler sends through the daemon's mailbox. If a schedule writes to the
agent while the agent is stopped or halted, the letter stays open in its inbox
until the loop can consume it.

## Prompt integration

```yaml Tariboyfile.yaml
plugins:
  - name: schedule
skills:
  - dir: ../../skills/schedule
```

There is no schedule runtime marker. The packaged skill teaches the commands;
`scripts/schedule.sh ls` reads current state when needed.

## Related reference

- [Mail: schedules and scripts](/docs/reference/mail#schedules-and-scripts)
- [Mailbox architecture](/docs/architecture/messaging)
