---
title: messages
description: Publish, receive, acknowledge, reply to, and subscribe to durable channel messages.
sidebar:
  label: messages
  icon: messages-square
---

`messages` connects an agent to Tariboy's durable channel bus. It is a
historical core capability and is included in `basic:latest`; schema-v2 images
must declare it explicitly.

## Capability surface

| Operation | Commands |
| --- | --- |
| Publish | `tools message send`, `tools request` |
| Inbox | `tools message ls`, `processed`, `reply` |
| Recovery | `tools message dlq`, `tools message dlq requeue` |
| Subscriptions | `tools sources`, `tools channel subscribe`, `ls`, `unsubscribe` |

Messages are published to named channels and routed to durable per-agent
deliveries. Pending messages can wake an enabled loop and are inserted into the
next iteration in a bounded batch. A message remains pending until the agent
marks it processed; a reply also processes the source message.

`tools request` adds correlation metadata. With `--deadline`, the scheduler
publishes a timeout event to the requester's inbox if no correlated reply
arrives before the deadline.

## Subscriptions and providers

A subscription can narrow existing channel traffic with type globs and a JSON
matcher. Provider-backed channels additionally accept `--params`; the daemon
validates and fingerprints those parameters into a shared watch. Use matcher
fields to filter events already flowing and provider parameters to request work
that must be produced.

```bash
tools channel subscribe issue-provider:issues \
  --type 'issue.*' \
  --matcher '{"data.priority":"high"}' \
  --params '{"project":"checkout"}'
```

## Prompt integration

Use both the static instructions and runtime inbox markers when the agent
should receive and process messages in its prompt:

```yaml Tariboyfile.yaml
plugins:
  - name: messages
prompts:
  - file: $CURRENT_VERSION_STORE/skills/messages/prompt.md
  - runtime: messages
  - runtime: awaiting-replies
```

The runtime entries are snapshots for the current iteration. The database and
bus remain authoritative for delivery state.

## Managed workflow restrictions

During a workflow-managed assignment, direct send, reply, request, and group
coordination are denied unless the work packet grants the corresponding tool.
Raw channel subscription management is always denied; use `tasks observe` so
subscriptions remain assignment-scoped and match the workflow's channel
policy.

## Failure and recovery

Unprocessed messages are redelivered. After the delivery attempt limit they
move to the agent's dead-letter queue, where the agent can inspect and requeue
them. Publishing and processing are distinct durable operations, so a harness
failure does not silently acknowledge work.

## Related reference

- [Messaging architecture](/docs/architecture/messaging)
- [Channels reference](/docs/reference/channels)
- [Configurable task workflows](/docs/task-workflows#agent-tools-and-security-boundary)
