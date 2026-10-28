---
title: Remote hosts
description: Connect, provision, update, and remove Linux and macOS hosts through system OpenSSH.
sidebar:
  label: Remote hosts
  icon: server
---

Tariboy manages a remote daemon through the SSH configuration already used
by the operator. The desktop does not implement a second SSH stack.

## Compatibility

| Component | Alpha support |
| --- | --- |
| Desktop | macOS 12+, Apple Silicon |
| Automatic remote install | Linux x86_64 and aarch64, macOS arm64 (Apple Silicon) |
| SSH | system `ssh` and `scp` |
| Remote install root | `~/.local/lib/tariboy` |
| Remote CLI links | `~/.local/bin` |
| Daemon HTTP | remote loopback only |
| Tunnel | local loopback forwarding over SSH |
| Android app | signed arm64 APK in each release; forwarded loopback port or HTTPS server, no SSH hosts |

Automatic installation requires a writable `~/.local` and `python3`; Linux
hosts also need `flock`. The desktop app carries one server build per platform
and uploads the one matching the host's `uname -s` and `uname -m`: Linux
`x86_64` gets `linux-x86_64`, Linux `aarch64` (or `arm64`) gets
`linux-aarch64`, and macOS (`Darwin`) `arm64` gets `darwin-arm64`. Install and
update re-read the platform and architecture in preflight every time. Any other
combination, such as 32-bit `armv7l` or an Intel Mac (`Darwin/x86_64`,
including a shell running under Rosetta), is reported as unsupported and
nothing changes on the host.

macOS ships neither `flock` nor `sha256sum`, so on a Mac the installer takes
its lock through `python3` and verifies checksums with `shasum -a 256`. On
macOS, `/usr/bin/python3` works only once the Command Line Tools are installed
(`xcode-select --install`). Homebrew installs `tmux` and harnesses under
`/opt/homebrew/bin`; that directory must be on the login shell's `PATH` or the
daemon reports them as missing tools. A daemon started over SSH does not start
again after the Mac reboots; connect from Desktop to start it.
The Python runtime executes the standard-library agent tool scripts. Preflight
also reports `tmux`, `git`, `npx`, Claude, Codex, OpenCode, and the Cursor CLI
(its `agent` executable) so you can see which workflows are available; a missing harness is
listed as an optional tool and does not block installation. At least one chosen harness must be installed on the remote host.

## Account PATH and harness launch

A Desktop-managed local daemon is started **inside** the account's login shell,
so it inherits the whole environment a new terminal gets — `PATH` and every other
variable your shell startup files export. Desktop reads the login shell from the
operating-system account record, so this works even when Finder supplied no
useful `SHELL`. A daemon launched through SSH or an ordinary terminal already
inherits that environment from its launcher.

When no usable account shell exists, the daemon is started directly with a
bootstrap `PATH`: whatever the launcher had, plus `~/.local/bin` and the standard
system and package-manager locations. Such a daemon then resolves the account
`PATH` itself, once, from a bounded interactive-login-shell probe. If that probe
fails the daemon stays available with the inherited `PATH` and logs one stable
warning naming only the class of failure — never the path or shell startup
output.
Before each iteration, Tariboy checks the selected harness executable using
the final agent environment. For non-bare images it resolves `python3` from the
same environment and passes that absolute interpreter to skill launchers. An
agent's explicit `PATH` overrides the daemon baseline and therefore must retain
both prerequisites; non-bare images then prepend their agent bin directory,
while bare images do not.

After resolving `PATH`, the daemon appends `~/.local/bin` when it is missing,
so agents find the managed `tariboy` and `ttasks` links.

## Daemon host tools

Every `GET /api/daemon/status` checks the daemon `PATH`, the one iterations
start from, for the commands the daemon and its agents need:

| Tool | Needed for | Install |
| --- | --- | --- |
| `tmux` | agent terminal sessions | system package manager |
| `bash` | running iterations | system package manager |
| `python3` | agent tool scripts | system package manager |
| `git` | adding and refreshing Stores | system package manager |
| `npx` | building images that carry `skills-lock.json` | Node.js (npm ships `npx`) |
| `tariboy`, `ttasks` | Tariboy commands inside agents | Tariboy CLI links in `~/.local/bin` |
| `claude`, `codex`, `opencode` or `agent` | at least one agent harness | the harness vendor's installer |

The response carries `state` (`ready` or `error`), `missing_tools` (name,
purpose and install hint for each missing tool) and `message`, for example
`missing required tools: tmux, npx; install them on the daemon host`. The
check only looks for an executable on `PATH`; it does not install anything,
because install commands differ between systems. The `error` state blocks
nothing: the API, tunnel and running agents keep working, and the state
returns to `ready` on the next status read after you install the tool, without
a daemon restart.

Desktop shows a connected SSH host whose daemon reports missing tools as
**error** with the message, and the local daemon banner names the missing tools
in the desktop app and in a browser. A host's state is read when its tunnel
connects, so reconnect it after installing the tools.

## Add a host

1. Confirm the alias works in Terminal: `ssh build-box`.
2. Add the same alias under **Settings → Hosts**.
3. Choose one of the two setup modes:
   - **Install Tariboy** (default) runs preflight, uploads and installs the
     matching release, starts the daemon, and connects the tunnel.
   - **Connect to existing Tariboy** saves the host and connects without
     uploading, installing, activating, starting, or restarting anything.

Both modes use system OpenSSH and its existing host-key, ssh-agent, ProxyJump,
and authentication rules. **Connect to existing Tariboy** creates an
independent dynamic local forward from `127.0.0.1` to the remote daemon at
`127.0.0.1:9990`, then checks daemon health through that tunnel. A successful
probe marks the host Ready, and the ordinary saved-host reconnect restores the
tunnel after Desktop restarts.

If no daemon answers at the remote loopback port, Desktop keeps the saved host
and does not install or start anything automatically. The dialog offers to
retry the connection or run **Install Tariboy** explicitly. A daemon version
different from Desktop is allowed to connect; the existing **Update** indicator
reports the mismatch without triggering an update.

System OpenSSH honors `~/.ssh/config`, ProxyJump, ssh-agent, hardware-backed
keys, `known_hosts`, and organization authentication. Tariboy never adds
`StrictHostKeyChecking=no`, copies private keys, or stores prompt replies.

## Install and update behavior

**Update Tariboy** on a Linux host whose CLI supports `tariboy update
--cleanup` uploads nothing: Desktop runs
`tariboy update <Desktop version> --cleanup` on the server over SSH. The server
downloads and verifies that release from GitHub, installs it with the same
fixed installer, restarts the daemon, and deletes old releases, keeping the
five newest and the active one. Desktop then checks the reported version, as
below. When the command fails after the managed links already moved to the new
release, Desktop reactivates and restarts the previous release; a failure
before the switch leaves the host unchanged. The server needs outbound HTTPS to
GitHub, and the Desktop version must be a published release with a server
archive. Desktop does not fall back to uploading when the download fails.

The upload path below remains for the first install, for macOS hosts, and for
a host whose CLI predates `--cleanup`; such a host is updated by upload once,
and its next update runs on the server.

Desktop uploads five bundled binaries including the Tasks client and Telegram plugin, a version marker, checksums, and a
fixed installer script to a staging directory. The remote installer:

- validates checksums;
- serializes installs with `flock`, or with a `python3` file lock where `flock`
  is absent (macOS);
- publishes a versioned release directory;
- atomically switches managed CLI symlinks;
- verifies the daemon reports the requested version;
- rolls back the previous release on activation failure.

The upload runs over `scp` with SSH compression enabled, and its timeout is
derived from the payload rather than fixed: a floor of two minutes plus one
second per 128 KiB of bundle, capped at thirty minutes. A release bundle is
around 100 MB, so a fixed short timeout used to abort the transfer on ordinary
uplinks, leaving the update reported as `SSH command timed out` at the
**Upload release** step. The budget now assumes a slow, incompressible worst
case; a healthy link finishes far sooner and the timeout only bounds a stalled
transfer.

Reactivating a release that predates `tariboy-tasks` removes both newer Tasks
links transactionally. Selecting the newer release again restores them; every
selected release must contain its required payloads before links switch.

An update refuses to replace unrelated regular files in `~/.local/bin`.
Choosing **Update Tariboy** authorizes the version switch and daemon restart;
connect, upload, activation, restart, and reconnect remain one operation with
one operation ID. Daemon `0.10.1` and newer preserves running shims across that
restart, so active agents do not require a second confirmation. A legacy daemon
older than `0.10.1` instead fails closed before activation when active work
exists or cannot be checked.

## Update a host from its own shell

A host can also fetch and install a release itself, without the desktop app
uploading one:

```bash
ssh build-box 'tariboy update latest'
ssh build-box 'tariboy update 0.64.0'
```

`latest` resolves the newest published GitHub release; an explicit version
installs exactly that release. The command downloads the archive for its own
architecture, `tariboy_<version>_linux-x86_64.tar.gz` or
`tariboy_<version>_linux-aarch64.tar.gz`, and the release `SHA256SUMS`, verifies
the archive checksum before unpacking, stages it under
`~/.local/lib/tariboy/.stage-<token>`, and then runs the same fixed installer
the desktop app uploads — so checksum validation, `flock` serialization,
atomic symlink switching, and rollback are identical. A running daemon is
restarted onto the new release and must report the installed version; a stopped
daemon is left stopped.

The command updates only an installer-managed `linux-x86_64` or
`linux-aarch64` installation, where `~/.local/bin/tariboy` is a symlink into
`~/.local/lib/tariboy/<version>/`. It refuses anything else — a hand-placed binary or a macOS installation
belonging to the app bundle — and changes nothing. A Desktop-managed macOS
server is updated with the Desktop update button instead. `--force` reinstalls a
version that is already active. `TARIBOY_UPDATE_BASE_URL` overrides the GitHub
releases root for a mirror.

`--cleanup` deletes older releases from `~/.local/lib/tariboy` after the update
succeeds, or right away when the version is already installed. It keeps the
newest five releases by version number, or `--keep N`, and always the active
release among them. It removes only release directories whose `VERSION` file
matches their name, leaves staging directories and other files alone, and holds
the installer lock while it works. A failed daemon restart skips the cleanup so
the previous release stays available for rollback, and a cleanup error is only
a warning: the update itself has already succeeded.

```bash
ssh build-box 'tariboy update latest --cleanup'
ssh build-box 'tariboy update 0.64.0 --cleanup --keep 3'
```

Releases published before this archive existed cannot be fetched this way:
move to the first release that carries it through the desktop app once, then
`tariboy update` works from there.

The host dialog presents provisioning and updates as one vertical sequence:

1. Connect to host.
2. Check server.
3. Upload release; **Get release** for an update, which also covers the
   server-side download.
4. Install release.
5. Start or restart Tariboy.
6. Connect or reconnect to Tariboy.

Only one step is current, and a failure remains attached to the step that
failed even when rollback successfully reconnects the previous daemon. Process
stdout and stderr are diagnostic streams; stderr by itself does not mean that
the command failed. The main flow shows only the actionable result. Checksums,
raw JSON, and command output are available under the closed **Technical
details** disclosure and cannot expand the dialog horizontally.

The dialog offers one primary action for its current state: add, save and
reconnect, update, or retry. If update
reports that a path such as `~/.local/bin/tariboyd` is an existing
non-symlink, move that file out of the way yourself and retry. Desktop does not
delete or overwrite it automatically.

Host and agent lists show their compact **Update** action only when tunnel
health has reported a non-empty remote daemon version that differs from the
current Desktop bundle version. Matching or unknown versions do not show the
indicator. The manual **Edit host → Update Tariboy** action remains
available when an operator deliberately wants to reinstall the current
release.

Inside an SSH agent workspace, **Open in VS Code** opens the displayed effective
working directory through VS Code Remote SSH using the same saved SSH config
alias in a new editor window. Local agents open as ordinary local folders in a
new editor window. HTTPS hosts do not show the
action because they have no SSH identity.

Missing harnesses, `tmux`, `git` and `npx` are reported separately as agent
workflow prerequisites. They do not make an otherwise successful Tariboy update
appear failed. Platform, architecture, writable install root, `python3`, and,
on Linux, `flock` remain installation requirements.

## Connection lifecycle

The remote daemon binds `127.0.0.1`. The desktop starts an SSH local-forwarding
tunnel, probes health through it, retries bounded transient failures, and treats
authentication or host-key mismatch as terminal until the operator acts.

Quitting the app stops local tunnels but leaves remote daemons running. The next
launch reconnects saved hosts.

## Removing and uninstalling

Removing a host from Settings stops its tunnel and deletes local host metadata
and any Keychain token. It intentionally does not stop the remote daemon or
delete remote data.

To uninstall remotely after confirming no work is active:

```bash
ssh build-box 'set -eu
managed_cli=
for managed in tariboy:tariboy tariboyd:tariboyd tariboy-tasks:tariboy-tasks ttasks:tariboy-tasks tariboy-shim:tariboy-shim tariboy-plugin-telegram:tariboy-plugin-telegram; do
  name=${managed%%:*}
  source=${managed#*:}
  link=$HOME/.local/bin/$name
  if test -L "$link"; then
    target=$(readlink "$link")
    case "$target" in
      "$HOME"/.local/lib/tariboy/*/"$source")
        test "$name" != tariboy || managed_cli=$target
        ;;
      *) echo "refusing to remove unrelated link: $link -> $target" >&2; exit 1 ;;
    esac
  elif test -e "$link"; then
    echo "refusing to remove non-link: $link" >&2
    exit 1
  fi
done
test -n "$managed_cli"
"$managed_cli" daemon stop || true
for name in tariboy tariboyd tariboy-tasks ttasks tariboy-shim tariboy-plugin-telegram; do
  link=$HOME/.local/bin/$name
  test ! -L "$link" || rm -- "$link"
done
rm -rf -- "$HOME/.local/lib/tariboy"'
```

Deleting `~/.tariboy` removes remote agents, images, audit, and database
state. Back it up first and perform that separate destructive step only when
data removal is intended.

## Android app

The Android APK runs the same UI without a local daemon, SSH hosts, tunnels, or
host provisioning. Each release attaches `Tariboy_X.Y.Z_android-arm64.apk`; the
release notes give its signing certificate fingerprint (see
[Android release APK](/android-release)). It connects to two kinds of servers:

- **A forwarded loopback port.** You forward a daemon's listener to the phone
  yourself, for example `ssh -L 9990:127.0.0.1:9990 host` in Termux or
  `adb reverse tcp:9990 tcp:9990` from a computer. Then **Add host** with base
  URL `http://127.0.0.1:9990` and an empty token. Forward a token listener
  instead if you prefer, and enter its token.
- **An HTTPS server** with a bearer token, as in the browser.

Plain `http://` is accepted only for `127.0.0.1`, `localhost`, or `[::1]` with
a port; Android blocks cleartext to every other host. The app does not start or
restart the forward: while it is down the server shows as unreachable.

A forward can accept a connection and never answer, for example when the SSH
session behind it lost the server. The app stops each read after 30 seconds
and keeps the last data on screen. When you come back to the app, for example
after you restart the forward in Termux, it opens its live connections again
and rereads the screen. You do not have to restart the app. If the forward
comes back while the app stays open, reads recover in 30 seconds, but live
task updates can wait until you leave the app and come back.

The daemon must be this release or newer, because only it allows the Android
app's origin `http://tauri.localhost` on its loopback listener. An older daemon
answers, but the WebView blocks its responses; forward its token listener
instead. Any app on the phone can reach a forwarded tokenless port while the
forward is up. A token lives in WebView session storage and must be entered
again after Android stops the app; the server list is kept.

Web links open in the phone's default browser, never inside the app: links in
chat messages and task descriptions on a tap, and `http://` or `https://` text
in the Terminal tab on a tap (there is no Command key). Other link schemes open
nothing. If no browser can open the address, nothing opens and the app shows no
message.

On a phone or a tablet narrower than 1024 CSS pixels the agent list is a
drawer: the menu button at the left of the titlebar opens it, and picking an
agent, a server or a team closes it. See
[Web UI](/docs/architecture/web-ui#compact-screens).
