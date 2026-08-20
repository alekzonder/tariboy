---
title: The shim
description: tariboy-shim runs a single harness iteration under a watchdog, so a runaway iteration can be killed without touching the daemon.
sidebar:
  label: The shim
  icon: shield
---

The shim (`cmd/tariboy-shim`, `internal/shim`) runs a **single harness
iteration under a watchdog**. The daemon hands it:

- the iteration directory,
- the agent and iteration ids,
- a hard timeout,
- and a tmux session for interactive harnesses.

Everything after `--` is the harness command it supervises.

For tmux-backed interactive iterations, the shim uses a fixed `/bin/sh`
wrapper that preserves the harness argv, captures its real exit status, writes
the numeric value to an owner-private temporary file, and atomically renames it
within the iteration's private logs directory. After the tmux session ends,
the shim validates and removes that transient status file before writing
`result.json`. A non-zero harness status is preserved; a missing, malformed, or
unreadable status becomes unknown failure (`-1`), never success. Timeout and
explicit termination reasons still take precedence. The status file is not in
the support-bundle allowlist.

The tmux session wraps the harness's native interactive command, not its batch
renderer. In particular, interactive Codex agents run the top-level Codex TUI
with stdin attached, while non-interactive Codex iterations continue to use
`codex exec --json` outside tmux. The Desktop only attaches another tmux client;
disconnecting it does not replace or terminate the harness TUI.

When Tariboy creates an interactive tmux session, it disables tmux mouse
capture for that exact managed session. This leaves browser and OS text
selection available in Agent Console and Workspace without changing global tmux
options, user tmux configuration, or unrelated sessions.

Separating the watchdog from the daemon means a runaway or hung iteration can be
killed (`tariboy agent kill`) without touching the daemon or other agents. The
shim isn't a subcommand CLI — it has exactly one job.

It also makes daemon upgrades non-destructive. `tariboyd` cancellation does
not signal the shim or its harness. The old loop observer detaches while the
iteration row stays `running`; the new daemon probes the same per-agent shim
socket, restores the absolute hard deadline, and waits for the existing
`result.json`. Harness calls may see a brief local AI-proxy outage during the
restart, but supported harnesses retry the stable proxy URL and carried token.
While that wait is active, the replacement daemon keeps the adopted iteration
as a current shim target: terminal clients and operator control RPCs reach the
same socket, and Start or Exec cannot create a second engine or tmux session.
An `i-am-done` call also survives the handoff: after the same two-second grace
used by the original runner, adoption sends at most one cooperative Kill if no
`result.json` has appeared, then finalizes the original row from that result.
Each control request connects while the handoff lock is held, so later reuse of
the stable per-agent socket pathname cannot redirect it to a replacement
iteration; that connection attempt has a finite timeout so a wedged Unix accept
queue cannot hold daemon lifecycle operations indefinitely. When adoption
finishes, the daemon reloads the current Enabled value before starting the
normal loop engine. Restart also carries one explicit interactive-run intent
across the handoff. A short launch gate shared with Stop is acquired only after
tmux collision checks, covers the durable launch or manual-collision outcome,
and is released before runner preparation. If Stop races the final detached
process creation after winning stale-shim recovery, the runner briefly retries
the shim socket and requests cooperative Kill so the shim terminates its owned
harness process group or tmux session. If no shim becomes reachable, it kills
the interactive tmux session explicitly. Its final managed fallback sends
SIGTERM to the spawned shim group; a non-interactive shim handles that signal
by force-killing its separately owned harness group, even when the harness
ignores SIGTERM. The launcher SIGKILLs the outer group after a short bounded
cleanup window. Daemon handoff continues to preserve a shim whose durable
iteration remains `running`.

For an interactive iteration, each Desktop terminal connection asks the shim
to start a separate `tmux attach-session` client inside a PTY. The shim checks
`tmux has-session` first and keeps the attach client's own stderr outside the
PTY, so an iteration that already ended — including one that disappears during
attach startup — cannot leak tmux's `no sessions` diagnostic into xterm. The
browser side is always xterm.js, so only this attach client receives the fixed
outer terminal type `TERM=xterm-256color`. The shim invokes that client as
`tmux -u attach-session`: tmux therefore preserves Unicode output even when a
GUI launcher supplied no UTF-8 locale. Tariboy does not synthesize or
replace `LANG`, `LC_ALL`, or `LC_CTYPE`; the daemon, harness, agent, and tmux
session environments are unchanged. A failed attach writes only a stable
failure category and session identifier to `shim.log`; tmux stderr and
environment values remain excluded. Closing a browser connection tears down
only that attach client; it never kills the tmux session or harness.

The daemon distinguishes terminal lifecycle from transport lifecycle. A PTY
EOF is closed as WebSocket `1000/eof`; Desktop stops reconnecting immediately
and shows the stopped-session panel. A `4404` no-session response can also mean
that Start has queued an interactive iteration but its shim socket is not ready
yet, so Desktop retries it for at most five seconds with capped exponential
delay before showing the same panel. An abnormal WebSocket close remains
transient and reconnects indefinitely with capped delay, which lets a live shim
terminal recover across a brief daemon or network interruption.
