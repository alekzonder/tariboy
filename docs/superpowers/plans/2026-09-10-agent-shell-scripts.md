# Agent Shell Scripts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let operators save validated global and per-agent Bash snippets that run before every agent iteration.

**Architecture:** Reuse the command registry and its existing owner-only atomic file writer for two filesystem-backed settings. Wrap the existing harness argv once in the runner so Bash sources the global file followed by the agent file, then reuse one small React editor for both route-selected settings surfaces.

**Tech Stack:** Go 1.26 standard library, React 19, TypeScript 6, Vitest/Testing Library, Bash, Starlight MDX.

**Spec:** `docs/superpowers/specs/2026-09-10-agent-shell-scripts-design.md`

## Global Constraints

- Store only `<base-dir>/global-agent-shell.sh` and `<base-dir>/agents/<name>/agent-shell.sh`, with owner-only atomic writes.
- Validate saves with `bash -n` and preserve the last valid file after rejection.
- Source global before agent on every iteration; source failures must use the existing harness-error path.
- Every daemon and agent test uses temporary isolated paths and never touches live Tariboy state.
- Add no dependency, migration, shell selector, create/clone field, version bump, or generated Desktop artifact.
- Run `make check`, never `make full-check`.

---

### Task 1: Filesystem-backed command endpoints

**Files:**
- Modify: `internal/agentdir/agentdir.go`
- Create: `internal/commands/agent_shell_script.go`
- Create: `internal/commands/agent_shell_script_test.go`
- Modify: `internal/commands/daemon.go`

**Interfaces:**
- Consumes: `registry.Ctx.BaseDir`, `getAgent`, and `writeFileAtomic(path, data)`.
- Produces: `agentdir.Layout.ShellScriptPath() string`; GET/POST `/api/daemon/agent-shell-script`; GET/POST `/api/agents/{name}/shell-script`; JSON field `script`.

- [ ] **Step 1: Write failing command tests**

```go
func TestAgentShellScriptCommandsPersistAndRejectInvalidBash(t *testing.T) {
    c, agents, _ := ctxWithStore(t)
    if err := agents.Create(agent.Agent{Name: "a1", OnTimeout: "restart", OnError: "restart"}); err != nil { t.Fatal(err) }
    valid := "export COLOR=blue\n"
    if _, err := h(t, "agent.shell-script.set")(c, registry.Params{"name": "a1", "script": valid}); err != nil { t.Fatal(err) }
    if got, err := h(t, "agent.shell-script.get")(c, registry.Params{"name": "a1"}); err != nil || got.(map[string]any)["script"] != valid { t.Fatalf("get=%v err=%v", got, err) }
    if _, err := h(t, "agent.shell-script.set")(c, registry.Params{"name": "a1", "script": "if then\n"}); err == nil { t.Fatal("invalid Bash accepted") }
    data, err := os.ReadFile(agentdir.New(agentsDir(c), "a1").ShellScriptPath())
    if err != nil || string(data) != valid { t.Fatalf("file=%q err=%v", data, err) }
}
```

Add the equivalent global-path assertion and route registration assertions in the same test file.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/commands -run 'Test(Global|Agent)ShellScript'`

Expected: FAIL because the command paths and `ShellScriptPath` do not exist.

- [ ] **Step 3: Implement the minimum command surface**

```go
func validateBash(script string) error {
    cmd := exec.Command("bash", "-n")
    cmd.Stdin = strings.NewReader(script)
    if output, err := cmd.CombinedOutput(); err != nil {
        return api.UserError{Code: "invalid_script", Msg: strings.TrimSpace(string(output)), Status: http.StatusBadRequest}
    }
    return nil
}
```

Use one private `shellScriptGet(path)` and `shellScriptSet(path)` handler builder; agent handlers call `getAgent` before using `Layout.ShellScriptPath`, and global handlers join the configured base directory directly.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/commands -run 'Test(Global|Agent)ShellScript'`

Expected: PASS.

### Task 2: Source both scripts before the harness

**Files:**
- Modify: `internal/loop/runner.go`
- Modify: `internal/loop/runner_test.go`

**Interfaces:**
- Consumes: `<base-dir>` as `filepath.Dir(RunnerConfig.AgentsDir)`, `Layout.ShellScriptPath()`, the final iteration environment, and the adapter's unchanged `hargv`.
- Produces: a Bash argv that sources global then agent, shifts both paths, and `exec`s the adapter argv.

- [ ] **Step 1: Write the failing behavioral test**

```go
func TestAgentShellCommandSourcesGlobalThenAgent(t *testing.T) {
    base := t.TempDir()
    layout := agentdir.New(filepath.Join(base, "agents"), "a1")
    if err := os.MkdirAll(layout.Root, 0o700); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(base, "global-agent-shell.sh"), []byte("export ORDER=global\n"), 0o600); err != nil { t.Fatal(err) }
    if err := os.WriteFile(layout.ShellScriptPath(), []byte("export ORDER=$ORDER-agent\n"), 0o600); err != nil { t.Fatal(err) }
    argv := agentShellCommand("/bin/bash", base, layout, []string{"/bin/bash", "-c", `printf %s "$ORDER"`})
    output, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
    if err != nil || string(output) != "global-agent" { t.Fatalf("output=%q err=%v", output, err) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/loop -run TestAgentShellCommandSourcesGlobalThenAgent`

Expected: FAIL because `agentShellCommand` does not exist.

- [ ] **Step 3: Add the single wrapper at the shared launch point**

```go
const agentShellPrelude = `set -e; for script in "$1" "$2"; do [[ ! -f "$script" ]] || source "$script"; done; shift 2; exec "$@"`

func agentShellCommand(bash, baseDir string, layout agentdir.Layout, harnessArgv []string) []string {
    argv := []string{bash, "-c", agentShellPrelude, "tariboy-agent-shell", filepath.Join(baseDir, "global-agent-shell.sh"), layout.ShellScriptPath()}
    return append(argv, harnessArgv...)
}
```

Resolve Bash with `harness.FindExecutable("bash", env, cwd)` after the final environment is assembled, then wrap `hargv` before it is appended to `shimArgv`.

- [ ] **Step 4: Verify GREEN and runner regressions**

Run: `go test ./internal/loop`

Expected: PASS.

### Task 3: Shared textarea editor and current documentation

**Files:**
- Modify: `ui/src/lib/api.ts`
- Create: `ui/src/components/ShellScriptEditor.tsx`
- Modify: `ui/src/pages/settings/SettingsPage.tsx`
- Modify: `ui/src/pages/settings/SettingsPage.test.tsx`
- Modify: `ui/src/pages/AgentSettings.tsx`
- Modify: `ui/src/pages/AgentSettings.test.tsx`
- Modify: `docs/docs/architecture/index.mdx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/architecture/iteration-loop.mdx`
- Modify: `docs/docs/architecture/web-ui.mdx`
- Modify: `docs/docs/security-controls.mdx`

**Interfaces:**
- Consumes: explicit `ApiTarget`, `apiOn`, `agentGetOn`, `agentPostOn`, and the existing shadcn `Textarea`, `Button`, and Card components.
- Produces: `ShellScriptEditor({title, description, load, save})`; typed global/per-agent API helpers; accessible save errors and retained invalid drafts.

- [ ] **Step 1: Write failing UI tests**

```tsx
it("keeps an invalid global shell draft and shows Bash stderr", async () => {
  // Render GeneralSettings under an Outlet context for remote-1.
  // Return "export OK=1" from GET, then reject POST with "line 1: syntax error".
  fireEvent.change(await screen.findByLabelText("Global Agent Shell Script"), { target: { value: "if then" } });
  fireEvent.click(screen.getByRole("button", { name: "Save global agent shell script" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("line 1: syntax error");
  expect(screen.getByLabelText("Global Agent Shell Script")).toHaveValue("if then");
})
```

Add a focused AgentSettings test that loads and saves `/api/agents/alpha/shell-script` on the explicit target.

- [ ] **Step 2: Verify RED**

Run: `cd ui && npm test -- --run src/pages/settings/SettingsPage.test.tsx src/pages/AgentSettings.test.tsx`

Expected: FAIL because the editor and API calls do not exist.

- [ ] **Step 3: Implement the shared editor and wire both pages**

```tsx
export function ShellScriptEditor({ title, description, load, save }: Props) {
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  // Load on callback identity change; save draft verbatim; update saved only on success.
  return <Card>{/* labelled textarea, error, discard, and one save button */}</Card>;
}
```

Use `useOutletContext<ApiTarget>()` in `GeneralSettings`, and pass `target` from `AgentSettings`. Keep drafts only in React memory and send text unchanged.

- [ ] **Step 4: Verify GREEN**

Run: `cd ui && npm test -- --run src/pages/settings/SettingsPage.test.tsx src/pages/AgentSettings.test.tsx`

Expected: PASS.

- [ ] **Step 5: Document the filesystem exception, launch order, validation, permissions, and UI locations**

Describe the two exact paths, Bash validation, atomic owner-only writes, global-before-agent sourcing, source-time failure behavior, explicit-host targeting, and support-bundle exclusion in the listed current architecture documents.

- [ ] **Step 6: Verify the complete unchanged-state branch**

Run: `make check`

Run: `git diff --check`

Inspect: `git status --short` and `git diff --stat && git diff`

Expected: every command exits 0; only the intended source, tests, spec/plan, and current docs differ.
