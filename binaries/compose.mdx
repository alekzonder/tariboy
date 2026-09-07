---
title: The compose workflow
description: Bring up a whole set of images, groups, and agents declaratively from a tariboy-compose.yaml.
sidebar:
  label: Compose
  icon: boxes
---

`tariboy compose` (`internal/compose`) brings up a whole set of images,
groups, and agents declaratively from a `tariboy-compose.yaml`. The daemon
auto-starts if it is down.

```bash
tariboy compose up          # build images, create groups/agents, start loops
tariboy compose ps          # what compose manages
tariboy compose logs        # tail agent logs
tariboy compose down            # stop + remove agents (data preserved)
tariboy compose down --volumes  # also drop the shared/agent data
```

Verbs: `up`, `down`, `build`, `ps`, `status`, `start`/`stop`/`restart`/`kill`,
`rm`, `exec`, `logs`. Flags include `-f/--file`, `-v/--volumes`, `--no-start`,
`--tail`. `compose down` then `up` preserves data by default; `--volumes` is the
purge path.

Compose owns agent runtime choices: harness, model, effort, interactive and loop
settings, CWD, and environment. Schema-v2 image manifests supply none of those
defaults; they contain only plugins and the ordered prompt template. Compose
may build an image from an original relative context, but portable runnable
image artifacts do not replace that source directory for later rebuilds.

An agent may optionally override its per-agent Goal settings; omitted values
keep the daemon default or current value:

```yaml
agents:
  worker:
    goal:
      enabled: true
      wait_customer_timeout: 300s
```

`wait_customer_timeout` must be a positive whole-second duration.

## Task workflows and queues

Compose can publish a versioned Native Tasks workflow and bind it to a queue.
Workflow files are portable YAML definitions; a relative `source` is resolved
from the directory containing `tariboy-compose.yaml`, just like image
contexts.

```yaml
version: 1

workflows:
  development:
    source: ./workflow.yaml

task_queues:
  DEV:
    name: Development
    workflow: development
    pools:
      managers: [dev-ng-manager]
      developers: [dev-ng-developer]
      reviewers: [dev-ng-reviewer]
      qa: [dev-ng-qa]
```

In this example `workflow.yaml` owns `name: development` and `version: 2`.
The complete runnable `development@2` source and its lifecycle are documented
in [Configurable task workflows](/docs/task-workflows#one-complete-development2-example).

The workflow map key is a compose-local reference. The workflow file itself
owns its durable `name` and integer `version`. Changing a published definition
therefore requires incrementing that version; published versions are immutable.
The queue refers to the local workflow key and inherits that workflow when a
task is created in the queue.

Compose and workflow sources are parsed strictly: misspelled or unknown fields
fail with the source path instead of being ignored. Workflow names, compose
workflow keys, and pool names use URL-safe ASCII identifiers (`A-Z`, `a-z`,
digits, `.`, `_`, and `-`; the first character must be alphanumeric).

Every pool used by a workflow requirement (including the pool that receives
questions) must be declared with at least one compose agent. Pool membership is
an explicit list (normalized by agent name) and is independent of `groups`:
putting an agent in a group does not put it in a workflow pool, and a pool does
not alter group membership.

`compose up` first converges images, agents, and groups. It then validates,
publishes, and reconciles workflow state through the daemon's Native Tasks API:
queue, pools, and finally the active workflow binding. Pools are installed
before activation because the daemon rejects a workflow whose required pool is
empty. Repeating `compose up` is idempotent; it also restores changed queue
names and pool membership. If the source content differs semantically from an
already-published workflow with the same name and version, reconciliation stops
and asks for a version bump. Whitespace normalized by the workflow model does
not count as a semantic change.

`compose status` remains read-only and reports active workflow-version and pool
membership drift alongside agent, group, and budget drift. Existing compose
files without `workflows` or `task_queues` keep their previous behavior.
Commands that do not use desired workflow state (`build`, `down`, `start`,
`stop`, `restart`, `kill`, `rm`, `exec`, `logs`, and `ps`) do not open workflow
source files. They therefore remain usable to build images, inspect, stop, or
remove agents even when a referenced workflow file has moved or disappeared.

Task creation never accepts the compose workflow key or durable workflow name:
it names only `DEV`, and the daemon pins that queue's active published version.
