---
title: Native Tasks
description: Queue-owned, infinitely nested work shared by customers and agents, with comments, dependencies, answer tracking, and realtime Desktop updates.
sidebar:
  label: Tasks
  icon: list-tree
---

Native Tasks are Tariboy's central work domain. They live in the existing
host-local `tariboyd.db`.

Judge automation reserves queue prefixes `JUDGE` and `IMPROVE`; Apply creates
both if absent. Every scheduled fire has a `JUDGE-*` task, including skipped,
empty, and failed cycles. Approving an exact proposal revision creates its
idempotent `IMPROVE-*` task in the same database transaction.

Queues may optionally activate a [configurable task workflow](/docs/task-workflows).
New tasks in such a queue automatically pin that published version; callers do
not pass a workflow when creating the task. Existing tasks and unmanaged queues
retain the flexible model described below.

## Work model

Every task belongs to a queue and receives an immutable queue key such as
`TEST-1`. A task records its author, customer, assignee, optional group,
status, priority, pull request URL, block reason, parent, ordered siblings,
comments, dependencies, and principals whose answers are still required. A pull
request is an optional normalized absolute `http` or `https` URL without
embedded credentials; an empty value clears it. Priority is one
of `P0` Critical, `P1` High, `P2` Normal (the default), or `P3` Low. Root and
nested sibling sets sort by priority, then manual position, then task key.

Parent/child depth is unlimited. Assigning or authoring an ancestor exposes its
whole descendant subtree to that agent; otherwise an agent sees only nested
tasks it authored, tasks assigned to it, and tasks visible through its current
group. Authoring alone does not expose a root task: see **Filing a task into a
queue** below. Ancestors needed to render an allowed descendant are returned as
read-only context. Group membership is resolved when access is checked, not
copied into the task.

An agent explicitly mentioned in an open answer request receives response-only
access: it can read the task and add the answering comment, but cannot edit,
complete, move, or change relations unless another ACL rule also grants write
access.

`blocks` is directional and cycle-checked; `related` is symmetric. A task is
ready when it is open, unassigned, has no manual block, and has no active
incoming blocker. Completing a parent with active descendants returns a
conflict unless the caller explicitly chooses **complete anyway**. Reading one
task also returns `descendants` and `active_descendants` for its whole subtree,
so a tree that keeps growing instead of closing is visible without walking it.

Workflow-managed tasks are excluded from this legacy ready/claim path. Their
`assignee` is not a phase owner: the workflow materializes pool-scoped
assignments, leases one to a running agent iteration, and advances only through
declared outcomes. See [Configurable task workflows](/docs/task-workflows) for
work packets, artifacts, questions, and observations.

Flexible tasks use `open`, `in_progress`, `wait_customer`, `done`, or
`cancelled` status. When the assigned agent asks the task customer a question,
the same comment transaction moves a non-terminal task to `wait_customer`.
The last customer answer returns it to `in_progress`, unless an operator made an
intervening status change. Questions for another principal do not change status.

## Filing a task into a queue

Any agent identity may create a root task in any existing queue. Creating one
never grants queue ownership or triager status, and the queue's own tasks stay
hidden.

What the agent gets back depends on whether it runs that queue — that is,
whether it is the queue's `responsible_agent` or one of its owners:

- **Runs the queue.** Ordinary create: the agent may set an assignee and a
  group, and keeps write access through queue ownership.
- **Does not run the queue.** A create without an assignee is *filed*: the
  created task is unassigned and ungrouped, and its author has no access to it
  afterwards. The create response carries `filed: true` and no `access`, and
  `ttasks create` prints that the task is recorded but no longer visible, so the
  agent does not read the following `not_found` as a failure and file it twice.
  An explicit assignee, including the creating agent, instead owns that task
  and its descendants without gaining access to the rest of the queue. A group
  remains forbidden and is rejected with `report_cannot_assign`.

Filing is how an agent hands over work it should not schedule for itself — a
defect it noticed outside its current assignment. Triage stays with the queue's
`responsible_agent`, who receives a `task.triage` notification; when the queue
has none, that notification goes to the customer instead. To let a specific
agent triage a queue, the customer adds it to the queue's owners.

`ttasks move KEY --to-root` detaches a task from its parent, which is the
inverse operation: the task becomes a root, and the mover loses access to it
unless it is the assignee or runs the queue. A move without `--parent`,
`--before`, or `--to-root` is refused rather than treated as a detach.

## Questions and notifications

Comments recognize typed principals only:

```text
@agent:worker Can you verify the migration?
@user:login Which behavior should win?
```

Each mention creates or refreshes a durable `waiting_for` row on the task. The
mentioned principal's next comment resolves only its own open wait. This makes
“where is my answer required?” directly filterable for both agents and the
customer.

An enabled agent normally follows one sticky assigned task as its Goal. It
releases that selection when the task is done, cancelled, has a pull request,
loses assignment or visibility, remains in `wait_customer` beyond the agent's
configured grace period, or eligible assigned work has a strictly higher
priority. The goal is daemon-owned and read-only to the agent; see [Agents and
the iteration loop](/docs/architecture/iteration-loop).

Assignment, question, answer, and queue-triage events enter a transactional
outbox and publish through the existing channels/messages bus. Agent
notifications target its inbox channel; customer notifications target
`user:<login>`, where `<login>` is derived from the daemon account's OS login.
Every persisted agent has a protected subscription to its own inbox. Agent
creation establishes it and daemon startup repairs it for legacy rows before
loops start. Publishing a task notification therefore creates a normal bus
delivery and uses the standard enabled-loop wake mechanism; disabled agents
retain the pending delivery. The Desktop inbox uses the same durable
notification records.

## Desktop

The top-level **Tasks** tab is global. Each Agent workspace includes the same
component with `scope_agent` fixed to that agent. The layout is Tree-first:

- left rail: All, My, Waiting for me, Notifications, and Queues;
- remaining width: dense expandable tree ordered by priority at every depth,
  with inline task/child creation and same-queue drag reparenting; manual before/after drag
  order is limited to the task's current priority bucket. In-progress tasks
  show an explicit **In progress** label, while open and done tasks retain
  their existing row presentation. A small red indicator marks each task with
  an unread, non-dismissed customer question notification. The same state adds
  a red dot beside the agent that asked the question in the host's agent list;
  customer-authored questions do not add an agent dot. Both indicators clear
  when the notification is read or dismissed.

Clicking a task opens a right-side sheet over the tree with its description,
priority, assignment, status, block reason, pull request URL, comments, and open
answer requests. It starts at half the viewport width; drag its left separator
or use its keyboard controls to resize it, and the chosen width survives reloads.
**Back**, **Close**, Escape, or a backdrop click returns to the still-mounted
tree with its expansion and scroll position preserved. Unsaved task, comment,
or relation drafts require confirmation before being discarded.

Descriptions and comments render as Markdown. Their visual editor supports
headings, bold, italic, strikethrough, links, lists, checklists, code, and tables.
Switch to **Source Markdown** to edit the source directly; content the visual editor
cannot represent stays in source mode so unsupported syntax is preserved.
Both modes save the same Markdown strings used by the API and `ttasks`.

**Send files** beside Description and Comment uploads selected files to the
task's server and appends their absolute paths to that editor's current draft.
It works without an assigned agent, including in Agent Tasks. Uploading does
not save the task or send the comment; use **Save task** or **Send comment**.
If a later file fails, paths for files already uploaded are kept in the draft.

Files are stored on disk under `$TARIBOY_BASE_DIR/files` (normally
`~/.tariboy/files`), with a unique directory per upload so matching names never
overwrite earlier files. Each file is limited to 16 MiB. Files remain after a
draft is discarded or a task is closed; remove unneeded files manually on the
server when their paths are no longer needed.

The Desktop establishes a silent baseline when it first observes a host, and
again when an unavailable host recovers. Existing unread questions appear as
dots without replaying stale native notifications. A question first observed
later in the current Desktop session can send one native notification; repeated
realtime hints and identical HTTP snapshots do not send it again. Notification
permission denial or an unavailable native service does not remove the in-app
dots.

Opening a native question notification marks that exact inbox notification as
read and selects its task on its original server. The Desktop route is exactly
`/servers/:hostId/tasks?task=<key>` (for example,
`/servers/local/tasks?task=ASK-1`); in the packaged single-page application it
appears after the URL hash as `#/servers/local/tasks?task=ASK-1`. Routing keeps
the notification's explicit host identity and never substitutes the local
daemon for a missing remote host. The task still opens if the read request is
interrupted, so the customer can answer it and retry inbox cleanup separately.

HTTP remains usable if realtime is disconnected. `GET /api/tasks/ws?after=N`
replays visible durable event hints after `N`, then streams new hints. The
client reconnects with its last accepted sequence and performs a full refetch
when the server requests a reset. Revision conflicts (`409`) also refetch the
authoritative tree/detail before another edit.

The Tasks toolbar starts in **Active** view. Its `GET /api/tasks` request sends
`status_view=active`, and the daemon applies that predicate before it builds the
response: Active returns only `open` and `in_progress`; **Closed**
(`status_view=closed`) returns only the canonical completed status, `done`; and
**All** (`status_view=all`) returns every status, including `cancelled`. An
omitted `status_view` is Active when no explicit `status` filter is present;
an explicit `status` without a view keeps its legacy authoritative semantics.
When both are supplied, their predicates intersect. Other task filters and
access rules are applied alongside the selected view, so excluded rows are
never shipped to the client.
An invalid view is rejected with `400 invalid_status_view`.

Status filtering may exclude an otherwise visible parent while retaining an
eligible child. In that response the child keeps its `parent_key`, but the
client renders it as a root-level filtered orphan; the excluded parent is never
included merely to reconstruct the tree. Switching views always refetches the
authoritative selected view, and realtime refreshes retain that selection.

## Agent capability

Tasks are native daemon functionality, but prompt and command exposure is
optional per image:

```yaml
plugins:
  - name: tasks
```

`tariboy-tasks` is installed globally as `ttasks`; it uses identity-bound agent
mode only when `TARIBOY_TOOLS_SOCKET` is non-empty and otherwise uses the host
Unix daemon socket as the customer actor. Agent mode fails closed rather than
falling back to operator access. When enabled, the `tasks` capability also
provisions the bare legacy `tasks` shim. Schema-v2 images must still package the
Tasks Store skill; capability selection never packages instructions
automatically. See the [Tasks built-in reference](/docs/plugins/built-in/tasks)
for the complete boundary.

Every agent mutation is identity-bound by its per-agent socket. Body fields
cannot forge the author, customer, or acting principal.

### Command help

`ttasks` and `tariboy-tasks` provide the same local help. Run either without
arguments or with `--help` / `-h` for the command list and mode descriptions.
Every command and nested group accepts those help flags; a bare group lists
its subcommands. Command help explains its purpose, syntax, arguments, flags,
required fields, and examples:

```bash
ttasks --help
ttasks create --help
ttasks ask -h
ttasks work complete --help
ttasks queue pool --help
tariboy-tasks notifications read --help
ttasks --help-json
```

Help never executes the command, resolves a daemon socket, or requires a
running daemon. It lists both modes and labels operator-only administration
and agent-only workflow operations; showing help does not grant execution
access. `ask --help` describes both the flexible `KEY PRINCIPAL TEXT` form and
the workflow `ASSIGNMENT --question ...` form, including its required context,
blocking scope, revisions, and retry key.

Root `--help-json` returns the complete command tree with descriptions and
examples. Shared commands retain their `flags` name array and add `flag_help`
descriptions, `usage`, `arguments`, `help`, and `examples`. `--json` controls
command results; use `--help-json` for machine-readable documentation.

Write task descriptions and comments as valid Markdown: use real newlines,
blank lines before lists, closed code fences, and no raw HTML. Typed mentions
and explicit `ttasks ask` retain their existing answer-tracking behavior.

`ttasks create --queue` works in any existing queue. In a queue the agent does
not run, omitting `--assignee` files an unassigned task the agent can no longer
see; passing `--assignee` creates work owned by that agent without exposing the
rest of the queue. `--group` remains forbidden there. See **Filing a task into
a queue**.

```bash
ttasks mine
ttasks ready --claim
ttasks show TEST-12
ttasks create --queue TEST --title "Investigate failure" --priority P1
ttasks create --parent TEST-12 --title "Add regression test"
ttasks assign TEST-12 worker
ttasks update TEST-12 --priority P0
ttasks update TEST-12 --pull-request https://example.com/org/repo/pull/42
ttasks comment TEST-12 "Found the boundary"
ttasks ask TEST-12 user:login "Which behavior should win?"
ttasks move TEST-12 --parent TEST-3
ttasks move TEST-12 --to-root
ttasks block TEST-12 --by TEST-2
ttasks relate TEST-12 TEST-9
ttasks done TEST-12
```

For workflow-managed work, agents instead use `ttasks work next`, `ttasks work
show`, `ttasks artifacts add`, `ttasks ask`, and `ttasks work complete`. Direct
status/claim operations cannot bypass the pinned state machine.

Create, comment, and relation actions accept stable idempotency keys internally
so transport retries return the original result without duplicating a task,
comment, wait, relation, event, or notification. Relation changes also use the
same optimistic task revision guard as field and tree mutations. They require
write access to both endpoint tasks, hide relations whose other endpoint is not
visible, and emit realtime history events for both endpoints.
