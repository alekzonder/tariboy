---
title: goal
description: Receive or set the selected Native Task and AI usage attribution.
sidebar:
  label: goal
  icon: target
---

`goal` is included in the official Store's `basic` source. It provides one command:
`scripts/goal.sh set <TASK-KEY>`.

Before the harness starts, `tariboyd` reads the selected Goal once and stamps
the iteration's AI-proxy lease with its task key and top-level root key. Every
proxied request in that iteration therefore records `task_id` and `epic_id`
without an agent command. An iteration without a selected Goal remains
unattributed.

When an iteration begins without a Goal, the agent can create or claim a task
with the separate `tasks` capability, then set it as the current Goal. The
daemon accepts only an active task assigned to that agent, persists the
selection, resolves its top-level root, and updates the live proxy lease. Usage
already recorded before `set` remains unattributed; only later requests receive
the task and root keys. The command is idempotent for the same key and rejects
replacement with a different Goal.

Package the Goal skill and render the runtime value explicitly:

```yaml Tariboyfile.yaml
plugins:
  - name: goal
skills:
  - dir: ../../skills/goal
prompts:
  - runtime: goal
```

The Goal skill explains selection and attribution. Use the separate
[`tasks`](/docs/plugins/built-in/tasks) capability and skill to read or mutate
the authoritative Native Task.
