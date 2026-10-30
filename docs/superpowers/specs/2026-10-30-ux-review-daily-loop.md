> Research report produced on 2026-10-30 by a designer review of the Desktop UI against a live daemon (17 agents, 295 image tags) and a fresh isolated daemon. Screenshot paths refer to a temporary directory that is not committed. This file is the evidence base for the three design documents dated 2026-10-30; the design documents, not this report, are the source of decisions.

# 03 — The daily loop (after setup)

Lens: a user with a few agents wants to (a) give an agent work, (b) see what it
is doing now and whether it is stuck, (c) answer its questions, (d) review what
it produced, (e) stop it or change a setting. Everything else is secondary.

Reviewed on LIVE (http://127.0.0.1:5181/, read-only; every non-GET request was
blocked at the browser level, so even the "mark letter read" POST that opening a
letter or a task triggers was dropped) at 1440x900 and 1024x700, Simple and
Expert mode, agents `project8-techlead`, `tariboy-developer-bob`,
`project8-dev-manny`. Screenshots:
`/tmp/claude-1000/-home-agent-github-tariboy/fdc5ee93-2fe2-4e07-8711-18b2d3ea575a/scratchpad/shots/daily/`
(referred to below as `daily/`). Baselines are in `shots/baseline/`.
FRESH was not needed for this lens.

## 1. Summary

1. **"Does it need me / is it stuck" is invisible at a glance.** 16 of 17
   sidebar rows say `idle`; the header of every agent says `idle · Goal: No
   current goal` — including `tariboy-developer-bob`, whose only task has been
   waiting 29 minutes for *the user* to approve a plan, and `project8-techlead`,
   whose task is in `Wait customer`. The one signal that exists is a 6 px blue
   dot in a task row.
2. **Giving an agent work is five steps and the agent is not even
   pre-assigned.** `New task` on the agent's own Tasks tab opens an inline form
   whose queue dropdown defaults to `DAO` (first of 11 queues alphabetically),
   creates an *unassigned* task, then opens a 2,500 px drawer where the user
   types `agent:tariboy-developer-bob` into a free-text Assignee field and
   presses `Save task`.
3. **Mailbox is plumbing shown as a primary concept.** It is the first agent
   tab and the only badge in the server bar, yet all three agent inboxes are
   empty and all three server letters are machine notices about tasks whose
   body is `@user:customer The task is waiting for you in workflow status
   "approval"…` with a button that sends you to the task anyway.
4. **Terminal is a black rectangle with nine buttons and no sentence saying
   what it is for.** Two of three agents show pure black; the third says
   `Session not running or not interactive — Start the agent and enable
   interactive mode (Settings → Runtime config)` while the header says the
   agent is started (`Stop` is offered) and Interactive is on. The word
   "Autopilot" does not appear anywhere in the app.
5. **Configuration is a 3,600 px form with 31 fields in 8 sections**, of which a
   first-week user touches four (model, effort, interval, day budget). It also
   repeats `Start / Stop / Restart` under a heading `Master switch Enabled`
   while the header already has Start/Stop.

## 2. Findings

Severity: blocker / major / minor. Routes are relative to `/agents/local/<agent>/`
unless they start with `/servers`.

### F1 (blocker) — Sidebar and header do not say "needs you" or "working"

- Where: sidebar on every route; agent header on every tab.
- Saw: `daily/project8-techlead_tasks.png`, `daily/tariboy-developer-bob_tasks.png`,
  `daily/server_tasks.png`. Sidebar: 16× `idle`, 1× `stopped`. Header for bob:
  `● tariboy-developer-bob idle · This daemon (local) · tariboy-workflow-developer:latest ·
  claude · opus[1m] · medium · Goal: No curren…`. Meanwhile the server Tasks
  page shows `DEV-xbgf · tariboy-developer · Approval 3/7 customer · 10:08`
  with a blue dot, and `P8LEAD-xv43 · project8-techlead · Wait customer`.
- Why it hurts: the daemon's `LiveState` only knows `stopped / error / running /
  idle`. "Idle" is the state of both an agent with nothing to do and an agent
  blocked on the user. The user must open every agent's Tasks tab, or the
  server Tasks page, and read the Status column to find out whether anybody
  is waiting on them. `components/AgentRow.tsx` already has an
  `unreadMessages` badge prop; `SidebarAgentRow.tsx` never passes it.
- Proposal: derive a **user-facing state** per agent in the UI from data it
  already has (agent state + its active tasks + open questions + budget +
  halt reason): `needs you` (attention fill, count), `working` (live fill,
  elapsed), `idle` (quiet text), `off` (quiet), `error` (danger). Show it in
  the sidebar row and as the one status sentence in the header (see §3).

### F2 (blocker) — Giving a task takes five steps; assignee is free text

- Where: `tasks` (agent tab) and `/servers/local/tasks`.
- Saw: `daily/agent_new_task_form.png` — the form is `[DAO ▾] [What needs to be done?] [Create task] [Cancel]`.
  Queue options: `DAO, DEV, IMPROVE, JUDGE, MANAGER, P8LEAD, PROJECT8, SYSADMIN, TARI, TASK, TT`
  (8 of them hold 0 tasks, `daily/queue_menu.png`). `useTasksWorkspaceActions.create`
  sends only `{queue, parent_key, title}`; the agent whose tab you are on is
  not used. Then the drawer opens (`daily/task_detail_P8LEAD-xv43.png`): `Assignee`
  is a plain text input holding `agent:project8-techlead`; `Save task` is top-right.
- Why it hurts: the most frequent action of the daily loop is "tell bob to do
  X". Today: New task → pick queue (wrong default) → type title → Create →
  scroll drawer → type `agent:` prefix + exact name → Save. A typo in the
  assignee silently creates an unowned task. And nothing tells the user that
  the agent only picks it up when it is started and Goal is enabled.
- Proposal: a one-line composer at the top of the agent's Work tab, `Give bob
  a task…` + Enter. Assignee = this agent, queue = the agent's last-used queue
  (first time: the default `TASK` queue), priority P2. Optional `Details`
  disclosure for description/priority/queue. If the agent is off, the row
  shows `Bob is off — Start & run`. Assignee everywhere becomes a picker of
  agents (and "me"), never a typed principal string.

### F3 (blocker) — Terminal has no purpose statement and contradicts the header

- Where: `console`.
- Saw: `daily/tariboy-developer-bob_console.png`, `daily/project8-dev-manny_console.png`
  (black screen, toolbar `Send files · Compose · Esc · Enter · Ctrl-C · ↑ · ↓ · Scrollback · ?`),
  `daily/project8-techlead_console_compose.png` (empty state: `Session not running
  or not interactive. Start the agent and enable interactive mode (Settings →
  Runtime config). [Start] [Send files]`), `daily/project8-techlead_console_help.png`
  (help text explains keystrokes, selection and scrollback, not what the
  terminal is).
- Why it hurts: the header says `idle` and offers `Stop`, Configuration says
  `Interactive (tmux TUI)` on, the terminal says "start the agent and enable
  interactive mode" and names a section (`Runtime config`) that does not exist
  (it is `Runtime`). The real reason is that this agent has `interval 0` and
  a tmux session only exists while an iteration runs (`shim launch tmux
  agent=… iteration=…` in the audit). Nothing on screen says: "the terminal
  is the live screen of the current run; between runs there is nothing to
  show." The tour's only line is `Mail, tasks and the live terminal of one
  agent.` Nothing on screen relates Terminal (talk to it now) to the loop
  (give it tasks, it works on a timer/when mail arrives). The README's
  "Interactive → Autopilot" story is absent from the product.
- Proposal: keep Terminal, add a one-line state banner above it that is
  derived from the same state as the header (`Bob is working on DEV-xbgf —
  this is its live screen`, `Bob is idle; the screen appears when a run
  starts. Give it a task, or type here to talk to it directly (not recorded
  in tasks).`, `Bob is off — Start`). Fix the empty-state copy. Hotkey buttons
  (`Esc Enter Ctrl-C ↑ ↓`) default-hidden on desktop behind `Keys`; keep
  `Send files · Compose · Scrollback · ?`.

### F4 (major) — Mailbox is system plumbing presented as the first tab

- Where: `mailbox` (first agent tab), `/servers/local/mail` (first item with a
  badge in the server bar).
- Saw: all three agent inboxes `No open letters.` with `batch 10 · inbox 0/100
  · Clear inbox · Compose` (`daily/project8-techlead_mailbox.png`). Server inbox
  (`daily/server_mail_task_question.png`): three letters, `task.answered ×2`,
  `task.question ×1`. The question letter's body is `system:workflow asked for
  an answer on DEV-xbgf — @user:customer The task is waiting for you in
  workflow status "approval". Outcomes: approved, changes_requested.
  ./statuses/approval.md`, primary button `Answer on the task`. Sent is empty.
- Why it hurts: the user is asked to learn a second inbox concept (letters,
  requests vs notices, archive, dead letters, batch) to be told to go to the
  task. The one unread badge in the whole app (`Mailbox 3`) points at the
  wrong place. The tour itself says `Agents ask you questions in the task, and
  you answer there.` Mail is the daemon's wake-up mechanism (docs: "Mail is
  the layer behind message triggers"), i.e. plumbing.
- Judgement: **not a user concept.** Fold it: (1) task questions/approvals
  live in the task and in a "Needs you" list; (2) `task.answered` notices
  become a "Done / answered" feed in the same list, auto-cleared when the
  task is opened; (3) the raw mailbox (Inbox/Sent/Archive, Compose, Clear
  inbox, dead letters) moves to Expert mode under Advanced → Mail, for
  debugging the loop.

### F5 (major) — Configuration shows 31 fields; a new user changes 4

- Where: `configuration`.
- Saw: `daily/project8-techlead_configuration_full.png` (1440×3600 full page),
  per-section `daily/tariboy-developer-bob_configuration_{runtime,loop,goal,budgets,retention}.png`.
  Counted in the DOM: 23 text inputs, 7 selects, 2 switches, 49 buttons, 31
  labels across `General, Runtime, Loop and timeouts, Goal, Mail, Budgets
  (USD), Environment, Retention and cleanup`. Top of the page: `Configuration ·
  Master switch Enabled · [Start] [Stop] [Restart] · Run actions apply
  immediately.` General shows the image digest twice (`Current` and `Next
  iteration`, 64-hex each) plus `Image digest · Read only` a third time.
- Why it hurts: the fields a first-week user changes are Model, Effort,
  Interval (on/off), Day budget and perhaps Working directory. Everything else
  (`Hard timeout`, `On timeout`, `On error`, `Maximum idle iterations`, `AI
  inactivity timeout`, `Wait customer timeout seconds`, `Goal delivery cooldown
  seconds`, `Mail: letters per iteration`, `Mail: maximum open inbox`, `Agent
  shell script`, `Secrets`, `Keep iterations`, `Keep days`, `Prune now`) is
  operator tuning with jargon helper text (`Pruned iterations are archived
  with their database rows first`). The duplicate Start/Stop strip and
  "Master switch" give the user a second, differently-worded on/off control.
- Proposal: see §3 Settings tab — three short groups and an `Advanced`
  disclosure that contains the rest unchanged. Remove the run-state strip.

### F6 (major) — Activity, the best "what did it do" view, is hidden in Simple mode

- Where: `activity` (Expert only).
- Saw: `daily/project8-techlead_activity_tasks.png`: `Tasks 11 · Completed 10 ·
  Status changes 50 · Comments 25 · Iterations 23 · Cost $13.96` and a per-task
  table with iterations, comments, cost and last activity. `…_activity_iterations.png`
  is a raw event log (`iteration_prepared {"letter_ids":[…],"prompt_sha256":…}`),
  `…_activity_usage.png` tokens and a cost chart, `…_activity_status.png` "No
  status changes in this period".
- Why it hurts: (d) "review what it produced" is answered by the Tasks sub-view
  of Activity (which task, how many iterations, how much it cost, what
  outcome), but Simple mode hides it and shows Mailbox instead.
- Proposal: Simple-mode `Activity` = the totals + the Tasks table + cost over
  time. Iterations log, Usage breakdown and Status history stay under
  Expert/Advanced.

### F7 (major) — The header carries 11 items and two jargon dialogs

- Where: header on every tab.
- Saw: `daily/project8-techlead_header_menu.png`, `daily/project8-techlead_goal_help.png`,
  `daily/project8-techlead_exec_dialog.png`, `daily/small_project8-techlead_tasks.png`.
  Items: status dot, name, `idle` pill, host, image chip, `claude · opus[1m] ·
  medium` chip, `Goal: No current goal ?`, second line `cwd: … Open in VS Code`,
  then `Exec · Stop · Kill · ⋮(Settings, Delete agent)`. The `?` opens a
  90-word dialog ("chooses an assigned Native Task by priority, then prefers
  in-progress over open tasks… Strictly higher-priority eligible work can
  preempt it. A pull request URL does not affect Goal selection"). `Exec`
  opens `Exec one-shot iteration — Runs one iteration now; the optional text
  applies only to it.` `Kill` uses a browser `window.confirm`.
  At 1024 px the `Exec Stop Kill` buttons are painted over the runtime chip.
  With a long image name (`tariboy-workflow-developer:latest`) Goal truncates
  to `Goal: No curren`.
- Why it hurts: at a glance the user needs name, state sentence, and the one
  action that fits that state. Image, digest, harness/model/effort, host and
  cwd are identity facts that belong one level down. "Goal" is an internal
  scheduler term; the user cares about "what is it working on". "Exec" and
  "Kill" are process verbs.
- Proposal: see §3 header wireframe. Keep: name, state sentence (replaces pill
  + Goal + ?), primary action (`Give task` / `Answer` / `Start`), `Stop`,
  overflow `⋯` with `Run once…`, `Open folder in VS Code`, `Force-stop current
  run` (confirmation dialog, not window.confirm), `Settings`, `Delete`. Model,
  folder and daily spend go to a muted second line.

### F8 (major) — The task drawer buries the decision 900 px down

- Where: `/servers/local/tasks` → click `DEV-xbgf`.
- Saw: `daily/task_detail_DEV-xbgf.png` (top: Author, Customer, Queue, Group,
  Blocked, Parent, Title input, Description rich-text editor with H1/H2/H3
  toolbar, Priority, Assignee, Pull request, Manual block reason, Workflow
  stepper `backlog ✓ plan ✓ approval ● implement review complete done`, a code
  block of transition rules), then `daily/task_detail_DEV-xbgf_scroll1.png`
  (the actual decision: `[approved] → implement`, `[changes_requested] → plan`,
  message box, then Artifacts with the plan the user must read, then Script
  runs, Visits, Dependencies), then `_scroll2.png` comments (`WO system:workflow
  @user:customer The task is waiting for you…` ×2) and an 11-row raw History
  (`workflow.transition_requested · actor user:customer · outcome start ·
  request_id 1016 · state applied`).
- Why it hurts: the user opened this task because it needs their decision.
  The approve/reject buttons and the plan to approve are below an editable
  description nobody is editing and a transition-rule listing. `Send Ok` next
  to `Send comment` and the `Ask [agent]` dropdown are unexplained.
- Proposal: a "Needs you" card pinned at the top of the drawer whenever the
  task waits on the user: the question or the artifact to review, the outcome
  buttons, one message box. Metadata (Author/Customer/Queue/Group/Blocked/Parent)
  collapses into a `Details` row. History and the transition-rule block go
  under Advanced/Expert.

### F9 (major) — Two Tasks and two Mailboxes on one screen

- Where: any agent tab.
- Saw: server bar `Server: This daemon (local) · Mailbox 3 · Tasks · Activity ·
  Images · Workflow images · Stores · Settings` sits above agent tabs `Mailbox
  · Tasks · Terminal · Configuration` (`daily/project8-techlead_mailbox.png`).
- Why it hurts: same words, different scope, one pixel-row apart. The user
  cannot tell which Tasks is "mine" vs "bob's". The server bar also competes
  with the titlebar's `Agents | All tasks` switch — three places to find
  tasks.
- Proposal: the server bar collapses to a single `This daemon ▾` crumb while
  an agent is open; server-level pages are reached from the sidebar's server
  header or the titlebar. `All tasks` becomes `Needs you` + tasks.

### F10 (minor) — Deep link to an agent without a tab lands on "Pick an agent"

- Where: `/agents/local/project8-techlead` (no `/tab`).
- Saw: `daily/project8-techlead_simple_default.png`: URL rewritten to `/`,
  body `Pick an agent on the left, or create one.` Same for bob and manny.
  Route table has `/agents/:hostId/:agent/:tab/*` only.
- Proposal: `/agents/:hostId/:agent` redirects to the default tab.

### F11 (minor) — Task filter bar wraps and uses ambiguous labels

- Saw: `daily/tariboy-developer-bob_tasks.png` (1440: bar wraps, `Columns`
  and `New task` fall to a second row because the agent chip is long);
  `daily/small_server_tasks.png` (1024: wraps). Labels `My tasks` vs
  `Waiting for me` vs agent chip `tariboy-developer-bob`: "my" is the user,
  the chip is the agent, nothing says so. `Waiting for me` has no count
  (`daily/server_tasks_waiting_for_me.png` shows 11 rows, 9 of which are
  `Backlog 1/7 customer` tasks that merely sit in a workflow step the user
  owns).
- Proposal: inside an agent, drop `My tasks`, the agent chip and `Queue`; keep
  search, `Needs you (n)`, `Active / Done`. Queue is a detail in the row.

### F12 (minor) — Four vocabularies for one on/off switch

- Sidebar `idle/stopped`; header pill `idle` with `Stop`; Configuration
  `Master switch Enabled` with `Start/Stop/Restart`; Terminal `Session not
  running`; docs "started". Pick one: `On / Off` for the switch, and state
  sentences for what it is doing.

### F13 (minor) — Advanced tab is fine as Expert content

- `daily/project8-techlead_advanced_{prompt,audit,raw}.png`: `Prompt · Context ·
  Files · Scripts · Full audit · Raw usage · Raw settings`. Appropriate for
  Expert. `Full audit` duplicates Activity → Iterations; keep one.

## 3. Proposed agent workspace

Principles: one derived state per agent drives sidebar, header and empty
states; the default tab answers (a)–(d); Mailbox disappears from Simple mode;
Settings shows what a first-week user changes.

### Derived state (computed in the UI from existing data)

| State | When | Fill |
| --- | --- | --- |
| `needs you · n` | any of this agent's tasks is `wait_customer`, in a customer-owned workflow status, has an open `@user` question, or the agent is `error` / out of budget / paused | attention (accent) / danger for error & budget |
| `working · 12m` | `running`; elapsed since iteration start; task key if goal set | live (green) |
| `idle` | enabled, nothing running, nothing waiting on the user | quiet text |
| `off` | disabled | quiet text |

### Sidebar row

```
┌────────────────────────────────────────┐
│ ● tariboy-developer-bob      needs you 1│   <- accent pill, count
│ ● project8-techlead      working · 12m │   <- green dot, elapsed
│ ○ project8-dev-manny                   │   <- idle: no badge at all
│ ○ image-creator-dev-sentinel       off │   <- quiet text
└────────────────────────────────────────┘
```

Rules: only `needs you` and `working` get a pill; idle rows are silent, so a
list of 17 agents reads as "two things to look at". Hover shows the task key.
Sidebar footer gets a total: `2 need you · 1 working`.

### Agent header (all tabs)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ● tariboy-developer-bob                                   [Answer]  [Stop] [⋯]│
│   Waiting for you: approve the plan for DEV-xbgf (29 min)                    │
│   claude opus · ~/github/tariboy · $8.56 today                                │
├──────────────────────────────────────────────────────────────────────────────┤
│  Work   Terminal   Activity   Settings                                        │
└──────────────────────────────────────────────────────────────────────────────┘
```

State sentence and primary button by state:

| State | Sentence | Primary |
| --- | --- | --- |
| needs you | `Waiting for you: <what> for <KEY> (<age>)` | `Answer` (opens the Needs-you card) |
| working | `Working on <KEY> since 10:08 · iteration 3` | `Give task` |
| idle | `Idle — nothing to do. Give it a task.` | `Give task` |
| off | `Off. Start to let it work on tasks.` | `Start` |
| error | `<halt_reason>` in red | `Restart` |

`⋯` menu: `Run once…` (today's Exec), `Open folder in VS Code`, `Force-stop
current run` (today's Kill, with a real dialog), `Settings`, `Delete agent`.
Removed from the header: state pill, host label, image chip, `Goal: … ?`, cwd
line, `Open in VS Code` link, `Exec`, `Kill`. Host and image move to Settings
→ About; model, folder and spend go to the muted third line.

### Tab: Work (default in both modes)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ [ Give bob a task…                                               ] [Details ▾]│
├──────────────────────────────────────────────────────────────────────────────┤
│ NEEDS YOU (1)                                                                 │
│ ┌──────────────────────────────────────────────────────────────────────────┐  │
│ │ DEV-xbgf  Desktop app: zoom hotkeys…                   approval · 29 min │  │
│ │ Bob asks you to approve the plan.                     [View plan (62 l.)]│  │
│ │ [ Message (optional)                                                   ] │  │
│ │ [Approve → implement]  [Request changes → plan]                          │  │
│ └──────────────────────────────────────────────────────────────────────────┘  │
│ IN PROGRESS (0)                                                               │
│   —                                                                           │
│ QUEUED (0)                                                                    │
│   —                                                                           │
│ DONE · last 7 days (3)                                           Show all ▸   │
│   DEV-pjnk  development workflow scripts write no diagnostic…   Wed  $4.10    │
│   …                                                                           │
└──────────────────────────────────────────────────────────────────────────────┘
```

- The composer creates a task **assigned to this agent** in its last-used
  queue; Enter submits; `Details ▾` reveals description, priority, queue.
  If the agent is off: the composer's button reads `Start & give`.
- `NEEDS YOU` cards carry the question text, the artifact to review, outcome
  buttons and one message box — the answer happens here, not in a drawer and
  not in Mailbox. The same card component is reused by the server-level
  `Needs you` list and by the titlebar bell.
- Clicking a row opens the existing task drawer, restructured: Needs-you card
  at top (if any), then title/description, then comments, then a collapsed
  `Details` (author, customer, queue, group, parent, dependencies, PR) and a
  collapsed `History` (Expert).
- Removed from this tab: `Mailbox` tab, `Active/Closed/All` segment, `Queue:`
  menu, `My tasks`, `Waiting for me`, agent chip, `Columns`, refresh button,
  `Pri`/`Key`/`Duration` columns (key stays as a muted prefix). Search stays.

### Tab: Terminal

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ⓘ Bob is idle. This screen shows its live run; it appears when a run starts.  │
│   Type here to talk to bob directly — this is not recorded in a task.         │
│ ┌──────────────────────────────────────────────────────────────────────────┐  │
│ │                                                                          │  │
│ │                            (xterm)                                       │  │
│ │                                                                          │  │
│ └──────────────────────────────────────────────────────────────────────────┘  │
│ [Send files] [Compose] [Scrollback]                        [Keys ▾]      [?]  │
└──────────────────────────────────────────────────────────────────────────────┘
```

- Banner text comes from the derived state (`working on DEV-xbgf — live
  screen`, `off — Start`, `no interactive terminal — enable in Settings`).
- `Keys ▾` reveals `Esc Enter Ctrl-C ↑ ↓` (shown by default on touch
  devices, i.e. the Android client).
- The `?` help gets a first paragraph: *Terminal is for working with the
  agent by hand. For work it should carry out on its own, give it a task in
  Work; it runs it when it is on.* (This is the Interactive/Autopilot
  relationship, in product words.)

### Tab: Activity (Simple mode)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ [Today] [Week] [Month]                                                        │
│ Tasks done 10   Runs 23   Cost $13.96   Avg per task $1.40                    │
│ ┌────────────────────────────────────────────────────────────────────────┐    │
│ │ Task        Title                         Outcome      Runs  Cost  Last│    │
│ │ P8LEAD-xv43 spider: comments from vc.ru   wait customer  2  $1.50 10:01│    │
│ │ PROJECT8-jz2d spider: show comment link   done           1  $0.38 09:51│    │
│ └────────────────────────────────────────────────────────────────────────┘    │
│ Expert: [Runs log] [Usage] [Status history]                                   │
└──────────────────────────────────────────────────────────────────────────────┘
```

Rename "Iterations" to "Runs" in user-facing copy; keep the API term in Expert
views.

### Tab: Settings (replaces Configuration)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Settings                                                     [Save changes]   │
│                                                                               │
│ Model                                                                         │
│   Harness   [claude ▾]      Model [opus[1m]      ]   Effort [medium   ]        │
│   Takes effect on the next run.                                               │
│                                                                               │
│ Working folder                                                                │
│   /home/agent/github/tariboy                            [Change…] (agent off) │
│                                                                               │
│ Autopilot                                                                     │
│   (•) Only when given tasks or messages                                       │
│   ( ) Also every [  30 ] minutes                                              │
│   Stop a run after [ 120 ] min                                                │
│   [x] Interactive terminal                                                    │
│                                                                               │
│ Spending limit                                                                │
│   Per day   [ 0  ] USD (0 = none) · spent $8.56 today                         │
│   Per month [ 0  ] USD (0 = none) · spent $366.02 this month                  │
│                                                                               │
│ ▸ Advanced (image & digest, schedule, hard timeout, on timeout/error,         │
│   idle limit, AI inactivity, Goal timing, mail limits, hour/week budgets,     │
│   environment script, secrets, retention & cleanup)                           │
│                                                                               │
│ About: image tariboy-workflow-developer:latest · host This daemon (local)     │
│ Danger: [Delete agent]                                                        │
└──────────────────────────────────────────────────────────────────────────────┘
```

- Removed: the `Master switch Enabled · Start/Stop/Restart` strip (the header
  owns on/off), `Search settings`, `Changed only (0)`, the section chip row,
  the `Current / Next iteration` digest preview, `Image digest (read only)`,
  `Copy digest`.
- `Advanced` keeps every existing field and helper text verbatim, collapsed,
  so Expert users lose nothing. In Expert mode `Advanced` is expanded by
  default.
- "Interval 0 seconds" becomes a radio in words; "Timeout 7000 s" becomes
  minutes.

### Where Mailbox goes

- Agent tab: removed in Simple; in Expert it is `Advanced → Mail` (Inbox /
  Sent / Archive, Compose, Clear inbox, dead letters) for debugging the loop.
- Server bar `Mailbox 3`: becomes a titlebar bell `Needs you (1)` listing
  Needs-you cards across agents plus a muted "answered / done" feed; opening a
  card marks its letters read (what the Mailbox does today). The raw server
  mailbox lives under `Settings → Advanced → Mail`.
- `Compose` to an agent (a free-form letter) becomes `⋯ → Send a message…`
  in the agent header, Expert only. In Simple mode "talk to it" is Terminal
  and "give it work" is a task.

### Fewest steps for the five loop actions

| Action | Today | Proposed |
| --- | --- | --- |
| Give bob work | Tasks tab → New task → fix queue → title → Create → drawer → type `agent:…` → Save (7) | Work tab → type title → Enter (2) |
| Is it stuck / needs me | open each agent → Tasks → read Status column | sidebar pill `needs you 1`, header sentence |
| Answer a question | Mailbox 3 → letter → Answer on the task → scroll 900 px → outcome | header `Answer` or sidebar pill → Needs-you card (1–2 clicks) |
| Review what it produced | Expert mode → Activity → Tasks sub-view | Work tab `DONE`, Activity totals |
| Stop / change model | header Stop; Configuration → scroll to Runtime | header Stop; Settings → Model (first group) |

### Removed, merged, renamed (summary)

- Removed from Simple: Mailbox tab, Goal `?` dialog, `Exec`/`Kill` as top-level
  buttons, Configuration run-state strip, settings search/chips/changed-only,
  digest previews, hotkey buttons by default, task table columns `Pri`,
  `Duration`, filter chips `My tasks`/`Queue`/agent chip, raw History in the
  drawer.
- Merged: Mailbox → Needs-you cards in Work and the titlebar bell; Tasks +
  (agent) Mailbox → Work; Activity → Iterations and Advanced → Full audit →
  one Runs log; server bar Tasks + titlebar All tasks → one `Needs you / All
  tasks` page.
- Renamed: Configuration → Settings, Iterations → Runs, Exec → Run once…,
  Kill → Force-stop current run, Goal → "Working on …" sentence, Interactive
  (tmux TUI) → Interactive terminal, `Master switch` → gone, `Wait customer`
  → `Waiting for you`.

## 4. Open questions for the product owner

1. May the UI give each agent a default queue (last used, falling back to
   `TASK`) so the composer can create an assigned task in one step, or must
   the queue remain an explicit choice every time?
2. Does any human use Mail as mail (Compose to an agent, replies) in the daily
   loop, or is every letter either a task notice or a loop trigger? If the
   latter, it can leave Simple mode entirely.
3. Which conditions should count as "needs you" in the sidebar pill:
   `wait_customer`, customer-owned workflow status (today `Backlog 1/7
   customer` makes 9 untouched backlog tasks look urgent), open `@user`
   mention, error, out of budget, pause — all, or a subset?
4. Is `Kill` (force-stop the current run while leaving the agent on) a Simple
   mode action, or is `Stop` enough for a first-week user?
5. For interactive agents with interval 0, is it acceptable that Terminal is
   empty between runs, or should the daemon keep a persistent session so the
   tab always has a shell to talk to?
