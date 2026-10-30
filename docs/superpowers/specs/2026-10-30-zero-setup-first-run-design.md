# Zero-setup first run

Project 1 of 3 in the Desktop simplification series. The other two are
`2026-10-30-lean-navigation-design.md` (project 2) and
`2026-10-30-agent-workspace-daily-loop-design.md` (project 3). The projects
land in that order; each later one builds on the merged result of the earlier
ones. The research behind all three is in
`2026-10-30-ux-review-first-run.md`, `2026-10-30-ux-review-navigation.md`, and
`2026-10-30-ux-review-daily-loop.md`.

## Intent

A developer who installs Tariboy Desktop should have one agent working in one
of their repositories within two minutes, without learning what a store, an
image, or a harness is first.

Today the shortest working path from app start to "an agent is running my
instruction" is 13 clicks and 2 typed fields when the user already knows every
answer, plus 2 recovery clicks because the documented first step fails:
`Add tariboy Store → Build basic` returns HTTP 409 `reserved_image` on a fresh
daemon, because the daemon reserves `basic:latest` while the Store publishes
`basic` with the `latest` tag. The New agent dialog is 1,800 px tall with 28
controls, of which only the image is required, and that requirement is stated
in the footer. The onboarding tour has 13 steps, shows 11 of them, and ends on
the Workflow images page.

Target: 3 clicks and 1 typed field for the same path, and nothing on the
first-run screens that the user has to look up.

## Goals

- The default `tariboy` Store is registered by the daemon at startup, with no
  user action.
- The starter image `basic` is built by the daemon after the Store registers,
  with no user action, and the `basic:latest` reservation conflict is gone.
- The first screen of an empty host explains itself: a checklist that
  completes on its own (catalog connected, starter image ready, coding tool
  found) and one card that creates and starts the first agent.
- The New agent dialog has five visible fields; everything else is under
  Advanced with sensible defaults.
- Installed harnesses are detected on the host and pre-selected. `stub` is
  never offered in Desktop.
- A new agent opens on Terminal and the terminal tells the user what is
  happening while it starts, and why if it fails.
- The onboarding tour does not start by itself. It stays available from Help
  with its counter and ordering fixed.

## Non-goals

- Changing the CLI (`tariboy`, `ttasks`) or the compose format.
- Changing how images are built, stored, or versioned beyond the reservation
  rule.
- Redesigning the Images, Stores, or Workflow images pages beyond what the
  first-run path touches. That is project 2 (Library).
- Redesigning the agent workspace beyond the landing tab and the terminal
  status line. That is project 3.
- Multi-host first run. A second host is added later from Settings.
- Android.

## Anti-goals

- A wizard that asks questions the daemon can answer itself.
- A first run that needs the user to read the documentation.
- Breaking an existing installation: a daemon that already has a store named
  `tariboy`, or an image named `basic`, must not get a second one, and a user
  who removed the default store must not get it back on every start.
- Hiding a failure. If the store cannot be cloned or the starter image cannot
  be built, the home screen says so in one sentence, with the real reason
  behind "Details" and a Retry button.

## Constraints

- Follow `AGENTS.md`: typed command results, OpenAPI snapshot and generated UI
  types regenerated in the same change, no `map[string]any`, tests before
  production code, `make check` green, frontend complexity thresholds, bundle
  budget, Knip.
- The daemon host PATH contract in `docs/docs/development.mdx` governs how
  tools are looked up; harness detection uses the same lookup as the existing
  missing-tools check.
- No version bump.
- Documentation updated in the same change: `README.md` "Get started",
  `docs/docs/quickstart.mdx` sections 1, 3, and 4, `docs/docs/images/index.mdx`
  (reserved refs and the default store), `docs/docs/architecture/web-ui.mdx`
  (onboarding, create dialog), and `docs/docs/architecture/index.mdx` (new
  commands).

## Decisions

### D1. The daemon registers the default Store at startup

- A new daemon-side routine, run from the same startup phase that seeds the
  `TASK` queue and the `bare` image (`buildTasks` in `internal/daemon/phases.go`),
  registers the Store `tariboy` from
  `https://github.com/alekzonder/tariboy-store.git` when **both** hold:
  - no Store named `tariboy` is registered, and
  - the daemon config key `onboarding.default_store` is absent.
- The routine runs in the background so a slow or absent network does not
  delay daemon readiness. It records its result in `onboarding.default_store`:
  `registered` on success; `failed` on failure, with the error kept in memory
  for the status command (not persisted).
- A removal of the `tariboy` Store through `store.remove` sets
  `onboarding.default_store` to `removed`. The daemon never registers it
  again on its own; the user can re-add it from the Stores page, which keeps
  the existing "Add tariboy Store" action for that case.
- A failed registration is retried at the next daemon start and by the new
  command in D4. It is not retried on a timer.
- "No Store named `tariboy`" means no Store with that name **or** with the
  default source URL (the UI's `hasTariboyStore` rule). A daemon where the
  user removed the Store under an older release has no config key, so the
  first start after the upgrade registers it once more; that single
  re-registration is accepted, and the `removed` key then holds.
- Opt-out: the routine, and the starter build of D2, are skipped when the
  daemon starts with the environment variable `TARIBOY_ONBOARDING=off` or
  the matching daemon option. The default is on for every daemon, local and
  remote, because the Desktop-managed and the SSH-provisioned daemon must
  behave the same. Every test daemon fixture (Go daemon tests, the e2e
  scripts, `ui/tests/tasks-e2e-daemon.mjs`, the desktop Playwright fixture,
  `make ui-dev`) sets it off, so `make check` stays offline and fast. The
  knob and the outbound connection at startup are documented in
  `docs/docs/security-controls.mdx`.
- The single source of truth for the default Store's name and URL moves to
  the Go side (`internal/image` or a small `internal/onboarding` package); the
  UI constant `TARIBOY_STORE` in `ui/src/lib/api/stores.ts` reads the name and
  source from the onboarding status instead of hard-coding them, or is deleted
  if nothing else needs it.

### D2. `basic:latest` is no longer reserved; the daemon builds it

- `image.IsReserved` reserves only `bare:latest`. Every call site that
  branches on the reservation (`internal/image/ref.go`, `store_ops.go`,
  `internal/commands/image.go`, `image_source.go`,
  `internal/imageportable/service.go`) keeps protecting `bare` only. An
  existing legacy `basic:latest` with no provenance (as on long-running
  daemons) becomes an ordinary image: rebuildable, exportable, removable.
- After the default Store registers (or at startup when it is already
  registered), the daemon builds `tariboy/basic` through the existing Store
  build path (the same code `tariboy image build tariboy/basic` uses, as an
  operation visible in Settings → Operations) when **no image named `basic`
  exists** on the host. It never rebuilds an existing `basic`; Store refresh
  and automatic builds keep their current behavior.
- The build result is recorded in memory as the starter image state for the
  status command: `building`, `ready` (with the ref), `failed` (with the
  reason), or `skipped` (an image named `basic` already existed). When the
  build fails because a tool the Store build needs is missing (for example
  `npx` for a Store with `skills-lock.json`), the reason names the tool and
  its install hint from the existing missing-tools list; the checklist has
  no separate Node row.

### D3. Harness detection

- The daemon reports, for each harness type except `stub`, whether its CLI
  is on the daemon's PATH (same lookup as the missing-tools check) and the
  install hint already used for missing tools. `stub` is excluded from every
  Desktop list; it remains a valid value for the API and CLI.
- Detection is cached for the same 5 minutes as the model catalog and
  refreshed by the `refresh` flag.
- The `daemon.status` contract does not change: `toolcheck` keeps counting
  "at least one harness" among the missing tools, and `tariboy daemon status`
  keeps its output. The rule that harness absence is never a red banner is
  applied in the UI: when every missing tool is a harness, the banner is not
  shown and the checklist row carries the information; when another tool is
  missing, the banner shows only those tools.

### D4. New commands

Both are ordinary registry commands with named result structs, OpenAPI
snapshot and generated UI types updated, and a `typedCallSets` entry.

- `onboarding.status`, `GET /api/onboarding`, result `OnboardingStatus`:
  - `default_store`: `{state: "registered"|"failed"|"removed"|"pending", name, source, error}`;
  - `starter_image`: `{state: "ready"|"building"|"failed"|"skipped"|"pending", ref, error}`;
  - `harnesses`: list of `{type, found, install}` for claude, codex, opencode,
    cursor;
  - `agents`: the number of agents on this host.
  The command is cheap: it reads config, the store list, the image list, and
  the detection cache.
- `onboarding.retry`, `POST /api/onboarding/retry`: runs D1 and D2 again
  immediately (idempotent) and returns the same `OnboardingStatus`.

### D5. The empty-state home

Shown in the content island at `/` while every host of the workspace has
zero agents. Replaces "Pick an agent on the left, or create one." As soon as
any host has an agent the route behaves as today (project 2 later turns it
into a real Home); a host added later gets its checklist inside the New
agent dialog (D6) and on the Stores page. The checklist polls the host chosen
in the card's "Run on" select, which defaults to the first host in sidebar
order and is shown only with two or more hosts.

```
Welcome to Tariboy
Run an AI coding agent in one of your repositories.

Getting ready
[✓] Tariboy catalog connected
[✓] Starter image "basic" ready
[!] Coding tool: Claude Code found. Codex, OpenCode, Cursor not found.   Install another…

┌ Run your first agent ─────────────────────────────────────────────┐
│ Folder        [ ~/code/my-app                                 ]   │
│ Image         (•) basic — general coding agent                    │
│               ( ) Plain terminal — no instructions (bare)         │
│ Coding tool   (•) Claude Code  ( ) Codex ✗  ( ) OpenCode ✗  ( ) Cursor ✗ │
│ Name          [ my-app-basic ]                                    │
│ ▸ Advanced                                                        │
│                                               [ Start agent ]     │
└───────────────────────────────────────────────────────────────────┘
Later: connect another computer over SSH in Settings.
```

- The checklist rows come from `onboarding.status`, polled every 5 seconds
  while any row is not final. A failed row shows one sentence, a "Details"
  disclosure with the raw error, and "Retry" calling `onboarding.retry`.
- The daemon's missing-tools banner stays for tools other than harnesses
  (`tariboy`, `ttasks`, `tmail`, `tmux` and the rest). Harness absence is
  shown only in the checklist row, with per-tool install hints, and never as a
  red banner.
- Image radios: every built image name that is not `bare`, taken from the
  image list, with `basic` first and pre-selected; `bare` last as "Plain
  terminal". On a host with many images the radios become the same select as
  in the dialog (D6).
- Coding tool radios: pre-select the image's default harness when it is found,
  else the first found harness. A not-found tool is selectable but shows its
  install hint and keeps "Start agent" disabled with the reason next to the
  button.
- Name: generated as `<folder basename>-<image name>` (or `<image name>`
  when the dialog's Advanced "managed folder" option is chosen), made unique
  against the host's agents by a numeric suffix, editable.
- Folder: the existing path field with autocomplete; there is no native
  folder picker in this project. Folder is required in the card. When the
  folder does not exist (derived from the existing `fs.list` of its parent),
  the field shows "This folder does not exist. [Create it]"; the link sets
  the existing `create_cwd` flag for the request. The "Create directory if
  missing" switch is removed from the visible form.
- "Start agent" creates the agent with the documented defaults, `start now`
  on, Interactive on, and navigates to its Terminal tab. While the checklist
  is not final the button is disabled with the first unmet reason beside it.
- The card is the same component as the dialog in D6, rendered inline.

### D6. The New agent dialog

The dialog keeps its routes and entry points (`?new=1`, sidebar "+", "Run
Agent" on an image, clone). Clone mode keeps copying every field but presents
them through the same layout.

Visible fields, in this order: Run on (only when the workspace has more than
one host), Folder, Image, Coding tool, Name. Then one "Advanced" disclosure.
Footer: Cancel, Start agent. The disabled-reason text in the footer stays.

- Image select: one row per image **name** with the newest built tag
  pre-selected, showing the name, the newest version, and the Store it came
  from when known. Typing filters by name. `bare` is last, labelled "Plain
  terminal (bare)". A "version" sub-select under Advanced picks another tag.
  A host with no built image except `bare` shows the checklist rows of D5
  above the select instead of an empty list.
- Coding tool: radios as in D5. Model and effort are shown as one muted
  summary line ("Model: opus · Effort: medium") with "change" that reveals the
  existing two preset combos. Saved presets per harness keep working.
- Folder is required in the visible form. Advanced → Folder offers "Use a
  managed folder", which clears the field and sends an empty cwd, as an
  empty field does today.
- Advanced contains every other current field, grouped: Identity (alias,
  team, color, notes), Image (version, plugins), Folder (managed folder,
  environment variables), Autopilot (interval, soft and hard timeouts, timeout and error
  policies, idle limit, messages per run, inbox limit, standing prompt), Tasks
  (Goal switch, wait-customer timeout, delivery cooldown), and "Create without
  starting". Defaults are the ones documented today; nothing changes in the
  create request except that the UI fills the harness with the detected one
  instead of "image default" when the image declares none.
- Budgets, retention, mail settings, and schedule are not in the create
  dialog; they stay in the agent's Configuration (project 3 renames it).
- Labels: "Folder" for cwd, "Coding tool" for harness with the harness id in
  muted text ("Claude Code · claude"), "Image" stays "Image" with a one-line
  description when the manifest has one, "Autopilot" for the loop group.
  Helper texts are one sentence each and name the effect, not the field.

### D7. Landing and the terminal status line

- A newly created agent opens on Terminal regardless of interface mode.
- The terminal area shows a status line above the xterm element, derived
  from the agent's state: "Starting Claude Code in ~/code/my-app…" while the
  session is not yet attached, the real reason in plain words when the start
  failed (the existing `halt_reason`), with Retry (start again) and "Open
  settings". The empty-state text "Start the agent and enable interactive mode
  (Settings → Runtime config)" is replaced by that line; it must name the
  existing "Runtime" section.
- The agent header is otherwise unchanged in this project (project 3
  redesigns it), except that the Exec/Stop/Kill cluster wraps under the chips
  instead of overlapping them below 1200 px.

### D8. The tour

- The tour never starts automatically. On first start the UI stores the
  current version in `onboarding:state:v1` as if the tour were done. "What's
  new" keeps working for later releases.
- "Take the tour" in Help keeps the tour with these fixes: the counter counts
  only the steps that will be shown; the step order is Welcome, Agents
  (highlighting the "New agent" or "Start agent" button, not the sidebar
  "+"), Agent workspace, Agent settings, Tasks, Images, Store, Servers, Add a
  server, Queues, Workflows, Help; the last step returns to the route where
  the tour started; the tour does not start while a daemon error banner is
  visible.
- Three contextual one-liners replace the automatic tour, each shown once
  per agent until dismissed and remembered in `localStorage`: Terminal
  ("Type what you want done. To hand work over and let the agent run on its
  own, give it a task."), Tasks ("Tasks you create here are given to this
  agent."), Configuration ("Autopilot: set a schedule here to let the agent
  keep working without you. Everything else is optional.").

### D9. Stores page

- When the `tariboy` Store is registered, its card is not shown. The "Add
  Store" form folds under "Add another catalog". When the Store was removed
  (`onboarding.default_store = removed`), the card comes back.
- A Store build that fails shows the reason in the row, not only in a toast.

## Left to the builder

- Package placement of the onboarding routine and status (`internal/onboarding`
  or alongside stores) and the exact config key names.
- Whether `onboarding.status` includes the image description, and where the
  description comes from (the manifest has no description field today; an
  empty description is acceptable in this project).
- Component structure of the shared first-agent card and how clone mode maps
  onto it.
- How the terminal status line learns that the session is attached (the
  existing terminal socket states are enough).
- Test strategy, within `AGENTS.md`: Go unit tests for the seeding rules
  (idempotence, `removed`, failure), command tests for the two new commands,
  UI unit tests for the checklist states and the dialog's visible-field set,
  and the images Playwright fixture if the dialog is covered there.

## Acceptance

- On a fresh isolated daemon with network access, `claude`, `git`, and `npx`
  on PATH, and `TARIBOY_ONBOARDING` unset: open Desktop, pick a folder, click
  "Start agent", type an instruction in Terminal. Three clicks, one typed
  field. No tour, no red banner.
- With `TARIBOY_ONBOARDING=off` the daemon registers and builds nothing, and
  `make check` runs without network access.
- On the same daemon without network: the checklist shows "Could not connect
  the Tariboy catalog" with Details and Retry; "Plain terminal" is still
  available; "Start agent" works for it.
- On a daemon that already has a `tariboy` Store and a `basic` image: nothing
  is added or rebuilt; the Stores page shows no "Add tariboy Store" card.
- Removing the `tariboy` Store and restarting the daemon does not bring it
  back; the card is offered again.
- `tariboy image build tariboy/basic` succeeds on a fresh daemon; `bare:latest`
  is still refused.
- `make check` passes; the OpenAPI snapshot and generated types are updated.
