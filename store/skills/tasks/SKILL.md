---
name: tasks
description: Use when claiming, decomposing, delegating, questioning, updating, or completing work in Tariboy Native Tasks.
---

# Native Tasks

The Python script lives inside this skill directory under `scripts/` and calls
the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

Inspect work with `tasks mine`, `tasks ready`, `tasks ready --claim`, and
`tasks show <key>`. Create/decompose with `tasks create`; delegate with
`tasks assign`; keep decisions in `tasks comment`; advance with `tasks update`
and close only completed work with `tasks done`.

For a flexible task, ask with `tasks ask <key> user:<login>|agent:<name> <text>`.
A comment is not a blocking question.

For workflow-managed work, begin with `tasks work next` and
`tasks work show <assignment>`. Treat its packet as the complete authority:
use only declared actions, tools, outcomes, and channel patterns. Add artifacts,
ask assignment-scoped questions, observe through `tasks observe`, and complete
with an allowed outcome. Never invent another principal's identity.
