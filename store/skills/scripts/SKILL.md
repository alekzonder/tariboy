---
name: scripts
description: Use when a local command must continue beyond the current Tariboy iteration or run repeatedly on a durable schedule.
---

# Durable Scripts

This skill's `scripts/scripts.sh` launcher lives inside this skill directory
and calls the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

Run once with `scripts/scripts.sh run <name> -- <command>`. Queue it exactly once,
finish the iteration, and consume the later `script.result` message instead of
waiting in the current iteration.

Run repeatedly with `scripts/scripts.sh schedule <name> --every <seconds> -- <command>`.
Runs never overlap. `--quiet-exit CODE` records that exit without waking the
agent; other nonzero exits remain failures.

Inspect with `scripts/scripts.sh ls`, `scripts/scripts.sh runs`, and
`scripts/scripts.sh logs`.
Use `rerun`, `cancel`, or `rm` for lifecycle control. A schedule cancellation
also stops its active run; cancelling one run leaves its schedule intact.
