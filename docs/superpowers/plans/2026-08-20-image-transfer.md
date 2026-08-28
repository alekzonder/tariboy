# Cross-host runnable image transfer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transfer any exportable built image to a selected subset of ready Desktop hosts or all eligible hosts.

**Architecture:** `BuiltImages` captures its explicit source host and opens `ImageTransferDialog`. The dialog derives destinations from `DaemonProvider`, exports one archive against that source, then sequentially runs target-bound preview/apply calls while retaining the Blob only in mounted React state.

**Tech Stack:** React 19, TypeScript, Radix UI, Vitest/Testing Library, Playwright, Tauri WebDriver.

**Spec:** `docs/superpowers/specs/2026-08-20-image-transfer-design.md`

## Global Constraints

- Only non-reserved, `exportable` images receive the action.
- Eligible destinations are the implicit local host plus every configured `ready` host except the captured source; Team membership is irrelevant.
- Export once from the explicit source and send each preview/apply request to its explicit destination; never change the active daemon.
- Archive, selection, import IDs, and progress exist only while the dialog is mounted. Cancellation stops unstarted targets but cannot reverse an accepted import.
- Continue after target failures; same ref/digest renders **Already present**; conflict retry reuses the Blob and retries one target with a chosen ref.
- Do not change daemon routes, SSH verification, loopback boundaries, Keychain handling, or version declarations.

---

## File structure

| File | Responsibility |
| --- | --- |
| `ui/src/pages/images/ImageTransferDialog.tsx` | Target discovery, selection, sequential transfer state, cancellation/retry, accessible progress. |
| `ui/src/pages/images/ImageTransferDialog.test.tsx` | Explicit-target and state-machine unit coverage. |
| `ui/src/pages/images/BuiltImages.tsx` | Source-bound row action and source-list refresh. |
| `ui/src/pages/images/BuiltImages.test.tsx` | Action visibility and captured-source coverage. |
| `ui/tests/images-fixture.tsx`, `ui/tests/images-layout.pw.ts` | Browser fixture and visible flow. |
| `ui/tests/desktop/image-build-assign.pw.ts` | Isolated production Desktop smoke. |
| `docs/docs/images/index.mdx`, `docs/docs/architecture/web-ui.mdx` | Scope, memory, and host-bound request contract. |

### Task 1: Target-selection dialog shell

**Files:**
- Create: `ui/src/pages/images/ImageTransferDialog.tsx`
- Create: `ui/src/pages/images/ImageTransferDialog.test.tsx`

**Interfaces:**
- Consumes: `DaemonMeta`, `ApiTarget`, `downloadImageArchiveOn`, `uploadImageArchiveOn`, `applyImageArchiveOn`.
- Produces: `ImageTransferDialog({ open, onOpenChange, source, ref, daemons, onComplete })` and `eligibleImageTransferTargets(source, daemons)`.

- [ ] **Step 1: Write the failing target-discovery tests.**

```ts
expect(eligibleImageTransferTargets(null, [
  { id: "source", label: "Source", baseURL: "https://source", state: "ready" },
  { id: "ready", label: "Ready", baseURL: "https://ready", state: "ready" },
  { id: "offline", label: "Offline", baseURL: "https://offline", state: "error" },
])).toEqual([{ id: "ready", label: "Ready", baseURL: "https://ready", state: "ready" }]);
```

Cover a local source, explicit source, unready host exclusion, Select all, individual deselection, and the no-target disabled state.

- [ ] **Step 2: Run the new test to verify it fails.**

Run: `cd ui && npx vitest run src/pages/images/ImageTransferDialog.test.tsx`

Expected: FAIL because the component and helper do not exist.

- [ ] **Step 3: Implement the helper and accessible dialog shell.**

```ts
export function eligibleImageTransferTargets(source: ApiTarget, daemons: DaemonMeta[]): DaemonMeta[] {
  return daemons.filter((host) => host.state === "ready" && source !== null && host.id !== source.id);
}
```

Represent local as `null`, exclude it when source is local, key rows by host ID, and add an `aria-live="polite"` status region and labels such as `Transfer to Ready`.

- [ ] **Step 4: Run focused tests to verify they pass.**

Run: `cd ui && npx vitest run src/pages/images/ImageTransferDialog.test.tsx`

Expected: PASS.

- [ ] **Step 5: Commit the slice.**

```bash
git add ui/src/pages/images/ImageTransferDialog.tsx ui/src/pages/images/ImageTransferDialog.test.tsx
git commit -m "feat: add image transfer dialog target selection"
```

### Task 2: Sequential transfer, progress, cancellation, and retry

**Files:**
- Modify: `ui/src/pages/images/ImageTransferDialog.tsx`
- Modify: `ui/src/pages/images/ImageTransferDialog.test.tsx`

**Interfaces:**
- Consumes: `downloadImageArchiveOn(source, ref): Promise<Blob>`, `uploadImageArchiveOn(target, blob): Promise<{import_id; ref; digest}>`, `applyImageArchiveOn(target, importID, ref)`.
- Produces: terminal rows `completed`, `already-present`, `failed`, or `cancelled`; `retry(targetID, ref)` only re-previews/applies one target.

- [ ] **Step 1: Write the failing orchestration tests.**

```ts
await user.click(screen.getByRole("button", { name: "All servers" }));
await user.click(screen.getByRole("button", { name: "Start transfer" }));
await waitFor(() => expect(downloadImageArchiveOn).toHaveBeenCalledTimes(1));
expect(uploadImageArchiveOn).toHaveBeenNthCalledWith(1, targetA, archive);
expect(applyImageArchiveOn).toHaveBeenNthCalledWith(2, targetB, "import-b", "reviewer:v3");
```

Cover partial failure continuation, already-present output, cancellation before the next target, and a conflict retry that adds only one preview/apply call without another export.

- [ ] **Step 2: Run the focused tests to verify they fail.**

Run: `cd ui && npx vitest run src/pages/images/ImageTransferDialog.test.tsx`

Expected: FAIL because no state machine/retry control exists.

- [ ] **Step 3: Implement the minimal sequential state machine.**

Store `{status, importID?, error?, retryRef}` by host ID. Start by exporting one Blob from `source`; iterate selected target IDs in order, setting `previewing`, uploading, setting `importing`, then applying the original ref. Catch errors per host and continue. A conflict renders a `Retag and retry` input prefilled from the original ref; retry only recreates that target preview/apply using the retained Blob. Cleanup on close/unmount clears Blob and staged state.

- [ ] **Step 4: Run focused tests to verify they pass.**

Run: `cd ui && npx vitest run src/pages/images/ImageTransferDialog.test.tsx`

Expected: PASS with one-export, explicit-target, partial-failure, cancellation, idempotency, and retry assertions.

- [ ] **Step 5: Commit the slice.**

```bash
git add ui/src/pages/images/ImageTransferDialog.tsx ui/src/pages/images/ImageTransferDialog.test.tsx
git commit -m "feat: transfer runnable images to selected hosts"
```

### Task 3: Integrate the source-bound row action

**Files:**
- Modify: `ui/src/pages/images/BuiltImages.tsx`
- Modify: `ui/src/pages/images/BuiltImages.test.tsx`

**Interfaces:**
- Consumes: `hostId`, `useOptionalDaemons()`, `resolveDaemon(hostId)`, `listImagesOn(source)`, and `ImageTransferDialog`.
- Produces: `Upload to servers <ref>` only for `!image.bare && image.exportable`; source-scoped refresh after completion.

- [ ] **Step 1: Write failing action/captured-source tests.**

```ts
expect(await screen.findByRole("button", { name: "Upload to servers built:v1" })).toBeEnabled();
expect(screen.queryByRole("button", { name: /Upload to servers basic:latest/ })).toBeNull();
```

Mock a different active daemon and assert the transfer starts with `downloadImageArchiveOn(sourceTarget, "built:v1")`, never the active target.

- [ ] **Step 2: Run the BuiltImages tests to verify they fail.**

Run: `cd ui && npx vitest run src/pages/images/BuiltImages.test.tsx`

Expected: FAIL because the row has no transfer action.

- [ ] **Step 3: Implement source capture and dialog mounting.**

Fetch via `listImagesOn(sourceTarget)`; resolve the route `hostId` once, add the outlined action next to Export, store `{ref, sourceTarget}` on click, and pass the provider registry to the dialog. Increment the source revision only from `onComplete` while mounted.

- [ ] **Step 4: Run the integration tests to verify they pass.**

Run: `cd ui && npx vitest run src/pages/images/BuiltImages.test.tsx src/pages/images/ImageTransferDialog.test.tsx`

Expected: PASS while existing export/import behavior remains green.

- [ ] **Step 5: Commit the slice.**

```bash
git add ui/src/pages/images/BuiltImages.tsx ui/src/pages/images/BuiltImages.test.tsx ui/src/pages/images/ImageTransferDialog.tsx ui/src/pages/images/ImageTransferDialog.test.tsx
git commit -m "feat: expose image upload to servers action"
```

### Task 4: Browser/Desktop coverage and documentation

**Files:**
- Modify: `ui/tests/images-fixture.tsx`
- Modify: `ui/tests/images-layout.pw.ts`
- Modify: `ui/tests/desktop/image-build-assign.pw.ts`
- Modify: `docs/docs/images/index.mdx`
- Modify: `docs/docs/architecture/web-ui.mdx`

- [ ] **Step 1: Write failing browser and Desktop assertions.**

Extend the fixture with ready and unavailable hosts and distinct target responses. Assert Select all shows only ready non-source rows, transfer displays each terminal state, and one source export feeds target-specific preview/apply URLs. Extend the isolated Tauri test to assert the source route remains selected while an upload succeeds.

- [ ] **Step 2: Run browser coverage to verify it fails.**

Run: `cd ui && npx playwright test tests/images-layout.pw.ts --config=playwright.config.ts`

Expected: FAIL until the fixture and dialog flow exist.

- [ ] **Step 3: Implement fixtures, assertions, and documentation.**

Document all-ready-non-source scope, one browser-memory-only runnable archive, per-target continuation/retry, and explicit source/destination targeting that never changes active host. Do not describe the archive as source backup or persistent state.

- [ ] **Step 4: Run focused verification.**

Run: `cd ui && npx vitest run src/pages/images/BuiltImages.test.tsx src/pages/images/ImageTransferDialog.test.tsx && npx playwright test tests/images-layout.pw.ts --config=playwright.config.ts`

Run: `cd docs && npm run doctor && npm run build`

Expected: all exit 0.

- [ ] **Step 5: Commit coverage and docs.**

```bash
git add ui/tests/images-fixture.tsx ui/tests/images-layout.pw.ts ui/tests/desktop/image-build-assign.pw.ts docs/docs/images/index.mdx docs/docs/architecture/web-ui.mdx
git commit -m "test: cover cross-host image transfer"
```

### Task 5: Branch verification and local integration

**Files:** Verify all files above.

- [ ] **Step 1: Inspect exact changes.**

Run: `git diff main...HEAD && git diff --check`

Expected: only the planned image-transfer UI, tests, and docs changes; no whitespace errors.

- [ ] **Step 2: Run complete branch verification.**

Run: `make full-check`

Run: `. "$HOME/.cargo/env" && cd desktop/src-tauri && cargo test && cargo clippy --all-targets -- -D warnings`

Expected: all commands exit 0. The full check includes production Desktop Playwright/tauri-driver coverage on Linux x86_64.

- [ ] **Step 3: Commit corrections, merge, and verify main.**

```bash
git add <only-corrected-task-files>
git commit -m "fix: address image transfer verification"
git checkout main
git merge --no-ff tari-11-image-transfer
make full-check
git worktree remove /home/agent/github/tariboy-worktrees/tari-11-image-transfer
git branch -d tari-11-image-transfer
```

Expected: main passes its post-merge verification and the task worktree/branch are removed.
