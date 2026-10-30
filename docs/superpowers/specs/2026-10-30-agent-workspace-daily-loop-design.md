# Agent workspace for the daily loop

Project 3 of 3 in the Desktop simplification series. Builds on the merged
projects 1 (`2026-10-30-zero-setup-first-run-design.md`) and 2
(`2026-10-30-lean-navigation-design.md`). Research:
`2026-10-30-ux-review-daily-loop.md`.

## Intent

After setup, a user with a few agents wants to give an agent work, see
whether it is working or stuck, answer its questions, review what it
produced, and stop it or change its model. Today:

- The sidebar and the header say `idle` for 16 of 17 agents, including one
  whose only task has waited 29 minutes for the user's approval. The one cue
  is a 6 px dot in a task row.
- Giving an agent work takes seven steps, defaults the queue to the
  alphabetically first of eleven, creates an unassigned task, and then needs
  `agent:<name>` typed into a free-text field in a 2,500 px drawer.
- Mailbox is the first agent tab, yet every agent inbox is empty and every
  server letter is a system notice that points at a task.
- Terminal is a black rectangle with nine buttons; its empty state names a
  settings section that does not exist and contradicts the header.
- Configuration shows 31 fields in 8 sections, 3,600 px tall, with a second
  Start/Stop/Restart cluster under "Master switch".
- Activity, the one view that says what the agent did and what it cost, is
  hidden in Simple mode.

Target: a derived per-agent state that drives the sidebar, the header, and
every empty state; a Work tab where a task is given in one line and a
question is answered in place; Settings cut to what a first-week user
changes; Mailbox out of the everyday path.

## Goals

- One derived state per agent: `needs you · n`, `working · 12m`, `idle`,
  `off`, `error`. It is shown in the sidebar row and as one sentence in the
  header with one primary action.
- Agent tabs: Work (default), Terminal, Activity, Settings. Mailbox and
  Advanced are reachable from Settings, not tabs.
- Work: a one-line composer that creates a task for this agent, "Needs you"
  cards with the question or the artifact and the outcome buttons in place,
  then In progress, Queued, Done.
- Terminal: a state banner that says what the screen shows and what to do;
  hotkey buttons behind "Keys" on desktop.
- Activity in both modes: totals, the per-task table, cost; runs log, usage,
  and status history as secondary views.
- Settings: Model, Working folder, Autopilot, Spending limit, then Advanced
  holding every other field unchanged.
- The task drawer opens on the decision when the task waits on the user.
- The Simple/Expert interface mode is removed; progressive disclosure
  replaces it.
- `/agents/:hostId/:agent` without a tab opens the default tab.

## Non-goals

- Changing the daemon's agent state machine, Goal selection, dispatch, mail
  delivery, or the task model. Everything here is derived in the UI from
  existing data unless a decision below says otherwise.
- A persistent terminal session between runs for interval-0 agents. The
  banner explains the current behavior.
- The server-level pages, the sidebar structure, routes outside the agent
  workspace. Project 2.
- The Android client beyond keeping it building; the hotkey buttons stay
  visible there.

## Anti-goals

- Losing any Expert capability. Every view that is removed as a tab keeps a
  route and a link from Settings → Advanced.
- A "needs you" that cries wolf. A task sitting in the workflow's initial
  customer status (for example `backlog`) is not "needs you"; it is queued
  and not started.
- A composer that silently creates a task nobody will pick up.

## Constraints

- `AGENTS.md`: typed results, OpenAPI and generated types, frontend
  thresholds, bundle budget, Knip, `make check`.
- The Tasks Playwright suite (`npm run test:tasks-browser`) and the
  responsive suite cover the Work tab and the drawer; both stay green.
- The tour anchors (`agentTabs`, `agentConfiguration`) and the project 1
  contextual one-liners are updated to the new tab names.
- Documentation updated in the same change: `docs/docs/architecture/web-ui.mdx`
  (interface mode, agent dialog and configuration, agent tabs, Activity,
  Mailbox sections), `docs/docs/quickstart.mdx` sections 5 to 7,
  `docs/docs/autopilot.mdx`, the Desktop sections of `docs/docs/tasks.mdx`
  and `docs/docs/task-workflows.mdx`, and `docs/docs/reference/mail.md` where
  it names the Desktop Mailbox.
- No version bump.

## Sequencing

This project is planned and built on `main` after projects 1 and 2 are
merged. Everything below that names a project 1 or 2 element (the sidebar
footer, the Tasks badge, Home's "Needs you" list, the terminal status line,
the contextual one-liners) refers to code that exists by then. Additive
fields on list results (`AgentRow`, the task list item) are in scope when a
decision below needs them; they follow the API contract rules and are the
only daemon changes.

## Decisions

### D1. Derived agent state

Computed in the UI from the agent view, its assigned tasks, and open
questions, in this order; the first that matches wins.

| State | When | Shown as |
| --- | --- | --- |
| `error` | agent state `error`, or a `halt_reason`, or a budget `exhausted` | danger pill, the reason in the header |
| `needs you · n` | n > 0, where n counts this agent's tasks with `waiting_on` equal to `customer` whose status is not the workflow's initial status, plus this agent's tasks with `waiting_on` equal to `pause`, plus open questions addressed to the user on this agent's tasks | attention pill with the count |
| `working · 12m` | agent state `running`; the elapsed time of the current iteration | live pill |
| `off` | agent disabled | muted text |
| `idle` | otherwise | no pill |

- "This agent's tasks" means tasks whose assignee is this agent. A task that
  waits on a watch script (`waiting_on: script`) never counts. A task in the
  workflow's initial customer status (for example `backlog`) never counts;
  it is queued and not started.
- The sidebar footer and the Tasks badge of project 2 use the same predicate
  (sum of n over agents).
- Data: the task list item gains the fields the predicate needs when it
  lacks them: whether the task's status is the workflow's initial status,
  and the outcomes of the current status when it is customer-owned (for the
  Queued row buttons in D4). `AgentRow` gains the start time of the current
  iteration so 17 sidebar rows do not need 17 status polls. Both are additive
  list fields through the contract rules.
- Sidebar row: name, then the pill at the right. Idle rows carry no pill, so
  a list of 17 agents reads as "two things to look at". The `unreadMessages`
  badge prop that `AgentRow` already has is removed if nothing passes it.

### D2. Header

```
● tariboy-developer-bob                                [Answer] [Stop] [⋯]
  Waiting for you: approve the plan for DEV-xbgf (29 min)
  claude opus · ~/github/tariboy · $8.56 today
──────────────────────────────────────────────────────────────────────
  Work   Terminal   Activity   Settings
```

| State | Sentence | Primary |
| --- | --- | --- |
| needs you | "Waiting for you: <what> for <KEY> (<age>)"; with several, "Waiting for you: n things" | Answer (scrolls Work to the first card) |
| working | "Working on <KEY> since <time> · run <n>"; without a task, "Working since <time>" | Give task (focuses the composer) |
| idle | "Idle. Give it a task." | Give task |
| off | "Off. Start to let it work on tasks." | Start |
| error | the reason, in red | Restart |

- Second button: Stop when on, Start when off. The agent on/off switch has
  this one place.
- `⋯` menu: Run once… (today's Exec dialog), Force-stop current run (today's
  Kill, with a real confirmation dialog instead of `window.confirm`), Open
  folder in VS Code, Clone, Pin, Settings, Delete agent.
- Removed from the header: the state pill, the host label, the image chip,
  the runtime chip, "Goal: … ?" and its help dialog, the cwd line. The third
  line shows harness and model, the folder (truncated from the left), and
  today's spend when a budget or spend exists. Host and image are in Settings
  → About.
- The header never overlaps: below 1200 px the sentence and the third line
  wrap, and the buttons stay in their own row.

### D3. Tabs and routes

| Tab | Route | Content |
| --- | --- | --- |
| Work | `…/work` (default) | D4 |
| Terminal | `…/console` (kept) | D5 |
| Activity | `…/activity` | D6 |
| Settings | `…/settings` (`…/configuration` redirects) | D7 |

- `…/mailbox` and `…/advanced` keep their routes and content but are not
  tabs; Settings → Advanced links to them ("Open mailbox", "Prompt preview",
  "Context", "Files", "Scripts", "Full audit", "Raw usage", "Raw settings").
  The old `…/tasks` route redirects to `…/work`.
- `/agents/:hostId/:agent` with no tab redirects to `…/work`.
- The interface mode is removed: `uiMode.ts`, the Application settings
  switch, and `app:ui-mode:v1` reads. The stored key is ignored. The docs
  section "Interface mode" is replaced by a paragraph on progressive
  disclosure.

### D4. Work

```
[ Give bob a task…                                          ] [Details ▾]
NEEDS YOU (1)
┌ DEV-xbgf  Desktop app: zoom hotkeys…                 approval · 29 min ┐
│ Bob asks you to approve the plan.                      [View plan]     │
│ [ Message (optional)                                                ]  │
│ [Approve → implement]   [Request changes → plan]                       │
└────────────────────────────────────────────────────────────────────────┘
IN PROGRESS (0)
QUEUED (1)      TASK-12  Add a README                 not started  [Start]
DONE · last 7 days (3)                                       Show all ▸
  DEV-pjnk  development workflow scripts…        Wed   $4.10
```

- Composer: Enter creates a task with the typed title, priority P2, in the
  agent's default queue, and assigned to this agent. The default queue is the
  queue the user last used in this composer for this agent, stored in
  `localStorage`, else `TASK`. "Details" reveals description, priority, and
  queue. When the chosen queue is bound to a workflow, the create request
  carries no assignee (the daemon refuses one with `workflow_managed`) and
  the composer says "Dispatched by the <name> workflow; a pool member takes
  it" under the field; after the create it shows an inline line "<KEY>
  dispatched by the <name> workflow" linking to the drawer, because such a
  task is not this agent's until the workflow assigns it. When the queue is
  flexible, the request sets `assignee` to this agent. When the agent is
  off, the button reads "Start & give" and starts the agent after creating
  the task. When the agent's Goal setting is off, the composer shows "Goal is
  off: this agent does not pick up tasks. Turn it on in Settings" with a
  link, and still creates the task. No pool-membership detection.
- "Needs you" cards: one per task that matches the D1 predicate. The card
  shows the question text (from the open question) or the status's prompt
  (from the workflow view: the outcomes and the artifact to review, for
  example the `plan`), the artifact in a collapsible viewer, one message box,
  and one button per outcome labelled "<outcome> → <next status>". Answering
  a question is a customer comment on the task (the existing path that
  resolves the wait); choosing an outcome uses the existing advance route
  with `from` set. A paused task (`waiting_on: pause`) gets the card variant
  with the pause reason and the existing Continue and Release actions. The
  same card component renders in the task drawer (D8) and on Home's "Needs
  you" list (project 2) when a row is expanded.
- Sections: In progress (category `in_progress`), Queued (open, including
  tasks in an initial customer status, whose row shows one button per
  outcome of that status, like the card, with the first declared
  non-terminal transition as the primary button), Done (last seven days,
  "Show all" switches to the full table). Each row: key as a muted prefix, title, status in words
  ("Waiting for you", not `wait_customer`), age, cost when known. A row click
  opens the existing drawer.
- Removed from this tab: the Active/Closed/All segment, the Queue menu,
  "My tasks", "Waiting for me", the agent chip, Columns, refresh, and the
  Pri, Key, and Duration columns. Search stays as a filter box in the Done
  section header.

### D5. Terminal

- A one-line banner above the xterm element, from the D1 state: "Working on
  DEV-xbgf — this is its live screen"; "Idle. The screen appears when a run
  starts. Type here to talk to the agent directly; this is not recorded in a
  task"; "Off — Start"; "No interactive terminal. Enable it in Settings →
  Model" when Interactive is off. The project 1 status line for a starting
  or failed session stays and is merged into the same banner.
- Toolbar: Send files, Compose, Scrollback, "Keys ▾" revealing Esc, Enter,
  Ctrl-C, ↑, ↓, and `?`. The Android build shows the keys expanded.
- The `?` help begins with: "Terminal is for working with the agent by hand.
  For work it should carry out on its own, give it a task in Work; it runs
  it when it is on."

### D6. Activity

- Period segment (Today, Week, Month, Custom), the totals (tasks done, runs,
  cost, average cost per task), the per-task table from today's Activity →
  Tasks sub-view, and the cost chart from Usage.
- A "More" segment at the bottom switches to Runs log (today's Iterations
  view, which absorbs Advanced → Full audit), Usage, and Status history.
- "Iterations" is "Runs" in every label; API names are unchanged.

### D7. Settings

```
Settings                                                  [Save changes]
Model
  Coding tool [Claude Code ▾]   Model [opus[1m]]   Effort [medium]
  Takes effect on the next run.
Working folder
  /home/agent/github/tariboy                         [Change…] (agent off)
Autopilot
  (•) Only when given tasks or messages
  ( ) Also every [ 30 ] minutes
  Stop a run after [ 120 ] min
  [x] Interactive terminal
Spending limit
  Per day   [ 0 ] USD (0 = none) · spent $8.56 today
  Per month [ 0 ] USD (0 = none) · spent $366.02 this month
▸ Advanced
About   image tariboy-workflow-developer:latest · host Local
Danger  [Delete agent]
```

- Advanced contains, unchanged in behavior and helper text: image selection
  and digest, schedule (RRULE), hard timeout, timeout and error policies,
  idle limit, AI inactivity timeout, Goal settings, mail limits and "Clear
  inbox", hour and week budgets, environment, shell script, secrets,
  retention and cleanup, and the links of D3 to the mailbox and the raw
  views.
- Removed: the "Master switch · Start/Stop/Restart" strip, "Search
  settings", "Changed only", the section chip row, the Current/Next iteration
  digest preview (the digest stays once, read-only, under Advanced → Image).
- Interval 0 is the first radio; a positive interval is the second with the
  value in minutes. The soft timeout is shown in minutes. The existing draft
  and save model (`useConfigDraft`, `draftSave`) is reused; the page is a new
  layout over the same fields.

### D8. Task drawer

- When the task waits on the user, the D4 card is pinned at the top of the
  drawer. Then title and description, comments, a collapsed "Details"
  (author, customer, queue, group, parent, priority, assignee as a picker of
  agents and "me", pull request, block reason, dependencies), and a collapsed
  "History" with the raw events.
- The assignee field is a picker everywhere; a typed principal string is no
  longer accepted in the UI.
- "Send Ok" and the "Ask" dropdown get labels that say what they do ("Reply
  Ok to the question", "Ask an agent").

### D9. Renames

| Today | Now |
| --- | --- |
| Configuration | Settings |
| Tasks (agent tab) | Work |
| Iterations | Runs |
| Exec | Run once… |
| Kill | Force-stop current run |
| Master switch Enabled | (removed; the header owns on/off) |
| Goal: … | "Working on …" in the header sentence |
| Wait customer | Waiting for you |
| Interactive (tmux TUI) | Interactive terminal |

## Left to the builder

- Where the derived state lives (`agentWorkspaceModel.ts` is the natural
  home) and how the sidebar reads it without loading every agent's tasks
  twice.
- Whether the task list result needs new fields for the predicate, and the
  exact names.
- The Needs-you card's artifact viewer (Markdown rendering is already
  available).
- The composer's handling of a bound default queue when the user never
  picked one: the rule above (no assignee, a note) is enough; whether to
  prefer a flexible queue automatically is the builder's call as long as the
  note is shown.
- Test strategy within `AGENTS.md`: unit tests for the state predicate
  (every row of the D1 table, initial-status exclusion), the header
  sentence and primary action per state, composer request shapes for bound
  and flexible queues, the Settings layout's field mapping, the redirects,
  and the Tasks browser suite for the Work tab and the drawer card.

## Acceptance

- With real data, the sidebar shows a pill only on agents that need the user
  or are working; an agent whose task waits for approval shows `needs you · 1`
  and its header offers Answer.
- Giving an agent a task is: open Work, type, Enter. In a flexible queue the
  task appears under Queued or In progress with this agent as assignee; in a
  bound queue the composer shows the "dispatched" line with a link, and the
  task appears under Work once the workflow assigns it to this agent.
- Approving a plan is: click the pill or Answer, read the plan in the card,
  click "approved → implement".
- Terminal never shows a black rectangle without a sentence above it.
- Settings shows four groups above the fold; every field that existed is
  still editable under Advanced with the same effect.
- No route that existed before this project returns "Pick an agent".
- `make check`, the Tasks browser suite, and the responsive suite pass.
