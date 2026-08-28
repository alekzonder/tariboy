---
name: scripts
description: Use when a local command must continue beyond the current Tariboy iteration or run repeatedly on a durable schedule.
---

# Durable Scripts

Run once with `tools script run <name> -- <command>`. Queue it exactly once,
finish the iteration, and consume the later `script.result` message instead of
waiting in the current iteration.

Run repeatedly with `tools script schedule <name> --every <seconds> -- <command>`.
Runs never overlap. `--quiet-exit CODE` records that exit without waking the
agent; other nonzero exits remain failures.

Inspect with `tools script ls`, `tools script runs`, and `tools script logs`.
Use `rerun`, `cancel`, or `rm` for lifecycle control. A schedule cancellation
also stops its active run; cancelling one run leaves its schedule intact.
