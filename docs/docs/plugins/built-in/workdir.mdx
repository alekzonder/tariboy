---
title: workdir
description: Expose an agent's absolute managed workdir as an explicit schema-v2 prompt instruction.
sidebar:
  label: workdir
  icon: folder
---

`workdir` is an instruction-only built-in plugin. It gives an agent the
absolute path of its managed `agents/<agent>/workdir` even when the harness runs
in a different configured CWD. It adds no command, API route, shim, environment
variable, or filesystem permission.

## Prompt composition

Declare the plugin, its static Store instruction, and its runtime value
explicitly:

```yaml Tariboyfile.yaml
schema_version: 2
plugins:
  - name: workdir
prompts:
  - file: $CURRENT_VERSION_STORE/skills/workdir/prompt.md
  - runtime: workdir
```

The declarations are independent. Naming the plugin does not inject either
prompt entry, and including the entries does not infer the plugin. The image
therefore retains its exact declared prompt order.

At iteration preparation, the runtime entry renders:

```text
workdir: /home/alice/.tariboy/agents/worker/workdir
```

This is always the managed agent workdir, not the effective CWD. The two paths
are equal when an agent uses its default CWD and differ when it works in an
external repository.

## Use the path with scripts

Files in the managed workdir are available across ordinary iterations and
image activation. To schedule a script while working in another CWD, pass its
absolute path to the [`scripts` plugin](/docs/plugins/built-in/scripts):

```bash
tools script schedule poll \
  --description "Poll the queue" \
  --every 60 \
  -- /home/alice/.tariboy/agents/worker/workdir/scripts/poll-queue
```

The managed workdir is part of the rebuildable agent tree. Removing an agent
clears it before a later reprovision, and purging the agent removes it. Keep
project source and irreplaceable artifacts in storage whose lifecycle you
control.

## Security boundary

The prompt exposes only the managed workdir path, not the parent agent root
that contains images, iteration evidence, and daemon-owned files. The harness
already runs as the daemon account, so rendering this path grants no additional
access or sandboxing.

`basic:latest` declares `workdir` for newly created agents. Existing agents
remain pinned to their current image digest until an operator selects another
image.

## Related reference

- [Images: runtime placeholders](/docs/images#runtime-placeholders)
- [Agents and the iteration loop](/docs/architecture/iteration-loop)
- [scripts](/docs/plugins/built-in/scripts)
