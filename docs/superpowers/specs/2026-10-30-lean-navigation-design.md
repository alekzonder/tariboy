# Lean navigation

Project 2 of 3 in the Desktop simplification series. Builds on the merged
project 1 (`2026-10-30-zero-setup-first-run-design.md`). Project 3
(`2026-10-30-agent-workspace-daily-loop-design.md`) follows and owns the agent
workspace; this project does not change the agent header or tabs. Research:
`2026-10-30-ux-review-navigation.md`.

## Intent

On the agent Tasks tab a user sees five stacked bars (titlebar, server bar,
agent header, agent tabs, tab toolbar) and about 40 navigation controls before
any content. The same concept is reachable by two or three names in two or
three places: Tasks three times, Mailbox twice, Hosts and Daemons as the same
React component on two routes, Groups and Teams as one object, three `+`
buttons in a 260 px sidebar, a "Default" workspace dropdown with one entry.
Settings has 13 entries on three levels, two of them placeholders. There is
no home: `/` says "Pick an agent on the left".

Target: one primary navigation (the sidebar), one secondary navigation (the
tabs of the open object), nothing deeper for everyday work, one noun per
concept, and a Home that says what needs the user.

## Goals

- A fixed sidebar with six items: Home, Agents, Tasks, Inbox, Library,
  Settings. The agent list lives under Agents and stays visible, so switching
  agents is still one click.
- The server context bar, the titlebar workspace dropdown, the
  "Agents / All tasks" switch, the Servers/Agents sidebar tabs, and two of the
  three sidebar `+` buttons are gone.
- Images, Workflow images, and Stores become one Library with three tabs:
  Images, Workflows, Sources.
- The Images list works with 300 tags: one row per image name, search,
  "used by N agents", unused images hidden by default, authoring forms behind
  one "Add image" menu.
- Settings is one flat list of seven pages with no placeholders and no index
  page.
- Home shows what needs the user, what is running, today's numbers, and
  recent agents. The empty-state Home of project 1 stays for zero agents.
- Every old URL still works through a redirect.
- Nothing overlaps or wraps at 1024×700.

## Non-goals

- The agent workspace (header, tabs, Configuration, Terminal, Mailbox tab,
  Activity tab, Advanced tab) and the per-agent state pill. Project 3.
- Removing the Simple/Expert interface mode. Project 3 removes it together
  with the agent tabs that it hides. This project leaves `uiMode` and the
  Application settings switch untouched.
- Host workspaces as a feature: their data model and storage stay; only
  their placement changes.
- The task drawer's content. Project 3.
- Android layout beyond keeping it building and its tests green.

## Anti-goals

- Hiding power features behind a mode. Everything that moves stays
  reachable in at most two clicks from the sidebar, or one disclosure on the
  page that owns it.
- A sidebar that needs horizontal scrolling or an icon-only mode to fit.
- Losing the drag-and-drop order of agents and teams, pinned agents, or the
  sidebar width setting.

## Constraints

- `AGENTS.md`: typed results, OpenAPI and generated types in step, frontend
  thresholds, bundle budget, Knip, `make check`. The only daemon changes are
  the additive list fields named under Sequencing and in D1 and D3; they
  follow the contract rules.
- The tour anchors in `ui/src/lib/onboarding/tour.ts` are updated for every
  moved element; `tour.test.ts` must pass.
- The responsive Playwright suite (`npm run test:responsive-browser`) covers
  every main route; new routes are added to it.
- Documentation updated in the same change: `docs/docs/architecture/web-ui.mdx`
  (the sidebar, server sections, onboarding, settings, and code-splitting
  sections), `docs/docs/quickstart.mdx`, the Desktop sections of
  `docs/docs/tasks.mdx` and `docs/docs/task-workflows.mdx`,
  `docs/docs/images/index.mdx`, `docs/docs/workflow-images.mdx`, and
  `docs/docs/remote-hosts.mdx` where they name a Desktop location.
- No version bump.

## Sequencing

This project is planned and built on `main` after project 1 is merged. The
zero-agent Home at `/`, the Sources rule for the `tariboy` card, the tour
order and anchors, and the New agent dialog that "Run agent" opens are
project 1 code and exist by then. Additive fields on list results
(`AgentRow`, the task list item) are in scope when a decision below needs
them and follow the API contract rules; project 3 reuses them. No count
endpoint is required: Home and the badges poll the selected host's active
task list at the sidebar's interval, and each host's list when there are
several.

## Decisions

### D1. The shell

```
┌────────────────────────────────────────────────────────────────────┐
│ ☰  Tariboy                                   [Search ⌘K]   ?   ☼  │
├────────────┬───────────────────────────────────────────────────────┤
│ ● Home     │                                                       │
│   Agents   │                                                       │
│   Tasks  2 │                 content island                        │
│   Inbox  3 │                                                       │
│   Library  │                                                       │
│   Settings │                                                       │
│   …        │                                                       │
├────────────┼                                                       │
│ Local ● ok │                                                       │
└────────────┴───────────────────────────────────────────────────────┘
```

- Titlebar: sidebar toggle, product name, global search (the existing agent
  search, ⌘K, now also matching tasks by key and title), Help menu, theme
  toggle. The gear icon is removed; its page is Settings → Application. The
  workspace dropdown and the view switch are removed.
- Sidebar: the six items. Agents expands in place into the agent list
  (search, "New agent" text button, Teams as collapsible groups, then
  individual agents, grouped under a host heading only when the workspace has
  two or more hosts). The list keeps pinned-first ordering, drag-and-drop
  reorder of like siblings, context menus (Pin, Clone), and the resize handle.
  The Servers/Agents tabs are removed; with one host the flat list is shown,
  with several hosts the host-grouped list is shown.
- Sidebar footer: one line with the host label and reachability ("Local ·
  ready", "2 hosts · 1 unreachable"), linking to Settings → Hosts. The
  "Update all (N)" action moves to Settings → Hosts; the footer shows a dot
  when a host is behind.
- Badges: Tasks shows the number of tasks that need the user: tasks with
  `waiting_on` equal to `customer` whose status is not the workflow's initial
  status, plus tasks with `waiting_on` equal to `pause`. The task list item
  gains a field that says whether the task is in its workflow's initial
  status (an additive list field under the contract rules; project 3 reuses
  it). Inbox shows open letters addressed to the user.
- The Agents heading is not a route: it expands and collapses the agent list
  in place and the list is expanded by default. There is no `/agents` page.
- The content island keeps its current card styling. The server context bar
  is removed everywhere.

### D2. Routes

| Sidebar item | Route | Content |
| --- | --- | --- |
| Home | `/` | D3; the project 1 empty state when the host has no agents |
| Agents | (no route) | the heading expands the list in the sidebar |
| an agent | `/agents/:hostId/:agent/:tab` | unchanged (project 3 changes the tabs) |
| a team | `/agents/:hostId/teams/:team` | unchanged |
| Tasks | `/tasks` | the existing per-host Tasks workspace for the selected host, with its `?task=` deep link, question marks, queue settings and "Manage queues…", filters and columns kept; a host selector only with two or more hosts (see D4) |
| Inbox | `/inbox` | the existing server Mailbox for the selected host, with "Needs you" first (see D4) and the same host selector rule |
| Library | `/library/images`, `/library/images/:name`, `/library/images/:name/:tag/*`, `/library/workflows`, `/library/workflows/:name/:tag`, `/library/sources`, `/library/sources/:name` | D5 |
| Settings | `/settings/hosts`, `/settings/teams`, `/settings/usage`, `/settings/agent-defaults`, `/settings/integrations/:plugin?`, `/settings/maintenance`, `/app-settings` | D6 |

- Host scope: Tasks, Inbox, Library, and the Settings pages that are per
  host (Agent defaults, Maintenance, Operations) show a host selector in
  their page header only when the workspace has two or more hosts. The
  selected host is remembered in the existing active-daemon storage. With
  one host nothing about hosts is shown.
- Host in links: every host-scoped route accepts an optional `?host=<id>`.
  A link the UI builds itself (image links from an agent's configuration,
  operation toasts, notification links) carries `?host=` when the workspace
  has two or more hosts. A route with `?host=` naming a host other than the
  remembered one selects that host through the existing selection path and
  shows the existing connecting and failed states of the route host
  boundary while it does. Redirects from the old host-exact URLs translate
  the path host into `?host=`.
- Every current route redirects: `/servers/:hostId/mail` → `/inbox`,
  `/servers/:hostId/tasks` → `/tasks`, `/servers/:hostId/activity` → `/`,
  `/servers/:hostId/images/*` → `/library/images/*`,
  `/servers/:hostId/workflows/*` → `/library/workflows/*`,
  `/servers/:hostId/stores/*` → `/library/sources/*`,
  `/servers/:hostId/settings/*` → the matching `/settings/*` page
  (`hosts` and `advanced/daemons` → `/settings/hosts`, `advanced/groups` →
  `/settings/teams`, `advanced/usage` and `advanced/budgets` →
  `/settings/usage`, `operations*` and `advanced/ops` → `/settings/maintenance`,
  `advanced/plugins` and `integrations/*` → `/settings/integrations`, `cli`,
  `appearance`, and `advanced` → `/settings/hosts`). The older legacy
  redirects already in `routes.tsx` keep working through the new targets.
- Host workspaces (the former titlebar dropdown): the manager dialog opens
  from Settings → Hosts, and the selector appears there only when two or more
  workspaces exist. The sidebar shows the current workspace's hosts as today.

### D3. Home

Shown at `/` when the host has at least one agent.

```
Home                                                    [ + New agent ]

Needs you (5)
┌──────────────────────────────────────────────────────────────────┐
│ DEV-xbgf   Desktop app: zoom hotkeys…      approval · bob · 29 m │
│ P8LEAD-xv43 spider: comments…       waiting for you · techlead   │
│ ✉ workflow   Question on DEV-xbgf                          10:08 │
└──────────────────────────────────────────────────────────────────┘

Running now (1)                  Today
● project8-techlead · 12 min     35 tasks · 32 done · 101 runs · $145.23
                                                          [History ▸]
Recent
tariboy-dev-homer · idle · developer        project8 · 3 agents
tariboy-developer-bob · needs you           sysadmin · idle
```

- "Needs you" lists, across the workspace's hosts, tasks that wait on the
  user (the D1 predicate) and open letters addressed to the user. A row opens
  the task drawer or the letter. Empty: "Nothing needs you."
- "Running now" lists agents in state `running` with the elapsed time of the
  current iteration. `AgentRow` gains the start time of the current
  iteration (an additive list field under the contract rules) so the list
  needs no per-agent status call.
- "Today" shows the existing server Activity tiles for the rolling day; the
  link opens the Activity table, which keeps its page at `/activity` reachable
  from Home only.
- "Recent" shows the eight agents with the most recent conversation or
  iteration, as the sidebar already sorts them.
- Home polls with the same intervals the sidebar uses today; no new daemon
  command is required when the existing lists suffice.

### D4. Tasks and Inbox

- Tasks: the existing per-host Tasks workspace is the only task table; the
  cross-host All tasks workspace is removed, and its "every host" view is
  replaced by the host selector. The toolbar is one row at 1024 px: search,
  segmented Active/Done, a "Filters" menu holding Queue, Assignee ("My
  tasks", an agent), "Waiting for me", and Columns; refresh; New task. The
  queue menu and "Manage queues…" (for the selected host) stay in the
  Filters menu and the New task row. The Tasks Playwright suite moves from
  `/servers/local/tasks` to `/tasks` and keeps every scenario.
- Inbox: "Needs you" first (the same rows as Home, full list), then the
  existing Inbox/Sent/Archive letters and Compose. The `batch · inbox N/M`
  counters and "Clear inbox" are not shown here; they belong to the agent's
  mail settings (project 3). With several hosts the letters are merged with
  a host column, or a Host filter when merging is not practical; the builder
  chooses, the rule is that one host never needs a host control.

### D5. Library

Images tab:

```
Library        Images   Workflows   Sources
[Search images…]   ☐ Show unused (25)                     [Add image ▾]
┌──────────────────────────────────────────────────────────────────┐
│ developer                  0.22.1 · tariboy      Used by 3 agents│
│ tariboy-workflow-developer 0.7.1  · tariboy      Used by 7       │
│ basic                      0.5.0  · tariboy      Used by 0       │
└──────────────────────────────────────────────────────────────────┘
```

- One row per image name: name, newest version, the Store it came from when
  provenance says so, "Used by N agents" from the agents list (current or
  pending image), last built as a relative time, and a "Run agent" action.
  Sorted by used-by descending, then name. "Show unused" is off by default
  and hides names no agent uses; `bare` is always hidden from the list and
  reachable through the dialog's "Plain terminal" entry. Search filters by
  name.
- "Add image" menu: Build from directory, Import archive, Get from a source
  (opens Sources). The two forms move into dialogs opened from the menu.
- Image detail (`/library/images/:name`): the newest tag's Overview,
  Template, Skills, Files tabs as today, plus a Versions section listing the
  tags, collapsed to the newest five with "Show all N". Per-tag Run agent,
  Export, Remove stay on the tag rows. The Source column no longer wraps
  character by character: it truncates with a title.
- Workflows tab: one row per workflow name with its newest version and the
  queues it is bound to; versions and "Upload to servers" per tag are in the
  detail. The Build form moves behind "Add workflow".
- Sources tab: one card per Store (name, source, number of images, Refresh
  and Remove in a `⋯` menu). The automatic-build interval and per-image Auto
  checkbox move below the image table into a "Automatic builds" disclosure.
  Each catalog row shows name, version, status (Built, Update available, Not
  built, or the failure sentence with Details) and one Build or Update
  button. The project 1 rule for the `tariboy` card applies.

### D6. Settings

Seven flat pages in a left nav, no index page, no "Operator tools" heading:

| Page | Content |
| --- | --- |
| Hosts | the existing DaemonsPage (local and SSH hosts, Add host, Update, Update all), host workspaces manager and selector when two or more exist |
| Teams | the existing Groups page (create, wizard, import, export) renamed; the team page's "Manage team" links here |
| Usage & budgets | the existing Usage page and Budgets page as two sections |
| Agent defaults | the existing General page: shell script before each iteration, Codex approvals, backup and data retention; with a host selector when several hosts |
| Integrations | Telegram and plugin settings, plus the existing Plugins page as its "Installed plugins" section |
| Maintenance | the existing Ops page (backup and restore) and the Operations log as two sections; an operation detail keeps its own route under `/settings/maintenance/operations/:id` |
| Application | the existing standalone `/app-settings` page (Help, interface mode until project 3 removes it, Desktop updates, CLI install), unchanged in layout so it stays reachable without a daemon connection; the Settings nav lists it last as a link, and its "Back" returns to Settings |

CLI and Appearance placeholders are deleted. "Daemons" is deleted (it is
Hosts). Labels use "host", never "daemon" or "server", except in error text
that quotes the daemon.

### D7. Renames

| Today | Now |
| --- | --- |
| Mailbox (server) | Inbox |
| Workflow images | Workflows |
| Stores | Sources |
| Groups (Settings) | Teams |
| Ops, Operations | Maintenance |
| Daemons, Servers, "THIS DAEMON (LOCAL)", "1 server · 17 agents" | Hosts; the footer line of D1 |
| Latest image_version, Newest built tag, Tag built at | Version, Built |

### D8. 1024×700

- The shell has at most two bars above any content: the titlebar and the
  open object's tabs.
- Toolbars (Tasks, Inbox, Library) collapse secondary controls into a menu
  before they wrap; the responsive suite asserts one toolbar row at
  1024 px.
- The sidebar keeps its width; the agent list scrolls.

## Left to the builder

- Component split and lazy-loading groups so the entry chunk stays within
  the budget; the sidebar nav and Home are shell code, Library and Settings
  pages load on first open.
- Whether "Used by N agents" is computed in the UI from the agent list
  (sufficient today) or added to the image list result.
- The exact redirect implementation for nested legacy paths and how the
  tour's `route` field maps onto the new routes.
- Test strategy within `AGENTS.md`: unit tests for the route table and
  redirects, the sidebar model (grouping by host count, badges), Home's
  "Needs you" predicate, the Library list model (sort, unused filter, used
  by), and responsive coverage for every new route.

## Acceptance

- From a fresh start with real data, every item in the user's daily path is
  at most two clicks from the sidebar: an agent's terminal, a task, the inbox,
  an image, a host.
- No screen shows "Tasks" or "Mailbox" twice with different scopes.
- Every URL that worked before this change still opens the equivalent page.
- At 1024×700 no toolbar wraps and no control overlaps another on Home,
  Tasks, Inbox, Library (all tabs), Settings (all pages), and an agent's
  Terminal tab.
- The sidebar order, pins, width, and host workspaces survive the change
  without migration.
- `make check` passes, including Knip (the removed pages and their tests are
  deleted, not left unused) and the bundle budget.
