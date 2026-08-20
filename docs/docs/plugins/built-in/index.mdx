---
title: Built-in plugins
description: Reference for daemon-owned capabilities and instruction plugins shipped with Tariboy.
sidebar:
  label: All built-in plugins
  icon: list
---

Built-in plugins are implemented by Tariboy itself. Capability plugins gate
agent API routes, commands, or shims. Instruction-only plugins contribute no
tool surface and exist so a schema-v2 image can declare related static and
runtime prompt entries explicitly. Neither kind starts a supervised subprocess.

## Complete list

| Plugin | Class | In `basic:latest` | Agent surface | Durable state |
| --- | --- | :---: | --- | --- |
| [`whoami`](/docs/plugins/built-in/whoami) | Historical core | Yes | `tools whoami` | None |
| [`loop`](/docs/plugins/built-in/loop) | Historical core | Yes | `i-am-done`, `tools loop …` | Iteration and loop state |
| [`messages`](/docs/plugins/built-in/messages) | Historical core | Yes | `tools message …`, `request`, channels | Bus messages, deliveries, subscriptions |
| [`context`](/docs/plugins/built-in/context) | Optional | Yes | `tools context get/set` | Per-agent `CONTEXT.md` |
| [`status`](/docs/plugins/built-in/status) | Optional | Yes | `tools status [set]` | Agent row and audit timeline |
| [`workdir`](/docs/plugins/built-in/workdir) | Instruction-only | Yes | Prompt only | None |
| [`schedule`](/docs/plugins/built-in/schedule) | Optional | No | `tools schedule …` | Schedules and resulting bus messages |
| [`scripts`](/docs/plugins/built-in/scripts) | Optional | Yes | `tools script …` | Script records and logs |
| [`image-creator`](/docs/plugins/built-in/image-creator) | Optional | No | `tools image build` | Built image in the host image store |
| [`current-task`](/docs/plugins/built-in/current-task) | Optional | Yes | `tools task current` | Current iteration usage attribution |
| [`llm-as-judge`](/docs/plugins/built-in/llm-as-judge) | Optional | No | `tools judge …` | Judge runs, evidence, analyses, summaries |
| [`tasks`](/docs/plugins/built-in/tasks) | Optional | Yes | Bare `tasks` command | Native Tasks and workflow state in SQLite |

“Historical core” describes schema-v1 resolution, where `whoami`, `loop`, and
`messages` were added automatically. It does **not** mean schema-v2 images
receive them automatically.

## Enable a capability

Schema v2 accepts an explicit, ordered plugin list:

```yaml Tariboyfile.yaml
schema_version: 2
plugins:
  - name: whoami
  - name: loop
  - name: messages
  - name: context
  - name: workdir
prompts:
  - file: $CURRENT_VERSION_STORE/skills/whoami/prompt.md
  - runtime: identity
  - file: $CURRENT_VERSION_STORE/skills/messages/prompt.md
  - runtime: messages
  - file: $CURRENT_VERSION_STORE/skills/context/prompt.md
  - runtime: context
  - file: $CURRENT_VERSION_STORE/skills/workdir/prompt.md
  - runtime: workdir
  - runtime: user-prompt
  - file: $CURRENT_VERSION_STORE/skills/loop/finish.md
```

Plugin order is preserved, duplicate names are rejected, and an unknown name is
accepted only when an installed external plugin manifest resolves it. Static
prompt files and runtime placeholders remain separate from the plugin list.

:::warning[Plugins do not inject schema-v2 prompts]
Declaring `context` makes `tools context get/set` available. It does not add the
context instructions or the live context text to the prompt. Include the Store
prompt and `runtime: context` explicitly when the agent should receive both.
The same transparency rule applies to instruction-only `workdir`.
:::

## The default image

The daemon-managed `basic:latest` image declares nine built-ins:

```text
whoami, loop, messages, context, status, workdir, scripts, current-task, tasks
```

It deliberately excludes `schedule`, `image-creator`, and
`llm-as-judge`. New agents use the current managed generation. Existing agents
remain pinned to their assigned image digest until an image change is activated
for a future iteration.

## Capability enforcement

The per-agent API checks the agent's currently active plugin set on each gated
request. A missing capability returns `404 plugin_disabled`; it does not invoke
the underlying operation. Image activation rewrites image-owned shims before
promoting the new plugin set, so the bare `tasks` and `i-am-done` commands match
the active image.

Managed task workflows add a second authorization layer. They may deny direct
message and group tools, always replace raw channel subscriptions with
assignment-scoped `tasks observe`, and allow scheduled channel publishing only
when the work packet grants it.

## Prompt and state ownership

The canonical built-in prompt fragments live under the versioned Store. The
schema-v2 image chooses their exact position, alongside runtime values such as
identity, messages, context, and the user prompt. Changing an image does not
delete daemon-owned agent data such as context, audit, messages, schedules,
scripts, Tasks, or judge artifacts.

For the image schema and runtime marker list, see [Images](/docs/images). For
external process plugins, see [Plugins](/docs/plugins#external-plugins).
