---
name: workdir
description: Use when an agent needs the configured Tariboy workdir path for a command or file outside its current directory.
---

# Workdir

The `workdir: /absolute/path` line in the current prompt is the sole source of
the configured path. Do not infer it from the current directory, Git, or an
environment variable. If the line is absent, request the path and stop.

Resolve workdir-relative commands to absolute paths. For durable commands, use
the sibling Scripts launcher:

```bash
../scripts/scripts/scripts.sh run <name> -- <absolute-command>
../scripts/scripts/scripts.sh schedule <name> --every <seconds> -- <absolute-command>
```

Execute the launcher when command execution is available. Otherwise, return the
exact command instead; never claim it was queued or scheduled.
