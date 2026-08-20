---
title: The channel bus & messaging
description: Messages move over a store-backed fan-out bus — four tables, per-agent delivery queues, and durable ack with redelivery.
sidebar:
  label: Channel bus
  icon: radio
---

Messages move over a **store-backed fan-out bus** (fan-out/query logic in
`internal/bus`, schema in `internal/store/migrations/0003_bus.sql`). This page is
the architectural summary; the [channel bus reference](/docs/reference/channels)
covers the full model, tools, and debugging.

## Four tables

- **channels** — named streams (`agent:<a>:inbox`, `group:<g>:broadcast`, …);
- **messages** — immutable rows published to a channel;
- **subscriptions** — an agent's standing interest, optionally filtered by a
  content `matcher` and `type` globs;
- **deliveries** — one row per matching `(subscription, message)`; this *is* the
  per-agent queue.

## Publish is a write, not a push

Publishing does not push text into a process. `Publish` writes the message,
creates delivery rows for matching subscriptions, and nudges affected loops.

Publishers may provide a stable idempotency key. A retry returns the original
immutable message and does not recreate deliveries or fire the publish hook a
second time. Native Tasks uses this for its transactional notification outbox.

At iteration prepare time the runner drains pending deliveries — oldest first, up
to `messages_batch` (default 10) — into the prompt. Delivered messages are acked
only when the iteration finishes normally (`done` / `no_i_am_done`); harness
errors, timeouts, and kills leave messages for redelivery until they hit the DLQ
(max 5 attempts).

## Channel names

Channel name prefixes: `agent`, `group`, `user`, `chat`. Well-known shapes:

- `agent:<a>:inbox` — direct inbox for one agent,
- `agent:<a>:stream` — stream channel for one agent,
- `group:<g>:broadcast` — fan-out to all group members,
- `group:<g>:inbox` — group lead inbox,
- `chat:<name>` — chat / plugin-facing channel,
- `user:<name>` — user-facing channel.

Native Tasks publishes `task.assigned`, `task.question`, `task.answered`, and
`task.triage`. Agent recipients use their existing inbox channel and customer
recipients use `user:<login>`. Mentions and unresolved-answer state remain in
the task itself; the channel message is the delivery mechanism, not the source
of truth.

An agent's unfiltered subscription to its own inbox is protected system state.
Agent creation provisions it, and daemon startup reconciles all persisted
agents before starting their loops. Task notifications therefore follow the
ordinary `Publish -> delivery -> WakeMessage` path: enabled loops wake for
pending deliveries, while disabled loops leave them queued.

Workflow runtime wakes use the same bus/outbox path. An incoming message never
changes a workflow status directly. An operator-declared external trigger may
create a new task; an assignment-scoped, policy-allowed subscription may append
an observation and apply only its declared reaction (`record_only`, wake, hold,
or optional acknowledgement work). Late events degrade to record-only. See
[Configurable task workflows](/docs/task-workflows#channels-triggers-subscriptions-and-observations).

See the [channel bus reference](/docs/reference/channels) for message fields,
subscription matchers, schedules and scripts, groups, and failure-mode
debugging.
