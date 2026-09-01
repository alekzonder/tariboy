---
name: workdir
description: Use when an agent needs the configured Tariboy workdir path for a command or file outside its current directory.
---

# Workdir

The `workdir:` path in the current prompt is available for scripts and files
outside the current directory. The sibling Scripts skill's
`../scripts/scripts/scripts.sh` launcher lives inside that skill directory;
pass an absolute path from `workdir:` after `--` to its `run` or `schedule`
command.
