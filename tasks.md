---
title: Native Tasks
description: Queue-owned, infinitely nested work shared by customers and agents, with comments, dependencies, answer tracking, and realtime Desktop updates.
sidebar:
  label: Tasks
  icon: list-tree
---

Native Tasks are Tariboy's central work domain. They live in the existing
host-local `tariboyd.db`.

Every daemon starts with a default `TASK` queue named `Tasks`, with no owners
and no responsible agent. At each start `tariboyd` creates it when it is
absent and records a `task.queue_created` event by `system:tariboyd`; an
existing `TASK` queue, whether created by hand or by compose, is left
unchanged.

A queue with no workflow holds flexible tasks, the model described below. A
queue can instead be bound to a [workflow image](/docs/workflow-images): a task
created there pins that version and follows its statuses, each owned by an agent
pool, the customer, or a script. See [Task workflows](/docs/task-workflows).
Every task has a `category` field, which is one of the five statuses below and
equals `status` for a flexible task.

## Work model

Every task belongs to a queue and receives an immutable queue key such as
`TEST-fkt3`: the queue prefix, a dash, and four random characters drawn from
`23456789abcdefghjkmnpqrstuvwxyz` — the digits and lowercase letters that cannot
be confused for one another when a key is read aloud or copied by hand. The key
is random rather than a per-queue counter so that two daemons never mint the
same key, which is what lets a task keep its key when it
[moves to another server](#moving-a-task-to-another-server). Keys are matched
case-insensitively, so `test-FKT3` finds the same task.

Daemons upgraded from a numeric key such as `TEST-12` rewrite every task to a
random key on first start and keep the old key as a permanent alias, so a key
already written into an agent context, a branch name, a pull request title or a
comment keeps resolving to its task.

A task records its author, customer, assignee, optional group,
status, priority, pull request URL, block reason, parent, ordered siblings,
comments, dependencies, and principals whose answers are still required. A pull
request is an optional normalized absolute `http` or `https` URL without
embedded credentials; an empty value clears it. Priority is one
of `P0` Critical, `P1` High, `P2` Normal (the default), or `P3` Low. Root and
nested sibling sets sort by priority, then manual position, then task key.

Parent/child depth is unlimited. Assigning or authoring an ancestor exposes its
descendant subtree in the same queue to that agent; otherwise an agent sees only
nested tasks it authored, tasks assigned to it, and tasks visible through its
current group. Authoring alone does not expose a root task: see **Filing a task
into a queue** below. Ancestors needed to render an allowed descendant are
returned as read-only context. Group membership is resolved when access is
checked, not copied into the task.

A queue boundary stops both rules. Write access does not pass from a parent to
a child in another queue, and the read-only ancestor chain stops at a parent in
another queue. See [Subtasks in another queue](#subtasks-in-another-queue).

An agent explicitly mentioned in an open answer request receives response-only
access: it can read the task and add the answering comment, but cannot edit,
complete, move, or change relations unless another ACL rule also grants write
access.

`blocks` is directional and cycle-checked; `related` is symmetric. A task is
ready when it is open, unassigned, has no manual block, and has no active
incoming blocker. Completing a parent with active descendants returns a
conflict unless the caller explicitly chooses **complete anyway**. This check
counts every descendant, also one the caller cannot see. Reading one task also
returns `descendants` and `active_descendants` for the part of its subtree that
the reader can see, so a tree that keeps growing instead of closing is visible
without walking it.

The same read (`tasks.get`, also `ttasks show --json`) also returns the direct family of
the task. The visibility rule of `tasks.list` decides what the reader sees:

| Field | Content |
| --- | --- |
| `parent` | `key`, `title`, `queue`, and `access` of the parent. The field is absent for a root task and for a parent that the reader cannot see. |
| `children` | The direct children that the reader can see, from every queue and every category, closed ones included. Each item has the fields of a `tasks.list` row. The order is the tree order: priority, position, key. |
| `hidden_children` | The number of direct children that the reader cannot see. The response gives no key, title, or queue for them. |

Flexible tasks use `open`, `in_progress`, `wait_customer`, `done`, or
`cancelled` status. When the assigned agent asks the task customer a question,
the same comment transaction moves a non-terminal task to `wait_customer`.
The last customer answer returns it to `in_progress`, unless an operator made an
intervening status change. A customer comment that mentions the assigned agent
(`@agent:NAME`) also returns a `wait_customer` task to `in_progress` once no
question to the customer stays open. Questions for another principal do not
change status.

A task that follows a workflow reports the workflow status (`plan`, `review`, ...)
as `status` and one of those five values as `category`, which the daemon derives
from the status owner; `waiting_on` says whether a `wait_customer` task waits on
the `customer`, on a `script`, or on a `pause`. A `pause` is a task the daemon
stopped because it stalled (too many idle iterations, rejected requests, or
script failures, or a holder that cannot work): it keeps its status and
assignee, shows for the customer as a task in `wait_customer` with an open wait
and a comment that names the reason and the three commands to decide, and
resumes only with `ttasks workflow resume KEY --decision continue|release` or
ends with `ttasks cancel`; a plain reply does not resume it. See
[Pauses](/docs/task-workflows#pauses). The daemon assigns the task, so `ttasks done`,
`ttasks update --status`, `ttasks assign`, and `ttasks ready --claim` are
refused with `workflow_managed`. The agent or customer that owns the status
leaves it with `ttasks advance`; a transition that declares checks applies only
after the workflow's check scripts pass, and a status owned by a script is left
when its watch script reports an outcome. Status filters, blocking relations,
and the active-descendants check use the category for every task. See
[Task workflows](/docs/task-workflows).

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

The same rule applies to a child that an agent creates in a queue it does not
run (`ttasks create --parent KEY --queue OTHER`). A filed child differs in one
point: its author keeps read-only (`context`) access to it, and the create
response carries `filed: true` and `access: "context"`. The author cannot
change, claim, comment on, or complete that child.

`ttasks move KEY --to-root` detaches a task from its parent, which is the
inverse operation: the task becomes a root, and the mover loses access to it
unless it is the assignee or runs the queue. A move without `--parent`,
`--before`, or `--to-root` is refused rather than treated as a detach.

## Subtasks in another queue

A task can have a child in another queue. The child keeps its own queue, key,
workflow, and status. Only its parent link crosses the queue boundary.

Create a child in another queue with `--parent` and `--queue`:

```bash
ttasks create --parent TARI-ab12 --queue DESIGN --title "Draw the screens"
```

The creator must have write access to the parent. In the child's queue, the
ordinary rules of that queue apply:

- If the creator runs the queue, the create is ordinary. `--assignee` and
  `--group` are allowed.
- If the creator does not run the queue, a create without `--assignee` is
  filed (see **Filing a task into a queue**). A create with `--assignee` gives
  that agent the task and its same-queue descendants. `--group` is refused with
  `report_cannot_assign`.
- If the queue has a workflow, the child enters the workflow's initial status,
  and an explicit `--assignee` is refused with `workflow_managed`.

The child takes the parent's customer. It does not take the parent's group,
because a group gives write access without the tree. Without `--queue`, or with
the parent's queue, a child takes the parent's queue and group as before.

Access stops at the queue boundary:

| Who | Access to the child in another queue | Access to the parent |
| --- | --- | --- |
| An agent that writes the parent, by queue, assignment, or group | None | Unchanged |
| The child's assignee, or an agent that runs the child's queue | Write, and write on its same-queue descendants | None |
| The child's author, with no other rule | Read-only (`context`) | Unchanged |
| The customer | Write | Write |

A workflow task keeps its workflow rules: the status holder, the customer pool,
and `workflow_managed`. A visible child shows its `parent_key` even when the
reader cannot see the parent. `ttasks list --subtree KEY` returns only the
descendants that the reader can see, each with its `queue` and `status`.

Move an existing task under a parent in another queue with `--parent`:

```bash
ttasks move DEV-ab12 --parent TARILEAD-cd34
```

A move changes only the parent. The task keeps its queue, key, workflow, and
status. To change the queue of a task, use `ttasks requeue`, which creates a new
task (see [Moving a task to another queue](#moving-a-task-to-another-queue)).
These rules apply to the move:

- The mover must have write access to the task and to the new parent.
- The task and the new parent must have the same customer. Otherwise the move
  is refused with `scope_change`. A group mismatch is refused only when the new
  parent is in the same queue.
- The same-queue descendants of the task move with it. Its children in other
  queues stay under it.
- Order is kept per queue under a parent. `--before KEY` must name a task of the
  moved task's queue with the same priority. A task of another queue is refused
  with `invalid_before`. Without `--parent`, the task moves under the parent of
  the `--before` task, even when that parent is in another queue.
- `--to-root` on a task whose parent is in another queue also needs write
  access to that parent. Otherwise the owner of the child could take ordered
  work out of a tree that it does not run. For a parent in the same queue, write
  access to the task is enough.

Events for the child, such as `task.created` and `task.moved`, are recorded on
the child. Only a reader that can see the child receives them. The owner of the
parent who cannot see the child gets no event and no notification about it.

A hidden active child still blocks the completion of its parent with
`active_descendants`. The error gives only the count. The customer sees the
keys.

The export for [Moving a task to another server](#moving-a-task-to-another-server)
refuses a tree with a task in another queue with `cross_queue_subtree`. Move or
detach those tasks first.

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

The customer can also close a question without an answer, for example a
question an agent asked after the task was done.
`POST /api/tasks/{key}/waits/dismiss` (`tasks.waits.dismiss`) works for a task
in any status. Only the customer can call it; another caller gets `forbidden`
(HTTP 403). In one transaction it:

- closes each open wait on the customer, with no resolving comment;
- closes the matching `task.question` letters with
  `close_note = task-wait-dismissed`;
- moves a flexible task from `wait_customer` to `in_progress` when no question
  to the customer stays open, as an answer does. A workflow task gets the
  state an answer gives it. A done or cancelled task keeps its status;
- records a `task.waits_dismissed` event and sends each asker a
  `task.answered` letter that says no answer is needed.

The workflow's own wait on the customer stays open: an outcome or
`workflow resume` resolves it. Questions to agents stay open too. With no open
question to the customer, the call changes nothing and returns an empty
`resolved_waits`.

A mentioned name starts and ends with a letter or a digit. A dot, `_` or `-`
inside the name is part of the name. Punctuation after the name is not part of
it, so `@agent:worker.` and `(@agent:worker)` both mention `agent:worker`, and
`@agent:a.b` mentions `agent:a.b`.

A mention opens a wait only on a principal that exists. An agent principal
exists when the daemon has an agent with that name. A user principal exists
when its login is the customer login. A comment that mentions a missing
principal is saved as plain text: it opens no wait, the `mentions` of its
`task.comment_added` event do not list that principal, and the task status
does not change. `ttasks ask` on a missing principal fails with
`principal_not_found` (HTTP 400) and saves no comment. A question to a
principal opens one wait on it, also when the question text mentions that
principal again.

An older daemon also opened waits on principals that do not exist, for example
`agent:tariboy-pm.` with the closing dot of a sentence. No answer can resolve
such a wait. When `tariboyd` starts, it closes each open wait on a principal
that does not exist or that has a name a mention cannot match. It also closes
the open question letters of that wait. The daemon log gets one `Info` record
for each closed wait, with the task key, the wait ID, the principal and the
reason (`principal_not_found` or `invalid_principal_name`). One more record
gives the number of closed waits. Waits on the customer and on existing
agents stay open, and the task status does not change. If the customer login
is empty, all waits on `user:*` principals stay open.

An enabled agent normally follows one sticky assigned task as its Goal. It
releases that selection when the task is done, cancelled, loses assignment or
visibility, remains in `wait_customer` beyond the agent's configured grace
period, or eligible assigned work has a strictly higher priority. A task's pull
request URL does not affect Goal selection or release. The goal is daemon-owned
and read-only to the agent; see [Agents and the iteration
loop](/docs/architecture/iteration-loop).

Assignment, question, answer, and queue-triage events enter a transactional
outbox and are sent as [letters](/docs/architecture/messaging) from the
principal that caused them: the asker, the assigner, or `system:workflow`. An
agent notification goes to `agent:<name>`; a customer notification goes to
`user:<login>`, the configured customer login. The kinds are `task.assigned`,
`task.question`, `task.answered` and `task.triage`, and `data` carries
`task_key`, `queue` and `notification_id`. A notification therefore wakes an
enabled agent like any other letter; a disabled agent keeps it open in its
inbox.

A `task.question` letter is a request that is answered on the task, never by
mail: `tmail reply` refuses it with `answer_on_task` and names the `ttasks
comment` command. When the wait resolves, the tasks service closes the
matching question letters with `close_note = task-comment:<id>`, for an agent
recipient and the customer alike; a workflow outcome that resolves the
customer's wait closes them with `task-outcome:<status>`. The other three
kinds are notices; the agent's Goal letter drives the work.

## Desktop

**Tasks** in the sidebar opens `/tasks`, the tasks of one host. With two or
more hosts in the workspace, a host selector at the start of the toolbar
picks the host, and the URL carries it as `?host=<id>`. The number next to
**Tasks** counts the tasks that need you on every host of the workspace: a
workflow task that waits for you past its first status, or a paused workflow
task. The layout is Tree-first:

- one toolbar row that does not wrap: search, the Active/Done/All segment, a
  **Filters** menu with **Queue** (with its `Manage queues…` dialog),
  **Assignee** (**Anyone**, **My tasks**, or one agent), **Waiting for me**,
  and **Columns**, then refresh and **New task**. The **Filters** button shows
  how many filters are on;
- full width: dense expandable tree ordered by priority at every depth,
  with inline task/child creation and same-queue drag reparenting; manual before/after drag
  order is limited to the task's current priority bucket. In-progress tasks
  show an explicit **In progress** label, while open and done tasks retain
  their existing row presentation. A small red indicator marks each task with
  an unread `task.question` letter in the customer's mailbox; opening that task
  marks those letters read and clears the indicator. A question letter stays
  open until it is answered on the task. `tariboy mail inbox` lists the
  customer's open letters, the questions among them.

A task in a queue bound to a [workflow](/docs/task-workflows) shows its
workflow status, for example **Approval**, instead of one of the five fixed
labels; a long status ID is clipped and its full text is the tooltip. The
colour of the label follows the task's category (`open`, `in_progress`,
`wait_customer`, `done`, `cancelled`), not the status ID, so a closed workflow
task reads as closed whatever its last status was. Beside the label an open
task shows its step as `3/6`, its position among the workflow's statuses
(hidden on a phone), then a small **customer** or **script** marker that says
who the task waits on. A problem mark takes the marker's place when something
needs a person: red when a check could not run or the status's watch script
failed, accent-coloured when a check rejected the request; its tooltip carries
the script's words and the count behind it (see
[Attention summary](/docs/task-workflows#attention-summary)). A red
**paused** marker outranks both when the daemon paused the task (its tooltip
gives the reason). Active, Closed, and All use the category too. There is no
filter by workflow status. Flexible tasks keep the labels described above.

A [subtask in another queue](#subtasks-in-another-queue) stands under its
parent in the tree, as a subtask of the same queue does:

- If its parent is in the tree and in another queue, the **Key** cell shows a
  queue marker (a layers icon) between the chevron and the key. Its tooltip
  and accessible name give the queue, the queue name, the workflow, and the
  parent queue, for example
  `Queue TARIDESIGN · Design · workflow design-flow (parent queue TARI)`. A
  queue without a workflow reads `flexible tasks`. A root task and a subtask in
  its parent's queue have no marker.
- Each row shows the status by the model of its own task. A workflow subtask
  under a flexible parent shows its workflow status, step, and markers. A
  flexible subtask under a workflow parent shows a flexible label.
- A subtask in another queue cannot move among its siblings with the mouse. It
  cannot go inside a row of another queue either. A drop on the tree root
  detaches it from its parent and keeps its queue. A refused drop shows
  `Tasks can only move inside their queue and outside their own subtree`.
- The **+** button of a row makes the new subtask in the queue of that row. The
  form shows `Queue: {PREFIX}` in place of the queue select. The Desktop does
  not make a subtask in another queue.

The **Queue** filter keeps each task of the chosen queue with its whole
subtree from every queue. A subtask in the chosen queue under an ancestor in
that queue stays in the subtree and does not repeat as a root. Rows of other
queues in the subtree are not dimmed. A task of the chosen queue whose parent
is in another queue becomes a root. After its title it shows `in {KEY}`, a
link that opens the parent in the sheet. On a phone the link is only an icon
with a 24×24 px target. If the parent is not in the loaded list, the link is
text with the tooltip `You cannot open this task`. The parent queue then comes
from the parent key, up to its last `-`. The queue counts in the menu do not
change with the filter: each task counts only in its own queue.

Clicking a task opens a right-side sheet over the tree. The sticky header
shows the key, one title, status, and assignee. The pencil edits the title in
that same place. Save task appears only while task fields have unsaved changes.
For a workflow task, a compact decision panel sits directly below the header
in the same sticky block. It shows the current stage, owner, wait reason, and
available outcomes, including the initial customer action that starts work.
The block has a height limit and scrolls on a short screen.

Decision materials appear below the sticky block, before the description and
comments. View plan and the named material links open and focus their artifacts
there. An Outcome message belongs to the chosen outcome. To answer a question
or add a comment, use the comment form. Completed tasks offer no transitions,
and flexible tasks have no workflow decision panel.

The header of a subtask shows `Subtask of {KEY} {title}` above the title, on
one line. The line is a button that opens the parent in the same sheet. If the
parent is in another queue, a queue marker with the text
`Parent queue {PREFIX} · {name}` stands before the key. If the reader cannot
see the parent, the line is `Subtask of {KEY}` as text, with the tooltip
`You cannot open this task`.

A **Subtasks** section follows the description and comes before the comments.
Its heading gives the number of visible direct subtasks, for example
`Subtasks 3`. Each row shows a queue marker for a subtask in another queue,
the key, the title, the status as the tree shows it, and the assignee without
`agent:`. The rows come from the `children` field in tree order. Closed
subtasks stay in the list in every Active, Done, or All view. A row opens its
subtask in the same sheet. On a phone a row has two lines: the marker, key,
and status, then the title. The assignee is hidden there. If the reader cannot
see some subtasks, a line under the list says
`1 more subtask is hidden from you.` or `{N} more subtasks are hidden from you.`
The section is hidden for a task without subtasks. Two placeholder rows show
while a list loads. If a later read of the task fails, the section shows
`Could not load subtasks.` and **Retry** in place of the list. Realtime task
events refresh the section with the rest of the sheet.

Opening the parent or a subtask from the sheet asks for confirmation first
when the sheet holds unsaved drafts, as closing does. After the move, the key
of the opened task takes the focus. If that task is hidden or no longer
exists, the sheet says `You cannot open {KEY}. It is hidden from you or no
longer exists.` A task sheet opened from an agent, usage, or a letter moves
the same way.

A collapsed Details section holds the author, customer, queue, group, blocked
state, parent, flexible status, priority, assignee, pull request URL, block
reason, and dependencies. The parent is a button that opens it, or text when
the reader cannot see the parent. History is collapsed by default. The flexible status
`wait_customer` reads Waiting for you. The flexible assignee list contains
Unassigned, me, and each agent. You cannot type a principal. In the comment
form, Ask an agent mentions an agent, and Reply Ok to the question posts `Ok`.
When a question to you is open, the "Waiting for an answer" banner at the top
of the sheet has **Dismiss question**, in every status, also for a done or
cancelled task. The Needs you card has the same button.

The sheet starts at half the viewport width. Drag its left separator or use
its keyboard controls to resize it. The chosen width survives reloads.
Back, Close, Escape, or a backdrop click returns to the still-mounted tree
with its expansion and scroll position preserved. Unsaved task, comment,
relation, or outcome-message drafts require confirmation before the sheet
discards them. A pending decision prevents closure until it finishes.

A workflow task has no flexible status select, Done action, or Move to another
server action. Its lower Workflow disclosure holds the status graph,
artifacts, runs, logs, visits, and operator actions. Existing confirmations
still protect actions that override the workflow or cancel the task. In an
open, unpaused pool status, the Assignee control can reassign the holder to a
member of that pool. See [Desktop](/docs/task-workflows#desktop) in the workflow
guide. Queue bindings, pools, environment variables, and secrets remain in the
Manage queues dialog.

Descriptions and comments render as Markdown. Their visual editor supports
headings, bold, italic, strikethrough, links, lists, checklists, code, and tables.
Switch to **Source Markdown** to edit the source directly; content the visual editor
cannot represent stays in source mode so unsupported syntax is preserved.
Both modes save the same Markdown strings used by the API and `ttasks`.

**Send files** beside Description and Comment uploads selected files to the
task's server and appends their absolute paths to that editor's current draft.
It works without an assigned agent. Uploading does
not save the task or send the comment; use **Save task** or **Send comment**.
If a later file fails, paths for files already uploaded are kept in the draft.

Files are stored on disk under `$TARIBOY_BASE_DIR/files` (normally
`~/.tariboy/files`), with a unique directory per upload so matching names never
overwrite earlier files. Each file is limited to 1 GiB. Files remain after a
draft is discarded or a task is closed; remove unneeded files manually on the
server when their paths are no longer needed. Desktop streams each selected
file directly rather than creating a base64/JSON copy in memory.

The Desktop establishes a silent baseline when it first observes a host, and
again when an unavailable host recovers. Existing unread questions appear as
dots without replaying stale native notifications. A question first observed
later in the current Desktop session can send one native notification; repeated
realtime hints and identical HTTP snapshots do not send it again. Notification
permission denial or an unavailable native service does not remove the in-app
dots.

Opening a native question notification marks that exact question letter as
read and selects its task on its original server. The Desktop route is exactly
`/tasks?host=<hostId>&task=<key>` (for example,
`/tasks?host=local&task=ASK-1`); in the packaged single-page application it
appears after the URL hash as `#/tasks?host=local&task=ASK-1`. An older
`/servers/<hostId>/tasks?task=<key>` link opens the same page. Routing keeps
the notification's explicit host identity and never substitutes the local
daemon for a missing remote host. The task still opens if the read request is
interrupted, so the customer can answer it and mark the letter read later.

HTTP remains usable if realtime is disconnected. `GET /api/tasks/ws?after=N`
replays visible durable event hints after `N`, then streams new hints. The
client reconnects with its last accepted sequence and performs a full refetch
when the server requests a reset. Revision conflicts (`409`) also refetch the
authoritative tree/detail before another edit.

The Tasks toolbar starts in **Active** view. Its `GET /api/tasks` request sends
`status_view=active`, and the daemon applies that predicate to the task's
category before it builds the response. A flexible task's category equals its
status; a workflow task's category is the one its workflow status maps to.
Active returns the categories `open`, `in_progress`, and `wait_customer`;
**Closed** (`status_view=closed`) returns the finished categories, `done` and
`cancelled`; and **All** (`status_view=all`) returns every category. An omitted
`status_view` is Active when no explicit `status` filter is present;
an explicit `status` without a view keeps its legacy authoritative semantics.
When both are supplied, their predicates intersect. Other task filters and
access rules are applied alongside the selected view, so excluded rows are
never shipped to the client.
An invalid view is rejected with `400 invalid_status_view`.

Filtering by category may exclude an otherwise visible parent while retaining an
eligible child. In that response the child keeps its `parent_key`, but the
client renders it as a root-level filtered orphan; the excluded parent is never
included merely to reconstruct the tree. Switching views always refetches the
authoritative selected view, and realtime refreshes retain that selection.

### The agent's Work tab

The **Work** tab of an agent shows the tasks assigned to that agent. It does
not use the Tasks tree. At the top, a field reads "Give \<agent\> a task…".
Type a title and press Enter to create a P2 task. **Details** reveals the
queue, the priority, and the description. The queue is the last one you used
for this agent, else `TASK`. In a flexible queue the task is assigned to the
agent. In a queue bound to a workflow the task has no assignee: the composer
says that the workflow dispatches it to a pool member, and after the create it
shows a link to the task. When the agent is off, the button reads **Start &
give**: it creates the task, then starts the agent. If the start fails, the
task stays. When the agent's Goal setting is off, the composer says that the
agent does not pick up tasks and links to Settings.

Below the composer, the tab has these sections:

- **Needs you**: each card has a key and title button that opens the task sheet
  on the agent's host. Opening it sends no answer. The card's answer fields,
  outcomes, and links work independently. Closing the sheet restores focus to
  the button and retains the card's draft. Home and Inbox keep their expandable
  Needs you cards.
- **In progress**: the other active tasks.
- **Queued**: open tasks, and workflow tasks in their first status. Such a
  workflow task has one button per outcome of that status, so you can start
  it from the row.
- **Done · last 7 days**: closed tasks, with a search box for the key and the
  title. **Show all** shows every closed task.

A row shows the key, the title, the status in words (for example **Not
started** or **Waiting for you**), the age, and the cost when it is known.
Click a row to open the task sheet. The agent header's **Answer** button
opens the tab at the first task's card, and **Give task** focuses the
composer.

## Task budget

A task budget is a USD limit on the AI spending of a task and all of its
subtasks. The field `budget_usd` holds it. The value `0` means no limit, and
every task starts with `0`.

Only the customer sets the limit, with `ttasks update KEY --budget-usd 5`, with
`tariboy tasks update`, or in the **Budget, USD** field of the task panel. An
agent that sends `--budget-usd` gets `forbidden`. An agent sees the limit in
`ttasks show`. The panel shows under the field what the task and its subtasks
spent, from `GET /api/tasks/{key}/budget`.

The spending of a task is the sum of `cost_usd` of every AI request whose
`task_id` is the key of the task or of one of its subtasks. Old keys from
`task_key_aliases` count too. The limit has no time window.

Before the AI proxy sends a request upstream, it checks the task of the request
and each of its parents. If the subtree of one of them spent its limit or more,
the proxy answers 429 with `budget exceeded: task KEY` and records the request
as `budget_block`. Then the daemon reacts on the task of the request:

| Task | Reaction | How the customer lifts it |
| --- | --- | --- |
| A workflow task | The task pauses with the reason `budget_exhausted`. The pause question names the task with the limit, the spending and the limit. | Raise the limit or set `0`, then resume with `ttasks workflow resume KEY --decision continue` or `release`. While the limit is still spent, the resume fails with `budget_exhausted`. |
| A flexible task | The task gets the block reason `budget exhausted: task KEY spent $X of $Y`, and a comment by `system:budget` mentions the customer. A blocked task is not a Goal, so the agent takes other work. | Raise the limit above the spending or set `0`. The daemon then lifts, in the same write, each block in the subtree that names this task. A block with other text stays. The customer can also clear a block with `ttasks update KEY --manual-block-reason ""`. An agent cannot clear a budget block. |

A task that is already paused, blocked or closed does not change. A parent with
a limit is not blocked itself: each task of the subtree is paused or blocked at
its own first refusal.

Limits of a task budget:

- Requests that already started can go over the limit, because the proxy knows
  the cost only after the answer. Several agents in one subtree can go over it
  at the same time.
- Requests without a price, such as Cursor requests, cost `0` and are not
  limited.
- A request of an iteration without a Goal has no `task_id` and does not count.
- If the database read of the check fails, the proxy lets the request through
  and logs the error.
- A move to another queue creates a new task without the limit. A move to
  another server does not carry the limit.

## Moving a task to another queue

A task key names its queue, so a task that belongs in another queue of the same
host is moved as a new task. In the task panel, the header actions menu opens **Move to queue…** for any open task, workflow or not. The dialog
lists the other queues of this host and says what happens; for a workflow task
it says that the current workflow stops. After the move the Tasks workspace
opens the new task. The same move is `ttasks requeue KEY --queue QUEUE` for the
operator; agents cannot move tasks between queues.

The daemon does the whole move in one transaction: it creates the new task in
the target queue with the same title, description, priority, pull request,
group, and comments, and cancels the old task. If the old task followed a
workflow, its scripts stop and its questions close, as with `ttasks cancel`. If
the target queue has a workflow, the new task enters it like any new task
there; if not, the new task is a flexible `open` task. The two tasks are
related and each has a comment that names the other. A task with open subtasks
is refused (`requeue_has_children`); move or close the subtasks first. See
[Operator move, retry, and cancel](/docs/task-workflows#operator-move-retry-and-cancel)
for the details.

## Moving a task to another server

Each host keeps its own tasks, so work that belongs with another host is moved
rather than copied by hand. In the task panel, the header actions menu opens **Move to another server**, which lists the other hosts registered
in the desktop app.

The desktop app is the only component authenticated against both daemons, so it
drives the move: it exports the task tree from this daemon, imports it into the
chosen one, then comments on the task here and sets it to `cancelled`. Nothing
is deleted — the original task, its history and its events stay on the source
host — so a failure part-way leaves the task readable on both sides.

What moves: the task, its whole descendant subtree, their comments with the
original authors and times, and the relations whose two ends are both inside
that subtree. What does not move: the event log, notifications, reminders,
relations pointing outside the subtree.

This daemon refuses the export when the subtree has a task in another queue
(`cross_queue_subtree`) or a task that follows a workflow (`workflow_managed`).

The target daemon refuses the import, changing nothing, when:

- it has no queue with the same prefix (`queue_not_found`);
- a task or a retired key alias already uses one of the keys
  (`task_key_taken`).

The two endpoints behind the menu are `GET /api/tasks/{key}/export` and
`POST /api/tasks/import`. Both are customer-authenticated; agents do not
transfer tasks.

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
falling back to operator access. `tariboy tasks …` runs the same client and
always uses operator mode, also when an agent runs it, so it sees every task.
When enabled, the `tasks` capability also
provisions the bare legacy `tasks` shim. Schema-v2 images must still package the
Tasks image skill; capability selection never packages instructions
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
ttasks queue trigger --help
ttasks queue pool --help
tariboy-tasks workflow get --help
ttasks --help-json
```

Help never executes the command, resolves a daemon socket, or requires a
running daemon. It lists both modes and labels operator-only administration
and agent-only operations; showing help does not grant execution access.

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
ttasks list
ttasks ready --claim
ttasks show TEST-fkt3
ttasks create --queue TEST --title "Investigate failure" --priority P1
ttasks create --parent TEST-fkt3 --title "Add regression test"
ttasks create --parent TEST-fkt3 --queue DESIGN --title "Draw the screens"
ttasks assign TEST-fkt3 worker
ttasks update TEST-fkt3 --priority P0
ttasks update TEST-fkt3 --pull-request https://example.com/org/repo/pull/42
ttasks comment TEST-fkt3 "Found the boundary"
ttasks ask TEST-fkt3 user:login "Which behavior should win?"
ttasks move TEST-fkt3 --parent TEST-9rmq
ttasks move TEST-fkt3 --to-root
ttasks block TEST-fkt3 --by TEST-2xwv
ttasks relate TEST-fkt3 TEST-hb4k
ttasks done TEST-fkt3
```

### Listing and searching tasks

`ttasks list` lists the tasks the caller can see: in agent mode the agent's
visible tasks, in operator mode every host task. `ttasks mine` is an alias with
the same flags and output. Search is `list --text`, which matches the key,
title, and description without regard to case.

Filters combine with AND, and a comma-separated value matches any of its items:

| Flag | Selects |
| --- | --- |
| `--status S` | The status `show` prints: the workflow status of a workflow task, the stored status of a flexible task. A category value also finds workflow tasks in that category. |
| `--category C` | Only the category: `open`, `in_progress`, `wait_customer`, `done`, `cancelled` |
| `--priority P` | `P0` to `P3` |
| `--queue`, `--assignee`, `--group` | The queue prefix, the assigned agent, the task group |
| `--author`, `--customer` | A principal, `user:login` or `agent:name`; a bare name means `agent:name` |
| `--parent KEY`, `--subtree KEY` | Direct children, or every descendant, of `KEY`; not both |
| `--waiting-for P` | Tasks with an open question to `P`; `me` is the current agent |
| `--blocked true` or `false` | Blocked or unblocked tasks |
| `--view V` | `active` (the default without `--status`), `closed` (done and cancelled), or `all` |

`--sort` is `tree` (the queue tree order, the default), `updated` or `created`
(newest first), or `priority` (`P0` first, then newest). A page holds `--limit`
tasks, 50 by default and 500 at most. The output is a table of key, priority,
status, assignee, update time, and title. A `B` before the key marks a blocked
task, and a workflow task shows its category in brackets. When more tasks
remain, a last line `next:` prints the command for the next page: the same
filters with `--after KEY`. `--json` prints the page as `tasks`, `next_cursor`,
and `sequence`.

Every filter applies inside the caller's visibility. A `--parent` or
`--subtree` key the caller cannot see gives an empty list, the same as a key
that does not exist, and a subtree never shows a descendant the caller cannot
see.

```bash
ttasks list --status plan,review
ttasks list --text "login bug" --sort updated --limit 20
ttasks list --subtree TEST-fkt3 --view all
ttasks list --waiting-for me --json
```

On a task in a queue bound to a workflow, use the workflow commands instead of
`done`, `update --status`, and `ready --claim`:

```bash
ttasks workflow get TEST-fkt3
ttasks artifacts set TEST-fkt3 plan --file plan.md
ttasks advance TEST-fkt3 --outcome planned --message "Plan ready"
ttasks workflow runs TEST-fkt3
ttasks workflow log TEST-fkt3 4
```

`advance` waits for the transition's checks and prints `rejected:` with the
script's message or `failed:` with the run log to read. `workflow move`,
`workflow upgrade`, `cancel`, `queue workflow`, and `queue secret` are operator-only. See
[Task workflows](/docs/task-workflows) for the full flow.

Create, comment, and relation actions accept stable idempotency keys internally
so transport retries return the original result without duplicating a task,
comment, wait, relation, event, or notification. Relation changes also use the
same optimistic task revision guard as field and tree mutations. They require
write access to both endpoint tasks, hide relations whose other endpoint is not
visible, and emit realtime history events for both endpoints.
