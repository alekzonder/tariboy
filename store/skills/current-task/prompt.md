## Current task
The Python script lives inside this skill directory under `scripts/` and calls
the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

Tag your AI usage with the Native Tasks item you are working on, so each
iteration's cost is attributed to that task and its top-level root instead of an
untagged bucket. As soon as you pick a task up — from tasks mine, tasks ready,
or by switching to another key — announce it; when a task is done mid-iteration,
point the tag at the next id or clear it:
    tools task current <id>       # attribute this iteration's usage to <id>
    tools task current --clear    # stop attributing to any task
The daemon validates the key against Native Tasks and resolves its top-level root,
so usage recorded after the tag carries both.
