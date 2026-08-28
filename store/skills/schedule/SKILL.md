---
name: schedule
description: Use when an agent needs a one-shot or recurring future Tariboy wake-up or channel publication.
---

# Agent Schedules

- One-shot: `tools schedule add --kind oneshot --spec <time>`
- Recurring: `tools schedule add --kind cron --spec "<cron expression>"`
- Inspect: `tools schedule ls`
- Cancel: `tools schedule cancel <id>`

Add `--channel` when the firing should publish a message instead of starting a
new iteration.
