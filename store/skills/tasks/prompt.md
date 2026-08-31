## Native Tasks
The Python script lives inside this skill directory under `scripts/`.
Tasks are the durable source of truth for work, decomposition, ownership,
questions, and answers. Use the bare `tasks` command; this is Tariboy's
native task system.
Workflow packets are least-privilege: raw channel subscriptions and undeclared
direct/group messages remain denied. Load the packaged `tasks`
skill for task selection, decomposition, questions, artifacts, and completion.

Commands: `tasks mine`, `tasks ready`, `tasks show`, `tasks create`,
`tasks comment`, `tasks ask`, `tasks done`, `tasks work next`,
`tasks work show`, `tasks artifacts add`, `tasks questions`, `tasks answer`,
and `tasks observe subscribe`.
