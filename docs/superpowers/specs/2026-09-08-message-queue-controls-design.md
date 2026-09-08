# Message Queue Controls Design

## Summary

Tariboy will let an operator physically clear one agent's pending message
queue with one server-side operation. The operation deletes only that agent's
unacknowledged, non-DLQ deliveries and message rows that become orphaned as a
result. It preserves Archive, DLQ, and every delivery owned by another agent.

The same change lowers the default maximum pending queue for new agents from
1,000 to 100, exposes the existing per-agent message batch and queue limits in
Configuration, and reports a full pending queue in agent status and the Agent
workspace header. The existing overflow-to-DLQ and retry behavior is unchanged.

## Goals

- Clear a selected agent's complete pending Queue quickly with one operator
  command, HTTP request, and SQLite transaction.
- Make physical deletion explicit, confirmed, atomic, and scoped to the
  selected agent.
- Preserve processed deliveries, DLQ deliveries, shared messages, and other
  agents' queue state.
- Support the operation through Desktop, HTTP, and
  `tariboy agent inbox clear NAME`.
- Expose `messages_batch` and `messages_max_queue` in Agent Configuration.
- Use 100 as the maximum pending queue default for newly created agents while
  preserving existing configured values.
- Make queue saturation visible from the daemon-authoritative agent status.

## Non-goals

- Changing the existing overflow policy: publishes beyond the pending limit
  still create a DLQ delivery with result `queue_limit`.
- Changing the five-attempt retry/DLQ policy or adding a configurable retry
  limit.
- Clearing Archive or DLQ.
- Deleting messages or deliveries belonging only to another agent.
- Adding automatic retention or background queue cleanup.
- Replacing the existing subscription management surfaces.
- Adding a new configuration model, table, migration, dependency, or explicit
  process-wide lock.

## Current behavior

The queue is not a separate per-agent table. A message is an immutable row in
`messages`; each matching subscription creates a row in `deliveries`. An
agent's pending queue is the distinct message set represented by its
unacknowledged, non-DLQ deliveries.

`messages_max_queue` currently defaults to 1,000. Once an agent reaches that
pending limit, publish still persists the message and a delivery for that
agent, but marks the delivery DLQ with result `queue_limit`. The limit therefore
bounds runnable pending work, not total message or DLQ storage. This behavior
will remain unchanged.

Desktop currently implements **Mark all processed** by paging through the
pending inbox and calling the per-message processed endpoint serially. That is
recoverable and audit-friendly, but it is not physical deletion and scales as
one HTTP/SQL operation per message.

## Chosen approach

Add one bus operation, registry command, and HTTP route for atomic bulk
deletion. This is faster and has a smaller failure surface than a client-side
loop. A general retention/prune operation is rejected because its scope is
wider than one agent's pending Queue.

No new persistence abstraction is needed. The bus already owns message and
delivery lifecycle, the command registry already projects commands to CLI and
HTTP, and Agent Configuration already has reusable section-draft behavior.

## Persistence and transaction semantics

The bus exposes `ClearPending(agent)` and returns:

- `deleted_deliveries`: pending delivery rows removed for the selected agent;
- `deleted_messages`: message rows removed because the operation left them
  with no deliveries.

The operation begins one SQLite transaction and verifies that the agent exists.
Within that transaction it identifies the selected agent's delivery rows where
`acked_at IS NULL AND dlq = 0`, captures their distinct message IDs, deletes
those delivery rows, then deletes only captured message IDs for which no
delivery remains and workflow ingress has already consumed the message.
Capturing the affected IDs prevents the operation from deleting unrelated
messages that already had no deliveries; the ingress check prevents queue
cleanup from racing away a workflow observation.

An agent can receive one message through several subscriptions. All live
pending deliveries owned by that agent are removed. A DLQ or processed delivery
for the same agent is retained. A message row remains whenever any retained
delivery still references it, including a delivery owned by another agent.

The operation is atomic: any SQL error rolls back both delivery and orphan
message deletion. An empty Queue succeeds with zero counts. The bus emits one
bounded audit event with both counts after commit; it does not emit one event
per deleted row.

SQLite's normal write serialization defines concurrency. A delivery that no
longer matches the pending predicate before deletion is preserved. A publish
serialized after the clear transaction remains in Queue. No additional global
lock is introduced.

## Queue state and saturation

The bus exposes a read-only pending count for an agent. Queue limiting, clear,
and status use one shared internal pending predicate:

```sql
s.agent = ? AND d.acked_at IS NULL AND d.dlq = 0
```

Counts are by `COUNT(DISTINCT d.message_id)`, matching prompt-level and inbox
deduplication across multiple subscriptions.

`GET /api/agents/{name}/status` adds:

- `messages_pending`: current distinct pending messages;
- `messages_max_queue`: the selected agent's configured limit;
- `messages_queue_full`: `messages_pending >= messages_max_queue`.

The status handler returns its existing error response if the pending count
cannot be read; it must not report a false `messages_queue_full: false`.

The Agent workspace header renders
**Message queue full: pending / limit** in destructive styling when the flag is
true, alongside the existing budget status. It renders nothing when false.
The existing three-second status refresh makes the indicator appear or clear
without a separate client-side estimate.

## Operator API and CLI

The command registry adds:

```text
agent.inbox.clear
POST /api/agents/{name}/inbox/clear
tariboy agent inbox clear NAME
```

The success response contains `deleted_deliveries` and `deleted_messages`.
An unknown agent returns the stable `not_found` user error. An empty Queue is a
successful idempotent response with both counters zero. SQL failures preserve
the existing internal-error contract and return a non-zero CLI exit status.

The CLI uses normal registry rendering; it does not implement deletion itself.
Desktop calls the same HTTP route.

## Desktop Queue experience

In **Messages → Queue**, the current bulk **Mark all processed** action becomes
**Clear queue**. Individual **Mark processed** and **Reply** actions stay
unchanged.

Opening **Clear queue** shows an in-app destructive confirmation that states:

- every currently pending message delivery for this agent is physically
  deleted and cannot be recovered;
- Archive and DLQ are not changed;
- deliveries and shared messages still needed by other agents are retained.

The confirmation does not request a result string. While the request is in
flight it cannot be submitted twice or dismissed. The UI performs no optimistic
row deletion. On success it closes the dialog, reloads Queue, and reports the
server counters. On failure it leaves the current rows visible, restores the
controls, and reports the error for retry.

## Configuration and defaults

Agent **Configuration** adds a **Messages & Channels** section with two
positive whole-number fields:

- **Messages per iteration** — `messages_batch`;
- **Maximum pending queue** — `messages_max_queue`.

The section reuses the current dirty-draft, discard, validation, stable serial
save, and explicit-host behavior from `AgentSettings`. Because there is no
generic update-agent endpoint, the backend adds two narrow field commands in
the same style as the existing loop and Goal settings. The UI still presents
one **Save message settings** action and sends only changed values.

Both commands load the agent, reject values below 1 with stable field-specific
user errors, update the existing agent row, and return the canonical value. A
failed field leaves it and every unsent field dirty, following the existing
section save contract.

The default `messages_max_queue` becomes 100 in every new-agent path:

- the agent store's zero-value creation fallback;
- `loop.Manager.Run`'s omitted-value fallback;
- the Desktop new-agent draft;
- the fresh-database column default.

Clone continues to copy the source agent's explicit persisted value. Existing
agent rows are not rewritten and no migration changes their value.
`messages_batch` remains 10 by default.

## Error and safety behavior

- Destructive scope is enforced in SQL, not inferred from the currently loaded
  UI page.
- Agent identity comes from the route/command argument and existing operator
  authorization boundary.
- Unknown agents fail before mutation.
- Partial deletion is impossible because delivery and orphan cleanup share one
  transaction.
- Archive and DLQ predicates are excluded from deletion even if a stale browser
  still displays a formerly pending row.
- Shared message rows survive whenever any delivery remains.
- UI errors retain authoritative rows and never claim success optimistically.
- No test uses the live daemon, live base/runtime directory, or listener.

## Testing

Implementation follows RED/GREEN vertical slices.

### Bus and persistence

A focused bus test will create pending, processed, DLQ, multi-subscription, and
cross-agent deliveries. It will prove that clear:

- deletes every live pending delivery for only the selected agent;
- preserves that agent's processed and DLQ deliveries;
- preserves other agents' deliveries and their shared message rows;
- deletes only affected message rows that become orphaned;
- ignores unrelated pre-existing messages with no deliveries;
- returns exact counters and succeeds with zero counters on retry.

Focused count tests will prove distinct-message counting and the
`pending >= limit` boundary.

### Registry, HTTP, CLI, and defaults

Command tests will cover the `agent.inbox.clear` registry shape, success
counters, `not_found`, and the generated CLI path
`tariboy agent inbox clear NAME`. Agent, manager, and create-draft tests will
change the new default to 100 while preserving explicit and cloned values.
Configuration command tests will cover positive validation and persistence.

### Status and React UI

Backend status tests will cover the three additive fields below, at, and above
the limit and will preserve count errors.

React tests will cover:

- the queue-full header indicator and its absence below the limit;
- destructive confirmation copy and duplicate-submit protection;
- one bulk request, success counters, and Queue reload;
- failure without optimistic removal;
- positive validation, dirty/discard behavior, and stable saving for the
  **Messages & Channels** section;
- the new-agent default of 100 and unchanged clone behavior.

## Documentation and verification

Current product documentation will update:

- messaging architecture and the channel reference for physical clear,
  saturation, DLQ preservation, configuration, and the new default;
- state model for the derived queue-full status;
- Web UI architecture for Queue confirmation, header status, and Configuration;
- generated operator command reference through the registry contract.

Focused package and UI tests run while implementing. Final branch verification
is exactly:

```bash
make check
git diff --check
```

`make full-check` is intentionally not run per the customer's explicit
instruction. The task does not change Desktop native-host, packaging, or
generated committed UI artifacts and does not bump the product version.
