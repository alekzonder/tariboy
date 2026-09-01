---
name: schedule
description: Use when an agent needs a one-shot or recurring future Tariboy wake-up or channel publication.
---

# Agent Schedules

This skill's `scripts/schedule.sh` launcher lives inside this skill directory
and calls the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

- One-shot: `scripts/schedule.sh add --kind oneshot --spec <time>`
- Recurring: `scripts/schedule.sh add --kind cron --spec "<cron expression>"`
- Inspect: `scripts/schedule.sh ls`
- Cancel: `scripts/schedule.sh cancel <id>`

Add `--channel` when the firing should publish a message instead of starting a
new iteration.
