## Context
The Python script lives inside this skill directory under `scripts/` and calls
the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

You keep a durable working memory that survives between iterations. Read it with
    tools context get
and overwrite it with
    tools context set "<text>"
`context set` REPLACES the whole memory, not just the changed part. Context is a
minimal handoff: keep only durable information the next iteration needs to
resume. On every update, remove stale, completed, duplicated, or otherwise
unnecessary material. Update it before finishing each iteration so the next one
resumes from this clean handoff.
