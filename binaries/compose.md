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
purge path. `down`, `stop` and `kill` skip a member that is absent on the
daemon, so a partly present project still cleans up; any other error still
fails the command. `start` and `restart` of an absent member fail.

Compose owns agent runtime choices: harness, model, effort, interactive and loop
settings, CWD, and environment. An agent is interactive only when its
`harness.interactive` is `true`; compose always sends the value explicitly, so
the create endpoint's interactive default does not apply. `loop.enabled`
(default `true`) is the agent's on/off switch: `compose up` starts an agent
whose value is `true` and stops one whose value is `false`. Schema-v2 image manifests supply none of those
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

## How `up` converges

`compose up` builds the declared images and reads the live state once: the
groups, agents, budgets, task queues and their pools, the queue workflow
bindings, and the pending image of each running agent whose image differs from
the file. It then plans the actions that move the daemon toward the file and
applies them in this order: groups, agents, budgets, task queues, and queue
workflows. A setting that already matches makes no call, so running `up` again
on a converged file changes nothing and reports every resource `ok`.

An existing agent converges in place:

- **Image.** A different `image` is set as the agent's pending image
  (`POST /api/agents/{name}/image`). The agent keeps running and adopts the
  image on its next iteration; there is no restart. A bare name means
  `NAME:latest`.
- **Harness.** A different `harness.type` is stored
  (`POST /api/agents/{name}/harness`), and a running agent is then stopped and
  started so the new harness takes effect. The restart runs after the agent's
  other changes, so the restarted iteration also uses the new image, CWD, loop,
  and Goal settings. The restart interrupts the current iteration, as a manual
  restart does. A stopped agent is re-provisioned
  instead (see below), which starts it with the new harness. An agent whose
  file sets `loop.enabled: false` is not restarted; its next start uses the new
  harness.
- **Stopped agent.** An agent left stopped by a data-preserving `down` is
  re-provisioned in place with the file's image, keeping its context,
  iterations, and audit log, and then converges as above.
- Group membership, CWD, timeout, loop settings, and the Goal settings the file
  sets converge to the file's values.
- **Environment and interactive mode.** When a key in the file's `env` is
  missing from the agent or has another value, `up` sets the agent env to its
  current env merged with the file's keys. Keys the file does not declare, such
  as env from the image manifest, are kept; removing a key from the file does
  not remove it from the agent (use `tariboy agent env NAME` to replace the
  whole env). When the file has a `harness` block, `up` sets the interactive
  mode to `harness.interactive`; without a `harness` block it keeps the
  agent's mode. Neither change restarts the agent: an iteration that is already
  running keeps its old values, and the next iteration uses the new ones.

Model, effort, and plugins are applied only when compose creates an agent; `up`
does not change them on an existing agent.

A failed action does not stop the run. The actions that depend on it are
skipped: for example, a failed agent create skips that agent's start and its
budget, and a failed harness change skips the restart. Independent actions,
including the other agents, still run. When the run ends, `up` prints one line
per resource with its outcome:

```text
changed  group:research-team
failed   agent:writer: create agent writer: ...
skipped  budget:agent:writer: an action it depends on failed
ok       agent:scout
up: 1 ok, 1 changed, 1 failed, 1 skipped
```

| Outcome | Meaning |
| --- | --- |
| `ok` | Already matched the file; nothing was done. |
| `changed` | Every planned action succeeded. |
| `failed` | An action failed; the line shows the first error. |
| `skipped` | An action it depends on failed, so it was not attempted. |

If any resource failed, `up` exits non-zero and prints a one-line summary such
as `compose up: 1 resource failed` to standard error; the report above already
shows each error. The resources that succeeded stay
applied, so the daemon can be partly converged; fix the cause and run `up`
again to converge the rest.

`compose status` reads the same plan without applying it. It prints one line
per group, agent, budget, task queue, and pool, then `drift: N`. An agent's line
lists each setting `up` would change; an image that is already pending shows as
`image pending` until the agent adopts it. Environment drift lists the key
names only, never their values. Model and plugins are not checked, and status
says so on its last line.

## Task queues and pools

Compose can declare task queues and the explicit agent pools of each queue.

```yaml
version: 1

task_queues:
  DEV:
    name: Development
    pools:
      developers: [dev-ng-developer]
      reviewers: [dev-ng-reviewer]
```

The file is parsed strictly: a misspelled or unknown field fails instead of
being ignored. Queue prefixes and pool names use URL-safe ASCII identifiers
(`A-Z`, `a-z`, digits, `.`, `_`, and `-`; the first character must be
alphanumeric). A pool is an explicit list of compose agents, normalized by agent
name, and independent of `groups`: putting an agent in a group does not put it in
a pool, and a pool does not alter group membership.

An agent no longer takes a `subscribe:` key: the mailbox replaced channel
subscriptions, so the key fails as an unknown field. Remove it from older
files; group mail and task notifications need no declaration.

The top-level `workflows:` map belonged to the removed task workflow engine. A
non-empty value is rejected with an error that names the key. An empty
`workflows: {}`, as written by older team exports, is accepted and ignored.

`compose up` first converges images, agents, and groups. It then reconciles the
queue and its pools through the daemon's Native Tasks API, and last binds queue
workflows. Repeating `compose up` is idempotent; it also restores changed queue
names and pool membership. `compose status` remains read-only and reports queue,
pool membership, and workflow drift alongside agent, group, and budget drift.
Existing compose files without `task_queues` keep their previous behavior.

## Queue workflows

A queue may name the [workflow image](/docs/workflow-images) it follows:

```yaml
task_queues:
  DEV:
    name: Development
    workflow: team/development:0.1.0
    pools:
      developers: [dev-a, dev-b]
```

`workflow` takes one of four forms:

| Value | `compose up` |
| --- | --- |
| `NAME` | Binds the already built `NAME:latest`. The queue follows `latest`. |
| `NAME:TAG` | Binds the already built `NAME:TAG`. The queue follows the tag. |
| `STORE/NAME` | Builds the workflow source `NAME` from the registered Store `STORE`, then binds the digest that build produced. The binding is pinned. |
| `STORE/NAME:TAG` | Builds `STORE/NAME`, then binds `NAME:TAG`. The queue follows the tag. |

A tag of `.` or `..` is a validation error.

Each Store workflow is built once per run, before any queue is bound, and a
bind runs after the queue's pools exist, since a bind needs the pools the
workflow names. A queue already bound to the resolved digest and tag is left
alone. A queue that follows a tag moves to each new build of that tag; see
[Binding a queue](/docs/task-workflows#binding-a-queue). A
form without a Store prefix fails when the image is not built. Rebinding a
queue to a newer version leaves its existing tasks on the version they pinned.

Compose never clears a binding: removing `workflow` from the file leaves the
queue bound; unbind with `ttasks queue workflow clear`. Compose never handles a
secret value. When the workflow lists `requires_secrets` that the queue does
not have, the bind fails with `workflow_secret_missing`, and the error names the
secrets and the command to set one:

```bash
printf '%s' "$GH_TOKEN" | ttasks queue secret set DEV GH_TOKEN
```

`compose status` never builds. A `STORE/NAME` workflow is compared with the
`workflow_version` of its Store source, as the daemon reads it from the Store,
and reported as `(Store source)`; when the Store or the source cannot be read,
and for every other form, status resolves the declared workflow through
`NAME:TAG`, `latest` when no tag is given. It reports a queue whose binding is
missing, differs, or names an image that is not built yet. A bound queue whose
file declares no workflow is listed as not declared. When the daemon fails to
return a queue's binding, that queue shows `workflow UNKNOWN (<error code>)` and
status goes on with the next one. See [Task workflows](/docs/task-workflows).
