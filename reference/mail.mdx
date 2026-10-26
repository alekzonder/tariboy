---
title: Mail reference
description: The tmail and tariboy mail commands, the mail routes, the prompt block, the agent settings, and how to debug an agent through its inbox, sent and archive.
sidebar:
  label: Mail
  icon: mail
---

# Mail

Every message in Tariboy is a letter from one principal to another. This page
is the reference for working with mail: the `tmail` client, its operator form
`tariboy mail`, the HTTP routes, the prompt block an agent sees, the agent
settings, and debugging. The [mailbox architecture](/docs/architecture/messaging)
page explains the model: principals, requests and notices, closing rules,
dead letters, the wake path and the producers.

## Mental model

- A letter has a sender, a recipient, a subject, a body, a `kind` (default
  `message`) and a JSON `data` object. A send to several recipients writes one
  letter per recipient.
- A letter is a **request**, which its recipient answers with a reply, or a
  **notice**, which closes when its recipient has read it. A reply is a notice
  unless `--ask` makes it a request.
- **Inbox** is the open letters addressed to you, **Sent** the letters you
  sent, **Archive** the closed letters addressed to you, including dead ones.
- An open letter in an agent's inbox wakes the agent's loop and is rendered
  into its prompt until it closes. A letter rendered five times without
  closing is dead-lettered.

## `tmail` and `tariboy mail`

`tariboy-mail`, installed with the managed link `tmail`, is one client with
two modes:

- **Agent mode.** When `TARIBOY_TOOLS_SOCKET` is set, every command is
  `POST /tools/mail/<command>` on the agent's identity-bound socket. The daemon
  fixes the sender to the calling agent and scopes every read to that agent's
  mailbox. Agent provisioning puts `tmail` on the agent's `PATH`.
- **Operator mode.** Without that socket, `tmail` calls the operator routes on
  the host daemon socket. `tariboy mail …` runs the same client and is always
  in operator mode, also inside an agent. Operator mode acts as the customer,
  `user:<customer_login>`, unless `--as <principal>` names another principal.

`tmail --help` and `tmail <command> --help` print local help and never run a
command; `tmail --help-json` prints the whole command tree. `--json` prints
results as JSON, `--version` prints the client build. Pass long or multi-line
text with `--text -`, which reads standard input.

| Command | What it does | Agent mode |
| --- | --- | --- |
| `tmail inbox [--all] [--limit N] [--before CURSOR]` | Open letters, oldest first; `--all` lists every letter to you, newest first | yes |
| `tmail sent [--limit N] [--before CURSOR]` | Letters you sent, newest first, with their state | yes |
| `tmail archive [--limit N] [--before CURSOR]` | Closed letters, newest first; dead ones tagged `dead:REASON` | yes |
| `tmail show ID` | One letter and the reply that closed it, if any | yes |
| `tmail send --to P[,P] --subject S --text T [--kind K] [--deadline D] [--notice] [--data JSON]` | Send a letter; a request unless `--notice` | yes |
| `tmail reply ID TEXT [--ask]` | Reply to a letter addressed to you, which closes it | yes |
| `tmail requeue ID` | Put one of your dead letters back in the inbox | yes |
| `tmail close ID [--note TEXT]` | Archive a letter without a reply | operator only |
| `tmail read ID...` | Mark letters read; a read notice closes | operator only |
| `tmail unread` | Count open and unread letters, and open letters by sender and kind | operator only |
| `tmail clear --as agent:NAME [--yes]` | Delete every open letter of an agent; asks for confirmation unless `--yes` | operator only |

Every operator command takes `--as`. In agent mode `--as` and the operator-only
commands are refused: an agent always acts as itself and answers its letters
instead of archiving them.

- `--to` takes a comma list of `agent:<name>`, `user:<login>`,
  `plugin:<name>`, `system:<component>`, `queue:<prefix>` and `group:<name>`.
  A group expands to every member except the sender.
- `--deadline` is a Go duration such as `5m` or an RFC 3339 time. When an
  agent's request is still open at that time, the agent receives a
  `deadline` notice from `system:schedule` with `data.request_id`. A reply or
  an archive cancels it. A deadline on a request from any other principal is
  stored and listed but sends no notice.
- `reply` also takes the text as `--text`; a `TEXT` of `-` reads standard
  input. With `tariboy mail reply ID TEXT --as agent:NAME` an operator replies
  on an agent's behalf; the reply carries `data.on_behalf = "operator"`.
- `sent`, `archive` and `inbox --all` print a last `next:` line with the
  command for the next page when a page is full. The open inbox does not page:
  raise `--limit` instead.

Examples:

```bash
tmail inbox
tmail reply m-20261026T104200Z-000007 'Done: the bucket is created.'
tmail reply m-20261026T104200Z-000007 'Which region?' --ask
tmail send --to agent:reviewer,agent:qa --subject 'Build ready' --text 'Build 42 is green.' --notice
printf '%s\n' "$REPORT" | tmail send --to agent:lead --subject Report --kind report --text -
tmail send --to agent:worker --subject 'Take TARI-12' --text 'Please take TARI-12.' --deadline 5m
tmail send --to group:dev-team --subject 'Stand by' --text 'Release freeze until 15:00.' --notice
```

## Routes

Operator routes are registry commands on the daemon socket and the loopback
listener; the OpenAPI snapshot describes their arguments and results. `as`
defaults to the customer.

| Route | Purpose |
| --- | --- |
| `GET /api/mail?box=inbox\|sent\|archive&as=&limit=&before=` | One mailbox view; a full page returns `next_before`, an opaque cursor to pass back as `before`. The inbox pages oldest first, the other boxes newest first, and letters sharing one `created_at` are never skipped |
| `GET /api/mail/{id}?as=` | One letter and its reply |
| `POST /api/mail` | Send: `to`, `subject`, `text`, `kind`, `data`, `deadline`, `notice`, `as` |
| `POST /api/mail/{id}/reply` | Reply: `text`, `ask`, `as` |
| `POST /api/mail/{id}/close` | Archive without a reply: `note`, `as` |
| `POST /api/mail/read` | Mark read: `ids`, `as` |
| `POST /api/mail/{id}/requeue` | Requeue a dead letter: `as` |
| `GET /api/mail/unread?as=` | Open and unread counts, open letters by sender and kind |
| `POST /api/agents/{name}/mail/clear` | Delete an agent's open letters |
| `GET /api/mail/ws` | Live hints, one frame `{id, from, to, kind, ts}` per written letter |

Agent routes: `POST /tools/mail/<action>` with a JSON body, where the action is
`inbox` (`all`), `sent`, `archive`, `show` (`id`), `send` (`to`, `subject`,
`text`, `kind`, `data`, `deadline`, `notice`), `reply` (`id`, `text`, `ask`)
or `requeue` (`id`). The `mail` capability gates them. An image declares it
as plugin `mail` with the packaged skill `mail`; an image built before the
mailbox declares `messages` with its `messages` skill, which the image
contract accepts as an alias.

Plugin routes: a plugin sends with `POST /api/plugin/mail` (`to`, `subject`,
`text`, `kind`, `data`, `notice`, `idempotency_key`), authenticated by its
plugin token, and receives the letters addressed to it at its own
`POST /deliver`. See [Plugins](/docs/plugins#mail).

Every route returns letters in one JSON shape:

```json
{"id":"m-20261026T104200Z-000007","seq":7,"from":"user:customer","to":"agent:worker",
 "subject":"Re: TARI-42","body":"...","kind":"message","data":{},
 "needs_reply":true,"in_reply_to":"m-...","deadline":"","created_at":"2026-10-26T10:42:00.000000000Z",
 "read_at":"","answered_at":"","answer_id":"","close_note":"",
 "attempts":1,"dead_at":"","dead_reason":"","produced_in_iteration":""}
```

## The prompt block

The platform-owned `# Task Processing Order` block has a `## Messages`
section. When the agent has open letters, it lists them oldest first, up to
`messages_batch`, requests first and notices after them:

```text
## Messages

Open letters for this iteration, oldest first. Each is listed with its id.

### Requests

- id m-20261026T104200Z-000007  from user:customer  kind message  attempts 1
  subject: Re: Bucket for TARI-42
  in_reply_to: m-20261026T104000Z-000003
  Use S3; the credentials are in the queue secrets.

### Notices

- id m-20261026T104500Z-000009  from agent:qa  kind message  attempts 1
  subject: Build ready
  Build 42 is green.

Requests: answer each one. A reply closes it:
    tmail reply <id> "<your answer>"
A task.question is answered on its task, not by mail; the letter closes
itself. A request you do not answer is shown again next iteration and is
dead-lettered after 5 attempts. A reply to a system:* sender records your
result; nobody reads it, so state what you did.
Notices: read them; they close when this iteration finishes.
```

A letter shows `deadline:` and `data:` lines only when they are set. A
`task.question` letter carries one more line,
`answer: ttasks comment <task-key> "<answer>"`. With no open letters the
section reads `No incoming messages for this iteration.` Requests the agent
sent with a deadline and no answer follow under `### Awaiting replies`, one
line each with the id, the recipient, the age and the deadline.

The image placeholders `messages` and `awaiting-replies` keep their names and
map to the `mail` capability. The standing instruction is platform text, so an
agent whose image still ships the old `messages` skill is told `tmail reply`.

## Agent settings

| Setting | Default | Effect |
| --- | --- | --- |
| `messages_batch` | 10 | Most letters rendered into one prompt; a value of 0 or less falls back to 10 |
| `messages_max_queue` | 100 for new agents | Most open letters in the agent's inbox; a letter beyond it is stored dead with reason `queue_limit` |
| `enabled` | — | Set by `agent start`, cleared by `agent stop`. A stopped agent keeps its letters open and does not run for them. A stop-policy halt has the same effect until the next start |
| `interval_s` | — | Timer interval. A letter wakes a started agent without a positive interval |
| `group` | — | Membership that `group:<name>` expands over |

Set the two mail fields with `tariboy agent messages batch NAME VALUE` and
`tariboy agent messages max-queue NAME VALUE`, in the Desktop agent
configuration, or with the compose `messages` block. Existing values and
values copied by Clone are not rewritten when a default changes. Agent status
reports the open inbox count as `messages_pending` and `messages_queue_full`
when it is at the cap.

## Schedules and scripts

A schedule sends a letter from `system:schedule` each time it fires. It stores
`agent`, `kind` (`oneshot` or `cron`), `spec`, its recipient, a message
template, `next_fire_at` and `enabled`. The agent route
`POST /tools/schedule/add` takes `to`, a principal; without it the schedule
writes to the owning agent. The template may set `subject`, `text`, `kind`
(default `schedule.fire`), `data` and `notice`; a schedule letter is a request
unless `notice` is set. An older skill that still sends `channel` is mapped:
`chat:service:<agent>` and `agent:<agent>:inbox` become `agent:<agent>`, and
any other channel the owning agent.

Scripts run only local commands. A non-quiet run sends a `script.result`
request from `system:scripts` to the owning agent. Its `data` holds the script
and run ids, name, mode, status, optional exit code and the absolute
`log_path`; the log starts with the resolved working directory, and the
combined output stays in that file. Sending the result also stops a recurring
definition, so one failing command cannot flood the inbox;
`scripts/scripts.sh rerun scr-...` resumes it once the agent has handled the
letter. A quiet run, one that exits `111`, sends nothing and keeps the
schedule running.

## Errors

| Code | When |
| --- | --- |
| `unknown_recipient` | A recipient is not an existing agent, the customer, an enabled plugin that declares `mail.receive`, a `system:*` principal, an existing queue or an existing group |
| `unknown_sender` | `--as` names a principal that cannot send |
| `empty_group` | The group has no member other than the sender |
| `forbidden` | Reply, close, read or requeue by a principal that is not the recipient; show by one that is neither sender nor recipient |
| `answer_on_task` | A reply to a `task.question`; the message names the `ttasks comment` command |
| `queue_limit` | A requeue while the agent's inbox is at its cap (HTTP 409); close letters first |
| `inbox_full` | `POST /api/plugin/mail` to an agent whose inbox is at its cap; the letter is stored dead |
| `not_found` | No letter has that id |
| `invalid_argument` | A missing recipient, a reply without text, a bad deadline, a requeue of a letter that is not dead |

## Debugging an agent through its mail

An agent's mail is its message history. Read it as the agent with `--as`:

```bash
tariboy mail inbox   --as agent:worker      # what it still has to answer
tariboy mail sent    --as agent:worker      # what it sent, and whether it was answered
tariboy mail archive --as agent:worker      # what it answered or read; dead letters tagged
tariboy mail show    m-20261026T104200Z-000007 --as agent:worker
tariboy mail unread  --as agent:worker
tariboy mail archive --as system:goal      # Goal reports; letters to system:* are stored closed
```

- **A letter was sent, the agent does not act on it.**
  - Check the agent is started; a stopped agent keeps its letters open.
  - Check the letter is in its inbox, not in its archive as dead. Requeue a
    dead letter with `tariboy mail requeue ID --as agent:NAME`.
  - Check `messages_queue_full` in agent status. At the cap new letters are
    stored dead with `queue_limit`; plugins receive `409 inbox_full`.
- **A letter appears in every prompt.** It is a request nobody answered, or a
  notice rendered into iterations that failed. The agent closes a request with
  `tmail reply`; an operator can reply on its behalf or archive it with
  `tariboy mail close ID --as agent:NAME`. After five renders it is
  dead-lettered with reason `attempts`.
- **A `task.question` letter does not close.** It closes only when the
  question is answered on its task or a workflow outcome resolves it; `tmail reply` refuses it with
  `answer_on_task`.
- **A send fails with `unknown_recipient`.** Check the principal prefix and
  name; a channel name such as `agent:worker:inbox` is not a principal.
- **An old image fails with `unknown route`.** Its `scripts/messages.sh`
  calls routes the mailbox removed. The prompt still names `tmail reply`;
  rebuild the image with the `mail` skill.
- **A plugin does not receive replies.** Read `tariboy mail inbox --as
  plugin:NAME`: open letters there are waiting for a successful `/deliver`,
  and dead ones in its archive failed five times.
- **An agent's inbox must be emptied.** `tariboy mail clear --as agent:NAME`
  deletes its open letters after a confirmation prompt (`--yes` skips it);
  `tariboy agent mail clear NAME` does the same without a prompt. Closed and
  dead letters stay.
