# Mailbox design

Replaces the channel bus (channels, messages, subscriptions, deliveries) and
the chats layer (chats, chat_participants) with one per-principal mailbox.
Agreed in conversation with the customer on 2026-10-26; this document is the
source for the implementation plan and for the DEV queue tasks that carry it.

## Intent and why

Every message in Tariboy moves today over a store-backed fan-out bus: a
producer publishes to a named channel, subscriptions with matchers decide
which agents get a delivery row, the delivery row is the agent's queue, and a
chat entity on top gives the customer a conversation view. Six tables, a
subscription language (type globs, content matchers, provider params and
watches), three provisioned chats per agent, echo suppression rules, and two
separate obligations on the agent (process the delivery, answer the message).

The customer's conclusion: this is more machinery than the product uses. The
only external plugin is Telegram, and it never uses matchers, params or
watches. Every producer already knows who it is talking to. What the product
needs is a mailbox: an agent has an inbox, every letter in it has a sender, a
recipient, a subject and a text, the agent must answer every letter, and the
inbox, the sent letters and the answered letters are stored so that an agent
can be debugged by reading its mail.

The behaviour the product relies on stays exactly as it is: a letter in an
agent's inbox wakes the agent's loop; unanswered letters are shown again on
the next iteration and dead-letter after five attempts; the inbox has a size
cap; task notifications, Goal wakes, script results, schedule alarms, group
requests and Telegram all keep working. Only the plumbing changes.

## Goals

- One mail table and one principal model carry every message in the system.
- An agent's inbox, sent and archive are three filters over that table, so an
  operator can read exactly what an agent saw, sent and answered.
- A letter in an agent's inbox wakes the agent through the same loop wake path
  as today, with the same redelivery, attempt limit and inbox cap.
- Answering is the one obligation: a letter leaves the inbox when the
  recipient replies to it. There is no separate "processed" step.
- Agents work with mail through one CLI, `tmail`, built like `ttasks`; the
  operator uses the same client as `tariboy mail`.
- The customer is a principal with a mailbox like any other. Task
  notifications become letters to the customer, and the Desktop reads them
  from the customer's inbox.
- The Chat tab of an agent becomes its Mailbox tab; the server gets a Mailbox
  tab that shows the customer's mailbox.
- The channel bus, chats and plugin subscriptions are removed, not wrapped.

## Non-goals

- Migrating existing bus or chat history into mail. The mail table starts
  empty; the customer resolves outstanding work by hand after the upgrade.
- Multi-participant conversations where every participant sees every reply.
  A letter may have several recipients; a reply goes to the sender only.
- Subscriptions, content matchers, provider parameters or watch lists in any
  form. A plugin that wants an agent to receive something sends it a letter.
- Changing the loop, Native Tasks, workflows, schedules or scripts beyond the
  way they send and receive messages.
- A version bump. Release tooling bumps the version when the release is cut
  (minor, since this removes and adds user-facing commands).

## Anti-goals

- An agent that stops being woken by something that wakes it today (a task
  assignment, a Goal, a script result, a schedule alarm, a group request, a
  Telegram message). This is the failure that must not happen.
- A second queue or state table beside the mail table "for the transport".
  Per-recipient state lives on the letter row.
- A hidden write path that puts a letter into an inbox without going through
  the one send operation, which would make the Sent and Inbox views lie.

## Constraints

- `AGENTS.md` rules apply: typed command results, OpenAPI snapshot and
  generated UI types regenerated in the same change, `make check` on mixed
  changes, documentation updated in the same task, no attribution trailers,
  linear branch history.
- Test daemons use isolated `TARIBOY_BASE_DIR` and `TARIBOY_RUNTIME_DIR`;
  nothing touches the live daemon.
- The `messages` skill and the `tasks` skill text that refers to it live in
  `~/github/tariboy-store`, a separate repository. The store change is a
  separate pull request and is sequenced after the daemon change.
- Compose files, images and the Telegram plugin currently reference
  channels; the product is on `0.x`, and these contracts change without a
  compatibility layer.

## 1. Data model

One migration drops `channels`, `messages`, `subscriptions`, `deliveries`,
`chats`, `chat_participants` and `task_notification_state`, and creates:

```sql
CREATE TABLE mail (
    seq            INTEGER PRIMARY KEY AUTOINCREMENT, -- monotonic, for ingress
    id             TEXT NOT NULL UNIQUE,              -- m-<ts>-<n>, the public handle
    sender         TEXT NOT NULL,                     -- principal
    recipient      TEXT NOT NULL,                     -- principal
    subject        TEXT NOT NULL DEFAULT '',
    body           TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL DEFAULT 'message',   -- message | task.question | task.goal | ...
    data           TEXT NOT NULL DEFAULT '{}',        -- JSON, structured fields
    needs_reply    INTEGER NOT NULL DEFAULT 1,        -- 1 request, 0 notice
    in_reply_to    TEXT NOT NULL DEFAULT '',          -- id of the letter this answers
    deadline       TEXT NOT NULL DEFAULT '',          -- RFC3339; a request with a timeout
    idempotency_key TEXT,                             -- producer retry guard
    produced_in_iteration TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    read_at        TEXT NOT NULL DEFAULT '',
    answered_at    TEXT NOT NULL DEFAULT '',          -- closed: replied, archived or dead
    answer_id      TEXT NOT NULL DEFAULT '',          -- id of the reply, '' when archived
    close_note     TEXT NOT NULL DEFAULT '',          -- operator archive note, task comment ref
    attempts       INTEGER NOT NULL DEFAULT 0,        -- times rendered into a prompt
    dead_at        TEXT NOT NULL DEFAULT '',
    dead_reason    TEXT NOT NULL DEFAULT ''           -- attempts | queue_limit | decode_error
);
CREATE UNIQUE INDEX idx_mail_idempotency ON mail(idempotency_key, recipient) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_mail_inbox  ON mail(recipient, answered_at, created_at);
CREATE INDEX idx_mail_sent   ON mail(sender, created_at);
CREATE INDEX idx_mail_thread ON mail(in_reply_to);
```

Rules:

- **One row per recipient.** `send` with several recipients writes one row
  per recipient, each with its own id and its own state. Sent shows one row
  per recipient, which is what an operator wants to see when debugging a
  broadcast.
- **Inbox** of principal P: `recipient = P AND answered_at = '' AND dead_at = ''`,
  oldest first. **Sent**: `sender = P`. **Archive**: `recipient = P AND
  (answered_at <> '' OR dead_at <> '')`. Dead letters are archive rows with
  `dead_at` set; they carry a Requeue action.
- **Requests and notices.** Every letter is one of the two, chosen by the
  sender through `needs_reply`. A *request* leaves the inbox when the
  recipient answers it. A *notice* leaves the inbox when the recipient has
  read it. Without this split the obligation to answer would make every
  answer a new obligation: agent A asks B, B replies, A must reply to the
  reply, and so on until the attempt limit; and every agent answer would sit
  in the customer's inbox until archived by hand. A reply is a notice unless
  the sender marks it as a request (`tmail reply --ask`), because an answer
  that asks a follow-up question is still a question.
- **What "read" means** depends on the recipient. A `user:*` principal reads
  a notice by opening it in the Desktop or with `tariboy mail read`; both set
  `read_at` and close the notice. An agent reads a notice when it is rendered
  into a prompt and that iteration finishes successfully: the runner closes
  the rendered notices with `close_note = read` on success and leaves them
  open on an error, a timeout or a kill, so they are rendered again next
  iteration and dead-letter after five attempts like a request. A plugin
  reads a letter when its `/deliver` succeeds. A `system:*` or `queue:*`
  principal has no reader, so letters to it are stored closed.
- **Closing** sets `answered_at`. A reply sets `answer_id` to the reply's id.
  An archive without a reply, or a notice read, leaves `answer_id` empty and
  carries a `close_note` (`read`, `delivered`, an operator note, or
  `task-comment:<id>`). A letter is never deleted by answering it.
- **Attempts and dead letters** keep today's semantics. Rendering a letter
  into a prompt increments `attempts` in the same transaction as the read; a
  letter whose next render would exceed five attempts is marked dead with
  reason `attempts` instead of being rendered. A letter sent to an agent
  whose open inbox already holds `messages_max_queue` letters is stored dead
  with reason `queue_limit`. A row whose `data` does not decode is marked
  dead with reason `decode_error`. Requeue clears `dead_at`, `dead_reason`,
  `attempts`, `answered_at` and `answer_id`.
- **Idempotency.** A send with an `idempotency_key` that already exists
  returns the existing row and writes nothing. Native Tasks' notification
  outbox and the Telegram plugin use it.
- **`seq`** is the monotonic ingress number that `task_workflow_message_sequence`
  provides today for queue triggers; the separate sequence table and the
  publish-transaction hook go away.

### Principals

The principal grammar is unchanged: `agent:<name>`, `user:<login>`,
`plugin:<name>`, `system:<component>`. The customer is `user:<customer_login>`
with the existing `customer_login` default and adoption rule, which now
rewrites `mail.sender` and `mail.recipient` instead of channel names.

System senders replace channel-shaped sources:

| Today | Sender |
| --- | --- |
| `system:tasks` on `chat:tasks:<a>` / `user:<c>` | the principal that caused the notification (the asker, the assigner, `system:workflow`) |
| Goal reconciler on `chat:service:<a>` | `system:goal` |
| `script.result` on `chat:service:<a>` | `system:scripts` |
| schedule alarms, deadline timeouts | `system:schedule` |
| `budget.warn` from `proxy` | `system:proxy` |
| Telegram | `plugin:telegram` |

Task notifications are sent by the principal that caused them rather than by
`system:tasks`. The bus needed `source = system:tasks` only so that its echo
suppression did not drop a notification an agent addressed to itself; mail has
no echo suppression and a letter to oneself is an ordinary letter.

A letter addressed to a `system:*` principal is stored already closed
(`answered_at = created_at`), because nothing reads a system inbox. It still
appears in the sender's Sent view and can be read with
`tariboy mail inbox --as system:goal`, which is how an operator sees what an
agent reported on its Goal letters.

Recipient validation at send time: `agent:<name>` must name an existing
agent; `user:<login>` must be the customer; `plugin:<name>` must be an
installed plugin; `system:*` is always accepted; `queue:<prefix>` must name
an existing task queue (see queue triggers); `group:<name>` is an alias that
expands to every member of the group except the sender. Anything else is
rejected with `unknown_recipient`.

## 2. Send, reply, wake

`internal/mail` replaces `internal/bus`. Its surface:

- `Send(Letter) ([]Letter, error)`: validates sender and recipients, expands
  groups, writes one row per recipient in one transaction, applies the inbox
  cap, and after commit calls the wake hook with the set of agent recipients
  and the event hook with one hint per row.
- `Reply(principal, id, text, needsReply bool) (Letter, error)`: the caller
  must be the recipient of `id`; writes a letter to the original sender with
  subject `Re: <subject>` (unless the subject already starts with `Re:`),
  `in_reply_to = id`, `needs_reply` as given (a notice by default), and closes
  the original with `answer_id` in the same transaction. A reply to a closed
  letter is allowed and recorded; it does not reopen anything. A reply to a
  `task.question` letter is refused with `answer_on_task`, whose message names
  the `ttasks` command: the answer belongs on the task, and the tasks service
  closes the letter when the wait resolves.
- `Archive(principal, id, note)`: closes without a reply. Allowed for
  `user:*` principals and for the operator acting on an agent's behalf; the
  agent-mode tool does not expose it, because an agent answers.
- `Inbox(principal, limit)`, `Sent`, `Archive`, `Show`, `MarkRead`,
  `Requeue`, `ClearInbox(agent)` (deletes open letters of one agent, the
  existing destructive operation), `OpenCount(agent)`, `Awaiting(agent)`
  (sent letters with a deadline and no answer).
- `RenderBatch(agent, n)`: the prompt drain. Reads the oldest `n` open letters
  for the agent, increments `attempts` and dead-letters over-limit rows in
  one transaction, and returns the letters.
- `CloseRead(agent, ids)`: called by the runner when an iteration that
  rendered `ids` finishes successfully; closes those of them that are
  notices and still open. Requests are untouched.
- `Deadline`: a request with a deadline arms a one-shot schedule keyed on
  the request letter's id; the reply that closes the request cancels it in
  the same transaction, as today with the correlation id.

The wake path is byte-for-byte the current one: the daemon wires the mail
wake hook to `Manager.WakeAgents`, the engine's message peek becomes
`OpenCount(agent) > 0`, and the loop starts a message-triggered iteration
only for a started agent with open mail. The Goal reconciler's rule that an
open Goal letter suppresses another Goal letter, that a dead one does not,
and that the cooldown applies, is rewritten against `mail` and keeps its
idempotency key.

Live hints: `GET /api/mail/ws` replaces `/api/messages/ws`, one frame per
row `{id, from, to, kind, ts}`, still only a refetch hint. The agent-state
socket and the event hub are untouched.

### What each producer does

| Producer | Today | With mail | Request or notice |
| --- | --- | --- | --- |
| Native Tasks notifications | outbox row → publish to `chat:tasks:<a>` or `user:<c>` | outbox row → `Send` from the causing principal to the agent or the customer, kind `task.assigned`, `task.question`, `task.answered`, `task.triage`; `data` keeps `task_key`, `queue`, `notification_id`; idempotency key = notification id | `task.question` is a request closed by the answer on the task, never by `tmail reply`; `task.assigned`, `task.triage`, `task.answered` are notices (the Goal letter drives the work) |
| Customer or agent answers a task question | tasks service marks `task_notification_state` read | tasks service closes the matching `task.question` letter with `close_note = task-comment:<id>` when the wait resolves, for an agent or a customer recipient alike | |
| Goal reconciler | `task.goal` to `chat:service:<a>` | letter from `system:goal` to the agent, kind `task.goal`, same idempotency key | request: the agent's reply is its report on the Goal |
| Script results | `script_result_outbox` → `chat:service:<a>` | outbox → letter from `system:scripts`, kind `script.result`, same `data` | request: the agent reports what it did with the result |
| Schedules | `channel` column, default `chat:service:<a>` | `recipient` column, default the owning agent; the migration rewrites existing rows (`chat:service:<a>`, `agent:<a>:inbox` → `agent:<a>`; anything else → the owning agent). Sender `system:schedule` | request |
| Request deadlines | one-shot schedule publishes `timeout` to the inbox | one-shot schedule sends a letter from `system:schedule` to the requester, kind `deadline`, `data.request_id` | notice |
| AI proxy budget | `budget.warn` to the inbox | letter from `system:proxy`, kind `budget.warn` | notice |
| Plugin host lifecycle events | `plugin.state`, `plugin.deliver_failed` to `system:plugins`, no subscriber | dropped as messages; they remain log and audit events only | |
| Operator / Desktop send | `POST /api/messages` as the customer | `POST /api/mail` as the customer, or `--as` another principal | request by default; `notice: true` to send an FYI |
| Agent send / request / reply | `/tools/message/send`, `/tools/request`, `/tools/message/reply` | `/tools/mail/send` (with `--deadline` for a timed request, `--notice` for an FYI), `/tools/mail/reply` (`--ask` for a reply that asks again) | sender's choice |
| Group send / request | `agent:<m>:inbox`, `group:<g>:direct:<m>` | `tmail send --to agent:<m>` with or without `--deadline`; `tmail send --to group:<g>` for the broadcast. The group routes under `/tools/group/` that only wrap a send are removed with the skill; `info`, `status`, `observe` and `loop` stay | sender's choice |
| Telegram | `/api/plugin/publish` to `chat:telegram:<a>`, sink subscription, `/deliver` | see section 5 | request: the customer wrote and expects an answer |
| Plugin workflow event | `/api/plugin/publish` to a plugin channel matched by a queue trigger | letter to `queue:<prefix>`, see queue triggers | stored closed |

### Groups

The group provisioner stops creating channels and subscriptions. Group
membership remains the data that `group:<name>` expands over. Lead and member
roles are unchanged where other features read them.

### Queue triggers

A workflow queue trigger today fires for a plugin-sourced message whose
channel matches the trigger's pattern and whose `correlation_id` matches the
trigger's optional correlation key, and creates a task in the trigger's
queue. Without channels, the queue itself is the address: a plugin that
wants a workflow event to create a task sends a letter to `queue:<prefix>`.
Such a letter is stored closed (nothing reads a queue inbox) and exists for
ingress. A trigger of queue `<prefix>` fires for a letter whose recipient is
`queue:<prefix>`, whose sender matches the trigger's `pattern` (a principal
glob such as `plugin:*` or `plugin:telegram`), and whose
`data.correlation_key` equals the trigger's correlation key when one is set.
The task title is the letter's subject (or body when the subject is empty),
and the description carries the letter id, sender and `data`, as it carries
the event today. The ingress reads `mail.seq`; the retention condition that
keeps unconsumed rows is rewritten against `mail`.

A Telegram message to an agent is addressed to the agent, not to a queue, so
it never trips a trigger; a plugin that wants both sends two letters. The
`scripts/workflow-e2e.sh` scenario in `full-check` is rewritten to send its
event to `queue:<prefix>`.

## 3. The agent's view: prompt and `tmail`

### Prompt

The platform-owned `# Task Processing Order` block keeps its shape. Its
`## Messages` section lists the agent's open letters, oldest first, up to
`messages_batch`, requests first and notices after them, each as:

```text
- id m-20261026T104200-7  from user:customer  kind message  attempts 1
  subject: Re: Вопрос по TARI-42
  in_reply_to: m-20261026T104000-3
  deadline: 2026-10-26T11:00:00Z        (only when set)
  data: {"task_key":"TARI-42"}          (only when non-empty)
  Используй S3, креды уже в queue secrets.
```

A `task.question` request carries one more line,
`answer: ttasks comment <task-key> "<answer>"`, and the two groups are
followed by the standing instruction:

```text
Requests: answer each one. A reply closes it:
    tmail reply <id> "<your answer>"
A task.question is answered on its task, not by mail; the letter closes
itself. A request you do not answer is shown again next iteration and is
dead-lettered after 5 attempts. A reply to a system:* sender records your
result; nobody reads it, so state what you did.
Notices: read them; they close when this iteration finishes.
```

`### Awaiting replies` lists the agent's own letters that carry a deadline and
have no answer yet, with age and deadline, as today. The runtime placeholders
`messages` and `awaiting-replies` keep their names in image templates and map
to the `mail` capability.

The agent configuration fields `messages_batch` and `messages_max_queue` keep
their names, API fields and compose keys. Only the Desktop labels change
("Mail: letters per iteration", "Mail: maximum open inbox"), and agent status
reports the open inbox count and whether it is at the cap, as it does now.

### `tmail` and `tariboy mail`

A new binary `tariboy-mail` with the managed link `tmail`, built on a package
`internal/mailcli` modelled on `internal/taskcli`:

- With `TARIBOY_TOOLS_SOCKET` set, agent mode: every action is
  `POST /tools/mail/<action>` on the identity-bound socket, and the daemon
  fixes the sender to the calling agent and scopes every read to that agent's
  mailbox.
- Without it, operator mode on the daemon socket: `tariboy mail` is the same
  client, always in operator mode, exactly as `tariboy tasks` wraps `ttasks`.
  Operator mode acts as `user:<customer_login>` unless `--as <principal>` is
  given.
- `--json` and `--help` follow `ttasks`. Text arguments follow the `cli-text`
  conventions; `--text -` reads the body from standard input.

| Command | Agent mode | Operator extras |
| --- | --- | --- |
| `tmail inbox [--all]` | open letters, oldest first; `--all` adds closed | `--as` |
| `tmail sent` | letters I sent | `--as` |
| `tmail archive` | closed letters, including dead ones (tagged) | `--as` |
| `tmail show <id>` | one letter with its reply, if any | `--as` |
| `tmail send --to <p>[,<p>] --subject <s> --text <t> [--kind k] [--deadline 5m] [--notice] [--data json]` | sender is me; a request unless `--notice` | `--as` |
| `tmail reply <id> <text> [--ask]` | closes the letter; the reply is a notice unless `--ask` | `--as agent:<name>` replies on the agent's behalf |
| `tmail requeue <id>` | my dead letter back to the inbox | `--as` |
| `tmail close <id> [--note t]` | not available: agents answer | archive without a reply |
| `tmail read <id>` | not needed: rendering is reading | marks a letter read |
| `tmail clear --as agent:<name>` | not available | deletes the agent's open letters after confirmation |

Agent-facing routes under `/tools/mail/` are gated by the `mail` capability,
which replaces `messages` in the image contract and in `plugincaps` core
capabilities. The contract keeps accepting the plugin name `messages` and
the runtime placeholders `messages` and `awaiting-replies` as aliases of
`mail`, so every image built before the store update still provisions and
still renders the letters block; `mail` is the canonical name and the store
images move to it. The toolcheck entry for `ttasks` gets a twin for `tmail`,
and agent provisioning puts `tmail` on the agent's PATH the same way.

The prompt's standing instruction names `tmail reply`, which is platform
text, so an agent whose image still ships the old `messages` skill is told
the right command even before its image is rebuilt. Its old
`scripts/messages.sh` fails against the removed routes with a clear
`unknown route` error rather than silently doing nothing.

Removed agent routes: everything under `/tools/message/`, `/tools/channel/`,
`/tools/sources`, `/tools/chat/unanswered`, `/tools/request`,
`/tools/group/send`, `/tools/group/request`.

## 4. Operator API and CLI

Registry commands (each with a named result struct, `WithResult`, a
`typedCallSets` entry, and the OpenAPI snapshot regenerated):

| Command | Route | Notes |
| --- | --- | --- |
| `mail.ls` | `GET /api/mail?box=inbox\|sent\|archive&as=&limit=&before=` | `as` defaults to the customer; `before` is a `created_at` |
| `mail.show` | `GET /api/mail/{id}` | the letter and its reply |
| `mail.send` | `POST /api/mail` | `to` (comma list), `subject`, `text`, `kind`, `data`, `deadline`, `notice`, `as` |
| `mail.reply` | `POST /api/mail/{id}/reply` | `text`, `ask`, `as` |
| `mail.close` | `POST /api/mail/{id}/close` | `note`, `as` |
| `mail.read` | `POST /api/mail/read` | `ids` |
| `mail.requeue` | `POST /api/mail/{id}/requeue` | |
| `agent.mail.clear` | `POST /api/agents/{name}/mail/clear` | replaces `inbox/clear`, same response shape |
| `mail.unread` | `GET /api/mail/unread?as=` | counts per sender and kind; the Desktop badges |

Removed commands and routes: `channel.*`, `message.send`,
`agent.subscribe`, `agent.unsubscribe`, `agent.subscriptions`,
`agent.inbox.*`, `chat.*`, `channel.watches`, `tasks.notifications.*`
(`/api/task-notifications*`), the plugin API's `/api/plugin/subscriptions`
and `/api/plugin/watches`, and `GET /api/messages/ws`. The CLI groups
`tariboy channel`, `tariboy chat`, `tariboy message` and the agent
`subscribe`/`unsubscribe`/`subscriptions`/`inbox` subcommands go with them.

Compose: the `subscribe:` key is removed from the agent schema and rejected
as an unknown field. The `messages` block (batch and max queue) is unchanged.

Maintenance, backup and per-agent table operations are rewritten for `mail`:
retention purges closed letters older than the configured age and never a
row the workflow ingress has not consumed; backup and restore re-prefix mail
ids; deleting an agent deletes letters addressed to it and keeps letters it
sent, because they sit in other inboxes.

Audit events: `mail_sent`, `mail_answered`, `mail_closed`, `mail_dead`,
`mail_requeued`, `mail_cleared` replace the `message_*` events.

## 5. Plugins and Telegram

The plugin protocol loses channels. A plugin manifest declares
`mail: {send: true, receive: true}` instead of `channels.publish` and
`channels.subscribe`, and the plugin types `channel-source` and
`channel-sink` become `mail-source` and `mail-sink`.

- Inbound: `POST /api/plugin/mail` with `{to, subject, text, kind, data,
  notice, idempotency_key}`; the daemon sends it from `plugin:<name>`. The
  Telegram plugin sends each authorized topic message to the mapped agent,
  keeping the `Forwarded from` first line and the `telegram_forward` data
  object. When the recipient's inbox is at its cap the letter is stored dead
  with `queue_limit` and the route answers `409 inbox_full`, which replaces
  today's `require_delivery_agent` check and lets the plugin tell the user.
- Outbound: the plugin host drains the inbox of `plugin:<name>` exactly as
  the sink drainer drains deliveries today (wake after a committed send,
  5-second fallback), calls the plugin's `/deliver` with the letter, and
  closes the letter with `close_note = delivered` on success. A failed
  delivery stays open and follows the attempt and dead-letter rules. No echo
  suppression is needed: the plugin receives only letters addressed to it.
- The Telegram `/deliver` handler routes by `sender`: a letter from
  `agent:<a>` goes to that agent's topic. An agent answers a Telegram letter
  with `tmail reply`, which addresses `plugin:telegram` by construction; an
  unprompted message to the customer's Telegram is `tmail send --to
  plugin:telegram`.
- The plugin no longer subscribes agents to anything; the topic ↔ agent map
  is the only state it keeps.

## 6. Desktop

Two tabs, both named **Mailbox**, both the same two-pane component: a list on
the left (sender or recipient, subject with a kind tag, preview, time, unread
dot), the selected letter on the right (From, To, Subject, Received, state,
threading line, body, actions). On a narrow screen the list and the letter
alternate. A segmented control switches Inbox / Sent / Archive; Inbox shows
its open count.

**Server → Mailbox**, a new server section `mail` placed before Tasks. It
shows the customer's mailbox. Actions on a letter: Reply (opens a composer;
the reply lands in the sender's inbox), Archive (close without reply), and for
a letter with `data.task_key` a link that opens the task drawer. A
`task.question` letter shows "Answer on the task" instead of Reply, because
the task is the source of truth and the answer there closes the letter. The
toolbar has Compose: recipient (any agent), subject, text, and a "no reply
needed" switch that sends a notice. Opening a letter marks it read, which
closes a notice and moves it to Archive; a request stays in Inbox until it is
answered or archived. The tab carries the count of open letters.

**Agent → Mailbox** replaces the Chat tab and its Chat / Channels / Queue
views, in both full and simple UI mode. It shows that agent's mailbox. The
toolbar shows `batch N · inbox K/M`. Actions: Reply on the agent's behalf
(operator reply; lands in the agent's Sent and closes the letter), Requeue on
a dead letter, Compose (a letter from the customer to this agent), and a
destructive Clear inbox with the existing confirmation. The tab carries the
agent's open inbox count.

Customer notifications: `CustomerQuestionNotifications` and the per-agent
Tasks tab badge read the customer's unread `task.question` letters grouped by
sender agent (`GET /api/mail/unread`) and refresh on `/api/mail/ws` hints.
Native desktop notifications fire for fresh `task.question` letters, as they
do now; a letter answered on the task disappears from the badge because the
tasks service closed it. The task drawer no longer calls the removed
`/api/task-notifications` routes.

Removed UI: `ChannelsPage`, `AgentMessages` and `agentMessages/*`,
`pages/agents/chat/*`, `useMessagesSocket` (replaced by a mail socket hook),
the `chatTypes` filter and its `localStorage` key, `channelName.ts`, the
`TaskNotification` type. `ui/src/lib/api/messages.ts` becomes `mail.ts`
re-exporting the generated mail types.

## 7. Documentation

- `docs/docs/architecture/messaging.mdx` is rewritten as the mailbox
  architecture page; `docs/docs/reference/channels.md` becomes
  `docs/docs/reference/mail.md` (model, `tmail` and `tariboy mail`, routes,
  failure modes). Sidebar labels and cross-links follow.
- `docs/docs/architecture/web-ui.mdx` (API layer, API types exceptions),
  `docs/docs/plugins/index.mdx` (manifest, inbound and outbound mail),
  `docs/docs/task-workflows.mdx` (queue triggers match a sender),
  `docs/docs/tasks.mdx` (notifications are letters), compose documentation
  (`subscribe` removed), `docs/docs/images/index.mdx` (`mail` capability),
  `docs/docs/development.mdx` and `README.md` where they name channels, chats
  or `scripts/messages.sh`.
- Store repository (`~/github/tariboy-store`, separate pull requests, done
  by the `tariboy-image-creator` agent through the TASK queue):
  - skill `mail` replaces `messages`: `SKILL.md` describes the mailbox
    (inbox, reply closes, `system:*` senders, deadlines) and a launcher
    `scripts/mail.sh` that execs `tmail`, like `tasks/scripts/tasks.sh`; the
    `messages` skill directory and its Python client are removed;
  - the `tasks` skill's "Answers from a chat or channel" section becomes
    "Answers from a letter": a letter from `plugin:telegram` or from the
    task's customer is a decision under the same two conditions, recorded
    on the task, then closed with `tmail reply`;
  - the `schedule` skill drops `--channel` in favour of `--to <principal>`;
  - every image `Tariboyfile.yaml` lists plugin and skill `mail` instead of
    `messages` (`basic`, `developer`, `reviewer`, `tariboss`,
    `tariboy-image-creator`, `tariboy-release-publisher`,
    `tariboy-workflow-developer`, `llm-as-judge` where it lists it), and
    each `instructions.md` rewrites its "a message is delivered" row and
    every "chat or channel" phrase for letters;
  - the development, pr-review and research workflows are checked for
    `scripts/messages.sh`, channel or chat wording in their status texts and
    scripts, and updated where found.

## 8. Delivery

The customer approves this document and the task breakdown below; from then
on the work runs autonomously under the tech lead (the session that wrote
this document), who owns the result.

Daemon and Desktop work lands through the DEV queue's development workflow
(`backlog → plan → approval → implement → review → complete → done`, pool
`developers`, agents assigned automatically). The tech lead starts each task
with the `start` outcome, reads the developer's plan in `approval` and
approves it or requests changes, and keeps at most two tasks past `backlog`
at any time. The DEV queue runs with `AUTO_MERGE=true`: the review watch
merges a pull request on its own once every check on the verified head has
passed, so nobody merges by hand. Store work lands through the TASK queue (flexible tasks) assigned to
the `tariboy-image-creator` agent, one task per store pull request.

The suggested cut, each leaving `make check` green:

1. **Mail core (daemon).** Migration, `internal/mail`, wake and event hooks,
   `/tools/mail/*`, prompt rendering, `internal/mailcli` with `tmail` and
   `tariboy mail`, operator `/api/mail/*` commands, and every producer moved to
   `Send` (tasks notifications and question auto-close, Goal, scripts,
   schedules and deadlines, proxy budget, groups). The bus packages, chats,
   channel and chat commands, plugin subscriptions, compose `subscribe`,
   queue-trigger matching, maintenance, backup and per-agent table code are
   removed or rewritten in this same task, because the prompt reads only
   mail from this point and nothing may keep writing to tables that no longer
   exist. Telegram moves to `/api/plugin/mail` and inbox draining here too.
   This is the large task; the plan splits it into ordered steps with tests
   first, but it ships as one pull request so that no intermediate commit
   has two message systems.
2. **Desktop Mailbox.** Both tabs, the mail API module and generated types,
   notifications over the customer inbox, removal of chat, channels and queue
   views. Depends on 1 for the API, so it starts when 1 reaches review.
3. **Documentation.** The pages in section 7 for this repository. Can run
   beside 2.
4. **Store: skills.** TASK queue, `tariboy-image-creator`: the `mail` skill,
   the `tasks` and `schedule` skill text. Starts when 1 reaches review, so
   the skill is written against the final `tmail` help; merges after 1.
5. **Store: images and workflows.** TASK queue, `tariboy-image-creator`:
   every image's `Tariboyfile.yaml` and `instructions.md`, the workflow
   status texts, image rebuilds and version bumps per the store's own rules.
   After 4 merges.

If task 1 proves too large for one review, the acceptable split is
"mail core with producers" first and "removal of the bus and plugin
protocol change" second, with the first pull request already rendering the
prompt from mail and the bus receiving no new publishes.

Deployment order matters once 1 merges: the daemon on the development host
is upgraded first (existing images keep provisioning through the `messages`
alias and agents are told `tmail reply` by the prompt), then the store
changes land and the pool agents are moved to the rebuilt images.

## 9. Left to the builder

- Exact id format for `mail.id` and the audit event payloads.
- Whether `Reply` in operator mode on an agent's behalf records the operator
  in `data` (recommended: `data.on_behalf = "operator"`).
- Test layout inside `internal/mail`, and which existing bus tests carry over
  as mail tests (wake, attempts, dead letters, idempotency, inbox cap, group
  expansion, system-recipient auto-close).
- The shape of the `/deliver` payload for plugins beyond the letter fields.
- The `mail.unread` response shape, as long as the Desktop can derive
  per-agent question counts from it.
- How the Desktop lists Sent rows for a broadcast (one row per recipient is
  acceptable).

## Decisions taken in conversation

- Keep every current flow working; only the plumbing changes. (customer)
- One obligation: a reply closes a letter. No separate processed step. (my
  proposal, accepted)
- Plugin subscriptions, matchers, params and watches are removed; Telegram
  does not use them and there are no other plugins. (customer)
- The customer is a principal with a mailbox; task notifications become
  letters to the customer. (customer)
- The customer reads mail in a server-level Mailbox tab placed before Tasks;
  the agent's Chat tab becomes Mailbox; both tabs carry the same name.
  (customer)
- Two-pane layout: list on the left, letter on the right. (customer)
- No multi-participant chats; a letter to several recipients is enough.
  (customer)
- No data migration; the bus tables are dropped and mail starts empty.
  (customer)
- `messages_batch` and `messages_max_queue` keep their meaning for mail.
  (customer)
- Agents use `tmail`, built like `ttasks`; `tariboy mail` is the same client
  in operator mode; the dynamic prompt keeps listing the letters to answer.
  (customer)
- Field names `messages_batch` / `messages_max_queue` are kept rather than
  renamed, to avoid an API and compose change with no behaviour behind it.
  (my call)
- Task notifications are sent by the causing principal, not `system:tasks`.
  (my call)
- A `task.question` letter is closed by the answer on the task; the Desktop
  offers "Answer on the task" rather than a mail reply for it. (my call)
- Dead letters are shown in Archive with a tag and a Requeue action, so Inbox
  means exactly "needs an answer". (my call)
- Plugin host lifecycle messages (`plugin.state`, `plugin.deliver_failed`)
  stop being messages; they had no reader. (my call)
- A letter is a request or a notice. Requests are answered; notices are
  read. Replies are notices by default. This refines "the agent must answer
  every letter": without it two agents answer each other until the attempt
  limit, and every answer to the customer waits in the customer's inbox for a
  manual archive. (my call, after the builder check)
- Agents read notices by having them rendered into a prompt of an iteration
  that finishes successfully; a failed iteration leaves them open. (my call)
- A `task.question` letter cannot be answered by mail; the answer on the task
  closes it, for agents and the customer alike. (my call)
- A plugin workflow event is a letter to `queue:<prefix>`; a trigger matches
  the sender glob and `data.correlation_key`. (my call)
