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
| `improvement` | `ls`, `inspect`, `plan approve` / `reject` |
| `image-release` | `inspect`, `rollout approve` / `reject` / `stage`, `rollback` |
| `agent` | `run`, `ps`, `inspect`, `start`, `stop`, `restart`, `kill`, `rm`, `exec`, `screen`, `send-keys`, `status show`/`history`, `cp` / `push` / `pull` |
| `loop` | `enable`, `disable`, `interval`, `timeout`, `hard-timeout`, `on-timeout`, `on-error` |
| `channel` / `message` | `channel ls`/`inspect`/`tail`, `message send` |
| `group` | `create`, `assign`, `inspect`, `ls`, `rm` |
| `plugin` | `install`, `ls`, `inspect`, `logs`, `rm` |
| `telegram` | contributed by the running bundled plugin: `configure`, `chat setup`, `status` |
| `schedule` | `ls` |
| `tasks queue create` | Create a native task queue with owners and an optional responsible agent |
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

Plugin command groups are declarative and namespaced. The CLI merges validated
contributions from the selected running daemon; core help remains usable when
that daemon is unavailable, while contributed commands require it. Secret
arguments are accepted only through an owner-only file or stdin, never as a
token value in argv. See the [Telegram setup workflow](/docs/plugins#bundled-telegram-plugin).

Improvement and image-release decisions require the exact revision or release
hash shown by `inspect`; an approval for older content cannot authorize changed
content. See the [controlled improvement workflow](/docs/images-and-groups/llm-judge#controlled-improvement-workflow).

Workflow definition, queue binding, pool, trigger, and execution inspection are
operator REST routes rather than hand-written CLI verbs. Compose is the normal
declarative client and the generated OpenAPI describes the raw API. See
[Configurable task workflows](/docs/task-workflows#rest-api).

Agent create and update accept `--goal-enabled` and
`--goal-wait-customer-timeout-s`; their defaults are enabled and 300 seconds.
Inspect and list output also reports the daemon-selected read-only
`current_goal_task_key`.
