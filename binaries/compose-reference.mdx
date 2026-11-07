---
title: Compose reference
description: A complete Tariboycompose.yaml, every field by section, the phase order of compose up, and every compose verb.
sidebar:
  label: Compose reference
  icon: list-tree
---

This page is the reference for `tariboy compose`. It gives a complete file,
the fields of each section, the phase order, and the verbs. The
[Compose](/docs/binaries/compose) page explains how each part works. The
[Compose decisions](/docs/binaries/compose-cases) page gives the behavior in
special cases, such as a partial failure or a secret.

## A complete file

This version 2 file uses every section and every field. Every field except
`version`, the source of each Store, workflow and image, `image` of an agent,
and `name` of a queue is optional.

```yaml
version: 2
tariboy: {min: 0.110.0, max: 0.120.0}

stores:
  team: {git: https://example.com/team-store.git, ref: v2}
  local: {path: ./store}

workflows:
  development: {store: team, source: development, tag: latest, version: 1.2.0}
  review: {path: ./workflows/review, tag: 0.3.0}

images:
  developer: {store: team, source: developer, tag: latest, version: 2.1.0}
  tester: {store: local}
  reviewer: {path: ./images/reviewer, tag: stable}

task_queues:
  DEV:
    name: Development
    description: Product development
    owners: [dev-lead]
    responsible_agent: dev-lead
    pools:
      developers: [dev-developer]
      reviewers: [dev-reviewer]
      customers: [dev-lead]
    workflow: development
    triggers:
      - pattern: plugin:telegram
        correlation_key: chat_id
        action: create_task
    env:
      CUSTOMER_AGENT_POOL: customers
      AUTO_MERGE: "false"
    secrets:
      GH_TOKEN: {from_env: DEV_GH_TOKEN}

agents:
  dev-lead:
    image: developer:latest
    group: dev
  dev-developer:
    image: developer:latest
    group: dev
    harness: {type: claude, model: opus, effort: high, interactive: false}
    plugins: [loop, mail, tasks]
    env_mode: merge
    env: {LOG_LEVEL: info}
    secrets:
      API_TOKEN: {from_env: DEV_API_TOKEN}
    cwd: /work/project
    create_cwd: true
    user_prompt: |
      Work only in your branch.
    alias: Dev
    notes: Project developer
    color: "#3366ff"
    messages_batch: 10
    messages_max_queue: 100
    mail_instant_senders: [dev-lead]
    retention: {keep_iterations: 200, keep_days: 30, max_bytes: 1073741824}
    budget: {limit_usd: 10, period: 24h, mode: warn}
    goal: {enabled: true, wait_customer_timeout: 300s, delivery_cooldown: 60s}
    loop:
      enabled: true
      interval: 30s
      timeout: 60m
      hard_timeout: 90m
      ai_stall_timeout: 30m
      on_timeout: restart
      on_error: restart
      max_idle_iterations: 5
      schedule: "RRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=0"
  dev-reviewer:
    image: reviewer:stable
    group: dev

groups:
  dev:
    lead: dev-lead
    budget: {limit_usd: 50, period: 24h, mode: block}
```

The parser is strict: an unknown field in any section is an error.
`scripts/compose-e2e.sh` applies a file of this form to an isolated daemon
(see [Development](/docs/development)).

## Fields by section

The "Phase" column names the phase of `compose up` that applies the field. See
[Phase order](#phase-order).

### Top level

| Field | Type | Default | Phase | Rules |
| --- | --- | --- | --- | --- |
| `version` | `1` or `2` | Required | All | Version 2 adds `tariboy`, `stores`, `workflows` and the image source fields. See [Version 1 and version 2](/docs/binaries/compose#version-1-and-version-2). |
| `tariboy.min`, `tariboy.max` | SemVer | No bound | Before any phase | Inclusive bounds. `min` must not be greater than `max`. See [The tariboy version range](/docs/binaries/compose#the-tariboy-version-range). |

### `stores`

| Field | Type | Default | Phase | Rules |
| --- | --- | --- | --- | --- |
| `git` | URL | None | `stores` | `https://HOST/PATH` only, without a user, a password, a query or a fragment. Exactly one of `git` and `path`. |
| `ref` | Text | The default branch | `stores` | A branch, a tag or a commit. Valid only with `git`. |
| `path` | Path | None | `stores` | A directory. A relative path starts at the directory of the compose file. |

See [Stores](/docs/binaries/compose#stores).

### `workflows`

| Field | Type | Default | Phase | Rules |
| --- | --- | --- | --- | --- |
| `path` | Path | None | `workflows` | A directory with `Workflowfile.yaml`. Exactly one of `path` and `store`. |
| `store` | Name | None | `workflows` | A key of `stores`. |
| `source` | Name | The entry key | `workflows` | The workflow name in the Store. |
| `tag` | `latest` or SemVer | `latest` | `workflows` | The tag that a queue binds. |
| `version` | SemVer | Not checked | `workflows` | The `workflow_version` that the source must give. |

The `name` in `Workflowfile.yaml` must equal the entry key. See
[Builds and versions](/docs/binaries/compose#builds-and-versions).

### `images`

| Field | Type | Default | Phase | Rules |
| --- | --- | --- | --- | --- |
| `path` | Path | None | `images` | A directory with `Tariboyfile.yaml`. Exactly one of `path` and `store`. |
| `store` | Name | None | `images` | A key of `stores`. |
| `source` | Name | The entry key | `images` | The image name in the Store. |
| `tag` | Text | `latest` | `images` | Any image tag except `.` and `..`. |
| `version` | SemVer | Not checked | `images` | The `image_version` that the source must give. |
| `context` | Path | None | `images` | Version 1 only. In version 2, use `path`. |

### `task_queues`

The key is the queue prefix.

| Field | Type | Default | Phase | Rules |
| --- | --- | --- | --- | --- |
| `name` | Text | Required | `queues` | |
| `description` | Text | Keep the live value | `queues` | An empty value keeps the live value. |
| `owners` | Agent names | Keep the live value | `queues` | An owner can be outside the file. |
| `responsible_agent` | Agent name | Keep the live value | `queue bindings` | Must be a key of `agents`. |
| `pools` | Map of pool name to agent names | None | `queue bindings` | Each agent must be a key of `agents`. |
| `workflow` | `NAME`, `NAME:TAG`, `STORE/NAME` or `STORE/NAME:TAG` | No binding | `queue bindings` | In version 2, `NAME` is a key of `workflows`. See [Queue workflows](/docs/binaries/compose#queue-workflows). |
| `triggers[].pattern` | Sender | Required | `queue bindings` | For example `plugin:telegram` or `plugin:*`. |
| `triggers[].correlation_key` | Text | None | `queue bindings` | |
| `triggers[].action` | `create_task` | `create_task` | `queue bindings` | |
| `env` | Map of key to text | None | `queues`, and `queue bindings` for `CUSTOMER_AGENT_POOL` | See the key rules below. |
| `secrets.KEY.from_env` | Variable name | Required | `queues` | The variable of the `compose` process that holds the value. |

An env or secret key holds letters, digits and underscores. It does not start
with a digit or with `TARIBOY_`. A key is env or a secret, not both. See
[Task queues and pools](/docs/binaries/compose#task-queues-and-pools).

### `agents`

The key is the agent name. If the file omits a field, `up` keeps the live
value.

| Field | Type | Default | Phase | Rules |
| --- | --- | --- | --- | --- |
| `image` | `NAME[:TAG]` | Required | `agents` | A bare name means `NAME:latest`. In version 2, `NAME` is a key of `images`. |
| `group` | Group name | No group | `groups` | Must be a key of `groups`. |
| `harness.type` | `claude`, `codex`, `opencode`, `cursor` | The daemon default | `agents`, restart in `start` | |
| `harness.model`, `harness.effort` | Text | Keep the live value | `agents` | An empty value keeps the live value. |
| `harness.interactive` | Boolean | `false` | `agents` | Set only when the file has a `harness` block. |
| `plugins` | Plugin names | The plugins of the image | `agents` | The exact list. Compared as a set. |
| `env` | Map of key to text | None | `agents` | |
| `env_mode` | `merge` or `replace` | `merge` | `agents` | `replace` removes every key that the file does not declare. |
| `secrets.KEY.from_env` | Variable name | Required | `agents` | The key rules of queue secrets. A key cannot be in `env` too. |
| `cwd` | Path | The directory of the compose file | `agents` | `$CWD` and `$HOME` expand. |
| `create_cwd` | Boolean | `false` | `agents` | `true` lets the daemon create `cwd`. |
| `user_prompt` | Text | Keep the live value | `agents` | Compared exactly. |
| `alias`, `notes` | Text | Keep the live value | `agents` | An empty string clears the value. |
| `color` | `#rrggbb` | Keep the live value | `agents` | An empty string clears the value. |
| `messages_batch`, `messages_max_queue` | Whole number, at least 1 | Keep the live value | `agents` | |
| `mail_instant_senders` | Agent names | Keep the live value | `groups` | `[]` refuses every agent. |
| `retention.keep_iterations`, `retention.keep_days`, `retention.max_bytes` | Whole number, at least 0 | Keep the live value | `agents` | 0 means the daemon default. |
| `budget.limit_usd` | Number, in US dollars | 0 | `agents` | |
| `budget.period` | Duration | `24h` | `agents` | |
| `budget.mode` | `warn`, `block` or `enforce` | `warn` | `agents` | `enforce` is the same as `block`. |
| `goal.enabled` | Boolean | Keep the live value | `agents` | |
| `goal.wait_customer_timeout`, `goal.delivery_cooldown` | Duration in whole seconds | Keep the live value | `agents` | |
| `loop.enabled` | Boolean | `true` | `agents`, start in `start` | `false` stops the agent. |
| `loop.interval`, `loop.timeout` | Duration | Keep the live value | `agents` | |
| `timeout` | Duration | Keep the live value | `agents` | The older form of `loop.timeout`. If both are set, `loop.timeout` is used. |
| `loop.hard_timeout` | Duration, at least `0s` | Keep the live value | `agents` | |
| `loop.ai_stall_timeout` | Duration in whole seconds, at least `1s` | Keep the live value | `agents` | |
| `loop.on_timeout`, `loop.on_error` | `restart` or `stop` | Keep the live value | `agents` | |
| `loop.max_idle_iterations` | Whole number, at least 0 | Keep the live value | `agents` | 0 turns the idle stop off. |
| `loop.schedule` | RFC 5545 `RRULE`, optional `DTSTART` | Keep the live value | `agents` | An empty string clears the schedule. |

See [Agent settings](/docs/binaries/compose#agent-settings) for the daemon
call of each field, and
[Agent settings outside compose](/docs/binaries/compose#agent-settings-outside-compose)
for the agent commands that have no field.

### `groups`

The key is the group name. The members of a group are the agents whose `group`
names it.

| Field | Type | Default | Phase | Rules |
| --- | --- | --- | --- | --- |
| `lead` | Agent name | Keep the live lead | `groups` | Must be a member of the group in the file. |
| `budget` | `limit_usd`, `period`, `mode` | None | `groups` | The rules of an agent `budget`. |

## Phase order

`compose up` applies the file in this order. A phase starts after the phases
before it end.

1. `stores`: registers, checks out and refreshes each Store.
2. `workflows`: builds each workflow.
3. `images`: builds each image.
4. `queues`: creates each task queue and sets its fields, env and secrets.
5. `agents`: creates or updates each agent.
6. `groups`: creates each group, assigns its members, sets its lead and its
   budget.
7. `queue bindings`: binds the pools, `CUSTOMER_AGENT_POOL`, the responsible
   agent, the workflow and the triggers of each queue.
8. `start`: starts or restarts each agent that the file enables.

The pools, the workflow binding and the triggers of a queue need the agents,
so they come after the `agents` phase. See
[How `up` converges](/docs/binaries/compose#how-up-converges).

## Verbs

Each verb reads `-f FILE`, else `Tariboycompose.yaml`, else
`tariboy-compose.yaml` in the current directory. "Checks `tariboy`" means that
the verb stops before any change when the daemon is outside the range of the
file.

| Verb | Flags | Calls the daemon | Changes state | Checks `tariboy` |
| --- | --- | --- | --- | --- |
| `validate` | `-f`, `--daemon`, `--archive FILE` | Only with `--daemon`, read-only | No | Against the CLI, and with `--daemon` against the daemon |
| `up` | `-f`, `--dry-run`, `--no-start` | Yes. Starts the daemon unless `--dry-run`, `--no-start` or `TARIBOY_NO_AUTOSTART` | Yes. `--dry-run` changes nothing | Yes |
| `build` | `-f` | Yes | Stores, workflows and images only | Yes |
| `status` | `-f` | Yes, read-only | No | Yes |
| `ps` | `-f` | Yes, read-only | No | Yes |
| `logs` | `-f`, `--tail N`, member names | Yes, read-only | No | Yes |
| `start`, `stop`, `restart`, `kill` | `-f`, member names | Yes | The loop of the agents | Yes |
| `exec` | `-f`, member, prompt | Yes | Runs one iteration | Yes |
| `rm` | `-f`, member names | Yes | Removes stopped agents | Yes |
| `down` | `-f`, `-v`/`--volumes` | Yes | Removes the agents and groups of the file | Yes |
| `pack` | `-f`, `-o FILE`, `--no-build` | Yes. Does not start it | Stores, workflows and images, unless `--no-build` | Yes |
| `unpack` | `FILE`, `--dry-run`, `--yes` | Yes. Does not start it | Yes. `--dry-run` changes nothing | Yes, and the daemon must not be older than the archive producer |
| `archive` | As `pack` | As `pack` | As `pack` | Deprecated alias of `pack` |
| `import` | `--archive FILE` | As `unpack` | As `unpack` | Deprecated alias of `unpack` |

The command table in [Commands](/docs/reference/commands) lists the same verbs
with their help text.
