---
title: Command reference
description: The full command reference — operator commands and agent capability scripts.
sidebar:
  label: Commands
  icon: terminal
---

# tariboy command reference

tariboy has four command surfaces:

1. **Operator commands** — `tariboy <group> <command>` (`sa` below is a
   convenience alias you define yourself, e.g. `alias sa=tariboy`; the build
   ships no such symlink), run by a
   human or CI against the daemon. Global flags: `--json`, `--help`,
   `--help-json`, `--version`. `--json` counts as the global flag wherever a
   flag may appear, but not as the value of another flag. The list below is
   generated from the binary's registry (`tariboy --help-json`).
2. **Agent capability scripts** — the applicable packaged skill's
   `scripts/*.sh` launcher, run *inside* an agent over `$TARIBOY_TOOLS_SOCKET`.
3. **Native Tasks** — `tariboy-tasks`, installed as `ttasks`, runs shared task
   verbs for an operator or an identity-bound agent. `ttasks --version` reports
   its build and `ttasks --json …` requests JSON output. `tariboy tasks …` runs
   the same client, always in operator mode; the table below does not list it.
4. **Mail** — `tariboy-mail`, installed as `tmail`, reads, sends and answers
   letters for an identity-bound agent or an operator. `tariboy mail …` runs the
   same client, always in operator mode.

## Operator commands

| Command | Summary |
| --- | --- |
| `tariboy agent alias get NAME` | Read the agent display alias |
| `tariboy agent alias set NAME` | Get or set the agent display alias (omit value to read, empty to clear) |
| `tariboy agent budget get NAME` | Show an agent's calendar USD budget |
| `tariboy agent budget set NAME HOUR-USD DAY-USD WEEK-USD MONTH-USD` | Set an agent's calendar USD budget |
| `tariboy agent color get NAME` | Read the agent accent color |
| `tariboy agent color set NAME` | Get or set the agent accent color (omit value to read, empty to clear) |
| `tariboy agent cwd NAME` | Get or set the agent working directory (omit value to read, empty to clear); applies next iteration |
| `tariboy agent effort NAME` | Get or set the agent effort (omit value to read); applies next iteration |
| `tariboy agent env NAME` | Get or replace the agent environment (omit value to read); applies from the next iteration |
| `tariboy agent exec NAME` | Run a manual iteration now, with an optional one-shot prompt |
| `tariboy agent file create NAME PATH` | Create a file or folder in an agent's cwd (jailed) |
| `tariboy agent file delete NAME PATH` | Delete a file or folder in an agent's cwd (jailed) |
| `tariboy agent file list NAME` | List a directory in an agent's cwd (jailed) |
| `tariboy agent file read NAME PATH` | Read a file from an agent's cwd (text; binary/large =\> marker) |
| `tariboy agent file rename NAME FROM TO` | Rename/move a file within an agent's cwd (jailed) |
| `tariboy agent file write NAME PATH CONTENT` | Overwrite/save a file in an agent's cwd (jailed) |
| `tariboy agent goal-delivery-cooldown NAME SECONDS` | Set the Goal delivery cooldown |
| `tariboy agent goal-enabled NAME` | Enable or disable Native Task goal selection |
| `tariboy agent goal-wait-customer-timeout NAME` | Set the Goal customer-wait grace period |
| `tariboy agent harness NAME` | Get or set the agent harness type (omit value to read); restart the agent to apply |
| `tariboy agent image cancel NAME` | Cancel a pending agent image |
| `tariboy agent image set NAME IMAGE` | Select an image for the agent's next iteration |
| `tariboy agent image status NAME` | Show current, pending, and next agent image |
| `tariboy agent inspect NAME` | Show one agent's full config |
| `tariboy agent interactive NAME` | Get or set interactive mode (bool); restart the agent to apply |
| `tariboy agent kill NAME` | Kill the current iteration via its shim |
| `tariboy agent mail clear NAME` | Delete the open letters in an agent's inbox |
| `tariboy agent messages batch NAME VALUE` | Set the agent messages batch |
| `tariboy agent messages max-queue NAME VALUE` | Set the agent messages max queue |
| `tariboy agent model NAME` | Get or set the agent model (omit value to read); applies next iteration |
| `tariboy agent notes get NAME` | Read the agent notes |
| `tariboy agent notes set NAME` | Get or set the agent notes (omit value to read, empty to clear) |
| `tariboy agent ps` | List agents |
| `tariboy agent pull NAME PATH` | Read a base64 file from an agent's cwd (used by 'cp') |
| `tariboy agent reprovision NAME` | Re-unpack an agent's image tree in place and restart its loop, keeping its data |
| `tariboy agent restart NAME` | Restart an agent and run one iteration now |
| `tariboy agent rm NAME` | Remove an agent (stop it first, or --force) |
| `tariboy agent run IMAGE` | Create an agent from an image (image:tag); starts stopped — enable with 'agent start' |
| `tariboy agent screen NAME` | Capture the interactive screen (tmux capture-pane) |
| `tariboy agent send-keys NAME` | Send keys into the interactive session (raw items, or a line+Enter) |
| `tariboy agent shell-script get NAME` | Read an agent shell script |
| `tariboy agent shell-script set NAME` | Set an agent shell script |
| `tariboy agent start NAME` | Start (enable) an agent's loop |
| `tariboy agent status history NAME` | List an agent's status-message history (from the audit log) |
| `tariboy agent status show NAME` | Show one agent's runtime status |
| `tariboy agent stop NAME` | Stop an agent's loop (current iteration lives) |
| `tariboy agent usage NAME` | Per-agent AI usage: grouped rows + time-bucketed series |
| `tariboy backup TARGET` | Back up an agent (or 'all') to a portable tar.gz |
| `tariboy budget ls` | List configured budgets |
| `tariboy budget set` | Set a cost budget (scope agent:\<name\>\|group:\<g\>\|global) |
| `tariboy budget status` | Show current spend vs limit for each budget |
| `tariboy compose archive` | Create a compose-only portable team archive; transfer runnable images separately |
| `tariboy compose build` | Build the file's images |
| `tariboy compose down` | Remove the file's agents and groups; --volumes also deletes their data |
| `tariboy compose exec MEMBER [PROMPT...]` | Run an iteration of one of the file's agents now, with an optional prompt |
| `tariboy compose import` | Preview and import compose-only team/runtime configuration |
| `tariboy compose kill [MEMBERS...]` | Kill the current iteration of the file's agents, or the named agents and groups |
| `tariboy compose logs [MEMBERS...]` | Print recent events of the file's agents, or the named agents and groups |
| `tariboy compose ps` | List the file's agents and their live state |
| `tariboy compose restart [MEMBERS...]` | Restart the file's agents, or the named agents and groups |
| `tariboy compose rm [MEMBERS...]` | Remove the file's stopped agents, or the named agents and groups |
| `tariboy compose start [MEMBERS...]` | Start the file's agents, or the named agents and groups |
| `tariboy compose status` | Compare the file with the daemon and report drift |
| `tariboy compose stop [MEMBERS...]` | Stop the file's agents, or the named agents and groups |
| `tariboy compose up` | Build, create and start the file's images, groups and agents; start the daemon first |
| `tariboy context get NAME` | Read the agent's CONTEXT.md |
| `tariboy context set NAME` | Overwrite the agent's CONTEXT.md |
| `tariboy cp SRC` | Upload a file to the server: cp LOCAL_FILE; download: cp AGENT:SRC LOCAL_DST |
| `tariboy daemon agent-shell-script get` | Read the global agent shell script |
| `tariboy daemon agent-shell-script set` | Set the global agent shell script |
| `tariboy daemon config get` | Read daemon config (all keys, or one with --key) |
| `tariboy daemon config set KEY VALUE` | Set a daemon config key (runtime-mutable) |
| `tariboy daemon logs` | Tail ~/.tariboyd/tariboyd.log |
| `tariboy daemon reindex` | Rebuild ai_requests metadata from proxy-transcript.jsonl files |
| `tariboy daemon restart` | Restart the daemon (stop then start) |
| `tariboy daemon start` | Start tariboyd detached (no-op if already running) |
| `tariboy daemon status` | Show whether the daemon runs (exit 0) or is stopped (exit 1); --json prints its full status |
| `tariboy daemon stop` | Stop the daemon (SIGTERM, then SIGKILL on timeout) |
| `tariboy files upload NAME CONTENT` | Upload a file to the server and return its absolute path |
| `tariboy fs list` | List directories under the daemon filesystem root ($HOME-jailed) |
| `tariboy group assign NAME AGENT` | Assign an agent to a group (empty group leaves) |
| `tariboy group create NAME` | Create (or update) a group with an optional lead |
| `tariboy group inspect NAME` | Show a group's lead, members, mail address and shared dir |
| `tariboy group lead set NAME LEAD` | Set a group's lead to a current member |
| `tariboy group ls` | List groups (name/lead/member count) |
| `tariboy group member rm NAME AGENT` | Remove an agent from a group without deleting it |
| `tariboy group rename NAME NEW-NAME` | Rename a group without deleting its agents |
| `tariboy group rm NAME` | Remove a group (detach members; --volumes drops the shared dir) |
| `tariboy harness codex-approval get` | Read the codex approval mode of this daemon (yolo\|approve-for-me) |
| `tariboy harness codex-approval set MODE` | Set the codex approval mode of this daemon for the next iterations |
| `tariboy harness models TYPE` | List the models and efforts a harness supports on the daemon host |
| `tariboy image build` | Start a background build of an agent image from a Tariboyfile.yaml |
| `tariboy image file REF PATH` | Read a single file packed into an image |
| `tariboy image files REF` | List the files packed into an image |
| `tariboy image inspect REF` | Show an image manifest |
| `tariboy image ls` | List built agent images |
| `tariboy image prompt REF` | Print an image's assembled prompt |
| `tariboy image provenance REF` | Show local image source provenance |
| `tariboy image rm REF` | Remove a built image |
| `tariboy image source build NAME` | Build an image from an editable image source |
| `tariboy image source create NAME` | Create an editable image source |
| `tariboy image source file get NAME PATH` | Read an editable image source file |
| `tariboy image source files NAME` | List editable files in an image source |
| `tariboy image source inspect NAME` | Show editable image source metadata |
| `tariboy image source ls` | List editable image sources |
| `tariboy image source rm NAME` | Remove an editable image source without removing built images |
| `tariboy image source validate NAME` | Validate an editable image source |
| `tariboy image template REF` | Show an image prompt template |
| `tariboy image validate` | Validate an image source directory |
| `tariboy image version get` | Print image_version from a local Tariboyfile.yaml |
| `tariboy image version update PART` | Bump image_version in a local Tariboyfile.yaml |
| `tariboy iteration extend-timeout NAME ID` | Extend a running iteration timeout |
| `tariboy iteration inspect NAME ID` | Show one iteration |
| `tariboy iteration logs NAME ID` | Print an iteration's harness logs |
| `tariboy iteration ls NAME` | List an agent's iterations |
| `tariboy iteration tag add NAME` | Add tags to one or more iterations |
| `tariboy iteration tag rm NAME` | Remove tags from one or more iterations |
| `tariboy iteration tag set NAME` | Replace the tags of one or more iterations |
| `tariboy login REGISTRY` | Store a bearer token for a tariboy-store registry |
| `tariboy logs NAME` | Stream or print an agent's events (-f to follow) |
| `tariboy loop ai-stall-timeout NAME` | Get or set loop ai-stall-timeout (seconds); omit value to read |
| `tariboy loop hard-timeout NAME` | Get or set loop hard-timeout (seconds); omit value to read |
| `tariboy loop interval NAME` | Get or set loop interval (seconds); omit value to read |
| `tariboy loop max-idle NAME` | Get or set loop max-idle (seconds); omit value to read |
| `tariboy loop on-error NAME` | Get or set loop on-error policy; omit value to read |
| `tariboy loop on-timeout NAME` | Get or set loop on-timeout policy; omit value to read |
| `tariboy loop schedule NAME` | Get, set or clear the loop schedule (RFC 5545 RRULE, whole minutes); omit value to read |
| `tariboy loop timeout NAME` | Get or set loop timeout (seconds); omit value to read |
| `tariboy maintenance get` | Show database maintenance settings and the last run |
| `tariboy maintenance run` | Back up the database, delete expired data, and compact it now |
| `tariboy maintenance set` | Change database maintenance settings; unset flags keep their value |
| `tariboy onboarding retry` | Register the default Store and build the starter image again when they are missing |
| `tariboy onboarding status` | Report the first-run state: default Store, starter image, harness CLIs and agent count |
| `tariboy operation log ID` | Read the log of a background operation |
| `tariboy operation ls` | List background operations, newest first |
| `tariboy operation show ID` | Show a background operation |
| `tariboy plugin action NAME ACTION` | Invoke a generic action on a running plugin |
| `tariboy plugin contributions` | List enabled plugin CLI and settings contributions |
| `tariboy plugin inspect NAME` | Show one plugin's manifest, state and socket |
| `tariboy plugin install PATH` | Install and start a plugin from a directory with plugin.json |
| `tariboy plugin logs NAME` | Show captured plugin stdout/stderr |
| `tariboy plugin ls` | List installed plugins (name/version/types/state/health) |
| `tariboy plugin restart NAME` | Restart a single plugin in place (stop + start) without touching others |
| `tariboy plugin rm NAME` | Stop and remove a plugin |
| `tariboy plugin routes NAME` | Show a plugin's external routes, such as its chat bindings |
| `tariboy prompt get NAME` | Preview the agent's assembled next-iteration prompt |
| `tariboy prune AGENT` | Prune old iterations for an agent now (or 'all'); --dry-run lists victims |
| `tariboy pull REF` | Pull an image from a tariboy-store registry into the local image store |
| `tariboy push REF` | Push a local image to a tariboy-store registry |
| `tariboy restore FILE` | Restore an agent from a backup tar.gz (optionally under a new name) |
| `tariboy retention get AGENT` | Show the effective retention policy for an agent, with its own override and the default (or 'default') |
| `tariboy retention set AGENT` | Set the retention policy for an agent (0 = use the default) or 'default' (0 = unlimited) |
| `tariboy schedule ls NAME` | List an agent's schedules (read-only) |
| `tariboy script cancel NAME ID` | Cancel a script or one run |
| `tariboy script logs NAME ID` | Read a bounded script run log tail |
| `tariboy script ls NAME` | List an agent's local scripts |
| `tariboy script rerun NAME ID` | Run a stored script now, resuming a stopped schedule |
| `tariboy script rm NAME ID` | Remove an inactive script |
| `tariboy script run NAME DESCRIPTION COMMAND` | Run a local command once |
| `tariboy script run-get NAME ID` | Get one script run |
| `tariboy script runs NAME ID` | List a script's runs |
| `tariboy script schedule NAME DESCRIPTION COMMAND` | Schedule a fixed-interval local command |
| `tariboy secret ls NAME` | List secret keys (values are never shown) |
| `tariboy secret rm NAME KEY` | Remove a secret |
| `tariboy secret set NAME KEY` | Set a secret; value from --value or stdin |
| `tariboy store add NAME SOURCE` | Register a local or Git Store |
| `tariboy store auto NAME` | Set a Store's automatic refresh and build policy |
| `tariboy store list` | List registered Stores |
| `tariboy store refresh NAME` | Refresh a Store |
| `tariboy store remove NAME` | Remove a Store registration |
| `tariboy store show NAME` | Show a Store and its images |
| `tariboy team compose NAME` | Render a group as tariboy-compose.yaml |
| `tariboy team import archive apply ID` | Build images and create a staged portable team |
| `tariboy team import archive plan ID` | Validate and plan a staged portable team import |
| `tariboy team import archive status ID` | Inspect portable team import progress |
| `tariboy team import yaml YAML` | Preview pasted tariboy-compose.yaml |
| `tariboy transcript NAME ID` | Show the AI proxy transcript for an iteration (parsed; ?raw=1 for bytes) |
| `tariboy update` | Install a published release over this installer-managed installation |
| `tariboy usage` | Aggregate AI usage and cost from ai_requests |
| `tariboy user-prompt get NAME` | Read the agent's standing user-prompt |
| `tariboy user-prompt set NAME TEXT` | Set the agent's standing user-prompt |
| `tariboy version` | Print the Tariboy version |
| `tariboy workflow build` | Start a background build of a workflow image from a Workflowfile.yaml |
| `tariboy workflow inspect NAME` | Show a workflow image manifest |
| `tariboy workflow ls` | List built workflow images |
| `tariboy workflow rm NAME TAG` | Remove a workflow image tag, and its content when no tag remains |
| `tariboy workflow validate` | Validate a workflow source directory without building it |
| `tariboy workflow version get` | Print workflow_version from a local Workflowfile.yaml |
| `tariboy workflow version update PART` | Bump workflow_version in a local Workflowfile.yaml |

A test keeps this table equal to `tariboy --help-json`. Each row shows the
command's required positional arguments; `tariboy COMMAND --help` lists its
flags. After adding, removing or renaming a command, regenerate the table:

```bash
go test ./cmd/tariboy -run TestCommandsDocMatchesHelpJSON -update
```

### Host-local commands

Some of the commands above run in the CLI process instead of calling the
daemon API. They act on the host the CLI runs on and work while the daemon is
down:

- `tariboy daemon start|stop|restart|status|logs` control and inspect the local
  daemon process. `daemon status` prints `running (pid N, version V)` and exits
  0, or prints `stopped` and exits 1; with `--json` it prints the daemon's full
  status, or `{"running":false}`. The API still serves
  `GET /api/daemon/status`. `daemon logs [-f] [--tail N]` prints the last N
  lines (default 200) of `~/.tariboyd/tariboyd.log`.
- `tariboy update [VERSION|latest] [--force] [--cleanup [--keep N]]` installs a
  published release over this installer-managed installation, restarts a
  running daemon, and with `--cleanup` deletes all but the newest N releases
  (default 5).
- `tariboy compose VERB` reads a compose file (`-f FILE`, default
  `tariboy-compose.yaml`) and drives the daemon API; `compose up` starts the
  daemon first unless `--no-start` or `TARIBOY_NO_AUTOSTART` is set. The
  compose flags may also come before the verb, as in
  `tariboy compose -f FILE up`.

`tariboy update` updates only an installer-managed `linux-x86_64` or
`linux-aarch64` installation
and is described in [Remote hosts](/docs/remote-hosts#update-a-host-from-its-own-shell).

### Iteration tags

Tags annotate an iteration without changing it: one agent marks the iterations
of another agent it has already handled. An iteration can carry several tags.
A tag is at most 64 characters and contains no comma, whitespace, or control
character.

The three mutations take a batch of iteration ids and a batch of tags, and apply
in one transaction — the whole batch succeeds or nothing is written. An id that
belongs to another agent fails the batch with `not_found`.

```bash
tariboy iteration tag add worker --id worker-20260918T10-1 --id worker-20260918T11-2 --tag processed
tariboy iteration tag rm worker --id worker-20260918T10-1 --tag processed
tariboy iteration tag set worker --id worker-20260918T10-1 --tag processed --tag reviewed
tariboy iteration tag set worker --id worker-20260918T10-1   # no --tag clears every tag
```

`tariboy iteration ls` searches by tag and by iteration start date. `--tag`
takes a comma-separated list and matches an iteration carrying **any** of them.
Both date bounds are inclusive and accept RFC3339 or a bare `YYYY-MM-DD` day:

```bash
tariboy iteration ls worker --tag processed,reviewed
tariboy iteration ls worker --started-after 2026-09-01 --started-before 2026-09-18
```

`iteration ls` rows and `iteration inspect` both report a sorted `tags` list.

## Agent capability scripts

Run inside an agent; the socket comes from `$TARIBOY_TOOLS_SOCKET`.

| Tool | Purpose |
| --- | --- |
| `scripts/whoami.sh` | Print agent, cwd and current iteration |
| `scripts/status.sh` | Print the agent status |
| `scripts/loop.sh done` | Signal this iteration is finished (i-am-done) |
| `scripts/context.sh get` | Print the durable working memory (CONTEXT.md) |
| `scripts/context.sh set <text>` | Overwrite the durable working memory |
| `scripts/schedule.sh add --kind cron\|oneshot --spec S [--message JSON]` | Schedule a future letter; without a recipient it writes to you |
| `scripts/schedule.sh ls` | List your schedules |
| `scripts/schedule.sh cancel ID` | Cancel a schedule |
| `scripts/scripts.sh ls` | List your scripts |
| `scripts/scripts.sh run NAME [--description TEXT] -- COMMAND` | Queue exactly one local run |
| `scripts/scripts.sh schedule NAME --every SECONDS -- COMMAND` | Run now and repeat after each completion; a run that exits `111` (`TARIBOY_QUIET_EXIT`) stays quiet |
| `scripts/scripts.sh runs SCRIPT_ID` / `logs RUN_ID` | Inspect run history and bounded logs |
| `scripts/scripts.sh rerun SCRIPT_ID` | Run a completed one-shot or idle recurring definition immediately |
| `scripts/scripts.sh cancel SCRIPT_OR_RUN_ID` / `rm SCRIPT_ID` | Cancel work or remove inactive history |
| `scripts/image_creator.sh build --name NAME [--tag TAG] --path DIR` | Build a schema-v2 image from a daemon-readable source directory (`image-creator` only); relative paths start at the managed workdir |

## Mail (`tmail …`)

`tariboy-mail` is the real executable and `tmail` its managed alias. A
non-empty `TARIBOY_TOOLS_SOCKET` selects agent mode: the sender is the calling
agent and every read is scoped to its mailbox. Without the variable, the
client uses the host Unix daemon socket and acts as the customer unless `--as`
names another principal. `tariboy mail …` is the same client, always in
operator mode. See the [Mail
reference](/docs/reference/mail) for flags, routes and debugging.

| Command | Purpose |
| --- | --- |
| `tmail inbox [--all] [--limit N] [--before TIME]` | List open letters, oldest first; `--all` lists every letter, newest first |
| `tmail sent` / `tmail archive` | List the letters you sent / your closed and dead letters |
| `tmail show ID` | Show one letter and its reply |
| `tmail send --to P[,P] --subject S --text T [--kind K] [--deadline D] [--notice] [--data JSON]` | Send a letter; a request unless `--notice` |
| `tmail reply ID TEXT [--ask]` | Answer a letter, which closes it |
| `tmail requeue ID` | Return a dead letter to the inbox |
| `tmail close ID [--note T]` | Archive without a reply (operator only) |
| `tmail read ID...` | Mark letters read; a read notice to an agent closes (operator only) |
| `tmail unread` | Count open letters by sender and kind (operator only) |
| `tmail clear --as agent:NAME [--yes]` | Delete an agent's open letters (operator only) |

## Native Tasks (`ttasks …`)

`tariboy-tasks` is the real executable and `ttasks` its managed alias. A
non-empty `TARIBOY_TOOLS_SOCKET` selects agent mode; it uses only that socket
and fails closed if it cannot connect. Without the variable, the client uses
the host Unix daemon socket as the customer actor. The bare `tasks` command is
only the optional capability-controlled legacy agent shim.

`tariboy tasks …` is the same client with the same commands, flags, output,
and exit codes. It always runs in operator mode on the daemon socket
(`--socket` or the default path) and ignores `TARIBOY_TOOLS_SOCKET`, so an
agent that runs it sees every task and acts as the customer. Its help names
`tariboy tasks`; `tariboy tasks --help-json` prints the `ttasks` tree.

These shared verbs are available in both modes; in agent mode every mutation
derives identity from the socket.

| Command | Purpose |
| --- | --- |
| `ttasks list [--status S] [--text T] [--sort ORDER] [--limit N] [--after KEY]` | List and search visible tasks; [Native Tasks](/docs/tasks) lists every filter |
| `ttasks mine` | Alias of `ttasks list` |
| `ttasks ready [--queue Q] [--claim]` | List ready work; `--claim` requires agent mode |
| `ttasks show KEY` | Show task detail, comments, waits, and relations |
| `ttasks create --queue Q --title T [--assignee A]` | Create a queue-root task |
| `ttasks create --parent KEY --title T` | Create a child inheriting queue/context |
| `ttasks update KEY [--title T] [--description D] [--status S]` | Update fields with optimistic revision |
| `ttasks assign KEY ASSIGNEE` | Hand work to an agent name |
| `ttasks comment KEY [TEXT \| --body TEXT \| --file PATH]` (stdin when absent; up to 64 KiB) | Add a task comment |
| `ttasks ask KEY agent:name\|user:login [TEXT \| --file PATH]` (stdin when absent; up to 64 KiB) | Mention a principal and record an open answer wait |
| `ttasks move KEY [--parent KEY] [--before KEY] [--to-root]` | Reparent/reorder in the same queue |
| `ttasks block KEY --by BLOCKER` | Add a directed, cycle-checked blocking relation |
| `ttasks relate KEY OTHER` | Add a symmetric related link |
| `ttasks done KEY [--complete-anyway]` | Complete, optionally overriding active descendants |

These verbs drive a task that follows a queue's workflow image. Agent mode
sends `advance`, `request_get`, `artifact_set`, `artifact_ls`, `artifact_show`,
`workflow_get`, `workflow_runs`, and `workflow_run_log` over the identity-bound
socket; operator mode calls the REST routes as the customer.

| Command | Modes | REST route |
| --- | --- | --- |
| `ttasks advance KEY --outcome NAME [--from STATUS] [--message TEXT] [--no-wait]` (`--from` refuses with `status_changed` when the task has moved on; waits for the transition's checks, polling `GET /api/tasks/{key}/workflow/requests/{id}`, unless `--no-wait`) | agent, operator | `POST /api/tasks/{key}/advance` |
| `ttasks artifacts set KEY NAME [VALUE \| --file PATH]` (stdin when absent; stored as given, up to 64 KiB) | agent, operator | `PUT /api/tasks/{key}/artifacts/{name}` |
| `ttasks artifacts ls KEY` | agent, operator | `GET /api/tasks/{key}/artifacts` |
| `ttasks artifacts show KEY NAME` | agent, operator | `GET /api/tasks/{key}/artifacts/{name}` |
| `ttasks workflow get KEY` | agent, operator | `GET /api/tasks/{key}/workflow` |
| `ttasks workflow runs KEY` (newest first) | agent, operator | `GET /api/tasks/{key}/workflow/runs` |
| `ttasks workflow log KEY RUN [--max-bytes N]` (last 64 KiB by default, at most 1 MiB; queue secrets redacted; customer and the task's holders only) | agent, operator | `GET /api/tasks/{key}/workflow/runs/{id}/log` |
| `ttasks workflow move KEY --to STATUS [--reason TEXT]` | operator only | `POST /api/tasks/{key}/workflow/move` |
| `ttasks workflow upgrade KEY [--reason TEXT]` (pins the task to the queue's current workflow version and restarts it at that version's initial status; artifacts and holders stay; errors `workflow_current`, `workflow_unbound`, `workflow_closed`, `workflow_pool_empty`, `workflow_secret_missing`, `forbidden`) | operator only | `POST /api/tasks/{key}/workflow/upgrade` |
| `ttasks workflow resume KEY --decision continue\|release` (resolves the pause of a task the daemon paused: `continue` keeps the holder and zeroes the counters; `release` takes a pool task from its holder and dispatches it to another member, or leaves it `open` and unassigned; errors `invalid_decision`, `workflow_not_paused`, `forbidden`) | operator only | `POST /api/tasks/{key}/workflow/resume` |
| `ttasks workflow watch KEY` (starts the watch script of a script status now instead of at its next scheduled time and returns the pending run; errors `workflow_no_watch`, `script_running`, `workflow_closed`, `workflow_paused`, `forbidden`) | operator only | `POST /api/tasks/{key}/workflow/watch` |
| `ttasks cancel KEY` | operator only | `POST /api/tasks/{key}/cancel` |
| `ttasks requeue KEY --queue QUEUE [--revision N]` (creates a new task in `QUEUE` and cancels this one; errors `task_closed`, `same_queue`, `queue_not_found`, `requeue_has_children`) | operator only | `POST /api/tasks/{key}/requeue` |
| `ttasks queue workflow set QUEUE REF [--revision N]` | operator only | `PUT /api/task-queues/{queue}/workflow` |
| `ttasks queue workflow get QUEUE` | operator only | `GET /api/task-queues/{queue}/workflow` |
| `ttasks queue workflow clear QUEUE --revision N` | operator only | `DELETE /api/task-queues/{queue}/workflow` |
| `ttasks queue secret set QUEUE KEY [--value V]` (stdin when absent, one trailing newline stripped; up to 64 KiB) | operator only | `PUT /api/task-queues/{queue}/secrets/{key}` |
| `ttasks queue secret ls QUEUE` (keys only, never values) | operator only | `GET /api/task-queues/{queue}/secrets` |
| `ttasks queue secret rm QUEUE KEY` | operator only | `DELETE /api/task-queues/{queue}/secrets/{key}` |
| `ttasks queue env set QUEUE KEY [VALUE \| --value V]` (stdin when absent; the value is not secret) | operator only | `PUT /api/task-queues/{queue}/env/{key}` |
| `ttasks queue env ls QUEUE` (keys and values) | operator only | `GET /api/task-queues/{queue}/env` |
| `ttasks queue env rm QUEUE KEY` | operator only | `DELETE /api/task-queues/{queue}/env/{key}` |
| `ttasks queue source ls QUEUE` | operator only | `GET /api/task-queues/{queue}/sources` |
| `ttasks queue source log QUEUE RUN [--max-bytes N]` (redacted tail, 64 KiB by default) | operator only | `GET /api/task-queues/{queue}/source-runs/{id}/log` |

On a workflow task `ttasks done`, `ttasks update --status`, and
`ttasks ready --claim` are refused with `workflow_managed`; the error lists the
status's outcomes and the `ttasks advance` form to use, with `--from` set to the current status. For an outcome that
declares checks, `ttasks advance` exits `0` when the transition applied and `1`
when a check rejected it, a check failed, the request was cancelled, or the wait
ran out; the output names the next command. `ttasks show` prints
`status` (the workflow status), `category`, and `waiting_on`. See
[Task workflows](/docs/task-workflows#script-protocol) for the scripts behind
checks and watch statuses.

The following administration roots are operator-only and are documented by
`ttasks --help-json`: `queue` (including pools and triggers; see
[Task workflows](/docs/task-workflows)), `events`, `principals`,
and `activity` (`GET /api/task-activity`: the work on tasks in
a `--since`/`--until` window, optionally for one `--agent`). They are
unavailable to agent mode.

Resource identifiers are positional (or named flags), for example:

```bash
ttasks queue get OPS
ttasks queue update OPS --name Operations --revision 2
ttasks queue pool list OPS
ttasks events OPS-1 --after 7 --limit 10
```

Use each administration command's `--help` for its required fields.
