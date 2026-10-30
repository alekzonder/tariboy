> Research report produced on 2026-10-30 by a designer review of the Desktop UI against a live daemon (17 agents, 295 image tags) and a fresh isolated daemon. Screenshot paths refer to a temporary directory that is not committed. This file is the evidence base for the three design documents dated 2026-10-30; the design documents, not this report, are the source of decisions.

# Tariboy Desktop UX review — Navigation and information architecture

Lens: navigation and IA. Method: read-only Playwright inventory of LIVE
(http://127.0.0.1:5181/, 17 agents, 295 image tags across 32 image names,
2 stores, 1 team), cross-checked against `ui/src` (routes.tsx,
ServerContextBar.tsx, sidebar/, agents/, settings/). FRESH was not needed for
this lens and was not used. Screenshots live under
`/tmp/claude-1000/-home-agent-github-tariboy/fdc5ee93-2fe2-4e07-8711-18b2d3ea575a/scratchpad/shots/nav/`
(referred to below as `nav/…`); baseline shots under `…/shots/baseline/`.

## 1. Summary

- The owner is right. On the agent Tasks tab a user sees **five stacked
  horizontal bars** (titlebar, server bar, agent header, agent tabs, tab
  toolbar) and roughly **40 clickable navigation controls** before any content
  (`nav/12-agent-simple-tasks.png`). In Expert mode on Activity it is seven
  bars (`nav/41-agent-expert-activity-iterations.png`).
- There is no home. `/` is a blank island that says "Pick an agent on the
  left, or create one" (`nav/01-home-simple.png`). Nothing on the first screen
  says what needs the user's attention, what is running, or what to do first.
- The same concept is reachable by 2–3 names in 2–3 places: Tasks ×3, Mailbox
  ×2, Hosts/Daemons/"Add host"/"Add a server" ×4 (two of them are literally
  the same React component), Groups vs Teams, Images vs Workflow images vs
  Stores, "Default" workspace vs "THIS DAEMON (LOCAL)" vs "1 server".
- Settings is a dumping ground: 13 entries over 3 levels, two of them
  placeholders ("will appear here"), six hidden behind an "Advanced" index
  page that only shows a sentence (`nav/30-settings-advanced.png`).
- The Images page cannot be used with real data: 32 rows, no search, no
  sort, no "used by" column, build/import forms pinned above the list, and
  25 of the 32 image names are used by no agent at all.

## 2. Inventory

### 2.1 Surfaces and every control on them

**Titlebar** (`nav/01-home-simple.png`, `nav/02-titlebar-workspace-dropdown.png`,
`nav/03-titlebar-help-menu.png`)

| Control | Label / aria | What it does |
| --- | --- | --- |
| Sidebar toggle icon | "Hide agents" | Collapses the sidebar (`nav/16-sidebar-hidden-agent.png`) |
| "Default ▾" | "Workspace: Default" | Menu: WORKSPACES → "Default · 1 host ✓", "New workspace…", "Manage workspaces…" (opens a 720px "Manage workspaces" dialog). Workspaces are a localStorage grouping of hosts. |
| Segmented "Agents / All tasks" | "View" | Only on `/` and `/agents/*`. "All tasks" replaces the whole shell, sidebar included, with a cross-server task table (`nav/04-home-all-tasks.png`). |
| ? icon | "Help" | Menu: Take the tour, What's new, Changelog |
| Gear icon | — | Navigates to `/app-settings` (`nav/31-app-settings.png`): the same three Help buttons again, Interface mode Simple/Expert, Desktop updates |
| Sun icon | "Toggle theme" | Light/dark |

**Sidebar** (`nav/06-sidebar-servers-tab.png`, `nav/05-sidebar-agents-tab.png`,
`nav/07-sidebar-agent-row-context-menu.png`, `nav/08-sidebar-server-heading-context-menu.png`)

| Control | What it does |
| --- | --- |
| "Search agents ⌘K" | Filters the agent list |
| Tabs "Servers" / "Agents" | Servers = agents grouped by host, then Teams / Individual agents. Agents = one flat list of 17, no groups, no host. |
| `+` right of the tabs ("Add agent") | Opens New agent dialog |
| Heading "THIS DAEMON (LOCAL) ● READY" | Link to `/servers/local/mail` (the first server section). No context menu on right-click (empty). Remote hosts get a `⋯` menu (Edit host, Remove host) and an "Update" button; the local host gets neither. |
| `+` right of the heading ("new agent on This daemon (local)") | Opens the same New agent dialog as the tab `+` |
| "Teams" → "▾ project8" → 3 member rows | Team row links to `/agents/local/teams/project8` (`nav/11-team-page.png`: three cards + "Manage team", which goes to Settings → Advanced → Groups). No context menu. |
| "Individual agents" → 14 rows with state chips | Row click opens the agent. Right-click: Pin, Clone. |
| Footer "1 server · 17 agents" · green dot · `+` ("Add host") | Dot title "1 of 1 servers connected". `+` opens Add host dialog — the third `+` in a 260px column, and the only one that does something different. |

**Server context bar** (`baseline/servers_local_*.png`) — appears inside the
island on every server route *and* above every agent workspace:
`Server: This daemon (local)` then **Mailbox (3) · Tasks · Activity · Images ·
Workflow images · Stores · Settings**.

**Agent header** (`nav/12-agent-simple-tasks.png`, `nav/13-agent-header-kebab-menu.png`):
status dot, name, state pill "idle", host "This daemon (local)", image chip
`developer:latest`, runtime chip `claude · opus[1m] · medium`, "Goal: No
current goal ?", second line `cwd: … Open in VS Code`; buttons **Exec, Stop
(or Start), Kill, ⋮** → menu "Settings" (= Configuration tab), "Delete agent".

**Agent tabs**
- Simple: Mailbox · Tasks · Terminal · Configuration (opens on Tasks).
- Expert (`localStorage app:ui-mode:v1=expert`, `nav/40-agent-expert-console.png`):
  Mailbox · Tasks · Terminal · **Activity** · Configuration · **Advanced**
  (opens on Terminal).
- Activity sub-views (`nav/41-agent-expert-activity-*.png`): period
  Hour/Day/Week/Custom, 6 stat tiles, then Tasks · Iterations · Usage ·
  Status history; Iterations has its own tag filter, type filter, search,
  Copy, Export and a two-pane log.
- Advanced views (`nav/42-agent-expert-advanced-*.png`): Prompt · Context ·
  Files · Scripts · Full audit · Raw usage · Raw settings.
- Configuration (`nav/15-agent-simple-configuration.png`): its own
  Start/Stop/Restart cluster (a second lifecycle control 80px under the
  header's), "Search settings", "Changed only (0)", 8 section chips: General,
  Runtime, Loop and timeouts, Goal, Mail, Budgets (USD), Environment,
  Retention and cleanup.
- Mailbox (`nav/14-agent-simple-mailbox.png`): Inbox/Sent/Archive, "batch 10 ·
  inbox 0/1000", Clear inbox, Compose.
- Tasks: Search, Active/Closed/All, Queue, My tasks, Waiting for me, agent
  chip, Columns, refresh, New task — the same 9-control toolbar as the server
  Tasks page plus one chip.
- Terminal: Send files, Compose, Esc, Enter, Ctrl-C, ↑, ↓, Scrollback, ?.

**Settings left nav** (`nav/30-settings-general.png` … `nav/30-settings-advanced-groups.png`)
General · Hosts · CLI · Appearance · Operations · Advanced; INTEGRATIONS:
Telegram; and, only once you are inside Advanced, OPERATOR TOOLS: Usage ·
Budgets · Plugins · Ops · Daemons · Groups.
- General: Global Agent Shell Script, Codex approvals, Backup & data
  retention (long page).
- Hosts: "No remote hosts yet" + Add host. `routes.tsx` mounts `DaemonsPage`
  here **and** at `advanced/daemons` — identical page, two names
  (`nav/30-settings-hosts.png` vs `nav/30-settings-advanced-daemons.png`).
- CLI, Appearance: placeholders — "…will appear here" (`nav/30-settings-cli.png`).
- Operations: table of build jobs (one row).
- Advanced: a sentence. Six real pages are one level deeper.
- Ops: Backup / Restore (`nav/30-settings-advanced-ops.png`). "Ops" and
  "Operations" are siblings with unrelated meanings.
- Groups: Create group, "New group (wizard)", Groups list (project8), Import
  team (`nav/30-settings-advanced-groups.png`). The sidebar calls the same
  thing "Teams".

**/app-settings** (`nav/31-app-settings.png`): a full-screen page outside the
shell with "Back to workspace", Help (3 buttons duplicated from the ? menu),
Interface mode, Desktop updates.

### 2.2 The IA as it is (depth = clicks from the shell, `[n]`)

```
Shell
├── Titlebar
│   ├── [1] Hide agents
│   ├── [1] Workspace "Default ▾"
│   │   ├── [2] Default · 1 host
│   │   ├── [2] New workspace…  → [3] Manage workspaces dialog
│   │   └── [2] Manage workspaces…
│   ├── [1] Agents | All tasks            (All tasks hides the sidebar)
│   ├── [1] ? → [2] Take the tour / What's new / Changelog
│   ├── [1] ⚙ → [2] /app-settings (Help ×3, Simple|Expert, Updates)
│   └── [1] Theme
├── Sidebar
│   ├── [1] Search agents
│   ├── [1] Servers | Agents tabs      [1] + Add agent
│   ├── [1] THIS DAEMON (LOCAL)  → server Mailbox     [1] + new agent
│   ├── [1] Teams → project8 → [2] team page → [3] Manage team (→ Settings/Advanced/Groups)
│   ├── [1] Individual agents ×14 → [2] agent     (right-click: Pin, Clone)
│   └── [1] footer "1 server · 17 agents" ● [1] + Add host
├── Server bar (on every server and agent route)
│   ├── [1] Mailbox → [2] Inbox | Sent | Archive, Compose → [3] letter
│   ├── [1] Tasks   → [2] 9-control toolbar → [3] task
│   ├── [1] Activity → [2] Hour|Day|Week|Custom, 6 tiles, table → [3] task
│   ├── [1] Images  → [2] Build form, Import form, 32-row table
│   │                → [3] image name (tags table, Run Agent / Export per tag)
│   │                → [4] tag detail: Run Agent, Export, Remove
│   │                     → [5] Overview | Template | Skills | Files
│   ├── [1] Workflow images → [2] Build form, 16-row table, Upload to servers ×16
│   │                       → [3] workflow detail
│   ├── [1] Stores  → [2] Add Store form, 2 rows
│   │              → [3] store: Refresh, Remove store, Agent images | Workflow images,
│   │                   auto-build interval, Save, per-image Auto ☐ + Build
│   └── [1] Settings → [2] General | Hosts | CLI | Appearance | Operations | Advanced
│                      [2] Integrations: Telegram
│                      [3] Advanced → Usage | Budgets | Plugins | Ops | Daemons | Groups
│                      [3] Operations → [4] operation log
│                      [4] Groups → [5] New group (wizard) / Import team
└── Agent workspace  (server bar stays above it)
    ├── [2] header: Exec, Stop/Start, Kill, ⋮ → [3] Settings, Delete agent
    ├── [2] Mailbox → [3] Inbox|Sent|Archive, Compose, Clear inbox → [4] letter
    ├── [2] Tasks → [3] same toolbar as server Tasks → [4] task
    ├── [2] Terminal → [3] 9 terminal buttons
    ├── [2] Configuration → [3] Start|Stop|Restart, search, 8 section chips
    ├── [2] Activity (Expert) → [3] period, tiles, Tasks|Iterations|Usage|Status history
    │                         → [4] Iterations: tag, type, search, Copy, Export, log
    └── [2] Advanced (Expert) → [3] Prompt|Context|Files|Scripts|Full audit|Raw usage|Raw settings
```

Counts: 3 top-level nav bars, 7 server sections, 4–6 agent tabs, 13 Settings
entries, 7 Advanced views, 4 Activity views, 4 image-detail tabs. Deepest
everyday path: Settings → Advanced → Groups → wizard = 5; Images → name →
tag → Template = 5. The onboarding tour has 13 steps (`lib/onboarding/tour.ts`)
because it has to explain all of this.

## 3. Findings

### F1 (blocker) — No home screen
Where: `/`. Seen: `nav/01-home-simple.png`, `nav/50-small-home.png`.
A blank island with one sentence and a "New agent" button. The three letters
waiting in the inbox, the two tasks in "Approval", the 17 agents' states: none
of it is here. The user's first decision ("what do I do now?") is answered by
a 17-row list of names like `tariboy-release-publisher-postmerge-check`.
Proposal: make `/` a real Home: "Needs you" (questions, approvals, open
letters), "Running now", "Recent", and one primary action. See §4.

### F2 (blocker) — Three navigation bars compete for the same job
Where: every agent route. Seen: `nav/12-agent-simple-tasks.png`.
Titlebar (workspace, view switch), sidebar (server heading, tabs, 3 plus
buttons) and the server bar (7 sections) all sit on screen at once, and the
server bar stays above the agent header even though the agent is the subject.
A new user does not know whether "Tasks" in the server bar and "Tasks" in the
agent tabs are the same thing (they are the same table with one filter chip,
`nav/12` vs `baseline/servers_local_tasks.png`).
Proposal: one primary nav (sidebar), one secondary (tabs of the open object).
Remove the server bar; its sections become sidebar items (Tasks, Inbox,
Library, Settings) or move into Settings.

### F3 (major) — Tasks lives in three places, Mailbox in two
Where: `/?view=all`, `/servers/local/tasks`, `/agents/…/tasks`;
`/servers/local/mail`, `/agents/…/mailbox`.
What a new user thinks: "All tasks" = everything, server Tasks = everything on
this server, agent Tasks = this agent's tasks. They are right, but with one
server the first two are identical lists (`nav/04` vs `baseline/servers_local_tasks.png`)
and "All tasks" throws away the sidebar, which reads like a different app.
Server Mailbox shows system letters (workflow questions, task answers); agent
Mailbox shows that agent's queue with "batch 10 · inbox 0/1000" and "Clear
inbox" — internals a first-week user should not meet.
Proposal: one "Tasks" item at level 1 with Server and Agent as filters; agent
"Tasks" tab stays as the same component pre-filtered. One "Inbox" at level 1
(what needs me); the agent's queue becomes an "Inbox" tab in plain words
("3 letters waiting for this agent"), batch/queue numbers go to Settings.

### F4 (major) — Hosts are named four ways and configured in two identical pages
Where: Settings → Hosts; Settings → Advanced → Daemons; sidebar footer `+`
"Add host"; tour step "Add a server"; sidebar heading "THIS DAEMON (LOCAL)";
footer "1 server"; titlebar workspace "1 host".
`routes.tsx` lines 75 and 88 mount `DaemonsPage` at both `settings/hosts` and
`settings/advanced/daemons` (`nav/30-settings-hosts.png`, `nav/30-settings-advanced-daemons.png`).
A new user thinks Daemons is something lower-level than Hosts. It is not.
Proposal: one word, "Host" (or "Computer"); one page, Settings → Hosts; delete
`advanced/daemons`; the sidebar heading reads "This Mac" / the host label, not
"THIS DAEMON (LOCAL)".

### F5 (major) — Groups vs Teams
Where: sidebar "Teams"; team page "Manage team" → Settings → Advanced →
Groups → "Create group", "New group (wizard)", "Import team"
(`nav/11-team-page.png`, `nav/30-settings-advanced-groups.png`).
Same object, two names, and the management page is five levels deep under
"Operator tools" although the sidebar presents teams as everyday structure.
Proposal: "Teams" everywhere. Team page gets Add member / Set lead / Export;
creation moves into the New agent dialog ("Add to team") and a "New team"
button in the sidebar Teams header. Delete the Groups settings page.

### F6 (major) — Images / Workflow images / Stores form a three-page maze for one task
Where: server bar sections 4–6. Seen: `nav/21-server-images-top.png`,
`baseline/servers_local_workflows.png`, `baseline/servers_local_stores.png`,
`nav/24-store-detail-tariboy.png`.
What a new user thinks: Images = templates I can run; Workflow images = ?
(a tag-per-row table with 16 "Upload to servers" buttons and a "Bound to"
column of `DEV, PROJECT8, TT`); Stores = app store. Actually Stores are the
*source* of images, Images is the *built* cache, Workflow images are task
pipelines unrelated to agent creation. The quickstart's "Add tariboy Store →
build basic → New agent" crosses all three plus the dialog.
Proposal: one "Library" item: Images tab (built images with their source
store, "Get more" button that adds the default store), Workflows tab,
Sources (stores) as a sub-section of Images. See §4.4.

### F7 (major) — Images page is unusable at 295 tags / 32 names
Where: `/servers/local/images`. Seen: `nav/21-server-images-top.png`,
`nav/21c-server-images-bottom.png`, `nav/22b-image-tags-developer-top.png`.
- No search, no sort, no filter; 32 rows, alphabetical only.
- Two forms (Build from directory, Import runnable image) take the first
  330px; the list starts below the fold at 1024×700 (`nav/53-small-images.png`).
- Columns are `Latest image_version`, `Latest built at`, `Newest built tag`,
  `Tag built at` with raw ISO timestamps (`2026-10-30T05:57:52.717825807Z`).
  Nothing says *which agents use this image*. The API returns it: of 32 names
  only 7 are used by the 17 agents; 25 are scratch builds
  (`faust-developer-improve98-candidate`, `tariboy-release-publisher-base134`…).
- "Used by" appears only on the tag page, as text under `latest`
  ("Current: smm-boss-dev, tariboy-dev-homer, tt-dev-alice"), with a Source
  column that wraps to one character per line ("Sourc e CWD unava ilabl e").
- `tariboy-image-creator` alone has 57 tags; the tag page shows them all
  with a Run Agent and Export button each.
Proposal: list of image *names* as cards or rows: name, description from the
Tariboyfile, latest version, source store, "Used by N agents", last built as
a relative time; search box; "Show unused" filter off by default; Build /
Import behind one "Add image ▾" button. Tag history collapsed on the detail.

### F8 (major) — Store detail is an operator console, not a catalog
Where: `/servers/local/stores/tariboy`. Seen: `nav/24-store-detail-tariboy.png`,
`nav/25-store-detail-extra.png`.
Path shown twice; "Refresh" and "Remove store" are the only top actions; an
auto-build interval input and a 900px-wide "Save automatic builds" button sit
above the list; each row has an "Auto" checkbox and a "Build <name>" primary
button; failures are raw `lstat … no such file or directory` strings. Nothing
tells a new user what `basic` *is* or that it is the one to start with.
Proposal: catalog rows with name, one-line description, version, status
(Built / Update available / Not built) and one "Build" or "Use" action;
auto-build and refresh under a `⋯` menu; errors as a short sentence with
"Details".

### F9 (major) — Settings is three levels deep and half placeholder
Where: `/servers/local/settings/*`. Seen: `nav/30-settings-advanced.png`,
`nav/30-settings-cli.png`, `nav/30-settings-appearance.png`.
13 entries; CLI and Appearance say "will appear here"; "Advanced" is an index
sentence hiding Usage, Budgets, Plugins, Ops, Daemons, Groups; "Operations"
(build log) and "Ops" (backup) are unrelated siblings; General mixes a shell
script, Codex approval policy and nightly retention.
Proposal: a flat list of 7 (see §4.5), no placeholders, no "Advanced" index.

### F10 (major) — The "Default" workspace dropdown and the Agents/All tasks switch are top-level noise
Where: titlebar. Seen: `nav/02-titlebar-workspace-dropdown.png`.
The first control in the window is a dropdown whose only entry is "Default ·
1 host". A new user thinks it is a project or an account. It is a
localStorage grouping of SSH hosts. The view switch is a second top-level nav
that appears and disappears by route.
Proposal: remove both from the titlebar. Host groups move to Settings → Hosts
(only shown with ≥2 hosts); "All tasks" becomes the sidebar "Tasks" item.

### F11 (minor) — Three `+` buttons in the sidebar, two of them identical
Where: sidebar. Seen: `nav/06-sidebar-servers-tab.png`.
`+` after the tabs and `+` after the server heading both open New agent; `+`
in the footer opens Add host. Same glyph, same size, 40px apart, different
meaning only in the third case.
Proposal: one "New agent" button, text not glyph, at the top of the sidebar;
Add host lives in Settings → Hosts.

### F12 (minor) — Lifecycle controls duplicated and mislabeled
Where: agent header and Configuration tab. Seen: `nav/15-agent-simple-configuration.png`.
Header: Exec / Stop / Kill / ⋮; Configuration: Start / Stop / Restart /
"Master switch Enabled". "Exec" and "Kill" are Unix verbs. The header "Stop"
turns the agent off; "Kill" interrupts the current run; the Configuration
"Stop" is the same as the header one.
Proposal: one cluster in the header: **Start/Stop** (the switch), **Run now**
(Exec), **Interrupt** (Kill) under `⋮` or as a secondary button. Remove the
Configuration cluster.

### F13 (minor) — Help lives in two places; app settings leaves the shell
Where: `?` menu and `/app-settings` (`nav/31-app-settings.png`).
Take the tour / What's new / Changelog are in the `?` menu and again as
buttons on a full-screen page reached by the gear. The gear page has no
sidebar and a "Back to workspace" link, so it feels like leaving the app.
Proposal: delete `/app-settings`; "Interface" and "Updates" become two cards
in Settings; Help stays only in `?`.

### F14 (major) — 1024×700 crowding
Seen: `nav/50-small-home.png`, `nav/51-small-agent-console.png`,
`nav/51b-small-agent-tasks.png`, `nav/52-small-server-tasks.png`,
`nav/53-small-images.png`, `nav/54-small-settings-advanced.png`.
- Agent header: **Exec / Stop / Kill overlap the runtime chip**; the text
  reads "clau Exec u Stop Kill" (`nav/51-small-agent-console.png`). This is a
  layout bug, not just crowding.
- Agent Terminal: the terminal gets 445px of 700 (titlebar 40 + server bar
  38 + header 70 + tabs 36 + footer bar 50 taken by chrome).
- Agent Tasks and server Tasks: the toolbar wraps to two rows ("Waiting for
  me" and the agent chip fall to row 2), the task title column shrinks to
  ~90px ("spider: Была …") so no task is readable.
- Images: list starts at y=400; four rows visible; 32 rows below.
- Sidebar: only 10 of 17 agents visible; the team block takes 150px.
- Settings → Advanced: the 13-item left nav reaches the bottom of the window.
Proposal: the lean shell in §4 removes two bars (server bar, titlebar view
switch) and the Configuration lifecycle cluster; fix the header to wrap the
chips under the name below 1200px instead of letting the buttons overlay.

## 4. Proposed lean IA

Principles: one primary nav, one secondary nav for the open object, nothing
deeper for everyday work; one noun per concept; the host disappears while
there is only one; everything operational goes to Settings; everything
diagnostic goes behind a single "Advanced" disclosure inside the object that
owns it, not into the nav.

### 4.1 Proposed tree (same `[n]` notation)

```
Shell
├── Titlebar: [1] ☰ sidebar toggle · app name · [1] Search ⌘K · [1] ? Help · [1] theme
├── Sidebar (primary nav, fixed)
│   ├── [1] Home
│   ├── [1] Agents  (list: teams as collapsible groups, then agents; host badge only if ≥2 hosts)
│   │   ├── [1] New agent (text button, top of list)
│   │   └── [2] <agent>   (secondary nav = tabs)
│   │       ├── [2] Terminal   (default when Interactive is on)
│   │       ├── [2] Tasks
│   │       ├── [2] Inbox
│   │       ├── [2] History    (iterations, usage, status — the old Activity, calmer)
│   │       └── [2] Settings   (the old Configuration; ends with ▸ Advanced: prompt preview,
│   │                           files, scripts, raw)
│   ├── [1] Tasks   (all; filters: Host · Team · Agent · Status · Queue; "Waiting for me" default)
│   ├── [1] Inbox   (all letters and questions that need a person; Sent / Archive as filters)
│   ├── [1] Library
│   │   ├── [2] Images     (built names; "Get images" opens Sources)
│   │   │   └── [3] <image>  (About · Template · Skills · Versions; Run agent)
│   │   ├── [2] Workflows  (names; versions collapsed)
│   │   └── [2] Sources    (stores; "Add tariboy Store" one click)
│   └── [1] Settings (flat; no sub-index page)
│       ├── [2] Hosts            (local + SSH hosts; host groups only when ≥2)
│       ├── [2] Teams            (create, lead, members, import/export)
│       ├── [2] Usage & budgets  (usage tables + budget rules)
│       ├── [2] Agent defaults   (shell script, Codex approvals, retention)
│       ├── [2] Integrations     (Telegram + plugin settings; plugin install here)
│       ├── [2] Maintenance      (backup/restore, build log, operations)
│       └── [2] Application      (interface options, updates, CLI install)
└── Sidebar footer: [1] host status line ("Local · ready") → Settings → Hosts
```

Everyday path depth: Home 1, an agent's terminal 2, a task 2 (+ drawer), the
image to build 3. Nothing a first-week user needs is deeper than 2.

### 4.2 What is removed, merged, hidden, renamed

Removed outright
- Server context bar (7 sections) — sections redistributed below.
- Titlebar "Default ▾" workspaces menu and "Manage workspaces" dialog.
- Titlebar "Agents / All tasks" switch.
- Sidebar "Servers / Agents" tabs (the flat Agents view is the default; grouping by host is automatic with ≥2 hosts).
- Two of the three sidebar `+` buttons.
- Settings → CLI and Appearance (placeholders).
- Settings → Advanced index page; Settings → Daemons (duplicate of Hosts); Settings → Groups (becomes Teams); Settings → Ops (becomes Maintenance).
- `/app-settings` page.
- Configuration tab's Start / Stop / Restart cluster.
- Agent "Advanced" tab as a tab.
- Server Activity page (its tiles move to Home; its table is Tasks with a period filter).

Merged
- All tasks + server Tasks + agent Tasks → one Tasks table, three entry points, one component.
- Server Mailbox + agent Mailbox → Inbox (global) and Inbox (agent tab).
- Images + Workflow images + Stores → Library.
- Settings → Hosts + sidebar footer `+` + tour "Add a server" → Settings → Hosts.
- Usage + Budgets → Usage & budgets.
- Operations (build log) + Ops (backup) → Maintenance.
- Help (? menu) + Help card on /app-settings → ? menu only.

Hidden (progressive disclosure, not a mode)
- Prompt preview, Context, Files, Scripts, Full audit, Raw usage, Raw settings → "Advanced" disclosure at the end of the agent's Settings tab.
- Mail queue internals (batch, 0/1000, Clear inbox) → agent Settings → Mail.
- Store auto-build interval and per-image Auto checkbox → `⋯` on the source.
- Image tag history (57 tags) → "Versions" section on the image, collapsed to the latest 5.
- Host groups ("workspaces") → Settings → Hosts, shown only when there are 2+ hosts.

Renamed (plain words)
| Today | Proposed |
| --- | --- |
| Mailbox | Inbox |
| Workflow images | Workflows |
| Stores | Sources (under Library → Images: "Get images") |
| Configuration | Settings (agent) |
| Activity (agent) | History |
| Exec | Run now |
| Kill | Interrupt |
| Daemons, Servers, "THIS DAEMON (LOCAL)", "1 server" | Hosts; "Local" |
| Groups | Teams |
| Ops | Maintenance → Backup |
| Operations | Maintenance → Build log |
| Operator tools | (gone) |
| Master switch Enabled | On / Off |
| Global Agent Shell Script | Shell script run before each iteration |
| Latest image_version / Newest built tag / Tag built at | Version · Built (relative) |

### 4.3 Wireframes

Home (`/`), 1440 wide, sidebar 240:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ☰  Tariboy                                           [Search ⌘K]   ?    ☼    │
├────────────┬─────────────────────────────────────────────────────────────────┤
│ ● Home     │  Good morning                                  [ + New agent ]  │
│   Agents   │                                                                 │
│   Tasks  2 │  Needs you (5)                                                  │
│   Inbox  3 │  ┌───────────────────────────────────────────────────────────┐  │
│   Library  │  │ ? DEV-xbgf  Desktop app: zoom hotkeys…   approval · bob   │  │
│   Settings │  │ ? P8LEAD-xv43  spider: comments…      wait customer · lead│  │
│            │  │ ✉ workflow  Question on DEV-xbgf                   10:08  │  │
│            │  │ ✉ project8-techlead  Answer on P8LEAD-44p…         08:27  │  │
│            │  │ ✉ project8-techlead  Answer on P8LEAD-xv43         09:48  │  │
│            │  └───────────────────────────────────────────────────────────┘  │
│            │                                                                 │
│            │  Agents                      Today                              │
│            │  ┌──────────────────────┐   ┌───────────────────────────────┐   │
│            │  │ ● 0 running          │   │ 35 tasks · 32 done · $145.23  │   │
│            │  │ ○ 16 idle            │   │ 101 iterations                │   │
│            │  │ ■ 1 stopped          │   │                    [History ▸]│   │
│            │  └──────────────────────┘   └───────────────────────────────┘   │
│            │                                                                 │
│            │  Recent                                                         │
│            │  tariboy-dev-homer · idle · developer   project8 · 3 agents     │
│            │  tariboy-developer-bob · approval       sysadmin · idle         │
├────────────┼─────────────────────────────────────────────────────────────────┤
│ Local ● ok │                                                                 │
└────────────┴─────────────────────────────────────────────────────────────────┘
```

Agent open (`/agents/local/tariboy-dev-homer/terminal`):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ☰  Tariboy                                           [Search ⌘K]   ?    ☼    │
├────────────┬─────────────────────────────────────────────────────────────────┤
│   Home     │  ● tariboy-dev-homer   idle                 [ On ] [Run now] ⋮  │
│   Agents   │  developer:latest · claude opus · ~/github/tariboy · Goal: none │
│  [New agent]│                                                                │
│  ▾ project8│  Terminal   Tasks   Inbox   History   Settings                  │
│    techlead│ ───────────────────────────────────────────────────────────────  │
│    manny   │  ┌───────────────────────────────────────────────────────────┐  │
│    duck    │  │                                                           │  │
│  jerry     │  │                                                           │  │
│  jack      │  │                 (terminal, full height)                   │  │
│  bob       │  │                                                           │  │
│ ●homer     │  │                                                           │  │
│  publisher │  │                                                           │  │
│  …         │  └───────────────────────────────────────────────────────────┘  │
│   Tasks  2 │  [Send files] [Compose]            Esc Enter Ctrl-C ↑ ↓   ?     │
│   Inbox  3 │                                                                 │
│   Library  │                                                                 │
│   Settings │                                                                 │
├────────────┼─────────────────────────────────────────────────────────────────┤
│ Local ● ok │                                                                 │
└────────────┴─────────────────────────────────────────────────────────────────┘
   ⋮ menu: Interrupt current run · Clone · Pin · Open in VS Code · Delete agent
   Settings tab ends with:  ▸ Advanced (prompt preview, context, files, scripts, raw)
```

Two bars above the content instead of five. The agent list stays visible in
the sidebar under "Agents" so switching agents is one click, as today.

Library (`/library/images`):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ☰  Tariboy                                           [Search ⌘K]   ?    ☼    │
├────────────┬─────────────────────────────────────────────────────────────────┤
│   Home     │  Library                                                        │
│   Agents   │  Images   Workflows   Sources                                   │
│   Tasks  2 │ ───────────────────────────────────────────────────────────────  │
│   Inbox  3 │  [Search images…]   ☐ Show unused (25)          [Add image ▾]   │
│ ● Library  │                                                   Build from dir │
│   Settings │  ┌────────────────────────────────────────────┐  Import archive  │
│            │  │ developer            0.22.1 · tariboy      │  Get from source │
│            │  │ General coding agent…  Used by 3 agents    │                  │
│            │  │                              [Run agent ▸] │                  │
│            │  ├────────────────────────────────────────────┤                  │
│            │  │ tariboy-workflow-developer  0.7.1 · tariboy│                  │
│            │  │ …                            Used by 7     │                  │
│            │  ├────────────────────────────────────────────┤                  │
│            │  │ basic  ⚠ update available   0.5.0 · tariboy│                  │
│            │  │ Starter image            Used by 0         │                  │
│            │  └────────────────────────────────────────────┘                  │
│            │                                                                 │
│            │  Empty state (fresh install):                                   │
│            │  "No images yet. [Add the tariboy Store] to get `basic` and     │
│            │   other starter images."                                        │
├────────────┼─────────────────────────────────────────────────────────────────┤
│ Local ● ok │                                                                 │
└────────────┴─────────────────────────────────────────────────────────────────┘
```

Image detail: About (description, plugins, skills) · Template · Versions
(collapsed, latest 5, "Show all 57") · [Run agent]. Sources tab: each store
as a card (name, URL, N images, Refresh under `⋯`), rows with "Build" /
"Update" / "Built ✓", and the default store offered when none is registered.

### 4.4 Where Images / Stores / Workflows go, and the creation flow

They go under Library, and the New agent dialog becomes the main place a new
user meets images: its Image field is a picker that lists built images with
description and "used by", and when nothing is built it shows "Add the
tariboy Store and build `basic`" inline, performing both steps without leaving
the dialog. The Library exists for browsing and maintenance, not as a
prerequisite step.

### 4.5 Settings

Seven flat entries (Hosts, Teams, Usage & budgets, Agent defaults,
Integrations, Maintenance, Application). Per-host settings (shell script,
Codex policy, retention) are grouped under "Agent defaults" with a host
selector shown only when there are ≥2 hosts. Plugin install and the Telegram
page live together under Integrations.

### 4.6 Simple vs Expert mode: drop it

Today the mode is a switch on a page reached by the gear, hides exactly two
tabs (Activity, Advanced), and changes which tab an agent opens on (Tasks in
Simple, Terminal in Expert, `lib/uiMode.ts`). A new user never finds the
switch, does not know what it hides, and the quickstart tells them to "Open
Activity", which Simple hides. A global mode also means every screen needs
two designs.

Replace it with structure: everyday things are tabs (Terminal, Tasks, Inbox,
History, Settings), diagnostic things are an "Advanced" disclosure at the end
of the agent's Settings tab and of the image detail. The default tab follows
the agent, not a mode: Terminal when Interactive is on, Tasks when it is off.
If a mode must stay, keep it as a single "Show advanced sections" checkbox
in Settings → Application and nothing else.

## 5. Open questions for the product owner

1. Can "Stores" be invisible for the first run — the default `tariboy` store
   registered automatically, with Library → Sources only for adding more?
2. With a single host, may the host disappear from the UI entirely (no
   "Server:" label, no "1 server", no host badge on agents)?
3. Are host "workspaces" (the Default dropdown) used by anyone today? If not,
   can they be removed rather than moved?
4. Which of the 25 unused image names on LIVE are scratch builds that an
   "unused" filter should hide by default, and should the daemon expose a
   description per image (from the Tariboyfile) so the list can show one?
5. Is Workflow images a feature a first-week user should see at all, or
   can it be hidden until a store exposes a workflow?
