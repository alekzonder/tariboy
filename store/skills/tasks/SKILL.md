---
name: tasks
description: Use when claiming, decomposing, delegating, questioning, updating, or completing work in Tariboy Native Tasks.
---

# Native Tasks

This skill's launcher delegates to `ttasks`. The binary selects identity-bound
agent mode when `TARIBOY_TOOLS_SOCKET` is set; otherwise it uses operator mode.
The bare `tasks` command is an optional compatibility alias for `ttasks` in
agents whose image enables the `tasks` capability.

Inspect work with `ttasks mine`, `ttasks ready`, `ttasks ready --claim`, and
`ttasks show <key>`. Create/decompose with `ttasks create`; delegate with
`ttasks assign`; keep decisions in `ttasks comment`; advance with `ttasks
update` and close only completed work with `ttasks done`.

For a flexible task, ask with
`ttasks ask <key> user:<login>|agent:<name> <text>`.
A comment is not a blocking question.

For workflow-managed work, begin with `ttasks work next` and
`ttasks work show <assignment>`. Treat its packet as the complete authority:
use only declared actions, tools, outcomes, and channel patterns. Add artifacts
with `ttasks artifacts add <assignment>`, inspect assignment questions with
`ttasks questions <assignment>`, answer with `ttasks answer <question>`, and
subscribe with `ttasks observe subscribe <assignment> <pattern>`. Complete with an
allowed outcome. Raw channel subscriptions and undeclared direct or group
messages remain denied. Never invent another principal's identity.
