# Tariboy

**The control plane for autonomous coding agents.**

Define reusable images, run coding agents interactively, and let Autopilot
carry the work forward. Bring agents together when the job needs a team —
locally or on remote hosts, from one desktop app.

**Image → Agent → Interactive → Autopilot → Team**

[Get started](docs/docs/quickstart.mdx) ·
[Documentation](https://alekzonder.github.io/tariboy/) ·
[Development](docs/docs/development.mdx)

> **Internal alpha:** Tariboy is under heavy development. APIs, workflows,
> and configuration may change without notice. The alpha onboarding path is
> macOS 12+ on Apple Silicon, with remote Linux x86_64 hosts over SSH.

## From one agent to a team

### 1. Image — define how an agent works

Package plugins, Agent Skills, and an ordered prompt template into a reusable
image. Build from a directory containing `Tariboyfile.yaml`, inspect exactly
what the agent will receive, and export runnable images to another host.

Start with the bundled `basic:latest` image, or build a role-specific image.
Harness, model, and effort are configured on the agent.

[Explore images →](docs/docs/images/index.mdx)

### 2. Agent — give the image a place to work

Create a named agent from an image. Choose its host, working directory,
harness, model, and runtime settings. Use Claude Code, Codex, or OpenCode
installed on that host, and keep each agent independently configured.

[Create your first agent →](docs/docs/quickstart.mdx#4-create-an-agent)

### 3. Interactive — work with the agent directly

Open Console to guide the agent in a live session. Provide instructions and
follow the work as it runs. Interactive and Autopilot are independent controls.

For an ordinary terminal session without image instructions, choose
`bare:latest`. It does not support Autopilot.

[Start an interactive session →](docs/docs/quickstart.mdx#5-use-console)

### 4. Autopilot — let work continue

Run bounded iterations on a timer or in response to durable messages. Inspect
triggers, outcomes, usage, and cost; set budgets and deadlines; pause new
iterations or kill current work when needed.

Autopilot is managed by the host daemon. Closing the desktop app does not stop
it or delete the agent's data.

[Configure Autopilot →](docs/docs/autopilot.mdx)

### 5. Team — coordinate agents around shared work

Bring independently configured agents together with a lead and shared
channels. Use Native Tasks to decompose work, delegate it, track progress, and
keep questions and answers attached to the task.

Copy team configuration as compose YAML or export a portable team archive.
Transfer runnable images separately; keep their original sources for rebuilds.

[Build a team →](docs/docs/images-and-groups/index.mdx) ·
[Organize tasks →](docs/docs/tasks.mdx)

## Get started

You need an Apple Silicon Mac running macOS 12+, `Tariboy_0.51.0_aarch64.dmg` and
its checksums from the release owner, and a supported harness installed where
your agent will run. Remote use also needs a Linux x86_64 host reachable through
your existing SSH configuration.

1. **Install Tariboy.** Verify the checksum, move the app to Applications, and
   open it. Follow the [quickstart](docs/docs/quickstart.mdx) for the alpha
   signing instructions.
2. **Choose a host and image.** Keep the local host or add an SSH host, then
   select `basic:latest` or build your own image.
3. **Create an agent.** Pick its harness and model, enable Interactive, and open
   Console. Add Autopilot when you are ready, then grow into a team.

[Follow the ten-minute quickstart →](docs/docs/quickstart.mdx)

### Build from source

The control plane requires Go 1.26; agent skill scripts require Python 3.
Desktop builds also need Node.js/npm, Rust, and native Tauri prerequisites.
See the [development guide](docs/docs/development.mdx#prerequisites) for setup.

```bash
make build
./bin/tariboy version
./bin/tariboy --help
```

For native Desktop packages, follow the
[Desktop build instructions](docs/docs/development.mdx#rust-desktop-host).
The [CLI reference](docs/docs/reference/commands.md) covers automation and
advanced operations, including the `ttasks` Native Tasks client.

## Documentation

| I want to… | Read |
| --- | --- |
| Run my first agent | [Quickstart](docs/docs/quickstart.mdx) |
| Package prompts, skills, and plugins | [Images](docs/docs/images/index.mdx) |
| Run work autonomously | [Autopilot](docs/docs/autopilot.mdx) |
| Coordinate a team | [Images & groups](docs/docs/images-and-groups/index.mdx) |
| Delegate and track work | [Tasks](docs/docs/tasks.mdx) and [Task workflows](docs/docs/task-workflows.mdx) |
| Connect a remote host | [Remote hosts](docs/docs/remote-hosts.mdx) |
| Extend agent capabilities | [Plugins](docs/docs/plugins/index.mdx) |
| Understand the control plane | [Architecture](docs/docs/architecture/index.mdx) and [Binaries](docs/docs/binaries/index.mdx) |
| Set limits or troubleshoot | [Security & controls](docs/docs/security-controls.mdx) and [Support](docs/docs/support.mdx) |

## Development

Start with the canonical [contributor guide](docs/docs/development.mdx) for
the repository map, prerequisites, verification matrix, and generated-artifact
rules. Coding agents must also follow [AGENTS.md](AGENTS.md).

After installing the dependencies described in the guide, run the fast checks:

```bash
make check
```

Never test against the live `~/.tariboy`, `~/.tariboyd`, or
`127.0.0.1:9990`. Use the guide's isolated test environments.

## Safety and control

Each host runs its own `tariboyd`, which owns durable state and agent execution.
HTTP listeners bind loopback; remote access uses system OpenSSH forwarding and
the user's host verification. Pause prevents new autonomous iterations;
Kill stops current work. Quitting the app leaves daemons running.

Upgrading a daemon older than `0.10.1` requires it to be idle; see the
[quickstart](docs/docs/quickstart.mdx#1-install) for the upgrade path.

Read [Security & controls](docs/docs/security-controls.mdx) before onboarding
others, and use the [Support guide](docs/docs/support.mdx) for diagnostics.

## The name

Tariboy takes its name from the Indonesian *tukang tari*: the boy who sets the
rhythm for rowers in *Pacu Jalur* boat races. Tariboy sets the workflow rhythm
for agents and agent teams.
