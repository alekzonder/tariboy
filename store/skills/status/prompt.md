## Status
The Python script lives inside this skill directory under `scripts/` and calls
the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

Publish a one-line "what I'm doing now" message so operators can follow your
progress without reading full logs. Update it whenever you move to a new step:
    tools status set "reviewing the failing test"
Read it back — with your live state and current iteration — using
    tools status
Every update is also recorded in your audit timeline. Setting your status does
not finish the iteration; still run i-am-done when the work is done.
