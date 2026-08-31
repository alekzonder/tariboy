# Tariboy

**Tariboy** takes its name from the Indonesian
[*tukang tari*](https://www.google.com/search?tbm=vid&q=tukang+tari): the boy
who sets the rhythm for rowers in *Pacu Jalur* boat races. Likewise, Tariboy
sets the workflow rhythm for agents and agent teams.

**Tariboy — the control plane for autonomous coding agents.**

Tariboy gives one desktop workspace for work carried by agent sessions
running locally or on remote servers. Native **Tasks** are the durable center:
decompose work into an unbounded tree, delegate branches to agents, keep every
question and answer durably attached to the work, and follow status in real
time. Agents,
images, terminals, and teams are execution machinery around that work.

> **Warning:** This project is under heavy development. APIs, workflows, and
> configuration may change without notice.

The product model is deliberately progressive:

1. **Image** — an immutable set of plugins, optional Agent Skills, and an
   exactly ordered prompt template.
2. **Agent** — a named workspace created from an image.
3. **Interactive** — a live terminal session you control.
4. **Autopilot** — scheduled or message-triggered iterations the daemon controls.
5. **Task** — a queue-owned work item that can be decomposed and delegated,
   optionally under an immutable, queue-selected workflow.
6. **Team** — a first-class collection of independently configured agents,
   shared channels, and delegated work. Teams can be copied between servers as
   compose YAML; runnable image artifacts are transferred separately from their
   original build directories.

The first alpha focuses on the first four steps. Advanced channels, plugins,
groups, AI-proxy policy, budgets, and audit remain available, but they are not
required to get value from the desktop app.

## Ten-minute alpha path

The internal alpha supports macOS 12+ on Apple Silicon and remote Linux x86_64
hosts reachable through the user's existing SSH configuration.

1. Obtain `Tariboy_0.46.0_aarch64.dmg` and `SHA256SUMS` from the
   release owner, verify the checksum, drag `Tariboy.app` to Applications,
   and open it.
2. Select the local server in the Agents sidebar, then open **Settings → Hosts**
   in the server context and add an SSH config alias. Tariboy uses system
   `ssh` and `scp`; it does not copy private keys or bypass `known_hosts`.
3. In that server's **Images**, build from an original directory containing
   `Tariboyfile.yaml`, inspect the ordered template, or import a runnable image
   artifact. A required name and optional tag (`latest` by default) identify
   the immutable result.
4. In **Agents**, create an agent, choose harness/model/effort from the visible
   Runtime presets (or type custom model/effort values), and choose Interactive
   and Autopilot independently.
5. Open the selected server's **Tasks** to create a queue and a priority-ordered
   task tree. Each agent
   also has the same Tasks workspace scoped to its visible work. Priorities run
   from P0 Critical through P3 Low; drag a task onto another task to change its
   parent, or reorder it within the same priority bucket.
6. Select an agent for its Console, Autopilot, Activity, Tasks, Configuration,
   and Advanced tabs. The server row remains above those tabs, with Tasks,
   Images, and Settings scoped to that server. Open **Workspace** in the titlebar
   to drag several interactive agents into one cross-host terminal canvas.
   Large edge previews create
   nested left/right/top/bottom splits; pane headers move existing terminals
   and separators resize them. The sidebar icon beside the macOS window
   controls hides the agent list for full-width work.
7. Disable Autopilot to pause new autonomous work; use **Kill** only to stop the
   current iteration or interactive session immediately.

`bare:latest` is intentionally instructions-free, read-only, terminal-only. Use
it for an ordinary agent terminal session. It is not a base for Autopilot and it
cannot be rebuilt or edited.

`basic:latest` is the daemon-provided general-purpose default. Every packaged
`tariboyd` carries it and installs or refreshes it when that daemon version
is activated. Existing agents and pending assignments stay pinned to their
previous digest until explicitly changed, even when the managed ref advances.
It enables context, status, scripts, legacy current-task usage
attribution, and native Tasks; it also tells new agents where their managed
workdir is, even when they run in another CWD. Provider integrations remain
opt-in.

Packaged daemons also auto-enable the Telegram plugin. Create a Telegram
supergroup with topics, add a bot that can manage topics, then configure it with
`tariboy telegram configure` and bind it with `tariboy telegram chat setup` (or
use **Server Settings → Integrations → Telegram**). Tariboy never creates the
group or changes administrator rights. Empty allowed Telegram UIDs deny every
incoming command and message. See the [plugin guide](docs/docs/plugins/index.mdx#bundled-telegram-plugin)
for the topic and command model.

Schema-v2 images are deliberately transparent: they contain an explicit plugin
list, optional packaged Agent Skills, and an ordered `prompts` template. Prompt
files and skill directories can come from
`$STORE`, `$CURRENT_VERSION_STORE`, `$PLUGINS`, the original source directory,
or an absolute operator path. Runtime placeholders such as `identity`,
`workdir`, `messages`, and `context` stay in the image template and are
replaced with current values before every iteration; runtime identity includes
the active image ref and digest, while workdir names the absolute managed
`agents/<agent>/workdir` path. Harness, model, effort, environment,
policy, secrets, and evals belong to agent/compose runtime configuration. At
launch, Claude Code and OpenCode receive packaged skills through their native
plugin or skills configuration. Codex receives a bounded catalog of skill
names, descriptions, and absolute `SKILL.md` paths in the iteration prompt and
reads a matching skill on demand. None of these adapters changes the agent's
CWD or `HOME`.

Build and inspect one with:

```bash
tariboy image validate --path ./reviewer-image --name reviewer --tag v1
tariboy image build --path ./reviewer-image --name reviewer --tag latest --tag v1
tariboy image template reviewer:v1
```

An ordinary operator `tariboy image build` may rebuild a tag, including
`latest`. The prior digest is retained for active and pending agents, while
each requested tag reports its own ref and digest. Imports and retagged runnable
artifacts remain immutable.

The Images workspace can export/import runnable artifacts, assign a built image
to an existing agent for its next iteration, and open the original source CWD
in VS Code. Its read-only validation preview shows the exact resolved order,
plugin names, packaged skill metadata, runtime markers, source categories,
sizes, hashes, and warnings;
the image list shows agents using each ref now or pending its next-iteration
activation. Runnable exports never contain original source files; keep the
original directory and paths for rebuilding. Import defaults to the artifact's
ref; when that immutable ref conflicts, enter another name and tag in the
import preview to install a retagged runnable copy.

Runtime presets are curated Desktop suggestions, not live model discovery.
Custom model and effort overrides from successful creates are remembered
locally per harness and reused in later agent forms.

In **Advanced → Groups**, create a custom team with any number of independently
configured agents. Existing teams support rename, lead changes, arbitrary
member assignment/removal, compose YAML copy/import, and portable `tar.gz`
export/import. Removing a member or team never deletes its agents. **Images**
provides runnable-only image export/import; original source directories and
their paths remain the input for future rebuilds.

Desktop opens the workspace directly while daemon startup and saved-host
reconnection continue in the background. Reopening an already running app from
the Dock or menu bar raises that same workspace. The white pixel T is the
shared app, Dock, and menu-bar icon.

See the [desktop quickstart](docs/docs/quickstart.mdx), [remote host
guide](docs/docs/remote-hosts.mdx), [Tasks guide](docs/docs/tasks.mdx),
[task workflow guide](docs/docs/task-workflows.mdx), [image
guide](docs/docs/images.mdx), and [Autopilot guide](docs/docs/autopilot.mdx).

## Why a control plane

The desktop app is a client. A host-local `tariboyd` owns durable state,
agent processes, terminals, event delivery, the iteration loop, usage, budgets,
and audit. Local and remote daemons expose loopback listeners only. SSH hosts
are reached through supervised local-forwarding tunnels.

This separation gives operators:

- one UI across machines and harnesses;
- independent Interactive and Autopilot controls;
- durable message-triggered work instead of fragile prompt polling;
- native task queues, dependency trees, explicit answer tracking, and
  channel-backed notifications;
- versioned queue workflows with explicit agent pools, leased work packets,
  typed artifacts, blocking questions, and policy-bounded observations;
- per-iteration usage, cost, deadlines, and audit;
- explicit Pause, Kill, budget, and policy controls;
- no built-in product analytics; optional OTLP export is off unless an operator
  explicitly configures an endpoint.

Core state lives in SQLite under `~/.tariboy`; runtime socket, pid, and logs
live under `~/.tariboyd`. Remote releases are versioned below
`~/.local/lib/tariboy`. The Desktop menu's **Install/Update CLI** action
atomically points all four managed binaries (`tariboyd`, `tariboy`,
`tariboy-shim`, and `tariboy-plugin-telegram`) at the current app bundle, then
restarts the local daemon. Existing shim-owned iterations survive that restart,
retry through the brief AI-proxy outage, and are adopted by the new daemon.
Terminal attach, resize, input, and Kill continue to target the surviving shim
while adoption is pending; they do not wait for the interactive iteration to
exit or launch a duplicate session.
The one-time upgrade from a daemon older than `0.10.1` is blocked while it has
active work, because those older versions did not persist restart handoff state;
finish or stop that work and invoke the action again.
Remote **Update Tariboy** is likewise one uninterrupted operation: the click
authorizes activation and restart, and handoff-capable remote agents continue
through the brief daemon replacement without a second confirmation.

Agents run with the environment the account itself has. Desktop starts its
managed local daemon **inside** the account's login shell (read from the OS
account record), so the daemon — and every agent and harness under it — inherits
that shell's `PATH` and variables; terminal and SSH launches inherit the same
from their launcher. Without a usable account shell the daemon is started
directly with a bootstrap `PATH` that includes `~/.local/bin`, and it then
resolves the account `PATH` once from a bounded login-shell probe; a failed probe
keeps the inherited `PATH` and logs one safe warning naming the failure class,
never the path or shell output. An explicit agent `PATH` remains an intentional
override; non-bare images prepend their agent bin directory, while bare images do
not. Before an iteration starts, Tariboy verifies the harness executable
against that final environment.

## Developing

Start with the canonical [development
guide](docs/docs/development.mdx). It contains the repository map, task-to-docs
routing table, prerequisites, isolation rules, generated-artifact contracts,
and the complete verification matrix. Coding agents must also follow the root
[AGENTS.md](AGENTS.md) instructions and read the topical architecture documents
for every subsystem they modify.

Install every skill package under `ai/skills` into this repository for Codex:

```bash
make setup
```

The script installs project-local skills only; it does not modify global skill
installations.

There are two verification entry points, and they are the whole list:

```bash
make check       # fast, read-only, minutes
make full-check  # heavy, tens of minutes, includes check
```

`make check` runs `fmt-check`, `go vet`, the Go unit tests, the UI typecheck,
lint, unit tests and branding check, and the documentation
`doctor` plus `build`. It is safe to run often in a shared working tree: it
only reads. It never rewrites files (`make fmt` is deliberately not part of
it), never installs node modules, and never writes into `bin/`.

`make full-check` is everything that runs unattended on a developer machine:
`check` first, then `make build`, the four core E2E scripts, `full-smoke`, the
two Playwright browser suites, and one desktop step for the host: `desktop-e2e`
on Linux x86_64, the packaged app build plus `desktop-smoke` on macOS arm64, and
the version and lock gates alone on every other host. Neither target stops at
the first failure; both print a summary table of every step and fail at the end
if any step failed.

Three scripts stay outside `full-check` on purpose and are run by hand:
`scripts/product-alpha-e2e.sh` and `scripts/remote-provision-smoke.sh` need a
disposable SSH host (the first also consents to destroying its data), and
`scripts/check-alpha-artifacts.sh` is a release gate that takes a release
directory as its argument. The Rust host tests are also outside both targets;
run them directly when changing `desktop/src-tauri`:

```bash
. "$HOME/.cargo/env"
(cd desktop/src-tauri && cargo test && cargo clippy --all-targets -- -D warnings)
```

The individual targets these two compose are unchanged and still callable on
their own, so `make vet` or `make e2e` remains a legitimate way to run one
check while iterating.

Never test a daemon or agent against the live `~/.tariboy`,
`~/.tariboyd`, or `127.0.0.1:9990`; use the isolated targets and environment
described in the development guide.

## Build from source

Go 1.26 is required for the control plane, and running agent tools requires
`python3`. Desktop builds also need stable
Rust and Tauri CLI 2.x. macOS packaging additionally needs Xcode. Linux
packaging needs the Tauri Linux development libraries; Linux native E2E also
needs `tauri-driver`,
`WebKitWebDriver`, and either an existing display or Xvfb.

```bash
make build
./bin/tariboy daemon start
./bin/tariboy daemon status
```

Install or update all five server binaries as one versioned release:

```bash
make server-install
```

This writes `~/.local/lib/tariboy/<version>` and atomically updates the matching
links under `~/.local/bin`; it does not start or restart the daemon.

Build native Desktop packages for the current host:

```bash
. "$HOME/.cargo/env"
make desktop PLATFORM=darwin  # Darwin arm64: .app and .dmg
make desktop PLATFORM=linux   # Linux x86_64: .deb and .AppImage
```

`PLATFORM` defaults to the native supported platform. Packaging is native, not
cross-compiled: `darwin` requires Darwin arm64 and `linux` requires Linux
x86_64. The target rejects unknown values and host mismatches before building.
It resolves its own language-tool prerequisites: when needed, the first run
installs the UI packages pinned by `ui/package-lock.json`, installs the pinned
`tauri-cli`, and fetches the crates `Cargo.lock` pins, so it needs network once.
Later runs reuse the prepared UI and Cargo dependencies, validate the lock
offline, and never reach the network.

Build a non-packaged Linux debug executable and run the Playwright suite against
the real Tauri WebView through `tauri-driver`:

```bash
. "$HOME/.cargo/env"
make desktop-e2e-build
make desktop-e2e
```

The executable is
`desktop/src-tauri/target/debug/tariboy-desktop`. These developer targets do
not create packages; use `make desktop PLATFORM=linux` for a `.deb` and
`.AppImage`. See the development guide for prerequisites, isolated state
guarantees, and individual-spec execution.

Produce the reviewed internal alpha directory on an Apple Silicon Mac:

```bash
make desktop-alpha
```

The target builds both platform payloads and the SPA, creates an ad-hoc-signed
app and DMG, runs an isolated smoke test, validates signatures, versions,
architectures, checksums, and secret exclusions, then writes
`dist/releases/0.46.0/`. Publication is a separate manual action.

## CLI entry path

Desktop is the recommended introduction. The operator CLI remains fully
available for automation and advanced workflows:

```bash
tariboy version
tariboy --version
tariboy --help
tariboy --help-json
tariboy image --help
tariboy agent --help
tariboy channel --help
tariboy group --help
tariboy usage --help
```

Inside an image with `plugins: [{name: tasks}]`, an agent also gets the bare
`tasks` command and the matching task workflow prompt. Native Tasks are part of
`tariboyd`, not a supervised plugin.

The complete command and API-oriented material remains in
[Command reference](docs/docs/reference/commands.md), [Architecture
docs](docs/docs/architecture/index.mdx), [Binaries](docs/docs/binaries/index.mdx),
and [Channels](docs/docs/reference/channels.md).

## Safety and support

- Remote daemon HTTP listeners bind loopback only.
- SSH host trust is delegated to system OpenSSH and the user's config.
- HTTPS host tokens live in macOS Keychain, never `hosts.json`.
- Prompt replies are memory-only.
- Support bundles collect exactly one selected host and name the ZIP for that
  host. Agent output is included only through an explicit, unchecked sensitive
  data option for the newest ten iterations per agent. The collector never
  includes prompts, transcripts, audit, credentials, environments (including
  `PATH`), workdirs, user files, SSH aliases/configuration, or provisioning
  replies.
- Removing a host stops its local tunnel and deletes local host metadata; it
  does not stop the remote daemon or delete remote agent data.
- Quitting the app leaves daemons running by design.

Read [Security and controls](docs/docs/security-controls.mdx) and
[Support](docs/docs/support.mdx) before inviting alpha users.

## Repository map

- `cmd/`, `internal/` — Go daemon, CLI, agent runtime, APIs, plugins, and store.
- `ui/` — React desktop SPA.
- `desktop/src-tauri/` — native host, SSH, Keychain, tunnel, and packaging layer.
- `internal/builtinimages/source/` — canonical source for the embedded
  `basic:latest` image.
- `docs/docs/` — product, operator, and architecture documentation.
- `docs/releases/` — release notes and acceptance evidence.

The Go module, Rust package, product copy, and shipped artifacts all use
**Tariboy**. This is an incompatible namespace cut with no legacy aliases.
