# Chats design

Approved in IMPROVE-99bk: comment #3138 ("Go ahead") on plan #3134 and
clarifications #3137.

## Goal and scope

Replace "channels plus a chat projection" with a first-class chat entity: a chat
has an identifier, a kind, a title, principal participants, delivery, replies, a
queue of unanswered messages, and live events. The transport stays as it is:
`messages`, `subscriptions`, `deliveries`, and the
`Publish -> delivery -> WakeMessage` path do not change, so the agent wake logic
is preserved in full.

This task delivers the specification and the plan. Implementation proceeds in
separate tasks by plan phase; every phase leaves the bus working.

Completion mode: PR. GitHub preflight succeeded; local `main` is synchronized
with `origin/main` (`a54c9f0`). The task's only branch and worktree:
`improve-99-chats`, `/home/agent/github/tariboy/.worktrees/improve-99-chats`.

## What the code confirms

- The bus is four tables (`internal/store/migrations/0003_bus.sql`): `channels`,
  `messages`, `subscriptions`, `deliveries`. An agent's queue is the
  `deliveries` rows without `acked_at` and outside the DLQ
  (`internal/bus/store.go:554` `Pending`).
- There is no chat entity. A chat is assembled by a query on the fly:
  `chatRowsSQL` (`internal/bus/chat.go`) merges the customer's messages in
  `agent:<a>:inbox` with the agent's messages in `user:<customer>`. The chat
  identifier is the agent name, as the `/api/chats/{agent}` routes show
  (`internal/commands/chat.go`).
- There are no participants as data. The only membership table is
  `subscriptions`, and its `agent TEXT` column stores only agents. The customer
  is not a participant: their presence is inferred from the channel name.
- Read marks are a single `chat_read_v1` value in `daemon_config`
  (`internal/commands/chat.go:23`), a read-modify-write without a transaction.
  The known ceiling is recorded in a comment in the same place.
- The "conversation / service" split relies on a type filter
  (`bus.DefaultChatTypes`), not on separate chats: `task.goal`, `script.result`
  and schedule alarms are simply hidden from the feed.
- The reply route is point-to-point: `replyTarget`
  (`internal/bus/messages.go:316`) takes an explicit `reply_to`, otherwise the
  source's inbox, otherwise the original channel. A reply reaches the chat only
  because the UI puts `reply_to: user:customer` on every message
  (`ui/src/pages/agents/chat/AgentChat.tsx`). The second send path,
  `InboxComposer` (`ui/src/components/InboxComposer.tsx`), does not.
- Echo suppression is already a general bus rule and already multi-participant
  (`internal/bus/store.go:325`): the author gets no delivery of their own
  message on a non-inbox channel, while all other subscribers do. There is no
  suppression on the author's own inbox — and that is exactly why an agent reply
  without `reply_to` comes back to the agent as a new unprocessed message.
- Mandatory processing already exists: `MarkProcessed`
  (`internal/bus/messages.go:236`) requires a non-empty result text, `Reply`
  auto-processes the original message with the result `replied: <id>`, an
  unprocessed message returns in the next iteration, and after 5 attempts it
  goes to the DLQ.
- Reply threading is already in the schema: `kind`, `correlation_id`,
  `in_reply_to`, `reply_to`
  (`internal/store/migrations/0017_messages_threading.sql`). Only `Reply` fills
  `in_reply_to`; an ordinary publish does not.
- `GET /api/messages/ws` (`internal/api/messages_ws.go`) emits one hint per
  publish across the whole host; HTTP remains the source of truth.
- CLI commands and HTTP routes are described by a single registration
  (`registry.Command` with an `HTTP` field), so a new chat command automatically
  gets both a CLI and an HTTP surface.

## Assessment: what carries over and what is built

Carried over unchanged — delivery, redelivery, DLQ, `queue_limit`, wake,
mandatory processing with text, message threading, echo suppression.

Built anew — the chat entity with a kind and a title, principal participants, a
chat-oriented reply route, the unanswered queue, a per-participant read mark,
chat-addressed socket events, the split of chats by kind, grouping in the UI,
and multi-agent chats.

Conclusion: moving `channels` to `chats` as a transport is a correct and cheap
step, but the task is solved by a chats layer on top of it, not by a rename.

## 1. Data model

One migration, `0044_chats.sql`. The `messages`, `subscriptions` and
`deliveries` tables do not change; `channels` is kept as the transport's channel
registry.

```sql
CREATE TABLE chats (
    id           TEXT PRIMARY KEY,          -- slug: dm:worker, tasks:worker, team-alpha
    kind         TEXT NOT NULL,             -- direct | group | tasks | service | plugin
    title        TEXT NOT NULL DEFAULT '',
    channel      TEXT NOT NULL UNIQUE,      -- the chat's transport channel: chat:<id>
    legacy_agent TEXT,                      -- agent name for reading pre-migration history
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    archived_at  TEXT
);

CREATE TABLE chat_participants (
    chat_id   TEXT NOT NULL,
    principal TEXT NOT NULL,                -- user:<login> | agent:<name>
    role      TEXT NOT NULL DEFAULT 'member',
    joined_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    read_ts   TEXT NOT NULL DEFAULT '',
    muted     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (chat_id, principal)
);
CREATE INDEX idx_chat_participants_principal ON chat_participants(principal);
```

Rules:

- **One chat, one channel.** A chat's channel is always `chat:<id>`. It is
  nobody's inbox, so echo suppression works the same for every participant, and
  an agent's reply in the chat does not come back to that agent.
- **Messages stay immutable.** The migration rewrites no `messages` row: a
  message identifier includes the channel name, and the chat's pre-migration
  history lives on the two old channels. So a migrated direct chat gets
  `legacy_agent`, and reading merges the chat channel's messages with the
  existing `chatRowsSQL` projection for that agent. New chats have no
  `legacy_agent`, and the merge degenerates for them.
- **An agent participant materializes a subscription.** Adding an agent to a
  chat creates its `subscription` to the chat channel with `locked=1` (the
  column already exists in migration 0017); removing the agent revokes it. So
  `deliveries`, the DLQ, `queue_limit`, `HasPending` and wake keep working
  unchanged.
- **A user participant is membership plus a read mark**, without a
  subscription: the customer reads over HTTP, not through the delivery queue.
- **The read mark moves** from `chat_read_v1` to `chat_participants.read_ts` and
  stops being a read-modify-write across two windows. The value only moves
  forward — that rule is kept verbatim.

## 2. Chat kinds and separation

The chats themselves do the separation, not a type filter.
`bus.DefaultChatTypes` is kept as a display filter but stops being the
separation mechanism.

For every agent, the migration and the agent provisioner create three chats:

| Chat | Kind | Channel | Participants | Content |
| --- | --- | --- | --- | --- |
| `dm:<agent>` | `direct` | `chat:dm:<agent>` | customer, agent | the main customer–agent conversation |
| `tasks:<agent>` | `tasks` | `chat:tasks:<agent>` | customer, agent | `task.assigned`, `task.question`, `task.answered`, `task.triage` |
| `service:<agent>` | `service` | `chat:service:<agent>` | agent; customer as observer | `task.goal`, `script.result`, schedule firings |

Shared:

- `group:<group>` — kind `group`, channel `chat:group:<group>`, participants are
  all group members and the customer. It replaces `group:*:broadcast` as a
  conversation; the `group:<g>:broadcast` channel itself and the
  `group:<g>:direct:<agent>` feed stay transport channels and do not become
  chats.
- `telegram:<agent>` — kind `plugin`, channel `chat:telegram:<agent>`. This is
  the existing channel of the Telegram bundle: it becomes a `chats` row without
  a rename and without changing the plugin's external contract.
- An arbitrary slug — a multi-agent chat created by the operator.

A chat slug must be a valid continuation of a channel name (every segment
matches `^[a-z0-9][a-z0-9_-]*$`) and must not overlap the reserved prefixes
`dm:`, `tasks:`, `service:`, `telegram:`, `group:` unless the provisioner creates
the chat for the corresponding role.

Task notifications in the `tasks:<agent>` chat open the task card: the message
already carries the task key in `subject`, and the UI opens the existing
`TaskDrawer`.

## 3. Agent wake

Unchanged everywhere. Publishing into a chat is an ordinary `Publish` to the
`chat:<id>` channel: delivery rows are created in the same transaction, the
publish hook wakes the affected loops, and the engine checks `HasPending(agent)`
and starts an iteration only when the loop is enabled and the queue is not
empty. Only the channel name that Native Tasks, the goal reconciler and scripts
publish to changes; the mechanism is identical.

`agent:<a>:inbox` is kept as the agent's direct inbox: `group send`, replies to
requests with a deadline, system events. The agent's protected unfiltered
subscription to its own inbox remains system state.

## 4. Mandatory processing and the unanswered queue

These are two different questions, and they must not be mixed.

**Processing (transport).** The rule carries over verbatim: the agent must mark
every delivered message processed with a non-empty text comment; an empty result
is rejected; an unprocessed message returns in the next iteration; after 5
attempts it goes to the DLQ. A reply in the chat auto-processes the original
message with the result `replied: <id>`.

**Reply (conversation).** A chat message is **unanswered for a principal** when
all of the following hold:

1. the principal is a participant of that chat;
2. the message's author is not that principal;
3. the message is not older than the principal's `joined_at`;
4. the same chat contains no message from that principal whose `in_reply_to`
   equals this message's id.

The unanswered queue is a projection over `messages`, not a second delivery
queue. Marking a message processed does not fake a reply: the agent can process
a message without replying and it stays in the unanswered queue, and vice versa.

For the queue to be true, a reply in a chat goes through the reply path — only
that path fills `in_reply_to`. An ordinary publish into a chat does not count as
a reply.

## 5. Reply route

`replyTarget` becomes chat-oriented. Resolution order:

1. the message's explicit `reply_to` — an override for external sinks
   (Telegram);
2. the channel of the chat that owns the original message's channel, if such a
   chat exists;
3. the source agent's inbox;
4. the original channel.

This fixes an existing defect rather than adding a capability: today an agent's
reply to a message without `reply_to` goes to the agent's own inbox, returns to
it as a new unprocessed message, and never appears in the conversation. The
second rule fixes this for all senders at once, including `InboxComposer`, and
makes a multi-agent chat possible: agent B's reply to agent A's message is
published into the chat, not into A's personal inbox.

## 6. API

The existing routes are kept for compatibility and start answering from the new
model: `GET /api/chats`, `GET /api/chats/{agent}`,
`POST /api/chats/{agent}/read`, where `{agent}` resolves to `dm:<agent>`.

New routes:

| Route | Purpose |
| --- | --- |
| `GET /api/chats` | chat list: `id`, `kind`, `title`, `last_ts`, `last_from`, `last_type`, `last_text`, `unread`, `participants` |
| `GET /api/chats/{chat}/messages` | one chat's feed, oldest first, paged by `before` (timestamp) |
| `POST /api/chats/{chat}/read` | moves the calling participant's `read_ts` forward |
| `POST /api/chats/{chat}/messages` | publishes a message into the chat |
| `POST /api/chats/{chat}/messages/{id}/reply` | replies to a chat message |
| `GET /api/chats/{chat}/unanswered` | the chat's messages the participant has not answered yet, in chronological order |
| `POST /api/chats` | creates a chat |
| `POST /api/chats/{chat}/participants` | adds a participant |
| `DELETE /api/chats/{chat}/participants/{principal}` | removes a participant |

Agent surface (`scripts/messages.sh`, through `registry.Command`):

```bash
scripts/messages.sh chat ls
scripts/messages.sh chat messages <chat>
scripts/messages.sh chat pending
scripts/messages.sh chat send <chat> --text "..."
scripts/messages.sh chat reply <message-id> --text "..."
```

`chat pending` is the agent's work queue: the messages in all of its chats that
it has not answered yet. The agent replies to each one until the queue is empty.
A reply is published as an ordinary message in the same chat.

## 7. WebSocket

The socket stays a hint; HTTP stays the source of truth. The hint becomes
chat-addressed: the frame carries `{chat, agent, id, channel, type, from, ts}`
and an event type of `chat.message`, `chat.read` or `chat.participant`. The
client refetches only the affected chat, not the whole list. A frame without
`chat` (a message on a non-chat channel) keeps its current shape, so existing
consumers do not break.

## 8. UI

- The chat list is grouped by agent: main, tasks, service; shared and
  multi-agent chats go in a separate section. Unread counts use the customer
  participant's `read_ts`.
- A message in the tasks chat shows the task key and opens the existing
  `TaskDrawer`.
- A multi-agent chat shows the participant list and the author of each message.
- Sending and marking read remain the same actions. `AgentChat.tsx` stops
  filling in `reply_to` by hand: the reply route now knows the chat.

## 9. Compatibility and migration

- No message is deleted or rewritten.
- Migrated direct chats read their history through `legacy_agent`; new messages
  go to the chat channel.
- `chat_read_v1` is carried into `chat_participants.read_ts` and is not used
  after the migration.
- The `chat:telegram:<agent>` channel becomes a chat without a rename.
- The old `/api/chats*` routes keep their response shape.

## 10. How it is verified

`make check` in full, plus targeted tests:

- the migration on a copy of the real schema: channels and read marks carry
  over, messages do not change;
- the `Publish -> delivery -> wake` path is unchanged for a publish into a chat;
- processing without text is rejected;
- a regression test for the reply-route defect: a reply without `reply_to`
  lands in the chat, not in the author's inbox;
- the unanswered queue on reply chains, including a multi-agent chat;
- no duplicate chat events on the socket.

## 11. Limits and risks

- This is a large data-model change, so the implementation is split into
  phases; each phase is independently verifiable and does not leave the bus
  half-working.
- Risk: chats without participants turn into just another channel name.
  Participants are the source of subscriptions, and phase 3 pins this with a
  test.
- The unanswered queue on a long history is a scan over the chat's messages.
  For a single daemon this is cheap; as history grows, the "answered" mark will
  get its own index.
- The merge with `legacy_agent` is temporary code. It is removed once the
  pre-migration history is no longer needed, in a separate task, not in this
  plan.
