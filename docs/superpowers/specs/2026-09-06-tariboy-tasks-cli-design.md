# Tariboy Tasks CLI Design

## Status

Approved on Native Task `IMPROVE-6` on 2026-09-06. This spec records the
approved design without changing its scope.

## Goal

Restore a separately compiled `tariboy-tasks` client, install `ttasks` as its
managed command name, and give it two fail-closed modes selected only by the
presence of `TARIBOY_TOOLS_SOCKET`:

- an ordinary operator gets unrestricted customer access to Native Tasks on
  the host daemon;
- an agent gets only the identity and permissions of its per-agent capability
  socket.

The command must be available through the normal server and Desktop install
paths and the packaged Tasks skill must invoke it.

## Non-goals

- No new task authorization rules, persistence, workflow semantics, or daemon
  identity source.
- No TCP transport, remote credentials, or listener changes.
- No compatibility fallback from a failed agent socket to the operator socket.
- No new runtime dependency and no version bump.
- No UI behavior change or generated UI artifact change.

## Command and mode contract

`tariboy-tasks` and the `ttasks` symlink execute the same binary. `--version`
prints the canonical Tariboy version without contacting a daemon. `--json`
keeps machine-readable output, and unknown commands or flags fail before a
request is sent.

The existing agent-oriented verbs remain the shared day-to-day surface:
`mine`, `ready`, `show`, `create`, `update`, `assign`, `comment`, `ask`, `move`,
`block`, `relate`, `done`, `work`, `artifacts`, `questions`, `answer`, and
`observe`. Operator-only subcommands expose the existing queue, workflow,
execution-history, principals, events, and notification administration routes.
The binary adds no new task-domain operation.

Mode selection is intentionally one-way:

1. If `TARIBOY_TOOLS_SOCKET` is non-empty, use it and call the existing
   `/tools/tasks/{action}` capability endpoint. The daemon derives agent and
   current iteration from that socket. A missing or unreachable socket is an
   error; the client never retries through operator access.
2. Otherwise resolve the normal daemon runtime socket using the same
   `TARIBOY_RUNTIME_DIR` and default-path logic as `tariboy`, then use the
   existing operator task routes. Those routes construct the daemon customer
   actor, which already bypasses queue ACL filtering while retaining domain
   validation and workflow rules.

Both modes use one parser, request model, output formatter, version-mismatch
warning, and exit-code policy so their common syntax cannot drift.

## Components

### Go client

Add `cmd/tariboy-tasks` as a thin entry point over a focused internal package.
The package owns argument parsing, common output, mode selection, and transport
routing. It reuses `internal/client` for Unix HTTP and `internal/paths` for the
operator socket. The agent endpoint remains the current capability adapter;
operator commands map to the already registered `/api/tasks`,
`/api/task-queues`, `/api/workflows`, workflow execution, notification,
principal, and event routes.

No general CLI framework or dependency is introduced. Parsing behavior is
ported from the current standard-library Python Tasks client, then that Python
implementation is removed once parity tests pass.

### Agent skill

The Tasks skill launcher executes `ttasks` and its documentation names
`ttasks`. The optional legacy `tasks` agent-local compatibility shim may remain
and forward to the same launcher, preserving existing images and prompts while
ensuring there is only one task client implementation.

An image without the Tasks capability still does not receive the legacy
`tasks` shim. The globally installed `ttasks` executable does not grant an
agent operator access because every running agent has
`TARIBOY_TOOLS_SOCKET`; mode selection is based on that variable before any
socket resolution.

### Installation and packaging

Build `tariboy-tasks` with the existing Go binaries. Server releases and
Desktop bundles contain the real file as `tariboy-tasks`. Managed installers
atomically maintain both:

- `tariboy-tasks -> <version>/tariboy-tasks`
- `ttasks -> <version>/tariboy-tasks`

Preflight treats either command path as part of the transaction: unrelated
regular files, directories, or foreign symlinks abort before switching.
Rollback restores both prior links. Checksums cover the real binary once;
`ttasks` is a link, not duplicated release bytes.

The local Desktop installer, remote transactional installer, Makefile
`install`/`server-install` paths, upload manifests, architecture/version gates,
and uninstall/support instructions all use the same managed set.

## Security and failure behavior

- `TARIBOY_TOOLS_SOCKET` is the sole agent-mode signal. Other Tariboy
  environment variables do not grant or remove operator access.
- Agent mode never probes or falls back to the normal daemon control socket.
- Operator mode relies on the existing host-local customer actor and transport
  boundaries; it does not accept a caller-supplied actor.
- Agent workflow restrictions, leases, allowed tools, and task ACLs stay
  daemon-authoritative.
- Errors do not print socket paths, environment contents, credentials, or
  response bodies outside the existing bounded error envelope.
- Installer ownership checks and rollback remain fail-closed for both command
  names.

## Documentation

Update the repository entry point, Native Tasks guide, binary and agent-tool
references, command reference, development guide, remote-host guide, security
controls, and support/uninstall examples to describe the six real server
binaries plus the `ttasks` alias and the two-mode trust boundary.

## Verification

Follow TDD with focused checks for:

- shared parser compatibility and strict unknown-flag rejection;
- `TARIBOY_TOOLS_SOCKET` selecting agent mode and refusing fallback;
- operator mode seeing and mutating tasks across queues through an isolated
  daemon;
- the Tasks skill delegating to `ttasks` without Python;
- server and Desktop installers creating and rolling back both managed names;
- Desktop bundle membership, upload, architecture, and version validation.

Run focused Go, Store skill, shell contract, and Rust installer tests while
iterating. Final branch verification is `make check`, plus Rust `cargo test`
and Clippy if the Desktop crate changes. Do not run `make full-check`, per the
task instruction. Finish with `git diff --check` and complete-diff review.
