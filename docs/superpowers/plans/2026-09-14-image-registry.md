# Image registry visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the latest image version, tag history, and active versus next agent image accurately.

**Architecture:** Extend existing read projections and reuse ordinary multi-tag publication and the iteration activation gate. No new registry, persisted inventory, or migration.

**Tech Stack:** Go, existing image store and SQLite services, React/TypeScript and existing test tools.

**Spec:** `docs/superpowers/specs/2026-09-14-image-registry-design.md`, approved IMPROVE-99 #2502/#2504.

## Global Constraints

- Work only in `/home/agent/github/tariboy/.worktrees/improve-99-image-registry`, branch `improve-99-image-registry`.
- No product version bump, new dependencies, live daemon access, or real harness execution.
- All shell commands use Bash login shells. All test state is isolated.
- Only `make check` as aggregate verification; never `make full-check`. Focused RED/GREEN checks implement the approved test plan.
- Preserve explicit host routing, existing tag URLs, immutable/reserved protections and pending priority.
- Date means `built_at`, not import time. Missing metadata is unknown, never silently current.
- Commit only owned files, no attribution trailers. Root owns PR/monitor/task lifecycle; no agent merges.

### Task 1: Store latest version and build feedback

**Files:** `internal/stores/stores.go`, its existing tests, `ui/src/lib/stores.ts`, `ui/src/pages/StoresPage.tsx`, existing Store UI tests. Update Store paragraphs of `docs/docs/images/index.mdx` and `docs/docs/architecture/web-ui.mdx`.

**Interfaces:** Preserve `version`, `built_version`, `update_needed`, `error`; `built_version` now means manifest `image_version` of `name:latest`. Add `latest_status` with `built`, `missing`, `unversioned`, or `error`, plus `latest_error` for per-image inspection failure. Consumers must distinguish unversioned source from missing latest. Existing Build request remains selector-only.

- [ ] RED: extend catalog fixture tests with latest 1.0.0, newer tag 2.0.0, source 2.0.0; assert built_version 1.0.0 and update_needed true. Add missing latest, missing source/manifest versions, malformed latest with healthy sibling and irrelevant malformed non-latest tag.
- [ ] Observe expected regression failures with focused Store tests.
- [ ] Replace newest-tag scanning with direct latest inspection per source. Only compare two nonempty versions; missing latest reports missing and permits Build. Malformed latest is a row error separate from source validation. Preserve existing source validation and build serialization.

```go
// Comparison after a successful latest inspection:
row.BuiltVersion = manifest.ImageVersion
row.UpdateNeeded = row.Version != "" && row.BuiltVersion != "" && row.Version != row.BuiltVersion
```

- [ ] RED/GREEN UI: label Source image_version / Latest image_version; display missing latest, missing version, source error and latest error distinctly. Invalid source disables Build; unknown comparison never says Up to date. Manual build remains available with equal versions. Confirm successful default build publishes version-tag and latest, then reload inventory. Test existing host/stale-response behavior remains intact.
- [ ] Document version comparison ceiling: changed bytes with same version still require manual Build. Self-review diff and commit owned changes.

### Task 2: Registry-style Images navigation

**Files:** `internal/commands/image.go` and tests; `ui/src/lib/api.ts`; `ui/src/pages/images/BuiltImages.tsx`, `ui/src/pages/ImagesPage.tsx`, `ui/src/components/ImageLayout.tsx`, existing image route and detail components/tests. Update Images paragraphs of current docs.

**Interfaces:** Add optional `image_version` to each `image ls` row and its TS type. Preserve per-tag API rows and existing detail URLs. Introduce a name-level tag-list route using existing host routing.

- [ ] RED/GREEN command test: an arbitrary tag with manifest version 1.2.3 returns 1.2.3 (not tag spelling); old unversioned image remains readable.
- [ ] RED/GREEN UI grouping: one row per image name with latest version/build date and newest tag/build date. Choose newest by parsed built_at; tied timestamps prefer version-tag over latest, then stable lexical order. Missing/invalid date sorts last; no latest is explicit.
- [ ] Implement the name-level tags view with tag, version, date, digest, latest/newest badges and existing per-tag usage/actions. Keep import at root; transfer/export/remove remain tag actions. Preserve existing protections and links.
- [ ] Keep inspection manifest/files coherent: when mutable ref changes during detail loading, detect differing digest, report refresh and reload consistent data rather than silently mixing generations. Add focused regression coverage.
- [ ] Verify breadcrumbs, encoded names/tags, explicit host routing and empty/error states. Use existing tables/components and no new UI framework.
- [ ] Update documentation, self-review and commit owned changes.

### Task 3: Agent active and next iteration projection

**Files:** `internal/commands/agents.go` and image-status tests; `internal/loop/image_activation_test.go` and existing runner test fixtures (production loop only for reproduced defect); `ui/src/lib/api.ts`, `ui/src/pages/agents/AgentConfigurationTab.tsx` and tests. Update current image/iteration/UI documentation.

**Interfaces:** Keep current/pending payloads; add current `image_version` and a `next` object with ref/digest/image_version, selection reason and explicit inspection error. Resolve current using pinned ref+digest. Next priority is pending exact digest, current ordinary mutable ref generation, then current pinned image. GET remains read-only.

- [ ] RED/GREEN command tests: current pinned version survives rebuilt latest; next projects newer latest without writing pending; explicit pending wins; immutable ref stays pinned; missing metadata/error is explicit. Use existing publication gate for a coherent read.
- [ ] Implement the additive projection by reusing image InspectPinned and mutable resolution; never read current version from moving latest.
- [ ] RED/GREEN Configuration: Current/Activated version and Next iteration each show ref/version/digest. Show update by digest inequality even at equal versions. Show pending/next failures even with empty pending ref. Preserve Use next iteration/Cancel pending and disclose preview is not a reservation.
- [ ] Reload image projection with ordinary agent refresh and build events; reject stale host/agent responses without clobbering runtime drafts.
- [ ] Add deterministic A → production rebuild → B lifecycle test with isolated fake harness fixtures. A retains old snapshot/prompt; B gets new digest/version/template. Cover same-version changed bytes, unchanged latest, immutable ref, explicit pending and preparation failure. Do not launch live agents.
- [ ] Reuse existing atomic default-build tests; extend them only if version-tag/latest contract lacks coverage. Do not require their digests equal.
- [ ] Update docs, self-review and commit owned changes.

### Integration (root)

- [ ] Review each task for spec compliance and quality; resolve Important/Critical issues.
- [ ] Inspect whether shared changes affect `ui/store`; if so regenerate committed Store UI with `make store-ui` in this worktree.
- [ ] Run `make check`, inspect complete diff and `git diff --check`, then final whole-branch review.
- [ ] Create exactly one PR and durable monitor, record PR and schedule on IMPROVE-99, set Wait customer. Never merge; finish task only after observed merge, CI and main ancestry verification, and cleanup.
