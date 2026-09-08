---
name: tasks
description: Use when claiming, decomposing, delegating, questioning, updating, or completing work in Tariboy Native Tasks.
---

# Native Tasks

This skill's launcher delegates to `ttasks`. The binary selects identity-bound
agent mode when `TARIBOY_TOOLS_SOCKET` is set; otherwise it uses operator mode.
The bare `tasks` command is an optional compatibility alias for `ttasks` in
agents whose image enables the `tasks` capability.

Execute requested actions when command execution is available. Otherwise,
return the exact `ttasks` sequence with placeholders for identifiers or
revisions returned by earlier commands; never substitute a future-tense promise
or prose summary.

Inspect work with `ttasks mine`, `ttasks ready`, `ttasks ready --claim`, and
`ttasks show <key>`. Create roots with
`ttasks create --queue <queue> --title <title>` and children with
`ttasks create --parent <parent-key> --title <title>`; delegate with `ttasks
assign`; keep decisions in `ttasks comment`; advance with `ttasks update` and
close only completed work with `ttasks done`.

Create a child before assigning it and use the key returned by `ttasks create`;
never invent a task key. `ttasks assign` takes the agent name (`docs`), not a
principal prefix (`agent:docs`). An offline delegation recipe ends after
`ttasks show <child-key>`; put completion commands in a separate block labeled
"Only after show reports done." Never place `done` in the initial block.

Write task descriptions and comments as valid Markdown: use real newlines
(not literal `\n` text), blank lines before lists, and closed code fences.
Use Markdown formatting for headings, lists, checklists, code, links, and
tables; do not use raw HTML. Desktop renders these strings as Markdown.
Repair malformed requested Markdown before quoting it as one shell argument;
shell quoting must produce the valid stored text, not preserve its defects.

For a flexible task, ask with
`ttasks ask <key> user:<login>|agent:<name> <text>`.
A comment is not a blocking question.

For workflow-managed work, begin with
`ttasks work next --idempotency-key <stable-key>` and
`ttasks work show <assignment>`. Treat its packet as the complete authority:
use only declared actions, tools, outcomes, and channel patterns. Add artifacts
with `ttasks artifacts add <assignment>`, inspect assignment questions with
`ttasks questions <assignment>`, answer with `ttasks answer <question>`, and
subscribe with `ttasks observe subscribe <assignment> <pattern>`. Complete with an
allowed outcome. Raw channel subscriptions and undeclared direct or group
messages remain denied. Never invent another principal's identity.
