> Research report produced on 2026-10-30 by a designer review of the Desktop UI against a live daemon (17 agents, 295 image tags) and a fresh isolated daemon. Screenshot paths refer to a temporary directory that is not committed. This file is the evidence base for the three design documents dated 2026-10-30; the design documents, not this report, are the source of decisions.

# Tariboy Desktop: the first 15 minutes

Lens: a developer who just installed Tariboy Desktop, has never seen it, and
wants one coding agent working in one of their repos.

Method: walked the real first-run path with Playwright on the FRESH instance
(empty daemon, tour not suppressed), then compared the same screens on LIVE
(17 agents, 295 images, read-only). Screenshots are under
`/tmp/claude-1000/-home-agent-github-tariboy/fdc5ee93-2fe2-4e07-8711-18b2d3ea575a/scratchpad/shots/first-run/`
(abbreviated below as `shots/first-run/`).

Both instances responded. Network was available: the tariboy Store was added
and `developer:0.22.1` built in 25 s. The documented `basic` build did not
(see finding 1).

---

## 1. Summary

1. **The documented first path is broken.** README and quickstart say
   "Add tariboy Store, then build `tariboy/basic`". On a fresh daemon that build
   fails with HTTP 409 `reserved_image: image ref is managed by tariboyd:
   basic:latest`, and the UI only shows a toast "Build failed" with an "Open
   operation" button. A new user is stuck at step 3 of 7 with no explanation.
2. **The first screen explains nothing and the tour explains the wrong things.**
   The empty home says "Pick an agent on the left, or create one." (there is
   nothing on the left). The 13-step tour talks about servers, SSH, stores,
   queues and workflows before the user has done anything, ends on the
   "Workflow images" page, mis-counts its steps when no agent exists
   ("7 of 13" jumps to "10 of 13"), and a full-width red daemon error banner
   appears over it halfway through.
3. **"New agent" is a 1,799 px, 28-control, 5-section form for a decision with
   three real inputs** (folder, template, coding tool). Only one field is
   required (Image), but nothing says so until you scroll to the footer. Fields
   are labelled in system vocabulary (`cwd`, `harness`, `effort`, `interval
   seconds`, `goal delivery cooldown seconds`, `environment JSON`).
4. **The user must learn three concepts (store, image, harness) and do seven
   navigations before an agent exists**, and the product never pre-selects
   anything: store not connected, no template built (only `bare`), harness
   "image default", model "image default", folder "managed workdir".
5. **Giving the agent a task does not give the agent the task.** "New task"
   on the agent's own Tasks tab creates an unassigned task ("Open · agent
   unassigned"); the agent will never pick it up until the user opens the
   task's 16-field detail panel and types the agent's name into Assignee.

Count from app start to "an agent is running my task": **15 clicks + 2 typed
fields on the shortest working path (terminal), 20 clicks + 4 fields via
tasks, plus 10 extra clicks if the user follows the tour, plus 2 recovery
clicks because the documented build fails.** Full breakdown in section 4.

---

## 2. Findings

Severity: blocker / major / minor. Routes are on FRESH unless marked LIVE.

### F1. blocker: "Build basic" from the Store fails with a reserved-image conflict
- Where: `/servers/local/stores/tariboy`, button "Build basic".
- Saw: `shots/first-run/15-store-building.png` (toast "Build failed /
  Open operation"), `shots/first-run/16-store-after-build.png` (row still "Not
  built"). The daemon operation says
  `error_code: reserved_image, "image ref is managed by tariboyd: basic:latest"`
  (HTTP 409). Root cause: `internal/image/ref.go:45` reserves `bare:latest` and
  `basic:latest` for the daemon, and a Store build publishes `image_version`
  plus `latest` by default. On LIVE the same row reads "Latest image_version
  missing; comparison unavailable" (`shots/baseline/servers_local_stores_tariboy.png`),
  which is the same conflict surviving for months.
- Why it hurts: this is the exact step the README, the quickstart, the Stores
  page copy ("Images and workflows published by Tariboy") and the tour all point
  at. The toast gives no reason; "Open operation" leads to a raw error string
  with internal vocabulary. The user has no way to know that "Build developer"
  one row down would have worked.
- Proposal: either let the Store own `basic` (drop it from the reserved set,
  the daemon only needs `bare`) or rename the store image. Independently of
  that, a failed build must show the reason inline in the row ("Could not build:
  basic:latest is reserved. Build developer instead?") and never only in a toast.
  Best: the first-run flow builds the starter template itself (section 3), so
  the user never presses "Build".

### F2. blocker: the New agent dialog is a configuration dump
- Where: `/?new=1` (same on FRESH and LIVE).
- Saw: `shots/first-run/20-newagent-00.png`, `20-newagent-01.png`,
  `20-newagent-02.png` (three screens of scrolling at 1440x900;
  `scrollHeight` 1,799 px; 28 controls). Sections: Target (Host, Image),
  Identity (name, alias, group, color, notes), Runtime (Harness, Effort, Model,
  cwd, Create directory if missing, Interactive, environment JSON, plugins),
  Loop (interval seconds, soft timeout seconds, hard timeout seconds, timeout
  policy, error policy, maximum idle iterations, message batch size, maximum
  queued messages, Goal, wait customer timeout seconds, goal delivery cooldown
  seconds, standing user prompt), Lifecycle (Start now). Footer: "Choose an
  image" / "Create agent" disabled (`20-newagent-02.png`).
- Why it hurts: the only required field is Image, but the requirement is
  stated in the footer, 1,800 px below the field. Identity comes before Runtime,
  so the user fills name/alias/group/color before reaching the three things
  that matter. Every Loop field is a tuning parameter whose default is right
  for a first agent. The dialog description ("Choose a target and image, then
  configure identity, runtime, loop, and lifecycle settings") reads like a
  schema, not an instruction. At 1024x700 only two sections fit
  (`shots/first-run/37-live-newagent-1024.png`).
- Proposal: section 3.3 (one-screen flow) and section 3.5 (Advanced list).

### F3. major: the tariboy Store is not connected and "store" must be understood first
- Where: `/servers/local/stores`.
- Saw: `shots/first-run/11-stores-empty.png` / `tour-06.png`: a "Tariboy Store"
  card ("Images and workflows published by Tariboy, registered as `tariboy`
  from `https://github.com/alekzonder/tariboy-store.git`") with a blue "Add
  tariboy Store" button; below it a generic "Add Store" form with `name` and
  `Git URL or absolute directory`; below that "No Stores registered on this
  server." After adding, the list shows the store and the user must click the
  store name to find any images (`13-stores-added.png`, `14-store-detail.png`).
- Why it hurts: a first-week user does not need to know that templates come
  from a git repository. Two "add" affordances on one page (card + form) make
  the user wonder which one they need. The detail page leads with "Save
  automatic builds" and an interval input "0", an "Auto" checkbox column, and
  columns "Source image_version / Latest image_version" before the Build
  buttons.
- Proposal: register the tariboy Store on first daemon start (owner
  requirement). Stores page: drop the "Tariboy Store" card once connected; keep
  "Add Store" form folded under "Add another catalog". On the store detail,
  move the automatic-builds bar below the table and label the table
  "Templates" with columns Name, Description, Status, Build.

### F4. major: the tour is long, mis-targeted and mis-counted
- Where: first open of `/`.
- Saw: `shots/first-run/tour-01.png` to `tour-11.png`, `tour-after.png`.
  - 13 steps declared; only 11 shown because "Agent workspace" and "Agent
    settings" are skipped when no agent exists, but the counter still reads
    "7 of 13" then "10 of 13" (`tour-07.png`, `tour-08.png`).
  - Steps 2 and 3 are "Servers" and "Add a server (Connect another computer
    over SSH)" (`tour-02.png`, `tour-03.png`): the second and third things the
    product says are about multi-host setup.
  - Step 7 "Agents ... Create one from an image with this button" highlights
    the 20 px "+" at the top of the sidebar, not the big blue "New agent"
    button in the middle of the screen (`tour-07.png`).
  - From step 7 on, the red banner "tariboyd: missing required tools: tariboy,
    ttasks, tmail, claude | codex | opencode | agent; install them on the
    daemon host" sits above the tour (`tour-07.png` onwards).
  - Steps 10-12 are Tasks, Queues ("groups tasks under a key, such as DEV"),
    Task workflows ("statuses, owners and checks ... Attach one in queue
    settings"). The tour ends on the "Workflow images" page with "Build from
    directory /absolute/path/to/workflow" (`tour-after.png`), the most
    advanced empty screen in the product.
- Why it hurts: the tour is a product map, not a first task. It introduces
  nine nouns (server, daemon, section, image, store, agent, task, queue,
  workflow) in two minutes and leaves the user somewhere they should not be.
- Proposal: section 3.6. Short version: no automatic tour; the empty-state
  home is the guide; three contextual one-liners on first visit to Terminal,
  Tasks and Configuration; keep "Take the tour" in Help for the product map,
  with the counter fixed and the final step returning to the agent list.

### F5. major: raw daemon diagnostics are the first red thing a user sees
- Where: any route once the daemon reports status.
- Saw: `shots/first-run/tour-07.png`, `26-fresh-agent-created.png`: full-width
  red bar "tariboyd: missing required tools: tariboy, ttasks, tmail, claude |
  codex | opencode | agent; install them on the daemon host". Also the agent
  header error "halted: image activation failed: stub harness requires
  TARIBOY_STUB_HARNESS to point at the stub script" (`27-fresh-agent-console.png`).
- Why it hurts: on a Mac the bundled CLIs exist, so the real-world banner will
  be "missing required tools: claude | codex | opencode | agent". That is the
  single most likely first-run state (no coding CLI installed yet), and it is
  rendered as a daemon log line with pipe characters.
- Proposal: translate the tool check into a checklist on the home screen:
  "Coding tool: none found. Install Claude Code, Codex, OpenCode or Cursor CLI,
  then come back." with per-tool install links. Keep the raw string behind
  "Details".

### F6. major: harness, model and effort are never pre-selected and depend on each other invisibly
- Where: `/?new=1` Runtime section, LIVE and FRESH.
- Saw: `shots/first-run/32-live-newagent-after-image.png`: after choosing
  `developer:latest`, Harness still reads "image default", Effort and Model
  "image default". Opening Model shows "From harness: Choose a harness to list
  its values" (`33-live-newagent-model-open.png`, `34-...effort-open.png`).
  Harness options: `image default, claude, codex, opencode, cursor, stub`.
- Why it hurts: the user does not know what the image default is, whether the
  image has one, or which of the five harnesses is installed. `stub` is a test
  harness and should never be offered in the desktop UI.
- Proposal: detect installed harnesses on the host (the daemon already
  reports missing ones), show them as a radio row with "found / not found",
  pre-select the first found one, and show the model/effort that will apply as
  read-only text with an "Change" link under Advanced. Hide `stub`.

### F7. major: the image picker is a flat list of every tag
- Where: `/?new=1`, Image field, LIVE.
- Saw: `shots/first-run/31-live-newagent-image-open.png`: 295 options,
  `bare:latest, basic:0.1.1, basic:0.1.2, ... basic:0.5.0, blackjack-base:...`.
  Alphabetical by `name:tag`, so `bare` and twelve old `basic` versions come
  before anything useful; `latest` is not preferred; there is no description.
- Why it hurts: with 295 images the picker needs scrolling through dozens of
  dead tags; with 3 images (FRESH: `bare:latest, developer:0.22.1,
  developer:latest`, `22-fresh-newagent-image-list.png`) the user still has to
  understand that `developer:latest` and `developer:0.22.1` are the same thing.
- Proposal: group by name, one row per image showing name, one-line
  description from the manifest and the `latest` tag; a secondary "version"
  select only in Advanced. Put store templates (developer, reviewer) first,
  `bare` last under "Plain terminal".

### F8. major: a task created from the agent's Tasks tab is not given to the agent
- Where: `/agents/local/my-first-agent/tasks`, "New task".
- Saw: `shots/first-run/28-fresh-agent-newtask-row.png`: inline row with
  queue select `TASK` and "What needs to be done?" (good). After "Create task":
  `29-fresh-agent-task-created.png`: the detail panel opens with 16 controls
  (Author, Customer, Queue, Group, Blocked, Parent, Title, Description with a
  13-button rich-text toolbar, Status, Priority, Assignee, Pull request,
  Manual block reason, Dependencies "Blocks TEST-2 Add relation", Comments,
  Ask). The header says "Open · agent unassigned". Per
  `docs/docs/tasks.mdx`, an agent only works on an assigned task; the agent's
  Goal follows assigned tasks.
- Why it hurts: the user did exactly what the tab name and the filter chip
  "my-first-agent" suggest and got nothing. Nothing on screen says "assign it".
- Proposal: a task created from an agent's own Tasks tab is assigned to that
  agent by default (the inline row shows "for my-first-agent"). The detail
  panel opens in a reading layout (title, description, status, assignee);
  the other 12 fields go under "More".

### F9. major: after creation the user lands on an empty table, and the Terminal is a black box
- Where: `/agents/local/my-first-agent/tasks` (landing), `/console`.
- Saw: `shots/first-run/26-fresh-agent-created.png`: the new agent opens on
  Tasks with a toolbar of 9 controls (Search tasks, Active/Closed/All, Queue:
  all, My tasks, Waiting for me, my-first-agent, Columns, refresh, New task)
  and "No tasks match this view." The header shows `Exec | Stop | Kill` and
  `Goal: No current goal ?`. `27-fresh-agent-console.png`: Terminal is a black
  rectangle with 9 buttons under it (Send files, Compose, Esc, Enter, Ctrl-C,
  arrows, Scrollback, ?) and no text when the session failed to start.
- Why it hurts: the thing a developer wants to see is the agent's terminal
  with a prompt, and the thing they want to do is type. Landing on the Tasks
  tab says "this is a ticketing system". An empty black terminal says
  "something is broken" even when it is just starting.
- Proposal: land on Terminal. While the session starts, print a status line in
  the terminal area ("Starting Claude Code in ~/code/my-app..."); on failure,
  print the reason there in plain words with a Retry button. Keep "Exec" and
  "Kill" out of the header for a non-expert (section 3.5).

### F10. minor: the empty home points at nothing
- Where: `/`.
- Saw: `shots/first-run/10-home-empty.png`: "Pick an agent on the left, or
  create one." above a "New agent" button; the sidebar says "Individual agents
  / No agents."; the server bar is hidden on this route but visible everywhere
  else.
- Proposal: section 3.2.

### F11. minor: three navigation systems on one screen
- Where: every route.
- Saw: `shots/first-run/10-home-empty.png`, `26-fresh-agent-created.png`:
  titlebar segmented control "Agents | All tasks"; sidebar tabs "Servers |
  Agents | +"; server bar "Server: This daemon (local) | Mailbox | Tasks |
  Activity | Images | Workflow images | Stores | Settings"; then agent tabs
  "Mailbox | Tasks | Terminal | Configuration". A new user sees "Tasks" and
  "Mailbox" twice at two levels with different meanings.
- Proposal (for this lens only; another reviewer owns navigation): in Simple
  mode hide the server bar's "Workflow images", "Activity" and "Stores" entries
  and rename "Images" to "Templates". Show the server bar only when a server
  section is open, not above an agent.

### F12. minor: unexplained header controls and counters
- Where: agent header and Mailbox.
- Saw: `Exec`, `Stop`, `Kill`, `Goal: No current goal (?)`, "Master switch
  Enabled" (Configuration), "batch 10 · inbox 0/100" (Mailbox,
  `27-fresh-agent-mailbox.png`), "Run actions apply immediately."
- Proposal: in Simple mode show one switch "Agent: On/Off" and one button
  "Stop current work" (Kill). "Exec" moves to the overflow menu as "Open a
  shell in the agent's folder". Drop the "batch · inbox" counter from the
  Mailbox toolbar.

### F13. minor: Images page leads with authoring tools
- Where: `/servers/local/images`.
- Saw: `shots/first-run/tour-05.png`: "Build from directory" (absolute path,
  name, tag, Validate, Build) and "Import runnable image (Choose File)" above a
  one-row table (`bare 0.100.0 ... latest`). Columns: "Latest image_version",
  "Latest built at", "Newest built tag", "Tag built at".
- Proposal: list first, authoring forms behind "Build your own" at the bottom.
  Columns: Name, Description, Version, Built, "Run agent" button.

### F14. minor: 1024x700 header overlap
- Where: `/agents/local/<agent>/console` at 1024x700, LIVE.
- Saw: `shots/first-run/38-live-agent-console-1024.png`: the `Exec Stop Kill`
  buttons are drawn over the `claude · opus[1m] · medium` chip ("clau Exec u
  Stop Kill").
- Proposal: wrap the chips row under the name or move the actions to a
  second line below 1200 px.

### F15. minor: "Interactive" and "Create directory if missing"
- Where: `/?new=1` Runtime.
- Saw: toggle "Interactive: Attach a terminal console." (on) and "Create
  directory if missing: Make the cwd directory and its parents when it does
  not exist." (off). Typing a non-existent folder shows "No matching folders"
  (`shots/first-run/24-fresh-newagent-filled.png`) but does not say the create
  will be rejected without the toggle.
- Proposal: a folder field with a Browse button; if the folder does not exist,
  show inline "This folder does not exist. [Create it]". Interactive stays on
  and lives under Advanced as "Headless (no terminal)".

### What the screens did not tell me (hesitation log)

| Step | What I had to know that the screen did not say |
| --- | --- |
| Home | That "create one" needs a template first and the template comes from a "Store". |
| Tour step 2-3 | Whether I need a server. (No, the local one is already there.) |
| Stores | What a store is; that the blue card and the "Add Store" form do the same thing; that the store's images are behind the store name link. |
| Store detail | What "Source image_version / Latest image_version" are; what "Auto" does; that "Build basic" will fail and "Build developer" will not. |
| Build failed | Why it failed. (Only the operation log says.) |
| New agent: Image | That `developer:latest` and `developer:0.22.1` are the same; that `bare` means "no instructions". |
| New agent: Harness | Which of claude/codex/opencode/cursor/stub I have installed; what "image default" resolves to; what "stub" is. |
| New agent: Effort/Model | That they are empty because Harness is empty. |
| New agent: cwd | That "cwd" is the repo folder; that an absolute path is required; that a missing folder needs the toggle. |
| New agent: Loop | Whether "interval seconds 0" means "never" or "continuous". (It means timer off; messages still trigger.) |
| New agent: Goal | What a "Native Task goal" is and why "wait customer timeout" is 300. |
| Agent header | What Exec does; the difference between Stop and Kill; what "Goal" is. |
| Terminal | Whether black means starting, idle, or broken. |
| New task | That the task will not go to this agent unless I assign it. |

### Jargon encountered on the first path, with the word a first-week user would use

| On screen | Means | Say instead (first-run surfaces) |
| --- | --- | --- |
| image | agent template: prompt + skills + plugins | template (or role) |
| store | git repo of templates | catalog |
| harness | the coding CLI (Claude Code, Codex, ...) | coding tool |
| cwd | the repo folder | folder |
| effort | model reasoning level | keep, but under Advanced |
| iteration / loop / interval | one autonomous run / the schedule | Autopilot run / schedule |
| goal | the task the agent is currently assigned | current task |
| queue (TASK, DEV) | task key prefix + ownership | project / task list |
| workflow image | process definition for a queue | hide in Simple mode |
| Autopilot | timed / message-triggered runs | keep (good word) |
| bare | template with no instructions | plain terminal |
| Interactive | has a terminal session | terminal on/off |
| Exec | open a shell in the agent cwd | open shell (overflow menu) |
| Kill | stop the current run, keep agent on | stop current work |
| Master switch | agent on/off | agent on/off |
| shim | daemon-side wrapper around the harness | never show |
| daemon / tariboyd | background service | "Tariboy service" in errors only |
| image_version / digest | template version / content hash | version; hide digest |
| standing user prompt | extra instructions sent every run | extra instructions |

---

## 3. Proposed first-run design

Goal: from app open to an agent typing in my repo in under two minutes, with
no decision the user cannot make from what is on screen.

### 3.1 What happens automatically (no user action)

On first daemon start, in the background, with progress visible on the home
screen:

1. Register the tariboy Store as `tariboy` (owner requirement). Store removal
   stays possible on the Stores page.
2. Build the starter template (`developer`, or `basic` once F1 is fixed) and
   tag it `latest`. Rebuild in the background on store refresh.
3. Detect installed coding tools on the local host (claude, codex, opencode,
   cursor) using the existing tool check; remember which were found.
4. Never run the tour. Set `onboarding:state` as seen; the Help menu still
   offers "Take the tour".

The home screen shows these three as a checklist that completes by itself.
If step 2 fails, the row says why in one sentence and offers Retry; the user
can still create a plain-terminal agent.

### 3.2 Empty-state home screen (0 agents)

Replaces "Pick an agent on the left, or create one." Shown in the island when
the local host has no agents. The server bar is hidden on this route (as
today). Sidebar stays, showing "No agents yet".

```
+------------------------------------------------------------------------------+
| [=] Default v      [Agents] [All tasks]                              ? @ * |
+----------------+-------------------------------------------------------------+
| Search    cmdK |                                                             |
|                |   Welcome to Tariboy                                        |
| This computer  |   Run an AI coding agent in one of your repositories.       |
|   READY        |                                                             |
|                |   Getting ready                                             |
| No agents yet  |   [v] Tariboy catalog connected                             |
|                |   [v] "Developer" template ready                            |
|                |   [!] Coding tool: Claude Code found. Codex, OpenCode,      |
|                |       Cursor not found.                 Install another...  |
|                |                                                             |
|                |   +-------------------------------------------------------+ |
|                |   |  Run your first agent                                 | |
|                |   |                                                       | |
|                |   |  Folder      [ ~/code/my-app                ] [Browse]| |
|                |   |  Template    (o) Developer   ( ) Reviewer             | |
|                |   |              ( ) Plain terminal (no instructions)     | |
|                |   |  Coding tool (o) Claude Code   ( ) Codex  (not found) | |
|                |   |              ( ) OpenCode (not found) ( ) Cursor (nf) | |
|                |   |  Name        [ my-app-developer ]   (auto, editable)  | |
|                |   |                                                       | |
|                |   |  > Advanced (model, effort, schedule, budget, ...)    | |
|                |   |                                                       | |
|                |   |                              [ Start agent ]          | |
|                |   +-------------------------------------------------------+ |
|                |                                                             |
|                |   Later: connect another computer over SSH in Settings.     |
| 1 server  +    |                                                             |
+----------------+-------------------------------------------------------------+
```

Rules:
- The card is the same component as the new "New agent" dialog (3.3); here it
  is inline because there is nothing else to show.
- "Start agent" is disabled with an inline reason only while a checklist row
  is unfinished: "Waiting for the Developer template..." or "Install a coding
  tool first".
- Template radios come from the store's templates that are built, plus
  "Plain terminal" (`bare`). Each radio has a one-line description from the
  manifest ("Developer: implements tasks in a repo, runs tests, opens PRs").
- Coding tool radios show detection state. A tool that is not found is
  selectable but shows "Install: <link>" and keeps Start disabled.
- Name is generated from folder name + template; editable.
- If the folder does not exist: inline "This folder does not exist.
  [Create it]" (replaces the "Create directory if missing" toggle).
- With more than one host, a "Run on: This computer v" select appears above
  Folder. With one host it is not shown.

On LIVE (17 agents) this screen never appears; the sidebar list is the home.

### 3.3 "New agent" as one screen (the same card as a dialog)

Opened from the sidebar "+", the "New agent" button, "Run agent" on an image,
or `?new=1`. Works for 3 images and for 295.

```
+-----------------------------------------------------------------------+
| New agent                                                         [x] |
| Pick a folder and a template. Everything else has a sensible default. |
|                                                                       |
| Run on       [ This computer                     v ]   (only if >1)   |
| Folder       [ /Users/me/code/my-app                       ] [Browse] |
|              This folder does not exist.  [Create it]                 |
|                                                                       |
| Template     [ developer   Implements tasks, runs tests...        v ] |
|              version: latest (0.22.1)            change version >     |
|                                                                       |
| Coding tool  (o) Claude Code   ( ) Codex   ( ) OpenCode   ( ) Cursor  |
|              Model: opus (tool default)  Effort: medium   change >    |
|                                                                       |
| Name         [ my-app-developer                 ]                     |
|                                                                       |
| > Advanced                                                            |
|                                                                       |
|                                            [ Cancel ] [ Start agent ] |
+-----------------------------------------------------------------------+
```

- Template select: one row per image name (not per tag), store templates
  first, then the user's own images, `bare` last as "Plain terminal". Each row:
  name, description, newest version. Typing filters by name. The version
  sub-line expands to a tag select ("change version"). With 295 images this
  collapses the list to roughly 40 rows.
- Coding tool: radios with detection state (found / not found), pre-selected
  to the image default if that tool is found, else the first found tool.
  `stub` never shown.
- Model/Effort: shown as a one-line summary of what will apply; "change"
  reveals the two editable combos inline. Values come from the selected tool.
- "Start agent" creates and starts (Start now = on). Secondary "Create without
  starting" lives in the Advanced area.
- After Start: navigate to the Terminal tab of the new agent.

### 3.4 After "Start agent": the workspace

```
+-------------------------------------------------------------------------+
| my-app-developer  * running   developer   Claude Code     [Agent: ON ] |
| ~/code/my-app   Open in VS Code                      [Stop work] [...] |
| [Terminal] [Tasks] [Mail] [Settings]                                    |
| +---------------------------------------------------------------------+ |
| | Starting Claude Code in ~/code/my-app ...                           | |
| | $ claude                                                            | |
| | > _                                                                 | |
| +---------------------------------------------------------------------+ |
| (i) Type what you want done, like in a terminal. To hand work over      |
|     and let the agent run on its own, give it a task.   [Give a task]   |
| [Send files] [Compose] [Esc] [Enter] [Ctrl-C] [^] [v] [Scrollback] [?]  |
+-------------------------------------------------------------------------+
```

- Tabs reordered: Terminal first (it is where a new agent is used),
  Configuration renamed Settings. Mailbox renamed Mail (shorter; same thing).
- The (i) line appears once per agent until dismissed. It is the only
  "tour" a user needs at this point.
- The agent header keeps: status, template chip, tool chip, one On/Off
  switch, "Stop work" (today's Kill). "Exec", "Restart", "Remove", "Clone" go
  into the "..." menu. The `Goal:` chip is replaced by "Working on: TASK-mc8e
  Add a README" when a task is assigned, hidden otherwise.
- On start failure, the terminal area shows the reason as text, not a red
  header line: "Claude Code did not start: <reason>. [Retry] [Open settings]".

### 3.5 Create-agent fields: keep, hide under Advanced, remove

Kept on the main screen (5): Run on (only with >1 host), Folder, Template,
Coding tool, Name.

Under "Advanced" (grouped, collapsed, with these defaults):

| Field (current label) | New label | Default | Group |
| --- | --- | --- | --- |
| Model | Model | tool default (shown read-only until "change") | Coding tool |
| Effort | Effort | tool default | Coding tool |
| Interactive | Headless (no terminal) | off (i.e. Interactive on) | Coding tool |
| Image tag | Template version | latest | Template |
| plugins | Plugins | from template; read-only for schema-v2 | Template |
| environment JSON | Environment variables | none; key/value rows, not JSON | Folder & environment |
| Create directory if missing | (inline "Create it" link) | n/a | Folder |
| alias | Alias | empty | Identity |
| group | Team | none | Identity |
| color | Color | auto from name | Identity |
| notes | Notes | empty | Identity |
| interval seconds | Autopilot schedule | off (0) | Autopilot |
| soft timeout seconds | Run timeout | 2 h (7200) | Autopilot |
| hard timeout seconds | Hard stop | 3 h (10800) | Autopilot |
| timeout policy | After a timeout | restart | Autopilot |
| error policy | After an error | restart | Autopilot |
| maximum idle iterations | Stop after idle runs | never (0) | Autopilot |
| message batch size | Messages per run | 10 | Autopilot |
| maximum queued messages | Inbox limit | 100 | Autopilot |
| Goal | Follow assigned tasks | on | Tasks |
| wait customer timeout seconds | Wait for your answer up to | 5 min (300) | Tasks |
| goal delivery cooldown seconds | Re-send task after | 1 min (60) | Tasks |
| standing user prompt | Extra instructions every run | empty | Autopilot |
| Start now | Create without starting | off (Start now on) | footer |

Removed from the create dialog entirely (available later in agent Settings):
Budgets, Retention and cleanup, Mail settings, Schedule (RRULE), AI
inactivity timeout. None of them can be decided before the agent has run once.

### 3.6 What the tour becomes

Not needed as an automatic first-run step. The empty-state home plus the
one-screen flow are self-explanatory; the three concepts that still need a
sentence get it in place:

1. Terminal tab, first visit: the (i) line in 3.4.
2. Tasks tab, first visit: "A task is work the agent does on its own. Tasks
   you create here are assigned to this agent. [New task]".
3. Settings tab (today's Configuration), first visit: "Autopilot: set a
   schedule here to let the agent keep working without you. Everything else
   is optional."

Keep "Take the tour" in the Help menu for the product map, with fixes:
- The counter counts only the steps that will be shown.
- Reorder: Welcome, Agents (highlight the real "New agent" button), Agent
  workspace, Agent settings, Tasks, Templates, Catalog, Servers, Add a server,
  Queues, Workflows, Help. Multi-host and workflow steps last.
- The last step returns to where the tour started, not to Workflow images.
- Do not start the tour while a blocking daemon banner is visible.

### 3.7 Same screens with real data (LIVE, 17 agents, 295 images)

- Home: unchanged (sidebar is the home). The "Run your first agent" card is
  never shown; "New agent" opens the dialog of 3.3.
- Template select: ~40 names instead of 295 tags; typing "dev" narrows to
  `developer`, `dev-manager`, `dev-worker`, `designer`. Version change is one
  extra click for the rare case.
- Coding tool radios show the host's detection. Model/Effort summary shows
  the saved values for that tool ("Saved" group today), so a power user still
  gets their last `opus[1m] · medium` with zero typing.
- Stores page: the tariboy store row stays; "Add another catalog" folded.
- Agent header: the same On/Off switch and "Stop work"; the overflow menu
  holds Exec/Restart/Remove/Clone. Nothing a power user does today is lost,
  it moves one click.

### 3.8 Removed, merged, hidden, renamed (summary)

- Removed from first run: tour autostart; "Add tariboy Store" button (done
  automatically); "Build basic" as a user step; the "Tariboy Store" card.
- Merged: Target + Identity + Runtime + Loop + Lifecycle into one screen plus
  Advanced; "Create directory if missing" into the folder field.
- Hidden (Simple mode): `stub` harness; Exec; Workflow images; Activity;
  "Save automatic builds" above the store table; the "batch · inbox" counter;
  digests and `image_version` columns.
- Renamed (first-run surfaces and Simple mode): image to template, store to
  catalog, harness to coding tool, cwd to folder, Configuration to Settings,
  Mailbox to Mail, Kill to Stop work, Master switch to Agent On/Off, bare to
  Plain terminal, Goal to Working on.

---

## 4. Click and field count: app start to "an agent is running my task"

Measured on FRESH with the current UI, counting the shortest path a user who
already knows the answers would take. A real first user takes more because of
the hesitations in section 2.

Shortest working path (terminal):

| # | Action | Clicks | Typed fields |
| --- | --- | --- | --- |
| 1 | Tour: "Skip tour" (or 10x Next + Done = 11) | 1 | |
| 2 | Server bar: Stores | 1 | |
| 3 | "Add tariboy Store" | 1 | |
| 4 | Open store "tariboy" | 1 | |
| 5 | "Build basic" (fails, F1) | 1 | |
| 6 | Toast "Open operation" to learn why | 1 | |
| 7 | "Build developer", wait ~25 s | 1 | |
| 8 | "New agent" | 1 | |
| 9 | Image field, pick `developer:latest` | 2 | |
| 10 | Harness select, pick `claude` | 2 | |
| 11 | cwd: type the repo path | 1 | 1 |
| 12 | "Create agent" | 1 | |
| 13 | Agent lands on Tasks; click Terminal | 1 | |
| 14 | Type the instruction, Enter | | 1 |
| | Total | 15 clicks (+10 with the tour) | 2 fields |

Steps 5-6 are the recovery from F1; without the bug the path is 13 clicks.

Via tasks (what the product promotes), after step 12:

| # | Action | Clicks | Typed fields |
| --- | --- | --- | --- |
| 13 | "New task" | 1 | |
| 14 | Type the title | | 1 |
| 15 | "Create task" | 1 | |
| 16 | Open the task row | 1 | |
| 17 | Assignee: type the agent name | | 1 |
| 18 | "Save task" | 1 | |
| | Total | 20 clicks | 4 fields |

Proposed design, same goal (local host, Claude Code installed):

| # | Action | Clicks | Typed fields |
| --- | --- | --- | --- |
| 1 | Folder: Browse, pick repo (or type) | 2 | |
| 2 | "Start agent" (Developer + Claude Code pre-selected) | 1 | |
| 3 | Type the instruction in Terminal, Enter | | 1 |
| | Total | 3 clicks | 1 field |

Via tasks: + "Give a task", title, "Create" = 6 clicks, 2 fields, and the
task is assigned.

---

## 5. Open questions for the product owner

1. `basic:latest` is reserved by the daemon but the store publishes `basic`
   with a `latest` tag. Which side owns the name? Should the starter template
   be `developer` instead, or does the daemon stop reserving `basic`?
2. Which template should the first agent default to: a general "Developer"
   that works in any repo, or should the first-run card ask "What should this
   agent do?" with 2-3 roles? (Proposal assumes Developer default, Reviewer
   second.)
3. Should a task created from an agent's own Tasks tab be assigned to that
   agent automatically? This changes the "filed task" semantics in
   `docs/docs/tasks.mdx` for the UI path only.
4. Is "coding tool" acceptable as the user-facing word for harness, and
   "template" for image, in Simple mode while the CLI and docs keep the
   current terms? Or do you want one vocabulary everywhere?
5. May the daemon register the tariboy Store and build the starter template
   at first start without asking (network access at first launch), or should
   the home checklist ask once ("Connect the Tariboy catalog? [Connect]")?
