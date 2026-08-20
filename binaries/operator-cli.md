---
title: tariboy (operator CLI)
description: The human/CI client that talks to the daemon over its socket, grouped by noun.
sidebar:
  label: Operator CLI
  icon: user-cog
---

The human/CI client (`cmd/tariboy`) talks to the daemon over its socket.
Commands are grouped by noun. The full, authoritative list — generated from the
binary — is in the [command reference](/docs/reference/commands).

`daemon` subcommands (`start` / `stop` / `restart` / `status` / `logs`) work even
when the daemon is down.

| Group | What it does |
| --- | --- |
| `version` | print the canonical Tariboy version locally, without a daemon |
| `daemon` | `start` / `stop` / `restart` / `status` / `logs`, `config get`/`set`, `reindex` |
| `image` | `build`, `ls`, `inspect`, `prompt`, `rm` |
| `agent` | `run`, `ps`, `inspect`, `start`, `stop`, `restart`, `kill`, `rm`, `exec`, `screen`, `send-keys`, `status show`/`history`, `cp` / `push` / `pull` |
| `loop` | `enable`, `disable`, `interval`, `timeout`, `hard-timeout`, `on-timeout`, `on-error` |
| `channel` / `message` | `channel ls`/`inspect`/`tail`, `message send` |
| `group` | `create`, `assign`, `inspect`, `ls`, `rm` |
| `plugin` | `install`, `ls`, `inspect`, `logs`, `rm` |
| `schedule` | `ls` |
| `eval` | `ls`, `inspect` |
| `secret` | `set`, `ls`, `rm` |
| `rule` | `set`, `ls`, `rm` (proxy policy: rate-limit / model-policy) |
| `budget` | `set`, `ls`, `status` |
| `usage` / `logs` / `iteration` | AI usage/cost, event stream, iteration inspect/logs/ls |
| `retention` / `prune` | retention policy, prune old iterations |
| `backup` / `restore` | portable per-agent `tar.gz` |
| `user-prompt` | get/set the agent's standing user prompt |
| registry: `push` / `pull` / `login` | interact with a [`tariboy-store`](/docs/binaries/store) registry |

See the [full command reference](/docs/reference/commands#operator-commands) for
every command and a one-line summary of each.

`tariboy version` and `tariboy --version` are equivalent plain-text
forms. Both work without a running daemon.

Workflow definition, queue binding, pool, trigger, and execution inspection are
operator REST routes rather than hand-written CLI verbs. Compose is the normal
declarative client and the generated OpenAPI describes the raw API. See
[Configurable task workflows](/docs/task-workflows#rest-api).
