---
title: tasks
description: Use Native Tasks and leased workflow assignments through the globally installed ttasks client.
sidebar:
  label: Tasks
  icon: list-tree
---

`tariboy-tasks` is the real Native Tasks executable; installers make it
available everywhere as `ttasks`. Native Tasks live in Tariboy's SQLite
database.

## Command and identity boundary

`ttasks --version` reports its build, and `ttasks --json …` requests JSON
output. With a non-empty `TARIBOY_TOOLS_SOCKET`, `ttasks` uses only that
identity-bound agent socket and fails closed when it is unavailable. With no
such variable, it uses the host Unix daemon socket as the customer actor.

The bare `tasks` command is a legacy, capability-controlled compatibility shim:
when the image enables `tasks`, provisioning writes it into the agent bin
directory; removing the capability removes it. New examples use `ttasks`.

Every mutation derives its author or actor from that socket. Request fields
that could forge an author, customer, principal identity, or iteration are
discarded before the task action is dispatched.

## Flexible Native Tasks

Agents can inspect, create, organize, delegate, discuss, and complete visible
work:

```bash
ttasks mine
ttasks ready --queue OPS --claim
ttasks show OPS-12
ttasks create --parent OPS-12 --title "Add regression coverage"
ttasks assign OPS-12 worker
ttasks comment OPS-12 "The failing boundary is confirmed"
ttasks ask OPS-12 user:login "Which behavior should win?"
ttasks update OPS-12 --status in_progress --priority P1
ttasks done OPS-12
```

The same surface includes same-queue move/reorder operations, directed blocking
relations, symmetric related links, and explicit completion despite active
descendants. Access follows task assignment, authorship rules, queue ownership,
group membership, ancestry, and scoped answer requests.

## Workflow-managed Tasks

When a queue has a published workflow, agents do not use the flexible
ready/claim path. They claim leased assignments and operate on least-context
work packets:

```bash
ttasks work next --queue OPS --idempotency-key claim-ops
ttasks work show <assignment-id>
ttasks artifacts add <assignment-id> --name report --type markdown --content "..."
ttasks ask <assignment-id> \
  --question "Which region?" \
  --context "Required to continue" \
  --blocking-scope assignment
ttasks observe subscribe <assignment-id> 'deploy:*' --reaction wake_current
ttasks work complete <assignment-id> --outcome approved
```

The packet declares allowed actions, tools, outcomes, artifacts, and channel
patterns. Mutations require the current revisions and stable idempotency keys.
Direct channel subscriptions are replaced by `ttasks observe`, and undeclared
message or group tools are denied.

## Durable state and notifications

Queues, nested tasks, comments, priorities, dependencies, answer waits,
workflow versions, executions, assignments, leases, artifacts, questions,
holds, observations, and subscriptions are durable daemon state. Assignment,
question, answer, and triage events use a transactional outbox and the normal
messages bus, so notifications survive restarts and can wake enabled agents.

## Prompt integration

```yaml Tariboyfile.yaml
plugins:
  - name: tasks
skills:
  - dir: ../../skills/tasks
```

The packaged image skill teaches both flexible and workflow-managed operation. There is
no general Tasks runtime marker: ordinary work is queried through `ttasks`, and
a managed assignment supplies its work packet through the workflow launch
path.

## Failure behavior

The capability gate returns `plugin_disabled` when inactive. Domain validation
then distinguishes inaccessible tasks, revision conflicts, invalid transitions,
dependency cycles, active descendants, expired leases, undeclared outcomes,
and workflow policy violations. Unknown client flags fail locally with exit `2`
before any request reaches the daemon.

## Related reference

- [Native Tasks](/docs/tasks)
- [Configurable task workflows](/docs/task-workflows)
- [Command reference: Native Tasks](/docs/reference/commands#native-tasks-ttasks-)
- [Agent tools](/docs/binaries/agent-tools)
