# Desktop updater implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Download verified Desktop updates and install/restart them on explicit user action.

**Architecture:** Official Tauri v2 updater owns download verification and installation. One native owner retains the verified package; a global React provider presents state and schedules checks. Existing GitHub Releases host signed archives and the static catalog.

**Tech Stack:** Rust/Tauri v2, React/TypeScript, existing Vitest/Testing Library, Python standard library for release metadata, existing Go script tests.

**Spec:** `docs/superpowers/specs/2026-09-11-desktop-updater-design.md`

## Global constraints

- Approved design: Native Task comment 2035, approval 2059.
- Worktree: `/home/agent/github/tariboy/.worktrees/improve-80-desktop-updates`.
- Branch: `improve-80-desktop-updates`; completion mode PR; never merge it.
- Initial release target: macOS 12+ Apple Silicon (`darwin-aarch64`).
- Automatic download defaults to true; interval six hours.
- Preference key: `desktop:updates:auto-download:v1`.
- Fixed catalog: `https://github.com/alekzonder/tariboy/releases/latest/download/latest.json`.
- Only `make check` for executable verification; never `make full-check` or standalone Cargo/e2e/browser tests.
- Run all shell commands through Bash login shell; source `"$HOME/.cargo/env"` before Cargo.
- Tests never use live daemon state, runtime, or `127.0.0.1:9990`.
- No product version bump, archive disk cache, or remote daemon update action.
- Do not commit ignored Desktop bundles; changes to App and desktop bridge do not affect the separate store entry point.

## Before implementation: establish signing identity

- [ ] Obtain the release owner's permanent Tauri updater public key on the Native Task.
- [ ] Confirm matching private-key ownership and secure backup; do not ask for the private key in a comment.
- [ ] Record the key's provenance on the task and pin only its public value in the configuration.
- [ ] Confirm the release owner will set `TAURI_SIGNING_PRIVATE_KEY` and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` Secrets before publishing.

This is a security prerequisite, not a request to reapprove the design. Do not
invent or silently generate a production trust root. If a new key is needed,
the release owner can generate it on their trusted machine:

```bash
. "$HOME/.cargo/env"
install -d -m 700 "$HOME/.config/tariboy"
cargo tauri signer generate -w "$HOME/.config/tariboy/updater.key"
```

Use a new destination; never overwrite an existing key. Keep the private file
and password in the owner's secure backup. Share only the generated public
value and configure Secrets using GitHub's secret interface. Never put the
private file, password, or their contents in Git, task comments, or PR text.

### Task 1: Signed release artifacts

**Files:**

- Modify: `.github/workflows/desktop-release.yml`
- Modify: `desktop/src-tauri/tauri.conf.json`
- Create: `scripts/desktop-updater-manifest.py`
- Create: `scripts/desktop_updater_manifest_test.go`
- Modify: `docs/docs/development.mdx`, `docs/internal-alpha-release-runbook.md`

**Interfaces:**

- Consumes: verified release directory, canonical version, actual generated archive and signature, repository `alekzonder/tariboy`.
- Produces: `latest.json` and staged archive/signature beside the existing DMG.
- Script CLI: `python3 scripts/desktop-updater-manifest.py VERSION ARCHIVE SIGNATURE RELEASE_DIR`.
- Publication fields: `version`, `notes`, `pub_date`, `platforms.darwin-aarch64.url`, `platforms.darwin-aarch64.signature`.

- [ ] Add a Go fixture test invoking that Python CLI through `exec.Command` with temporary files. The first case supplies an empty signature and requires nonzero exit and no latest.json:

```go
cmd := exec.Command("python3", "desktop-updater-manifest.py",
    "1.2.3", archive, signature, releaseDir)
if err := cmd.Run(); err == nil {
    t.Fatal("accepted empty updater signature")
}
if _, err := os.Stat(filepath.Join(releaseDir, "latest.json")); !os.IsNotExist(err) {
    t.Fatal("published manifest for invalid release")
}
```

Use `t.TempDir()` and write a fake archive plus a fixture release.json declaring
version 1.2.3. Add cases for version mismatch, missing archive, malformed
signature, unsafe archive basename, and valid metadata. The valid case decodes
JSON and compares the exact platform key, pinned versioned URL, and signature.

- [ ] Run `make check`; record expected RED caused by the absent generator or missing rejection.
- [ ] Implement the generator with argparse, pathlib, json, base64, datetime, and shutil. Validate all inputs before writing. Require an exact stable numeric version matching release.json; require a nonempty regular archive and valid Tauri signature encoding. Derive basename from the archive and construct the versioned URL:

```python
url = f"https://github.com/alekzonder/tariboy/releases/download/v{version}/{archive.name}"
manifest = {
    "version": version,
    "notes": f"Tariboy {version}",
    "pub_date": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    "platforms": {"darwin-aarch64": {"url": url, "signature": signature}},
}
```

Write through a temporary manifest and rename only after all validation/staging
succeeds. Add archive/signature to checksums without dropping the existing DMG
entry. Metadata checks complement the actual plugin signature verification.

- [ ] Enable Tauri updater artifacts and pin the supplied public key. Keep the existing app/DMG targets. Add signing Secrets only to the build step environment; missing values fail before publication without printing them.
- [ ] After `make desktop-mac` in the workflow, find exactly one generated macOS updater archive/signature pair and stage through the generator. Build/publish into a draft release, upload all assets, then publish the draft; never expose a partial latest release. Preserve version checks and job-scoped permissions.
- [ ] Update the contributor guide and release runbook with permanent key ownership, Secrets, asset set, draft-to-public flow, and first-install DMG requirement.
- [ ] Run `make check`, confirm GREEN, inspect the complete diff, and commit this slice.

### Task 2: Native verified-package owner

**Files:**

- Modify: `desktop/src-tauri/Cargo.toml`, `desktop/src-tauri/Cargo.lock`
- Modify: `desktop/src-tauri/src/main.rs`
- Create: `desktop/src-tauri/src/updater.rs`
- Modify: `docs/docs/security-controls.mdx`, `docs/docs/support.mdx`

**Interfaces:**

- Consumes: pinned updater configuration and official plugin.
- Produces argument-free IPC `desktop_update_state`, `desktop_update_download`, `desktop_update_install`.
- Event: `desktop://update-state`.
- Snapshot schema:

```rust
#[derive(Clone, serde::Serialize)]
struct UpdateView {
    revision: u64,
    current_version: String,
    phase: String,
    version: String,
    downloaded_bytes: u64,
    total_bytes: Option<u64>,
    error: String,
}
```

Phases are `idle`, `checking`, `downloading`, `up-to-date`, `ready`,
`installing`, `error`. Every publication increments revision. The native
owner retains `Option<(tauri_plugin_updater::Update, Vec<u8>)>` separately
from the serializable view. Do not expose it through IPC.

- [ ] Add native regression tests before production transition code: install without a package; repeated check during each busy/ready phase; failed verification cannot set ready; failed installation retains ready and retry; successful installation is the only restart path. Tests construct state with fake results, without network, daemon, or real installation.
- [ ] Record that Rust RED/GREEN cannot be executed with the customer's `make check` restriction. Do not claim those tests ran.
- [ ] Add the official v2 dependency and regenerate Cargo.lock with the Rust environment loaded. Read the selected plugin version's download/install implementation to verify API and signature/restart ordering.
- [ ] Register the plugin and managed state alongside existing initialization, and add commands to `generate_handler!`. State reads are cheap; asynchronous commands perform network work outside the UI thread.
- [ ] Set checking atomically before yielding. Do not hold a standard Mutex guard across await. Reject prereleases and versions not greater than the installed version. Set a finite timeout. Progress updates snapshot bytes, but only successful `download().await` stores the package and changes to ready.
- [ ] Run installation through `tauri::async_runtime::spawn_blocking`. Move the prepared package into that operation, restoring it if install fails. On success call Tauri restart. Keep existing ExitRequested tunnel cleanup and daemon ownership unchanged.
- [ ] Use safe stage-specific error messages and make all failures leave a retryable state. Unsupported platforms report unavailable without attempting installation.
- [ ] Update security/support docs: signature trust, memory lifetime, retry, read-only DMG limitation, and recovery via verified DMG. Remove the obsolete statement that Desktop has no updater; retain Apple signing limitations.
- [ ] Run `make check`, inspect the diff, and commit; explicitly retain the unexecuted Rust/macOS validation limit in the task.

### Task 3: Global application settings and ready banner

**Files:**

- Modify: `ui/src/lib/desktop.ts`, `ui/src/App.tsx`
- Create: `ui/src/components/DesktopUpdates.tsx`
- Create: `ui/src/components/DesktopUpdates.test.tsx`
- Modify: `docs/docs/architecture/web-ui.mdx`

**Interfaces:**

- Consumes the exact IPC and snapshot schema from Task 2 through `invokeDesktop`.
- Produces `DesktopUpdatesProvider`, `AppSettings`, and `UpdateBanner`.
- The provider surrounds MainApp once; settings and banner consume its shared
  context. No server/daemon target appears in updater calls.

- [ ] Add Vitest tests with a mocked desktop bridge and fake timers. Start with automatic downloads disabled in localStorage; render settings, click manual check, and assert one native request:

```tsx
localStorage.setItem("desktop:updates:auto-download:v1", "false");
render(
  <MemoryRouter>
    <DesktopUpdatesProvider><AppSettings /><UpdateBanner /></DesktopUpdatesProvider>
  </MemoryRouter>,
);
fireEvent.click(await screen.findByRole("button", {
  name: "Проверить и скачать обновление",
}));
await waitFor(() => expect(download).toHaveBeenCalledTimes(1));
```

The mock returns a typed initial idle snapshot and exposes a callback for native
events. Add startup-enabled and six-hour scheduling, disabling/remount
persistence, storage failure, stale revision, downloading without total,
signature failure without a ready banner, ready-to-install, install failure
retry, and route change cases. Browser mode must make no native/network call.
Use existing Testing Library and Vitest; no new dependencies.

- [ ] Run `make check` and observe RED before adding the component.
- [ ] Extend desktop.ts with typed command wrappers and a subscription using its existing bridge. Ensure the provider reconciles a registered event stream with a current snapshot, so events during initial asynchronous registration are not lost.
- [ ] Implement provider state with a versioned boolean preference, one in-flight request guard, and six-hour scheduling. Never infer readiness from bytes/percent, transfer completion, or localStorage.
- [ ] Render AppSettings with existing accessible primitives and the agreed Russian action labels. Disable duplicate operations, show errors with retry, and show a browser-only unavailable explanation.
- [ ] Add `/app-settings` to App.tsx and a labeled settings link beside ThemeToggle. Mount UpdateBanner directly below the titlebar. Keep it independent of daemon banners and server routes.
- [ ] Document settings, default interval, memory-only package retention, explicit install, and daemon independence in web-ui.mdx.
- [ ] Run `make check`, inspect the complete diff, and commit.

### Task 4: Review and PR handoff

- [ ] Review the whole branch with requesting-code-review; resolve Critical and Important findings and rerun `make check` only after relevant changes or for the distinct required verification stage.
- [ ] Inspect `git diff --check` and the complete base-to-head diff; ensure no signing secrets, version bump, generated Desktop assets, or unrelated edits.
- [ ] Record actual `make check` results and explicitly untested Rust/macOS installation.
- [ ] Push the same branch and invoke github-pr-workflow ensure once/idempotently for one English-language PR. Do not put the Native Task key in the PR title/body.
- [ ] Record the PR URL, set Native Task Wait customer, start exactly one durable monitor, and record its stable schedule ID/state directory.
- [ ] On observed human/automation merge, follow the Native Task workflow for monitor cleanup, main fast-forward, distinct post-merge `make check`, worktree/branch cleanup, final task comment, done, and context cleanup.

## Plan review

Release output feeds the configured native plugin; native snapshot names and
IPC are identical to the UI contract. Release and native work share only the
updater configuration. UI changes are Desktop entry-point only. The mandatory
pre-implementation external input is the permanent public key; it must not be
substituted by a test key. The approved verification scope cannot prove native
compilation or real macOS replacement/restart, and that limit must remain
visible in the PR.
