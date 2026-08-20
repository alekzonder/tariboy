---
title: Plugins
description: Understand Tariboy's built-in capabilities and instructions and its separately installed external plugin processes.
sidebar:
  label: Overview
  icon: plug
---

Tariboy uses the word **plugin** for two related but operationally different
extension mechanisms. An image may name either kind, but their installation,
execution, and prompt behavior are not the same.

<CardGroup cols={2}>
  <Card title="Built-in plugins" href="/docs/plugins/built-in" icon="package-check">
    Daemon-owned capabilities such as context, messages, scripts, and Native
    Tasks, plus instruction-only plugins such as workdir. They do not run as
    separate plugin processes.
  </Card>
  <Card title="External plugins" href="#external-plugins" icon="unplug">
    Explicitly installed bundles that the daemon supervises as subprocesses.
  </Card>
</CardGroup>

## Choose the right model

| Property | Built-in plugin | External plugin |
| --- | --- | --- |
| Implementation | Tariboy daemon, agent API, bundled shim, or prompt instruction | Separately distributed executable |
| Installation | Ships with Tariboy | `tariboy plugin install <directory>` |
| Runtime process | No additional plugin process | One supervised process per active plugin name |
| Image declaration | `plugins: [{name: context}]` | The installed plugin name |
| Version selection | Tariboy release | Active installed version; images select by name |
| Prompt injection in schema v2 | Never implicit | Never implicit |
| Agent Skill packaging | Only when explicitly listed under image `skills` | Only when an explicit image skill directory resolves into the plugin tree |

Schema-v2 images receive exactly the plugin names they declare. Most built-ins
enable a capability; `workdir` is instruction-only. Neither class inserts
instructions into the prompt. List every static instruction and runtime marker
under the image's ordered `prompts` template. See [Images](/docs/images) for the
complete manifest contract.

Prompt assets and Agent Skills are separate declarations. A plugin directory
such as `$PLUGINS/jira/2.5.0/skills/triage` becomes a packaged Agent Skill only
when an image lists it under `skills: [{dir: ...}]`; placing content under a
plugin's `store/skills` does not opt it in automatically. At runtime Tariboy
uses a generated local plugin for Claude Code, a bounded prompt catalog for
Codex CLI, and an isolated config overlay for OpenCode. These integrations are
additive to each harness's normal global and CWD discovery. Native duplicate
precedence remains harness-owned; the Codex image entry points to its exact
absolute `SKILL.md` path rather than replacing a native entry.

## External plugins

An external plugin is a subprocess the daemon spawns and supervises. It binds
an AF_UNIX socket, prints a one-line JSON handshake, then serves `/health` and
its type-specific HTTP endpoints.

### Plugin types

Types are defined in `internal/plugins`:

- **`channel-source`** publishes inbound events through
  `POST /api/plugin/publish`.
- **`channel-sink`** receives subscribed bus messages at `POST /deliver`, with
  durable redelivery and a dead-letter queue.
- **`eval`** receives iteration checks at `POST /evaluate`.
- **`tool`** advertises an agent-facing capability out of band.
- **`harness`** is recognized, but its runtime wiring is deferred.

### Providers and parameterized channels

A provider plugin offers a channel that an agent subscribes to with parameters,
such as a ticket identifier, query, or tick interval. The daemon validates the
parameters against the channel's `params_schema`, fingerprints them into a
**watch**, and drives the plugin through `/watches` push or
`GET /api/plugin/watches` pull. The plugin therefore produces only work that
has an active consumer.

Configurable task workflows can consume provider events without giving the
plugin authority over the task state machine. A committed bus message becomes
an idempotent task observation; only a reaction declared by the pinned workflow
may wake, hold, or create work. See [Configurable task
workflows](/docs/task-workflows#channels-triggers-subscriptions-and-observations).

### Installation and versioning

Install an external bundle from a daemon-accessible directory:

```bash
tariboy plugin install ./my-plugin
tariboy plugin ls
tariboy plugin inspect my-plugin
```

External plugins are optional and are not compiled into Tariboy. Immutable
versions live side by side under `$PLUGINS/<name>/<version>`. Exactly one
version is active and one process is supervised per plugin name. Reinstalling
identical bytes is idempotent; different bytes at an existing name/version are
rejected.

An image selects an external capability by name, not by installed version.
Activating another installed version therefore changes what future launches of
that named plugin run without changing the image manifest. Keep any static
guidance explicit in the image template, normally with a `$PLUGINS/...` prompt
path.

## Related reference

- [Built-in plugins](/docs/plugins/built-in) lists every daemon-owned
  capability and its command surface.
- [Agent tools](/docs/binaries/agent-tools) explains the `tools` and `tasks`
  shims used inside an agent.
- [Channels](/docs/reference/channels) defines delivery, subscriptions,
  provider watches, schedules, and script results.
- [Command reference](/docs/reference/commands) lists operator and agent
  commands.
