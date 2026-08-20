---
title: schedule
description: Create agent-owned one-shot or cron schedules that wake the agent or publish to a channel.
sidebar:
  label: schedule
  icon: calendar-clock
---

`schedule` lets an agent arrange future bus events without keeping an iteration
open. It is an optional capability and is not included in `basic:latest`.

## Capability surface

```bash
tools schedule add --kind oneshot --spec 2026-08-20T10:00:00Z
tools schedule add --kind cron --spec "*/15 * * * *" \
  --channel agent:worker:inbox \
  --message '{"text":"check the queue"}'
tools schedule ls
tools schedule cancel <schedule-id>
```

`kind` is `oneshot` or `cron`. The schedule parser validates `spec`; a malformed
time or cron expression is rejected before a schedule is created. `--message`
must be valid JSON.

## Delivery behavior

Without `--channel`, the agent API targets the authenticated agent's own inbox.
A firing publishes a normal bus message, so it follows the same durable
delivery and loop-wake behavior as other messages. A one-shot disables itself
after firing. A cron schedule computes its next fire time and remains enabled
until cancelled.

Schedules are agent-owned. Listing and cancellation are scoped to the calling
agent; an agent cannot use an arbitrary name to manage another agent's records.

## State and lifecycle

The daemon persists the schedule ID, owner, kind, specification, destination,
message template, next firing time, and enabled state in SQLite. Daemon restart
therefore does not discard future wake-ups. Removing an agent's durable data
removes its schedule records.

The scheduler publishes on the daemon's bus. If a schedule targets the agent's
inbox while its loop is disabled, the delivery remains pending until the loop
can consume it.

## Prompt integration

```yaml Tariboyfile.yaml
plugins:
  - name: schedule
prompts:
  - file: $CURRENT_VERSION_STORE/skills/schedule/prompt.md
```

There is no schedule runtime marker. The Store prompt teaches the commands;
`tools schedule ls` reads current state when needed.

## Workflow restrictions

A workflow-managed assignment may create a wake-up for its own inbox. A
schedule that names an explicit channel additionally requires the
`schedule.publish` tool permission in the active work packet. Without it, the
API returns `workflow_tool_not_allowed` and creates nothing.

At firing time, the scheduler also checks for a still-active workflow lease
owned by the agent. It postpones the publish while that lease is active, then
retries the due schedule after the lease ends. Scheduled traffic therefore
cannot escape an assignment's live communication boundary.

## Related reference

- [Channels: schedules and scripts](/docs/reference/channels#schedules-and-scripts)
- [Messaging architecture](/docs/architecture/messaging)
- [Configurable task workflows](/docs/task-workflows#agent-tools-and-security-boundary)
