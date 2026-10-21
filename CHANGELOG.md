## [0.89.1] - 2026-10-21

### Fixed

- The macOS Desktop build succeeds again. A Web UI helper module whose
  name differed from a component only in letter case broke the build on
  case-insensitive file systems, so 0.89.0 shipped no Desktop release.
  0.89.1 delivers the 0.89.0 changes. A new check fails the build when
  two UI modules differ only in case.

[0.89.1]: https://github.com/alekzonder/tariboy/compare/v0.89.0...v0.89.1

## [0.89.0] - 2026-10-21

### Added

- The task workflow panel reads top down: a problem block explains a failed
  check, rejection, failing watch, busy pool or idle holder with the script's
  own message and what to do; a stepper maps the statuses in manifest order
  with return counts; outcomes and the artifacts they need come first.
  **Return to agent** sends a task back to its pool with a message the agent
  reads in its Goal.
- The task list shows a workflow task's step and its current problem. Task
  reads carry `workflow_step` and `workflow_attention`, and the task view
  describes every status with its transitions, checks, watch and limits.
- Workflowfile checks, watches and sources accept a one-line `description`
  shown in the task view, the agent Goal block and `ttasks workflow get`.

### Changed

- A queue bound to a workflow by `NAME` or `NAME:TAG` follows that tag:
  new tasks, builds, imports and daemon start move the binding to the
  tag's current digest after the bind checks, and a blocked move is
  recorded as `queue.workflow_follow_blocked`. Existing tasks keep their
  pinned version, and digest bindings stay pinned. Desktop shows the
  followed tag (migration 0068).
- The store writes every time in one canonical format and compares stored
  times directly (migration 0067); tasks store their status owner kind and
  open blocker count (migration 0069).

### Fixed

- Task lists, ready, the event feed, relations and activity no longer fail
  with "too many SQL variables" past about 32k visible tasks.

### Security

- Daemon HTTP servers set header and idle timeouts and a 64 KiB header cap.
  Command and agent tools API bodies are limited to 1 MiB (`413
  too_large`), with larger limits for the file editor and image sources.
  The terminal websocket checks the browser Origin on tokenless requests and
  caps frames at 8 MiB. Registry downloads have timeouts and stop past the
  64 MiB image limit. Backup restore rejects unknown archive columns.
- The event hub counts dropped events and sends a reset, so clients
  refetch instead of silently missing updates.

[0.89.0]: https://github.com/alekzonder/tariboy/compare/v0.88.0...v0.89.0

## [0.88.0] - 2026-10-19

### Added

- Task queues hold plain env vars beside their secrets. Both reach
  workflow scripts in the same layer and both satisfy `requires_secrets`;
  only secret values stay write-only and redacted. Manage them with
  `ttasks queue env set|ls|rm`, `PUT/GET/DELETE /api/task-queues/{queue}/env`,
  or the queue settings Environment tab (formerly Secrets), which shows Env
  and Secrets blocks and confirms before moving a key between them. Store
  migration 0066 keeps every existing entry as a secret.

### Changed

- Task keys in server and agent Activity, agent Usage task and epic rows,
  and the agent header Goal link open the task drawer over the current page
  instead of switching to the Tasks tab.
- The Desktop icon, web UI favicon and documentation site use the new
  Tariboy dancer logo on a dark rounded tile, so it stays visible on light
  docks and tabs.
- Agent Activity, Advanced and server Activity tabs use the same padding and
  spacing as the Configuration tab.

[0.88.0]: https://github.com/alekzonder/tariboy/compare/v0.87.2...v0.88.0

## [0.87.2] - 2026-10-19

### Fixed

- Desktop releases publish again: the Android release build reads the
  signing certificate fingerprint from the per-scheme output of current
  `apksigner` versions, so the signed APK passes its certificate check and
  the GitHub release is created. 0.87.0 and 0.87.1 were tagged but never
  published as GitHub releases; 0.87.2 ships their changes.

[0.87.2]: https://github.com/alekzonder/tariboy/compare/v0.87.1...v0.87.2

## [0.87.1] - 2026-10-19

### Fixed

- Desktop releases publish again: the Android release build installs
  ripgrep, which its version check needs, so the signed APK is built and
  the GitHub release is created. 0.87.0 was tagged but never published as a
  GitHub release; 0.87.1 ships its changes.

[0.87.1]: https://github.com/alekzonder/tariboy/compare/v0.87.0...v0.87.1

## [0.87.0] - 2026-10-19

### Added

- Agents can start loop iterations on an RFC 5545 RRULE schedule with an
  optional DTSTART: weekdays at a time, a day of the month, a single date
  and time, or every N hours or days. Set it with `tariboy loop schedule`,
  `POST /api/agents/{name}/loop/schedule`, or the Loop and timeouts block of
  the agent configuration page; agent views report `schedule` and
  `schedule_next_at`. Missed occurrences are not replayed.
- Desktop releases attach a signed arm64 Android APK with its SHA-256
  checksum again; the build rejects debug and unexpected signing
  certificates. A new Android release APK page documents the signing setup.

### Changed

- The New task forms in an agent Tasks tab and in All tasks preselect the
  queue of the last root task created on that server; a queue picked in the
  toolbar filter still wins.

[0.87.0]: https://github.com/alekzonder/tariboy/compare/v0.86.0...v0.87.0

## [0.86.0] - 2026-10-18

### Added

- `tariboy tasks activity` and `GET /api/task-activity` summarize work on
  tasks in a time window: per task AI iterations, requests, tokens and cost
  together with comments, outcomes, entered statuses and completion, for one
  agent or broken down per agent server-wide.
- The agent Activity tab has one Hour/Day/Week/Custom period kept in the
  URL, summary cards and a Tasks view whose keys open the task; Iterations,
  Usage and Status history follow the same period. Servers get an Activity
  section with Tasks and Agents views.
- Manage queues opens a two-column dialog with General, Workflow, Pools,
  Secrets and Sources tabs. The Workflow tab lists the statuses of the bound
  image, or of the image chosen before Bind, with each owner and outcome;
  the Pools tab shows one row per pool, including required pools that do not
  exist yet, with agent chips, suggestions and a per-row Save. On a phone the
  dialog fills the screen.

### Changed

- A click anywhere on a task row opens the task; the row's own controls keep
  their action.
- The agent Configuration tab has page margins, and its sticky search and
  save bars use the panel color and align with the cards.

### Fixed

- Pop-up lists in agent configuration blocks (Model and Effort presets,
  working directory suggestions) are no longer cut off at the block's edge.

[0.86.0]: https://github.com/alekzonder/tariboy/compare/v0.85.0...v0.86.0

## [0.85.0] - 2026-10-17

### Added

- The agent Configuration tab is one column of blocks (General, Runtime,
  Loop and timeouts, Goal, Messages, Budgets, Environment, Retention) with a
  search box and a **Changed only** filter. A sticky bar resets or saves all
  changes at once; Save validates every changed field first and stops at the
  first failure, keeping unsent drafts. Start, Stop and Restart move to the
  page header.
- On compact screens the agent list opens as a drawer.

### Changed

- An agent has one on/off switch. An enabled agent runs autonomous
  iterations unless a stop policy halted it or it uses the terminal-only bare
  image; Start clears a stop-policy error, so a halted agent resumes.
- Task tables, settings and editors fit phone widths; phone task headers and
  the drawer no longer change the saved column layout or sidebar state.
- The Android app opens web links in the system browser.

### Removed

- `tariboy loop enable|disable`, `POST /api/agents/{name}/loop/enable` and
  `/loop/disable`, the `loop` field of agent create, `loop` in Telegram
  `/agent set`, and `loop_enabled` in agent, status, tools and support-bundle
  responses. Use Start/Stop (`enabled`) instead. Compose keeps `loop.enabled`
  and converges it through start and stop.
- The Autopilot tab and the Loop/Autopilot switches in Configuration, the
  create dialog and the group wizard.

### Fixed

- After a stop policy halted an agent, Start left it idle forever.

### Upgrade notes

- Migration `0064` drops the `loop_enabled` column. Agents that were
  enabled with the loop off (and not halted or bare) are turned off first,
  so they do not start spending AI budget after the upgrade.

[0.85.0]: https://github.com/alekzonder/tariboy/compare/v0.84.0...v0.85.0

## [0.84.0] - 2026-10-16

### Added

- `ttasks list` lists the tasks the caller can see, in agent and operator
  mode; `ttasks mine` becomes its alias and search is `list --text`. New
  filters: `--category`, `--priority`, `--author`, `--customer`, `--group`,
  `--parent`, `--subtree`, `--blocked` and `--view`, plus `--sort tree|updated|created|priority`.
  A page holds 50 tasks; the table ends with the next-page command. The agent
  tools socket gains a matching `list` action.
- Task table columns can be resized and reordered by dragging, from the
  keyboard, or from a new **Columns** menu in the filter bar (the touch path
  on phones). The layout survives reloads and app restarts.
- The daemon writes every API request to a size-rotated
  `tariboyd.access.log` (50 MB, 3 copies). `tariboyd.log` keeps a WARN line
  only for requests that take 500 ms or more or fail with a 5xx status.
  Launchers rotate a `tariboyd.log` larger than 50 MB before starting the daemon.
- The new `GET /api/agent-states/ws` socket pushes agent state hints. The
  sidebar and the agent header refresh on hints and fall back to a 15 s poll
  (3 s against hosts without the socket).

### Changed

- `ttasks --status` matches the workflow status a task shows (a category
  value still finds workflow tasks in it), and `status_view=closed` now
  includes cancelled tasks.
- The daemon banner probes daemon status every 10 s instead of every 2–3 s.
- Polled API reads (daemon status, agents, chats, tasks, images) use a
  read-only SQLite connection pool, so they no longer wait behind writes.
  The chat and agent lists read in a fixed number of queries.
- The nightly maintenance backup no longer blocks API requests and agent
  writes while it runs.

### Fixed

- The Android WebView stays clear of the system bars and the keyboard.

[0.84.0]: https://github.com/alekzonder/tariboy/compare/v0.83.0...v0.84.0

## [0.83.0] - 2026-10-16

### Added

- `tariboy update --cleanup [--keep N]` deletes all but the N newest
  installed releases (default 5) after a successful update, or right away
  when the version is already installed. The active release is always kept,
  the installer lock is held, a failed restart skips the cleanup, and a
  cleanup error is only a warning.

### Changed

- Desktop **Update Tariboy** on a Linux host whose CLI supports
  `tariboy update --cleanup` no longer uploads the release: it runs
  `tariboy update <Desktop version> --cleanup` on the server over SSH, checks
  the daemon version, and rolls back to the previous release when a failure
  happens after the managed links switched. The server needs outbound HTTPS
  to GitHub. macOS hosts, first installs and older CLIs keep the upload
  path. The update dialog step "Upload release" now reads "Get release".
- Image validation accepts additional `SKILL.md` frontmatter fields, such as
  `version` or harness-specific keys, and preserves them unvalidated. Only
  `name` and `description` are required; known optional fields keep their
  type checks.

[0.83.0]: https://github.com/alekzonder/tariboy/compare/v0.82.0...v0.83.0

## [0.82.0] - 2026-10-16

### Added

- Nightly maintenance deletes chat history older than the new chat history
  period: delivered bus messages of every chat and agent, with the existing
  guards for unacknowledged and unconsumed messages. Set it with
  `tariboy maintenance set --chat-retention-days N`, the
  `chat_retention_days` maintenance setting, or **Keep chat history for
  (days)** in **Settings → General → Backup & data retention**. `0` leaves
  messages to the general retention period.

### Changed

- The chat history period defaults to 7 days, also for hosts whose stored
  maintenance settings predate it. After upgrading, the next nightly run
  deletes delivered chat messages older than 7 days, even when
  `retention_days` is `0`; older history stays only in database backups. Set
  `--chat-retention-days 0` before the next run to keep the previous behavior.

### Fixed

- Desktop queue settings no longer remount the Secrets section on every
  render of a workflow-bound queue.

[0.82.0]: https://github.com/alekzonder/tariboy/compare/v0.81.0...v0.82.0

## [0.81.0] - 2026-10-15

### Added

- Queue workflow scripts that run as the queue now start from the global env:
  the daemon environment after `global-agent-shell.sh`. A secret that the bound
  image lists in `requires_secrets` can come from that env with a non-empty
  value, so binding the workflow or removing the queue secret no longer
  requires a queue secret. A queue secret with the same name still overrides
  the global value. `ttasks queue secret ls` and its route report those names
  in `global_env`, and Desktop queue settings show `NAME present in global env`.

### Fixed

- Reading the global env no longer blocks the database. The daemon reads it
  before the bind or secret-removal transaction opens, stops sourcing shell
  scripts after 30 seconds, and hides the daemon access variables from the
  global script.
- The Desktop chat list no longer scans every agent inbox on each poll. That
  scan stalled `/api/tasks` and other requests on hosts with busy worker
  agents.

[0.81.0]: https://github.com/alekzonder/tariboy/compare/v0.80.0...v0.81.0

## [0.80.0] - 2026-10-15

### Added

- The operator can run the checks of a failed workflow transition request
  again with `ttasks workflow retry KEY [REQUEST]`,
  `POST /api/tasks/{key}/workflow/requests/{id}/retry`, or **Retry** on the
  Failed notice of the Desktop workflow panel. Only the task's latest request
  in its current visit can be retried, and only when it failed.

### Fixed

- Durable script runs and `run_as: agent` workflow checks now source the global
  and per-agent shell scripts, so their exports (such as tokens from a sourced
  secrets file) reach those commands.

[0.80.0]: https://github.com/alekzonder/tariboy/compare/v0.79.0...v0.80.0

## [0.79.0] - 2026-10-15

### Added

- Each server stores a Codex approval mode. `yolo` (default) runs Codex with
  `--yolo`; `approve-for-me` runs it with `--approve-for-me` (workspace-write
  sandbox with automatic approval review). Read and change it with
  `tariboy harness codex-approval get|set`
  (`GET`/`POST /api/harnesses/codex/approval-mode`) or the per-host Codex card
  in Settings → General. Batch and interactive Codex both use it.
- Desktop installs and updates Apple Silicon macOS (Darwin arm64) servers over
  SSH. Install picks the bundled build by `uname -s` and `uname -m`; Intel Macs
  and other platforms stay blocked with nothing changed on the host.
- The task list priority is a P0–P3 menu on editable tasks that saves only the
  priority, and the Pri column now comes first in every task table.

### Fixed

- Forwarded Telegram messages reach agents: media captions are used as text,
  the forward source is named (with a post link for public channels), the
  message carries `data.telegram_forward`, and forwarded text never runs as a
  command.
- The agent dialog says why **Create agent** is disabled, keeps image manifest
  errors visible until the host or image changes, and retries the manifest.
- The task detail header stays reachable when the description is long.

[0.79.0]: https://github.com/alekzonder/tariboy/compare/v0.78.0...v0.79.0

## [0.78.0] - 2026-10-12

### Added

- Desktop installs and updates Linux aarch64 (`arm64`) servers. The app
  bundles a `linux-aarch64` server build next to `linux-x86_64`, and install
  and update pick the build by the server's `uname -m` during preflight.
  Preflight and the server dialog accept aarch64. 32-bit ARM and other
  architectures are still blocked, and nothing changes on those hosts.
- Releases publish `tariboy_<version>_linux-aarch64.tar.gz` beside the x86_64
  archive. `tariboy update` downloads the archive that matches its own
  architecture, so installer-managed aarch64 hosts can update themselves.

### Changed

- The agent's live session tab is now called **Terminal** instead of
  **Console** in the UI and documentation. The `/console` route key stays,
  so saved links keep working.

## [0.77.0] - 2026-10-12

### Added

- `tariboy harness models TYPE` (`GET /api/harnesses/{type}/models`) lists
  the models and efforts the harness CLI on the daemon host offers: `codex
  debug models`, `agent models` for Cursor, `opencode models`, and a fixed
  alias and effort list for Claude Code. Successful catalogs are cached for
  five minutes; CLI failures come back as an `error` field. Agent creation
  and Configuration Runtime suggest these values under **From harness** and
  previously used values under **Saved**, and still accept free-form input.
- The daemon status reports host tools missing from the daemon PATH (tmux,
  bash, python3, git, npx, `tariboy`, `ttasks` and at least one agent
  harness) with `state`, `missing_tools` and `message`. Desktop preflight and
  the UI show them as an error with install hints.

### Changed

- New agents are interactive by default: `agent.run` (CLI and
  `POST /api/agents`) treats an omitted `interactive` as true and rejects a
  non-boolean value with `bad_interactive`. Pass `--interactive false` for a
  non-interactive agent. Compose files keep their declared mode.
- Pruning always archives an iteration, with its `iterations` and
  `ai_requests` rows in `db-rows.json`, before deleting it, and keeps an
  iteration whose archive fails. The retention `archive` flag is removed from
  the policy, CLI, API and UI.
- Iteration pruning runs in nightly maintenance after the database backup
  instead of hourly, and is reported as `iterations` in the deleted counts.
- The default retention policy merges per field: each non-zero agent field
  overrides it and 0 inherits. `retention get` also returns the agent's
  `override` and the `default`. Settings → General edits the default.
- Codex receives the effort as `model_reasoning_effort` and `opencode run`
  receives the model, so the selected values take effect.
- The sidebar shows Servers first and opens on Servers; the Groups tab is
  hidden.

### Fixed

- A customer comment that mentions the assigned agent on a flexible task in
  `wait_customer` resumes the task again once no customer question is open.
- Configuration Runtime no longer crashes when an agent view omits model or
  effort.
- A reconnecting SSH host no longer blanks Tasks, Configuration and the other
  server pages or drops unsaved drafts, and the chat follows the host's new
  port.

[0.78.0]: https://github.com/alekzonder/tariboy/compare/v0.77.0...v0.78.0
[0.77.0]: https://github.com/alekzonder/tariboy/compare/v0.76.1...v0.77.0

## [0.76.1] - 2026-10-08

### Fixed

- The macOS desktop app builds again: the `tariboy-desktop` binary links the
  task notification bridge and the `UserNotifications` framework, which the
  Android library target had cut off. 0.76.0 failed to build for macOS and
  was never published, so 0.76.1 is the first release that ships the 0.76.0
  changes below.

[0.76.1]: https://github.com/alekzonder/tariboy/compare/v0.76.0...v0.76.1

## [0.76.0] - 2026-10-08

### Added

- An Android app (arm64 APK, `make desktop-android`). It has no local daemon:
  it connects to a daemon port you forward to the phone (plain `http://` to
  loopback only) or to an `https://` server, keeps servers in the browser
  registry, and starts on an **Add server** screen. The loopback web listener
  accepts the app's `http://tauri.localhost` origin. Releases do not attach
  the APK yet: the release workflow's Android job is paused until its signing
  Secrets are configured.
- `cursor` harness: agents can run the Cursor CLI `agent` beside Claude Code,
  Codex and OpenCode (`tariboy agent harness NAME cursor`, compose
  `harness.type: cursor`, create-agent presets with Cursor model ids).
  Headless runs use `-p`, stream-json, `--force` and `--trust`. Cursor
  traffic goes through the AI proxy via `--endpoint`, and its Connect-RPC and
  `/auth` paths are forwarded to `api2.cursor.sh`. Token counts are recorded
  from the harness's stream-json `result` usage and persisted in the
  transcript so reindex rebuilds them. Cost stays unpriced, so USD budgets do
  not cap Cursor; a budget block returns a Connect `resource_exhausted`
  error. Desktop preflight lists the Cursor CLI as an optional tool.
- `ttasks queue source ls` (and `GET /api/task-queues/{queue}/sources`)
  returns each source's newest 20 runs in `runs`. `queue source log` can read
  a run that is still going. The queue settings show the bound workflow's
  **Sources** with their runs, an on-demand **Log** and **Refresh**.

### Fixed

- Iteration audit tailing no longer cuts `harness_output` lines at 8 KiB.

[0.76.0]: https://github.com/alekzonder/tariboy/compare/v0.75.0...v0.76.0

## [0.75.0] - 2026-10-06

### Added

- Workflow images can declare `sources`: scripts the daemon runs every
  `every` for each queue bound to the image, outside any task. Each item a
  source reports carries a key, and every key not reported before becomes a
  task in the queue's `initial_status`, created as the customer with the
  reported title (one line, up to 1 KiB), description, priority and
  artifacts. Keys are remembered per queue and source, so an item never
  becomes a second task. Source runs get the queue secrets, are redacted like
  other scripts, and a failed run records the queue event
  `queue.source_failed`.
- `ttasks queue source ls QUEUE` lists the sources of the bound image with
  their next run, failures in a row and last run;
  `ttasks queue source log QUEUE RUN` prints a redacted run log tail. Both are
  operator-only and backed by `GET /api/task-queues/{queue}/sources` and
  `GET /api/task-queues/{queue}/source-runs/{id}/log`.

[0.75.0]: https://github.com/alekzonder/tariboy/compare/v0.74.0...v0.75.0

## [0.74.0] - 2026-10-05

### Added

- The Store detail splits its sources into **Agent images** and **Workflow
  images** tabs. The Workflow images tab lists each workflow source with its
  `workflow_version`, compares it with the built `latest` tag on the host
  (missing, update needed, up to date, or "Comparison unavailable"), and
  **Build** publishes it through the workflow-image build endpoint using the
  `STORE/NAME` selector.

### Changed

- The agent **Chat** tab is now the only place for an agent's messaging. Its
  toolbar switches between **Chat**, **Channels** (subscriptions, channel tail,
  watches and send) and **Queue** (Queue/Archive/DLQ, mark processed, reply,
  requeue, Clear queue), kept in the URL as `?view=chat|channels|queue`.
- **Advanced** opens on Prompt and no longer has Channels or Messages. Old
  `advanced?view=channels` and `advanced?view=messages` links redirect to
  `chat?view=channels` and `chat?view=queue`. **Autopilot** shows the
  event-trigger count with a link to Chat → Channels instead of a second
  subscription editor.

### Fixed

- A message counts as unread only when another participant of the chat sent
  it. A `system:workflow` task assignment in an agent's Tasks chat no longer
  keeps the sidebar badge and the Chat tab dot lit after every conversation
  has been read.

[0.74.0]: https://github.com/alekzonder/tariboy/compare/v0.73.0...v0.74.0

## [0.73.0] - 2026-10-05

### Added

- Workflow images can be copied between daemons:
  `GET /api/workflow-images/{name}/{tag}/export` returns a portable archive and
  `POST /api/workflow-image-imports` publishes it like `workflow build`,
  refusing content whose digest differs (`workflow_digest_mismatch`).
- Desktop has a **Workflow images** page in each server workspace: build an
  image from a directory on the server, list tags with the queues bound to
  them, open the status graph, limits, artifacts, secrets, env and files of a
  tag, remove a tag, and upload an image to other servers. Queue workflow
  settings link the bound image to its page.

### Changed

- Workflow artifact values render as Markdown, with a Markdown / Text switch
  per artifact; expanded long values are shown in full.
- The workflow panel shows labeled **Move to status…** and **Cancel task**
  buttons instead of an icon menu.

### Fixed

- Menus, selects, tooltips and confirmation dialogs opened from the task
  detail sheet appear above it and accept clicks, so workflow move and cancel
  and the flexible task actions menu work again.

[0.73.0]: https://github.com/alekzonder/tariboy/compare/v0.72.0...v0.73.0

## [0.72.0] - 2026-10-03

### Added

- Workflow images: versioned, immutable images built from a
  `Workflowfile.yaml` source with status instructions and scripts, managed
  with `tariboy workflow validate`, `build`, `ls`, `inspect`, `rm` and
  `tariboy workflow version get|update`. A Store can hold workflow sources
  under `workflows/`, and the official Store provides `development` (pull
  request flow) and `research`.
- A queue can follow a workflow image, bound with
  `ttasks queue workflow set|get|clear`, Compose `task_queues.<QUEUE>.workflow`,
  or the **Workflow** section of Desktop queue settings. Its tasks pin the
  image version and move through statuses owned by an agent pool, the
  customer, or a script: owners leave a status with `ttasks advance`,
  transitions can require artifacts (`ttasks artifacts set|ls|show`) and pass
  check scripts, and script statuses are driven by watch scripts
  (`ttasks workflow get|runs|log`).
- Queue secrets (`ttasks queue secret set|ls|rm`) pass credentials to
  workflow scripts and are redacted from run logs.
- The daemon pauses a stalled workflow task (idle iterations, repeated
  rejections or script failures, or a holder that cannot work) and asks the
  customer; `ttasks workflow resume KEY --decision continue|release` or
  `ttasks cancel` decides it.
- Every task reports a `category` beside `status`. Desktop shows a workflow
  task's status coloured by its category with customer, script, or paused
  markers, and a **Workflow** panel to choose outcomes, edit artifacts, read
  script runs and logs, decide a pause, and move or cancel the task.
- Every script run receives `TARIBOY_QUIET_EXIT`.

### Changed

- The quiet exit code of a recurring script is always `111`, and the Scripts
  tab no longer has a **Quiet exit** field. `--quiet-exit` and the
  `quiet_exit` request parameter are deprecated and still accepted for one
  release; a code other than `111` wraps the command so that code is reported
  as `111`. Schedules stored with another quiet code are rewritten the same
  way on upgrade.
- `tariboy workflow validate`, `tariboy image validate` and
  `tariboy image source validate` print the result and exit 1 when the source
  is invalid.

### Removed

- The earlier configurable task workflow engine, including `ttasks work`,
  `ttasks workflows`, assignment-scoped questions, artifacts and observations,
  and the Compose `workflows:` map. On upgrade, its tasks become flexible tasks
  that keep their current status and record one `workflow.removed` event. A
  non-empty Compose `workflows:` map is rejected; an empty `workflows: {}` is
  ignored.

[0.72.0]: https://github.com/alekzonder/tariboy/compare/v0.71.2...v0.72.0

## [0.71.2] - 2026-10-02

### Changed

- The unused schema-1 Tariboyfile parser and image builder are removed.
  Schema-1 sources were already rejected with a migration message, which is
  unchanged; already-built schema-1 images still run.

### Fixed

- A task whose random key suffix is all digits is no longer renamed after a
  daemon restart. Legacy numeric keys are rewritten once, on the first start
  after the upgrade, and that run is recorded.
- Task search matches text case-insensitively in non-Latin scripts such as
  Cyrillic.
- A task opened from a `?task=` link no longer reopens its drawer every few
  seconds after it is closed.

[0.71.2]: https://github.com/alekzonder/tariboy/compare/v0.71.1...v0.71.2

## [0.71.1] - 2026-09-28

### Changed

- All tasks hides the agent sidebar and its titlebar toggle, and the agent
  in the route no longer narrows the table or preselects the New task agent.
  The saved sidebar state is kept for the Agents view.
- Every GitHub Release starts with a note that the macOS build is not
  notarized and how to allow a blocked DMG.

### Fixed

- Closed tasks with an unread agent question stay in the Active task list
  until the question is read, so the Tasks badge always has a row to open.
- Tag releases restore a Desktop Rust cache warmed on `main`, instead of
  compiling every Rust dependency from scratch.

[0.71.1]: https://github.com/alekzonder/tariboy/compare/v0.71.0...v0.71.1

## [0.71.0] - 2026-09-27

### Added

- Simple and Expert interface modes in Application settings. Simple (the
  default) shows an agent's Chat, Tasks, Console and Configuration tabs and
  opens agents on Chat; Expert keeps the full workspace.
- An Agents | All tasks switch in the titlebar. All tasks lists every
  server's tasks in one view and moving a task to another agent moves it to
  that agent's server.
- Host workspaces: a titlebar switcher narrows the sidebar, Servers tab and
  footer counts to a workspace's hosts, and a manager creates, renames and
  deletes workspaces and moves hosts between them. Default holds every
  unassigned host, so no migration is needed.
- Schema-v2 Tariboyfiles can inherit parent images through `extends`.
- The Tasks tab shows the number of tasks with an unread agent question.
- The audit log records the cause of `harness_error` iterations (launch
  error, or exit code and stderr tail).
- tariboyd creates a default `TASK` queue on start when none exists.

### Changed

- Task Duration is measured from the first move to `in_progress`
  (`started_at`, backfilled by migration 0049) instead of creation.
- New agents default to a 30-minute AI inactivity timeout (was 5 minutes);
  existing agents keep their value.
- Start and Stop live only in the agent header.
- The chat task drawer resizes and shares its width with the Tasks sheet.
- Chat history and chat list reads use a stored, indexed message sender
  (migration 0048) and agent budget windows are summed in one indexed
  query, removing multi-second API stalls on large databases. A chat with
  no messages reports an empty last sender instead of `system`.

### Removed

- The terminal Workspace canvas at `/workspace`. Its saved sidebar width and
  hidden state carry over.
- The Store Build name and tag inputs; use `tariboy image build --name` and
  `--tag` for overrides.

### Fixed

- AI proxy usage is recorded for gzip-compressed upstream responses
  (previously counted without model, tokens or cost).
- New agent resets Interactive to the selected image's default.
- Reordering servers inside a workspace keeps hidden hosts' order.

[0.71.0]: https://github.com/alekzonder/tariboy/compare/v0.70.0...v0.71.0

## [0.70.0] - 2026-09-24

### Added

- Nightly database maintenance. The daemon backs up `tariboyd.db` with
  `VACUUM INTO`, keeps the newest backups, then deletes data older than the
  retention period (finished Native Task trees, AI proxy Usage rows outside the
  longest budget period, fully delivered and consumed messages, daemon events,
  task idempotency records) and compacts the file when enough pages are free.
  Defaults: enabled, 03:00 local, 7 backups, 90 days, compaction at 10% free
  pages. Manage it with `tariboy maintenance get|set|run`, `/api/maintenance`
  or the Backup & data retention card in Settings → General.

### Changed

- Exec moved from the Console tab to the agent header, before Start/Stop, and
  opens a dialog for the optional one-shot prompt. It shows only while the
  agent is enabled and respects an unavailable host.
- The task drawer opens immediately with a loading state instead of waiting
  for the task and its history.
- Refreshing a managed Git Store clone now fetches, hard-resets to the upstream
  branch and removes untracked files, so local modifications no longer block
  it. Local Store checkouts keep fast-forward-only pulls.

### Security

- Database backups are written to the owner-only `<base-dir>/backups/db/`
  (files mode `0600`) and are complete, unmasked copies of the database,
  including agent secret values in plaintext. They are outside the support
  bundle allowlist.

[0.70.0]: https://github.com/alekzonder/tariboy/compare/v0.69.1...v0.70.0

## [0.69.1] - 2026-09-24

### Fixed

- Deliver `script.result` and channel-less self schedules to the agent again.
  Since 0.69.0 they were published to `chat:service:<agent>` with the agent as
  author, and the bus dropped an author's own delivery there, so the message
  was stored but never woke the agent. The service chat now delivers like the
  inbox; DM, tasks and group chats still suppress an agent's own replies.
- A failed Store refresh now returns `store_refresh_failed` (HTTP 409) with the
  tail of Git's error output instead of a generic internal error, so the UI and
  CLI name the cause, such as a local change blocking the fast-forward.
- Store builds restore each tracked `skills-lock.json` after
  `npx skills experimental_install` rewrites it, including on failure, so a
  build no longer leaves the checkout dirty and blocks a later Refresh.

[0.69.1]: https://github.com/alekzonder/tariboy/compare/v0.69.0...v0.69.1

## [0.69.0] - 2026-09-23

### Added

- Chats over the existing channel bus. Every agent has three chats:
  `dm:<agent>` for the conversation, `tasks:<agent>` for task notifications
  and `service:<agent>` for its own wakes. The customer can create a chat that
  belongs to no single agent and holds several:
  `tariboy chat create ID --title TITLE --participants agent:a,agent:b`,
  `tariboy chat join ID PRINCIPAL` and `tariboy chat leave ID PRINCIPAL`
  (`POST /api/chats`, `POST|DELETE /api/chats/{chat}/participants`). Joining
  a chat subscribes an agent to it. Leaving ends membership but keeps
  deliveries the agent already holds.
- The chat list and feed come from the new `chats` and `chat_participants`
  tables (`GET /api/chats`, `GET /api/chats/{chat}`,
  `GET /api/chats/{chat}/participants`), and
  `GET /api/chats/{chat}/unanswered` lists the messages a principal has not
  answered.
- The agent Chat tab opens the agent's tasks and service chats as tabs beside
  the conversation. The sidebar row sums unread messages across all three
  chats and previews whichever chat moved last.

### Changed

- Task `task.*` notifications now publish to `chat:tasks:<agent>`, and
  `task.goal`, `script.result` and channel-less schedules to
  `chat:service:<agent>`. Delivery, wake-up and idempotency behavior is
  unchanged. Schedules created earlier keep their stored channel.
- A reply without `--reply-to` lands in the chat that owns the original
  message, and websocket message hints carry that chat's id.
- An image ref can be rebuilt through every build path: a build publishes one
  ref derived from the image name and `image_version` and moves every
  requested tag, including `latest`, onto it. The mutable/immutable
  publication split is gone. The daemon converts an existing image store once
  at startup and keeps each archive's content digest as its ref id, so pinned
  digests keep resolving.

### Fixed

- The sidebar `Update all` button now asks for confirmation and updates every
  outdated server in turn, instead of only opening one edit dialog.
- Subscription ids no longer collide when two subscriptions are created
  within the same clock tick.

[0.69.0]: https://github.com/alekzonder/tariboy/compare/v0.68.0...v0.69.0

## [0.68.0] - 2026-09-21

### Added

- Show why an agent loop halted: a pre-launch failure (an image manifest that
  does not verify, a missing image skill bridge, an unusable harness) now halts
  the loop with its cause, records one `iteration_failed` audit event, and shows
  the halt reason in the agent workspace header next to the message-queue and
  budget lines, so it is visible on every tab instead of only on Autopilot.
  Start and Restart clear it.
- Report how long a task took: the agent task table gains a `Duration` column,
  measured from creation to completion, or to now while the task is still open.

### Changed

- Rebuild the Tasks workspace around one toolbar. `All tasks`, `My tasks` and
  `Waiting for me` become toolbar chips, the queue rail becomes a
  `Queue: <value>` control listing every queue with its task count, and queue
  administration opens as a 340px right sheet whose cards scroll on their own
  with queue creation in a footer. The list takes the full width of the island.
  The queue filter is applied in the client and now survives switching between
  an Agent tab and All tasks; it is per session on purpose.
- Give the task table one set of column widths shared by its header and rows.
  `Key` is a fixed 168px holding the tree indents, the chevron and the key, with
  deep indents capped and the key truncating rather than wrapping; `Task` takes
  the remaining width; the `Queue` column is gone, since the key prefix already
  names the queue; the drag grip moves into the row padding and appears on hover.

### Fixed

- Name the queue settings sheet for assistive technology and drop the
  `aria-label` that hid the task count and the check mark from a screen reader
  on each queue menu item.

[0.68.0]: https://github.com/alekzonder/tariboy/compare/v0.67.0...v0.68.0

## [0.67.0] - 2026-09-21

### Added

- A Store can refresh and rebuild itself on a schedule: `tariboy store auto NAME --interval MINUTES --image IMAGE` (`POST /api/stores/{name}/auto`) stores a whole-minute interval and the images selected for automatic builds, and the Store detail edits the same policy. Every minute the daemon refreshes each Store whose interval has elapsed and rebuilds the selected images that report an update, publishing both `image_version` and `latest`. Automatic refresh is disabled by default (`--interval 0`), a failed refresh skips that Store builds for the cycle, a failed build does not stop the remaining selections, and `tariboy store show` reports the policy.

[0.67.0]: https://github.com/alekzonder/tariboy/compare/v0.66.2...v0.67.0

## [0.66.2] - 2026-09-18

### Fixed

- Stop a recurring script after it publishes a result, so the agent handles one actionable result instead of receiving repeated wake-ups; a quiet configured exit remains scheduled, and a completed schedule can be explicitly resumed with Exec.

[0.66.2]: https://github.com/alekzonder/tariboy/compare/v0.66.1...v0.66.2

## [0.66.1] - 2026-09-18

### Fixed

- Diagnose a failed macOS release bundling run instead of reporting a bare
  `failed to run bundle_dmg.sh`: the packager prints the disk image state — free
  space, attached images, mounted volumes, running disk image processes and the
  partial bundle output — and reproduces the failure once with verbose bundler
  output, so `bundle_dmg.sh`'s own error reaches the build log.

[0.66.1]: https://github.com/alekzonder/tariboy/compare/v0.66.0...v0.66.1

## [0.66.0] - 2026-09-18

### Added

- Turn the agent Chat tab into a real thread: a single toolbar, author-grouped messages with avatars and hover actions, Markdown with code blocks and task links, day separators and a New messages rule, read receipts, and a composer with Markdown marks, attach, preview and auto-grow. A dot on the Chat tab shows unread.

### Changed

- Read the per-agent unread mark the daemon already keeps instead of tracking unread in the tab, and anchor the New messages rule to the mark the chat was opened at, so it holds its place while the customer reads and is dismissed only by Mark read, by sending a message, or by leaving the chat.
- Accept `--exact` on `tariboy chat read` (`exact` on `POST /api/chats/{agent}/read`), the one write allowed to move the read mark backwards, which is how a chat is marked unread. Without it the mark still only moves forward.

### Fixed

- Derive the remote install and update upload deadline from the payload instead of a fixed 120 seconds: a two-minute floor plus one second per 128 KiB, capped at thirty minutes, with SSH compression enabled. A ~100 MB bundle over a slow uplink no longer fails at "Upload release" with "SSH command timed out", leaving an abandoned staging directory behind.

[0.66.0]: https://github.com/alekzonder/tariboy/compare/v0.65.0...v0.66.0

## [0.65.0] - 2026-09-18

### Added

- Update a remote host from the host itself: `tariboy update [VERSION|latest]` resolves a release, downloads `tariboy_<version>_linux-x86_64.tar.gz` and `SHA256SUMS`, verifies the archive before unpacking it, and runs the release's own `remote-install.sh`, so checksums, locking, atomic activation and rollback stay with the installer. A running daemon is restarted through the managed `tariboyd` link and must report the installed version; a stopped daemon is left stopped. The release now publishes that server archive, and the artifact gate checks its entries, VERSION, internal checksums, installer identity and binary architecture.
- Tag iterations and search them: `tariboy iteration tag add|rm|set` applies a batch of ids and tags in one transaction, `tariboy iteration ls` gains `--tag`, `--started-after` and `--started-before`, and list and inspect report a sorted tags list. The Activity tab filters by the same criteria, keeps them in the URL, shows each row's tags, and edits the selected iteration's tags. Tags live in their own table and are retired with the iteration, so an iteration row stays immutable evidence of an execution.
- Move a task tree between servers: a tree can be exported from one daemon and imported into another under the same keys, driven from the desktop task menu, which exports, imports, comments and cancels the copy left behind. The target refuses an import when it has no queue with that prefix, when a key is already in use, or when the queue runs a workflow.

### Changed

- Mint task keys as the queue prefix plus four random characters instead of a per-queue counter, so two daemons no longer mint the same keys and a task keeps its identity across hosts. Existing numeric keys are rewritten on daemon start and recorded as aliases, so a key already written into an agent context, a branch name or a comment keeps resolving, and lookup normalizes casing.
- Drop the Notifications view from the Tasks workspace. Customer questions stay visible as messages in the agent Chat and as the indicator on the task row, and opening a task marks its unread question notifications read. Agent rows keep only the unread message count, an unanswered question still lifts a row to the top, and a chat draws a New messages line above the first unseen message.

### Fixed

- Render the task panel opened from a chat message, which appeared as an empty sheet and left the Tasks tab unable to load. The dialog drops its resize-grip column when there is no grip, and the panel opens the tasks socket at the host current sequence instead of replaying the whole task event log.
- Keep trailing blank lines out of the Markdown the rich text editor emits, so pressing Enter twice at the end of a description no longer produces a code block holding `&nbsp;`. The blank lines and the caret stay where the author put them.

[0.65.0]: https://github.com/alekzonder/tariboy/compare/v0.64.0...v0.65.0

## [0.64.0] - 2026-09-17

### Added

- Open a task straight from its chat message: a task key attached to a task notification, or written in the message text, is now a link that opens the full task panel over the conversation, with its own load, tasks-socket refresh, and writes for status, priority, assignee, pull request, description, comments, and relations.
- Show the configured runtime in the agent header as one compact `harness · model · effort` chip, so the operator no longer has to open Configuration to see it. Values the selected harness does not report are omitted, and the chip disappears when none remain.

### Changed

- Order the agent workspace tabs as Chat, Tasks, Console, Autopilot, Activity, Configuration, Advanced. Tab routes, tab contents, and the default entry tab are unchanged.
- Keep the open agent tab when switching agents in the sidebar instead of always reopening Console, falling back to Console for Workspace, a team, or a freshly created agent. A selected task is intentionally not carried over.

### Fixed

- Exclude a root-level `evals` directory from a built image when packaging a skill, so authoring evidence no longer adds bytes, file count, and hash input to the immutable image. Nested paths such as `references/evals/` are unaffected.

[0.64.0]: https://github.com/alekzonder/tariboy/compare/v0.63.0...v0.64.0

## [0.63.0] - 2026-09-16

### Added

- Read an agent inbox as a chat with the customer: a Chat tab beside Tasks with Chat and Channels views, a sidebar that orders unpinned agents by their most recent conversation and carries per-agent unread counts, and `GET /api/chats`, `GET /api/chats/{agent}`, and `POST /api/chats/{agent}/read` served from the existing messages table.
- Stream one publication hint per message for every agent on the host over `GET /api/messages/ws`, so Desktop refetches on delivery instead of polling while HTTP stays authoritative.

### Changed

- Fix the customer login at `customer`, overridable through the `customer_login` daemon config key, instead of following the account that runs the daemon. The first start after upgrading carries existing tasks, comments, waits, notification state, and the old `user:<$USER>` channel over to the new principal in one transaction.
- Attribute task notifications to the causing principal in `data.from` while keeping `source` as `system:tasks`, and attribute an operator publish to the customer principal with `reply_to` support, so an agent reply stays in the conversation instead of in its own inbox.

### Fixed

- Correct the channel bus reference and messaging architecture pages, which still described the removed automatic acknowledgement path, together with the channel prefix list, `group request` semantics, the subscription dedup key, and the repeated-message troubleshooting entry.

[0.63.0]: https://github.com/alekzonder/tariboy/compare/v0.62.3...v0.63.0

## [0.62.3] - 2026-09-16

### Fixed

- Keep macOS publication focused on version validation, signed packaging, and release assets while completing repository checks before the release tag is pushed.

[0.62.3]: https://github.com/alekzonder/tariboy/compare/v0.62.2...v0.62.3

## [0.62.2] - 2026-09-16

### Fixed

- Complete macOS release-check portability with cross-platform harness fixtures, Darwin non-terminal handling, canonical frozen skill paths, and BSD-compatible Desktop test path resolution while preserving live-state protections.

[0.62.2]: https://github.com/alekzonder/tariboy/compare/v0.62.1...v0.62.2

## [0.62.1] - 2026-09-15

### Fixed

- Make repository and release checks portable to macOS by accepting canonical image source roots through system symlinks, enforcing Darwin Unix-socket limits, handling non-terminal shim probes, and supplying test-only installer command fallbacks without weakening production requirements.

[0.62.1]: https://github.com/alekzonder/tariboy/compare/v0.62.0...v0.62.1

## [0.62.0] - 2026-09-15

### Added

- Organize the console sidebar into Agents, Groups, and Servers tabs, with pinned agents, unread-question priority, cross-server group membership, and server update actions.
- Allow trusted image-creator agents to build images from any source directory readable by the daemon.

### Changed

- Let builds, imports, retags, and registry publication advance any non-reserved image tag while retaining prior archives by digest for pinned assignments.
- Run the repository checks before packaging a tagged Desktop release.

### Removed

- Remove the Evals subsystem and its image manifest, plugin, CLI, API, UI, and runtime surfaces while retaining legacy persisted data for safe upgrades.
- Remove AI proxy Rules, including model routing, model allow/deny policy, rate limits, and their CLI, API, UI, and runtime surfaces.

### Fixed

- Flush queued AI usage before terminal iteration span aggregation.
- Hide stale image provenance and reject stale team snapshots after a tag advances to different image bytes.

[0.62.0]: https://github.com/alekzonder/tariboy/compare/v0.61.0...v0.62.0

## [0.61.0] - 2026-09-14

### Added

- Retry Store image builds after immutable-tag conflicts with an alternative target image name or tag.
- Show dependency titles and statuses in task details.

### Changed

- Simplify the task detail panel around the compact operator template.

### Removed

- Remove Judge reviews, automation, controlled improvements, and their CLI, API, UI, documentation, and persisted state.

### Fixed

- Reject immutable Store image targets before restoring skill locks, release build controls promptly, and prevent stale inventory reads from replacing newer build results.
- Allow Goal wake delivery to recover after an earlier delivery reaches the dead-letter queue.

[0.61.0]: https://github.com/alekzonder/tariboy/compare/v0.60.0...v0.61.0

## [0.60.0] - 2026-09-14

### Added

- Copy the original Markdown from task comments.
- Browse image registry names and tags with coherent details for the selected image version.
- Show each agent’s active image version and the next available image version.
- Filter console agents by agent name or server label.

### Changed

- Redesign the console shell, task table, task detail panel, and application theme around the denser metal-and-blue operator interface.
- Publish documentation automatically after a successful Desktop release.

### Fixed

- Preserve unsupported Markdown content when editing task text in the rich editor.
- Accept `argument-hint` in Agent Skill frontmatter.
- Report latest-image inspection errors and retain existing agent image status when inventory refresh fails.
- Use English labels in application settings.
- Keep task detail controls usable at narrow widths and preserve Queue settings labels.

[0.60.0]: https://github.com/alekzonder/tariboy/compare/v0.59.0...v0.60.0

## [0.59.0] - 2026-09-11

### Added

- Connect a saved SSH host to an existing Tariboy daemon without installing, starting, or restarting remote software.

### Fixed

- Preserve the selected SSH setup mode while retrying an unavailable daemon or falling back to installation.

[0.59.0]: https://github.com/alekzonder/tariboy/compare/v0.58.1...v0.59.0

## [0.58.1] - 2026-09-11

### Fixed

- Restore Desktop builds after adding the application updater.

[0.58.1]: https://github.com/alekzonder/tariboy/compare/v0.58.0...v0.58.1

## [0.58.0] - 2026-09-11

### Added

- Download, signature-verify, and explicitly install Desktop updates from application settings.
- Present one-shot instructions, messages, and Native Task goals in a platform-owned processing order for every non-bare agent.

### Changed

- Publish the checked DMG together with the signed updater archive, signature, and update catalog through an atomic draft-to-public release flow.
- Prepare locked Go, UI, and documentation dependencies automatically before fast checks.

### Fixed

- Compare Desktop update versions by SemVer precedence.
- Preserve Markdown headings in Native Task questions.
- Remove the release artifact scanner’s ripgrep runtime dependency.

[0.58.0]: https://github.com/alekzonder/tariboy/compare/v0.57.5...v0.58.0

## [0.57.5] - 2026-09-11

### Fixed

- Avoid false credential detections while verifying macOS release artifacts.

[0.57.5]: https://github.com/alekzonder/tariboy/compare/v0.57.4...v0.57.5

## [0.57.4] - 2026-09-11

### Fixed

- Publish macOS release artifacts without requiring Linux binary emulation.

[0.57.4]: https://github.com/alekzonder/tariboy/compare/v0.57.3...v0.57.4

## [0.57.3] - 2026-09-11

### Fixed

- Use the installed QEMU runner when verifying Linux x86_64 release binaries on macOS.

[0.57.3]: https://github.com/alekzonder/tariboy/compare/v0.57.2...v0.57.3

## [0.57.2] - 2026-09-11

### Fixed

- Publish signed macOS artifacts without running the isolated desktop smoke suite.

[0.57.2]: https://github.com/alekzonder/tariboy/compare/v0.57.1...v0.57.2

## [0.57.1] - 2026-09-11

### Fixed

- Validate schema-v2 editable image sources before building them.

[0.57.1]: https://github.com/alekzonder/tariboy/compare/v0.57.0...v0.57.1

## [0.57.0] - 2026-09-11

### Added

- Run eligible saved scripts immediately, including idle recurring scripts.
- Mark all unread task notifications as read from the Tasks inbox.
- Register editable image-source API routes.

### Fixed

- Confine agent-authored image builds to each agent's managed workdir.
- Register editable image-source routes.

[0.57.0]: https://github.com/alekzonder/tariboy/compare/v0.56.0...v0.57.0

## [0.56.0] - 2026-09-11

### Added

- Agent and global shell-script editors, validated and sourced before harness launch.
- Image version visibility and Store image update status.
- Persistent, keyboard-accessible sidebar ordering for servers, teams, and agents.
- Stalled AI-iteration reporting and configurable stall timeout.
- Tag-triggered macOS release publication.

### Fixed

- Shell-script loading and launch-argument preservation.
- Audit-export streaming and transcript deduplication.
- Task and goal selection with pull-request URLs.

[0.56.0]: https://github.com/alekzonder/tariboy/compare/v0.55.0...v0.56.0
