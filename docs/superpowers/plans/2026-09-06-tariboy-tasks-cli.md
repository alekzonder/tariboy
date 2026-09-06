# Tariboy Tasks CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore a separately compiled `tariboy-tasks` executable, install its `ttasks` alias everywhere Tariboy is installed, and select unrestricted operator or identity-bound agent access solely from `TARIBOY_TOOLS_SOCKET`.

**Architecture:** A small `internal/taskcli` package owns the existing Tasks syntax, strict parsing, output, and action-to-route mapping. It uses the existing Unix HTTP client for both sockets: a non-empty `TARIBOY_TOOLS_SOCKET` always selects `/tools/tasks/{action}`, while its absence resolves the normal daemon socket and maps the same verbs to existing operator routes; operator-only administration commands reuse exported command descriptors from `internal/commands` rather than duplicating API schemas. Packaging ships one `tariboy-tasks` file and creates both `tariboy-tasks` and `ttasks` managed links to it.

**Tech Stack:** Go 1.26 standard library and existing internal packages, POSIX shell, Rust/Tauri installer code, Python `unittest` only for Store skill contracts, MDX documentation.

**Spec:** `docs/superpowers/specs/2026-09-06-tariboy-tasks-cli-design.md`

## Global Constraints

- Do not change task authorization, persistence, workflow semantics, daemon identity, listeners, or credentials.
- A non-empty `TARIBOY_TOOLS_SOCKET` is the sole agent-mode signal and must never fall back to the operator socket.
- Keep one parser, formatter, version warning, and exit-code policy for shared verbs.
- Add no dependency and make no version bump.
- Keep all test daemons isolated from live `~/.tariboy`, `~/.tariboyd`, and `127.0.0.1:9990`.
- Build one real `tariboy-tasks` file; `ttasks` is a managed link to it, never duplicate bytes.
- Final verification is `make check`, focused Rust checks, and `git diff --check`; never run `make full-check`.

---

### Task 1: Shared Go Tasks Client and Fail-Closed Mode Selection

**Files:**
- Create: `cmd/tariboy-tasks/main.go`
- Create: `internal/taskcli/taskcli.go`
- Create: `internal/taskcli/parse.go`
- Create: `internal/taskcli/routes.go`
- Create: `internal/taskcli/taskcli_test.go`
- Modify: `internal/commands/tasks.go`

**Interfaces:**
- Consumes: `client.New(socket).Call(method, route, body)`, `paths.Resolve(getenv).Socket()`, canonical `version.Version`, and the existing task route descriptors in `internal/commands/tasks.go`.
- Produces: `taskcli.Run(ctx, argv, getenv, stdout, stderr) int`; parsed `request{action string, payload map[string]any}`; an exported read-only operator task command slice from `internal/commands`; and the `tariboy-tasks` process entry point.

- [ ] **Step 1: Write failing parser and mode tests**

  Add table tests in `internal/taskcli/taskcli_test.go` that exercise the current Python surface (`mine`, `ready`, `show`, `create`, `update`, `assign`, `comment`, both `ask` forms, `move`, `block`, `relate`, `done`, `work`, `artifacts`, `questions`, `answer`, and `observe`). Assert `--json` is global, `--version` returns `version.Version` without a caller, missing required values return `2`, and an unknown command or flag returns `2` without recording a request.

  Use a recording `Caller`:

  ```go
  type call struct { method, route string; body any }
  type recorder struct { calls []call; result json.RawMessage; err error }
  func (r *recorder) Call(method, route string, body any) (json.RawMessage, error) {
      r.calls = append(r.calls, call{method, route, body})
      return r.result, r.err
  }
  ```

  Include the security cases:

  ```go
  func TestAgentModeNeverFallsBack(t *testing.T) {
      env := mapEnv("TARIBOY_TOOLS_SOCKET", "/missing/agent.sock", "TARIBOY_RUNTIME_DIR", t.TempDir())
      code := Run(context.Background(), []string{"mine"}, env, io.Discard, io.Discard)
      if code != 2 { t.Fatalf("code = %d, want 2", code) }
      // No operator socket server is started: success would prove fallback.
  }

  func TestOnlyToolsSocketSelectsAgentMode(t *testing.T) {
      // TARIBOY_BASE_DIR and TARIBOY_RUNTIME_DIR alone still select operator mode.
  }
  ```

- [ ] **Step 2: Run the focused tests and observe RED**

  Run: `go test ./internal/taskcli -count=1`

  Expected: FAIL because `internal/taskcli` does not exist.

- [ ] **Step 3: Port the current parser without adding a framework**

  Move the behavior of `store/skills/tasks/scripts/tasks.py` into `internal/taskcli/parse.go` using only `strings`, `strconv`, and `encoding/json`. Preserve its action names and payload keys exactly, including `workflow_ask`, `workflow_answer`, `artifact_*`, and `observe_*`; preserve empty-value clearing for `manual_block_reason` and `pull_request`; preserve the `--flag=--value` form; reject every unread flag and extra positional argument before transport.

  Keep the parser data-driven with one command table rather than one exported type per verb:

  ```go
  type request struct {
      action  string
      payload map[string]any
  }

  func parse(argv []string) (request, error)
  ```

  Do not add Cobra, urfave/cli, reflection-based binding, or a second public command registry.

- [ ] **Step 4: Implement one runner with one-way mode selection**

  In `internal/taskcli/taskcli.go`, parse global flags before resolving any socket. Inject a private `newCaller func(string) Caller` seam for tests, but have production use `client.New`.

  ```go
  toolsSocket := strings.TrimSpace(getenv("TARIBOY_TOOLS_SOCKET"))
  if toolsSocket != "" {
      return runAgent(parsed, client.New(toolsSocket), jsonOut, stdout, stderr)
  }
  resolved, err := paths.Resolve(getenv)
  if err != nil { /* bounded usage error, exit 2 */ }
  return runOperator(ctx, parsed, client.New(resolved.Socket()), jsonOut, stdout, stderr)
  ```

  Agent mode always sends `POST /tools/tasks/{action}` and reports an unreachable tools socket as an agent-socket error. It never calls `paths.Resolve` and never retries. Both modes print JSON through `encoding/json`; human output preserves the current sorted `key: value` representation and the filed-task warning.

- [ ] **Step 5: Map shared verbs to existing operator routes**

  In `internal/taskcli/routes.go`, map the parsed request rather than inventing daemon operations:

  ```text
  mine               GET  /api/tasks
  ready              GET  /api/tasks (ready filter); claim resolves the first eligible task then POSTs /api/tasks/{key}/claim
  show               GET  /api/tasks/{key}
  create             POST /api/tasks
  update/assign      GET  /api/tasks/{key} when revision omitted, then PATCH /api/tasks/{key}
  comment/ask        POST /api/tasks/{key}/comments
  move               GET revision when omitted, then POST /api/tasks/{key}/move
  block/relate       POST /api/tasks/{key}/relations with type blocks/related
  done               GET revision when omitted, then POST /api/tasks/{key}/complete
  workflow history   existing /api/tasks/{key}/workflow* routes
  ```

  For shared workflow execution verbs, operator mode must return a local usage error explaining that leased execution requires agent mode; it must not forge an agent/iteration or add an operator bypass. Operator workflow inspection remains available through the administration commands below.

  Export the existing task route descriptors from `internal/commands/tasks.go` as a copied slice (for example `TaskOperatorCommands() []registry.Command`) and keep `BuildRegistry` consuming the same function. The task CLI strips only the leading `tasks.` path, makes those descriptors visible, and uses the existing `cli.Run` parser for operator-only `queue`, `workflows`, `workflow`, `events`, `principals`, and `notifications` subcommands. Do not copy their HTTP methods, paths, schemas, or argument definitions into `internal/taskcli`.

- [ ] **Step 6: Add the thin binary entry point and turn GREEN**

  `cmd/tariboy-tasks/main.go` should contain only signal context setup and:

  ```go
  os.Exit(taskcli.Run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
  ```

  Run: `go test ./internal/taskcli ./internal/commands ./internal/client -count=1`

  Expected: PASS, including strict no-request parsing failures and no-fallback mode tests.

- [ ] **Step 7: Commit the client slice**

  ```bash
  git add cmd/tariboy-tasks internal/taskcli internal/commands/tasks.go
  git commit -m "feat: restore tariboy tasks cli"
  ```

---

### Task 2: Operator and Agent Integration Contracts

**Files:**
- Create: `scripts/tariboy-tasks-e2e.sh`
- Modify: `Makefile`
- Modify: `store/skills/tasks/scripts/tasks.sh`
- Delete: `store/skills/tasks/scripts/tasks.py`
- Modify: `store/skills/tasks/SKILL.md`
- Modify: `store/skills/test_store_skills.py`
- Modify: `internal/agentdir/shims.go`
- Modify: `internal/agentdir/shims_test.go`

**Interfaces:**
- Consumes: the new `tariboy-tasks` binary, installed `ttasks` name, existing per-agent `TARIBOY_TOOLS_SOCKET`, existing isolated daemon helpers, and capability-controlled legacy `tasks` shims.
- Produces: a Python-free Tasks skill launcher (`exec ttasks "$@"`), an optional legacy `tasks -> skill launcher -> ttasks` chain, and an isolated operator/agent security contract.

- [ ] **Step 1: Write failing Store skill and shim tests**

  Update `store/skills/test_store_skills.py` to assert `tasks/scripts/tasks.sh` contains no `python3` or `tasks.py`, invokes `ttasks`, passes arguments unchanged, and propagates its exit code. Use a temporary fake `ttasks` earlier on `PATH` to capture argv.

  Extend `internal/agentdir/shims_test.go` to assert an image with `tasks` still gets the legacy `tasks` compatibility file, that it points to the skill launcher, and removing the capability removes only that compatibility shim. Do not add an agent-local `ttasks` shim because the installed system alias owns that name.

- [ ] **Step 2: Run Store and shim tests and observe RED**

  Run: `python3 -m unittest store.skills.test_store_skills && go test ./internal/agentdir -count=1`

  Expected: FAIL because the launcher still executes `tasks.py`.

- [ ] **Step 3: Switch the skill to the installed binary**

  Replace `store/skills/tasks/scripts/tasks.sh` with:

  ```sh
  #!/bin/sh
  exec ttasks "$@"
  ```

  Delete `tasks.py`; update `store/skills/tasks/SKILL.md` so every example uses `ttasks`, identifies `tasks` only as the optional compatibility alias, and states that the binary selects identity-bound agent mode from `TARIBOY_TOOLS_SOCKET`.

- [ ] **Step 4: Add an isolated end-to-end mode contract**

  In `scripts/tariboy-tasks-e2e.sh`, use two `mktemp -d` directories, export both `TARIBOY_BASE_DIR` and `TARIBOY_RUNTIME_DIR`, start `bin/tariboyd --http-addr ""`, and trap shutdown/removal. Create two queues and tasks through operator `ttasks`; assert operator `ttasks mine --json` sees both queues and can update both.

  Create an agent with only one queue/task visible, start a stub iteration, set its real `TARIBOY_TOOLS_SOCKET`, and assert `ttasks mine --json` omits the other queue. Then replace the tools socket with a missing path while leaving the operator daemon live and assert `ttasks mine` exits `2` without returning operator data.

  Add a `tariboy-tasks-e2e` Make target and include it in `backend-check` after focused Go tests; do not add it to `full-check` separately.

- [ ] **Step 5: Turn integration contracts GREEN**

  Run: `go test ./internal/agentdir ./internal/taskcli -count=1 && python3 -m unittest store.skills.test_store_skills && make tariboy-tasks-e2e`

  Expected: PASS with all daemon state isolated under temporary directories.

- [ ] **Step 6: Commit the integration slice**

  ```bash
  git add Makefile scripts/tariboy-tasks-e2e.sh store/skills/tasks internal/agentdir
  git commit -m "feat: route tasks skill through ttasks"
  ```

---

### Task 3: Server Build, Install, Alias, and Rollback

**Files:**
- Modify: `Makefile`
- Modify: `desktop/src-tauri/src/remote-install.sh`
- Modify: `scripts/server-install-contract-test.sh`

**Interfaces:**
- Consumes: `bin/tariboy-tasks`, the versioned release directory, and the existing transactional remote installer.
- Produces: real release file `<version>/tariboy-tasks`; managed links `tariboy-tasks -> <version>/tariboy-tasks` and `ttasks -> <version>/tariboy-tasks`; preflight and rollback for both names.

- [ ] **Step 1: Write the failing server installer contract**

  Extend `scripts/server-install-contract-test.sh` so its fake Go builder creates `tariboy-tasks`, its old release contains both links targeting the old real file, and after `make server-install` it asserts:

  ```sh
  test "$(readlink "$home/.local/bin/tariboy-tasks")" = "$home/.local/lib/tariboy/$version/tariboy-tasks"
  test "$(readlink "$home/.local/bin/ttasks")" = "$home/.local/lib/tariboy/$version/tariboy-tasks"
  test "$("$home/.local/bin/ttasks" --version)" = "$version"
  ```

  Add cases where a foreign `ttasks` regular file or symlink aborts before any managed link changes, and where a forced mid-switch failure restores both old links.

- [ ] **Step 2: Run the installer contract and observe RED**

  Run: `scripts/server-install-contract-test.sh`

  Expected: FAIL because neither binary nor alias is built/switched.

- [ ] **Step 3: Build one binary and model links separately from files**

  Add `tariboy-tasks` to `BINARIES` and the `build` recipe in `Makefile`. Keep checksums and release copies over real binaries only. Introduce an alias pair used by `install`, `server-install`, and `uninstall`; `install` writes the real executable once and creates `ttasks` as a relative or absolute link to it.

  In `remote-install.sh`, replace the same-basename loop assumption with a two-column managed set:

  ```text
  tariboyd:tariboyd
  tariboy:tariboy
  tariboy-tasks:tariboy-tasks
  ttasks:tariboy-tasks
  tariboy-shim:tariboy-shim
  tariboy-plugin-telegram:tariboy-plugin-telegram
  ```

  Validate every destination before mutation, checksum only source files, stage `.$link.new-$suffix`, back up `.$link.old-$suffix`, and restore by link name. A managed old `ttasks` link is valid only when its target is `$root/$old_version/tariboy-tasks`; a same-basename target is foreign.

- [ ] **Step 4: Turn the server contract GREEN**

  Run: `scripts/server-install-contract-test.sh && go test ./cmd/tariboy-tasks ./internal/taskcli -count=1`

  Expected: PASS, including preflight refusal and rollback.

- [ ] **Step 5: Commit the server packaging slice**

  ```bash
  git add Makefile desktop/src-tauri/src/remote-install.sh scripts/server-install-contract-test.sh
  git commit -m "feat: install tariboy tasks aliases"
  ```

---

### Task 4: Desktop Bundle and Transactional Local/Remote Install

**Files:**
- Modify: `Makefile`
- Modify: `desktop/src-tauri/src/bundle.rs`
- Modify: `desktop/src-tauri/src/cli_install.rs`
- Modify: `desktop/src-tauri/src/provision.rs`
- Modify: `desktop/src-tauri/src/menu.rs`
- Modify: `scripts/remote-provision-smoke.sh`
- Modify: `scripts/product-alpha-e2e.sh`
- Modify: `scripts/check-alpha-artifacts.sh`

**Interfaces:**
- Consumes: Desktop platform payload file `tariboy-tasks`, `bundle::BINARIES`, and the remote installer link/source mapping from Task 3.
- Produces: Desktop bundles containing the real binary; local and remote installers managing both command names atomically; updated upload, architecture, version, and alpha artifact gates.

- [ ] **Step 1: Write failing Rust bundle and installer tests**

  In `bundle.rs`, expect five bundled real binaries and assert `files_for_upload()` contains `tariboy-tasks` once. In `cli_install.rs`, replace test helpers that assume link basename equals source basename with a `ManagedLink { name, source }` set and add assertions that both `tariboy-tasks` and `ttasks` target the bundled `tariboy-tasks` file.

  Add occupied-`ttasks` and injected-rename-failure tests proving preflight and rollback leave every original link intact. Extend `provision.rs` tests so upload summaries include `tariboy-tasks`, stage/activate verifies the alias, and a remote install rollback restores it.

- [ ] **Step 2: Run Rust tests and observe RED**

  Run: `. "$HOME/.cargo/env" && (cd desktop/src-tauri && cargo test)`

  Expected: FAIL on missing bundle membership and alias behavior.

- [ ] **Step 3: Implement mapped local links and bundle membership**

  Add `tariboy-tasks` to `DESKTOP_BINARIES` and its build/copy recipes. In Rust define one static mapping:

  ```rust
  pub struct ManagedLink { pub name: &'static str, pub source: &'static str }
  pub const MANAGED_LINKS: [ManagedLink; 6] = [
      ManagedLink { name: "tariboyd", source: "tariboyd" },
      ManagedLink { name: "tariboy", source: "tariboy" },
      ManagedLink { name: "tariboy-tasks", source: "tariboy-tasks" },
      ManagedLink { name: "ttasks", source: "tariboy-tasks" },
      ManagedLink { name: "tariboy-shim", source: "tariboy-shim" },
      ManagedLink { name: "tariboy-plugin-telegram", source: "tariboy-plugin-telegram" },
  ];
  ```

  Keep `BINARIES` as the deduplicated real-file set for bundle validation and upload. Change `cli_install::install_all` to preflight and transact over `MANAGED_LINKS`, validating a managed prior link against its declared source rather than its basename. Update menu messages to say “six managed commands” without listing paths or changing daemon restart behavior.

- [ ] **Step 4: Update shell packaging gates**

  Add `tariboy-tasks` once to remote upload/checksum/architecture/version loops in `remote-provision-smoke.sh`, `product-alpha-e2e.sh`, and `check-alpha-artifacts.sh`. Add separate link assertions for `ttasks`; never upload or checksum an extra `ttasks` file.

- [ ] **Step 5: Turn Desktop checks GREEN**

  Run:

  ```bash
  . "$HOME/.cargo/env"
  (cd desktop/src-tauri && cargo test)
  (cd desktop/src-tauri && cargo clippy --all-targets -- -D warnings)
  make desktop-version-check
  ```

  Expected: PASS. Do not run Desktop production E2E or `make full-check`, per the explicit task instruction.

- [ ] **Step 6: Commit the Desktop packaging slice**

  ```bash
  git add Makefile desktop/src-tauri/src scripts/remote-provision-smoke.sh scripts/product-alpha-e2e.sh scripts/check-alpha-artifacts.sh
  git commit -m "feat: bundle ttasks with desktop installs"
  ```

---

### Task 5: Current Documentation and Final Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/docs/development.mdx`
- Modify: `docs/docs/architecture/index.mdx`
- Modify: `docs/docs/tasks.mdx`
- Modify: `docs/docs/task-workflows.mdx`
- Modify: `docs/docs/plugins/built-in/tasks.mdx`
- Modify: `docs/docs/binaries/index.mdx`
- Modify: `docs/docs/binaries/agent-tools.mdx`
- Modify: `docs/docs/reference/commands.md`
- Modify: `docs/docs/quickstart.mdx`
- Modify: `docs/docs/remote-hosts.mdx`
- Modify: `docs/docs/security-controls.mdx`
- Modify: `docs/docs/support.mdx`

**Interfaces:**
- Consumes: the completed command, trust boundary, real-binary set, and managed-link set.
- Produces: one current operator/agent story: `ttasks` is globally available, `tariboy-tasks` is the real executable, agents are scoped by `TARIBOY_TOOLS_SOCKET`, and installers own six command paths backed by five Desktop payload files.

- [ ] **Step 1: Update documentation with exact resulting behavior**

  Replace agent examples from `tasks` to `ttasks`, retaining one explicit paragraph that `tasks` is a legacy capability-controlled compatibility shim. Document `ttasks --version`, `ttasks --json`, shared verbs, and operator-only queue/workflow/history/principal/event/notification administration. State that operator mode uses the host Unix daemon socket and customer actor; agent mode is selected solely by a non-empty `TARIBOY_TOOLS_SOCKET` and fails closed.

  Update binary counts carefully: server builds have six real binaries (`tariboy-store` included); Desktop payloads have five real files; installers manage six command paths because `ttasks` aliases `tariboy-tasks`. Update uninstall loops so `ttasks` is validated against the `tariboy-tasks` source, not a nonexistent same-name file.

- [ ] **Step 2: Regenerate/check the command reference contract**

  Build only what is needed to inspect help:

  ```bash
  go build -o /tmp/tariboy-tasks-improve-6 ./cmd/tariboy-tasks
  /tmp/tariboy-tasks-improve-6 --help
  /tmp/tariboy-tasks-improve-6 --help-json
  ```

  Use that output to update `docs/docs/reference/commands.md`; do not hand-document commands the binary does not expose.

- [ ] **Step 3: Run focused documentation and formatting checks**

  Run:

  ```bash
  gofmt -w cmd/tariboy-tasks internal/taskcli internal/commands/tasks.go internal/agentdir
  go test ./internal/taskcli ./internal/commands ./internal/agentdir -count=1
  python3 -m unittest store.skills.test_store_skills
  (cd docs && npm run doctor && npm run build)
  git diff --check
  ```

  Expected: every command exits `0`.

- [ ] **Step 4: Run the required final branch verification**

  Run:

  ```bash
  make check
  . "$HOME/.cargo/env"
  (cd desktop/src-tauri && cargo test)
  (cd desktop/src-tauri && cargo clippy --all-targets -- -D warnings)
  git diff --check
  ```

  Expected: every command exits `0`. Do not run `make full-check`.

- [ ] **Step 5: Inspect the complete diff and commit docs/final corrections**

  Run: `git status --short && git diff --stat && git diff --check && git diff`

  Confirm no ignored Desktop output, generated Store UI, credentials, task-state files, or version changes are present. Resolve every Critical or Important review finding, then commit:

  ```bash
  git add README.md docs cmd internal store Makefile scripts desktop/src-tauri/src
  git commit -m "docs: document tariboy tasks cli"
  ```
