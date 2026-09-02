---
name: tasks
description: Use when claiming, decomposing, delegating, questioning, updating, or completing work in Tariboy Native Tasks.
---

# Native Tasks

This skill's `scripts/tasks.sh` launcher lives inside this skill directory and
calls the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.
The bare `tasks` command is a compatibility shim for this same native task
system, which is the durable source of truth for work, decomposition, ownership,
questions, and answers.

Inspect work with `scripts/tasks.sh mine`, `scripts/tasks.sh ready`,
`scripts/tasks.sh ready --claim`, and `scripts/tasks.sh show <key>`.
Create/decompose with `scripts/tasks.sh create`; delegate with
`scripts/tasks.sh assign`; keep decisions in `scripts/tasks.sh comment`;
advance with `scripts/tasks.sh update` and close only completed work with
`scripts/tasks.sh done`.

For a flexible task, ask with
`scripts/tasks.sh ask <key> user:<login>|agent:<name> <text>`.
A comment is not a blocking question.

For workflow-managed work, begin with `scripts/tasks.sh work next` and
`scripts/tasks.sh work show <assignment>`. Treat its packet as the complete authority:
use only declared actions, tools, outcomes, and channel patterns. Add artifacts,
ask assignment-scoped questions, observe through `scripts/tasks.sh observe`,
and complete with an allowed outcome. Raw channel subscriptions and undeclared
direct or group messages remain denied. Never invent another principal's identity.
