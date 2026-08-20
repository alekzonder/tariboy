---
title: Binaries & commands
description: The five core binaries built by make build.
sidebar:
  label: Overview
  icon: terminal
---

Five core binaries are built by `make build`:

| Binary | Role |
| --- | --- |
| `tariboyd` | The [daemon](/docs/architecture) — long-running, owns all durable state. |
| `tariboy` | The [operator CLI](/docs/binaries/operator-cli) — human/CI client over the socket. |
| `tariboy-shim` | Runs [one harness iteration under a watchdog](/docs/architecture/shim). |
| `tariboy-tools` | The [agent-facing `tools`](/docs/binaries/agent-tools) shim, run *inside* an agent. |
| `tariboy-store` | A standalone [image registry](/docs/binaries/store) server. |

Optional external plugins are built and distributed independently. The daemon
starts only plugins already installed in its configured data directory.

`make build` also assembles the canonical
`internal/builtinimages/source` image into an ignored bundle embedded in
`tariboyd`. On activation the daemon atomically
installs or refreshes the reserved `basic:latest` ref, so every binary-based
installation path receives the same default image.

## Client/daemon version drift

`tariboy` and `tariboy-tools` share one socket client, and it reads the
`X-Tariboy-Version` header the daemon stamps on
[every response](/docs/architecture#version-reporting). When that version differs
from the client's own build, the client prints a warning naming both versions and
its own executable path — once per process, strictly on **stderr**. It does not
change stdout or the exit code, so output parsing is unaffected. A daemon old
enough to send no header produces no warning.

This matters because an agent's `tools` / `tasks` / `i-am-done` shims are written
when the agent is created and stay pinned to that build: without the warning, a
client too old to know a newer flag looks like it simply did nothing.
`tools whoami` prints both `client_version` and `daemon_version` for exactly this
reason — it is the first command to run when the tools behave strangely.

The authoritative operator command list is generated from the binary
(`tariboy --help-json`) and documented in the
[command reference](/docs/reference/commands).

Versioned Native Tasks workflows are configured through
[`tariboy compose`](/docs/binaries/compose) or operator REST; agents execute
them with the identity-bound `tasks` shim. See
[Configurable task workflows](/docs/task-workflows).

## The three command surfaces

<CardGroup>
  <Card title="Operator commands" href="/docs/binaries/operator-cli" icon="user-cog">
    `tariboy <group> <command>` — run by a human or CI against the daemon.
  </Card>
  <Card title="Agent tools" href="/docs/binaries/agent-tools" icon="wrench">
    `tools <group> <command>` — run *inside* an agent, over its per-agent socket.
  </Card>
  <Card title="Native Tasks" href="/docs/tasks" icon="list-tree">
    `tasks <verb>` — optional identity-bound task workflow inside an agent.
  </Card>
</CardGroup>
