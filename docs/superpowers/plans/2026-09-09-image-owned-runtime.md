# Image-Owned Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove embedded Store content from Tariboy and validate/run every agent from its image-owned static prompt and skill bytes.

**Architecture:** A single daemon capability registry drives schema-v2 source validation, portable validation, activation, and runtime rendering. Compatibility shims resolve required launchers from the unpacked active image, while dynamic values and plugin implementations remain daemon-owned.

**Tech Stack:** Go 1.26, Bash, Python, MDX documentation.

**Spec:** `docs/superpowers/specs/2026-09-09-store-owned-image-content-design.md`

## Global Constraints

- Repository: `alekzonder/tariboy`; Native Task: `IMPROVE-46`; completion mode: PR.
- The user-approved shared `skills/loop/finish-iteration.md` requires the
  narrow declared-skill prompt resolver before the `IMPROVE-45` Store PR can
  pass; execute that prerequisite first, then continue the remaining runtime
  migration after Store integration.
- New behavior uses schema v2. New schema-v1 source authoring fails with a migration error; existing self-contained archives retain bounded compatibility.
- Preserve pending-image atomicity and rollback, plugin authorization, loopback boundaries, redaction, and test-daemon isolation.
- Run focused RED/GREEN tests and `make check`; never run `make full-check`.
- Do not change the product version or committed generated UI artifacts.

---

### Task 1: Centralize the daemon image contract

**Files:**
- Create: `internal/imagecontract/contract.go`
- Create: `internal/imagecontract/contract_test.go`
- Modify: `internal/plugincaps/plugincaps.go`
- Modify: `internal/imagefile/v2.go`
- Modify: `internal/image/template.go`
- Modify: `internal/image/build_v2.go`
- Modify: `internal/loop/prompt_v2.go`

**Interfaces:**
- Produces: `ValidateSource(*imagefile.V2, plugincaps.ExternalResolver) error`, `ValidateManifest(image.Manifest, plugincaps.ExternalResolver) error`, and runtime metadata used by `RenderPromptTemplate`.
- Consumes: packaged skill metadata (`Manifest.Skills`) and installed plugin resolution.

- [x] Allow static prompt files below an explicitly declared source-relative
  skill and resolve them from the same immutable `SourceSkills` snapshot;
  retain rejection for undeclared sibling paths.

- [ ] Add table tests proving unknown runtime names, unknown plugins, missing owning plugins, missing packaged built-in skills, and missing `loop`/`tasks` launchers fail with the exact offending name.
- [ ] Run `go test ./internal/imagecontract ./internal/imagefile ./internal/image` and observe RED before the new package/validation exists.
- [ ] Implement the smallest immutable registry for runtime name, owning plugin, renderer key, skill name, and optional launcher path; route parser/template/build/runtime validation through it.
- [ ] Keep external plugins accepted only through `ExternalResolver`; require their instructions/skills only when explicitly declared by the image.
- [ ] Run the focused command and require GREEN; commit as `feat: validate image runtime capabilities`.

### Task 2: Validate imported and pending images on the destination

**Files:**
- Modify: `internal/image/portable_validate.go`
- Modify: `internal/imageportable/service.go`
- Modify: `internal/loop/image_activation.go`
- Test: `internal/image/portable_validate_test.go`
- Test: `internal/imageportable/service_test.go`
- Test: `internal/loop/image_activation_test.go`

**Interfaces:**
- Consumes: `imagecontract.ValidateManifest` and the destination daemon's installed-plugin resolver.
- Produces: import/assignment failures before publication or promotion, preserving the old active digest.

- [ ] Add RED tests for an archive with an unavailable plugin/runtime contract and for pending activation retaining the old image directory, digest, and pending error.
- [ ] Run the three focused packages and confirm the new assertions fail for compatibility validation, not fixture setup.
- [ ] Call the shared validator from portable preview/apply and from activation before swapping directories.
- [ ] Run the same tests to GREEN; commit as `feat: enforce image compatibility on import and activation`.

### Task 3: Move compatibility shims into the active image

**Files:**
- Modify: `internal/agentdir/shims.go`
- Modify: `internal/agentdir/agentdir.go`
- Modify: `internal/loop/manager.go`
- Modify: `internal/loop/image_activation.go`
- Test: `internal/agentdir/agentdir_test.go`
- Test: `internal/loop/image_activation_test.go`

**Interfaces:**
- Produces: `WriteShims(Layout, agent.Agent) error`, resolving launchers below `Layout.ImageDir()`.
- Consumes: already-unpacked active or staged image content.

- [ ] Add RED tests asserting exact active-image launcher paths, regular/executable checks, startup refresh, candidate swap, rollback, and recovered-swap restoration.
- [ ] Run `go test ./internal/agentdir ./internal/loop` and observe failures against versioned Store paths.
- [ ] Remove `skillsDir` parameters and `ManagerConfig.SkillsDir`; use `image/skills/loop/scripts/loop.sh` and `image/skills/tasks/scripts/tasks.sh`.
- [ ] Preserve managed legacy-shim cleanup and atomic old-image restoration on every candidate failure.
- [ ] Run focused tests to GREEN; commit as `refactor: run agent tools from active images`.

### Task 4: Remove embedded Store and daemon basic seeding

**Files:**
- Delete: `store/assets.go`, `store/assets_test.go`, `store/images/**`, `store/skills/**`, `store/prompts/**`
- Delete: `internal/builtinimages/**`
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/paths/paths.go`
- Modify: `internal/imagefile/resolver.go`
- Modify: `internal/commands/image.go`
- Modify: `internal/loop/manager.go`
- Modify: `Makefile`
- Test: relevant files under `internal/daemon`, `internal/imagefile`, `internal/commands`, and `internal/loop`

**Interfaces:**
- Produces: startup that seeds only `image.BareRef`; schema-v2 builds that accept source-relative and plugin-relative static content without versioned Store roots.

- [ ] Add RED tests proving a fresh daemon creates `bare:latest` but not `basic:latest`, starts without `store/versions`, and rejects new schema-v1 source builds with the migration message.
- [ ] Run the focused packages and observe the existing basic/Store behavior fail those assertions.
- [ ] Remove Store installation, bundled fallback, basic generation/embed/install, `$STORE` and `$CURRENT_VERSION_STORE` resolver branches, and Makefile Store/basic-image gates.
- [ ] Leave old on-disk Store versions untouched; remove only code paths that read or create them.
- [ ] Replace repository tests with self-contained temporary skill/image fixtures.
- [ ] Run focused tests to GREEN; commit as `refactor: remove embedded agent Store content`.

### Task 5: Update product documentation and repository checks

**Files:**
- Modify: `README.md`
- Modify: `docs/docs/development.mdx`
- Modify: `docs/docs/architecture/index.mdx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/architecture/iteration-loop.mdx`
- Modify: `docs/docs/images/index.mdx`
- Modify: `docs/docs/images/agent-skills.mdx`
- Modify: `docs/docs/images-and-groups/index.mdx`
- Modify: `docs/docs/plugins/index.mdx`
- Modify: `docs/docs/binaries/index.mdx`
- Modify: quickstart/onboarding pages returned by Semble for `basic:latest` and versioned Store references.

**Interfaces:**
- Produces: current docs that register/build `tariboy-store` before creating an ordinary agent and accurately describe image-owned launchers and destination validation.

- [ ] Replace daemon-bundled basic/Store instructions with Store registration, refresh, SemVer build, and assignment commands.
- [ ] Document that static prompt/skill bytes live in the runnable image while runtime placeholders and plugin capabilities are validated against the selected daemon.
- [ ] Run documentation doctor/build while iterating, then run only the aggregate `make check` for the branch gate.
- [ ] Run `git diff --check`, inspect the complete diff, and resolve Important/Critical review findings.
- [ ] Commit as `docs: describe Store-owned agent images`.

### Task 6: Publish and complete after human integration

**Files:** no production changes.

**Interfaces:**
- Consumes: verified branch head and GitHub PR state.
- Produces: one monitored PR and completed Native Task after observed merge.

- [ ] Run `make check` once on the final unchanged branch state and record exit 0.
- [ ] Push `improve-46-image-owned-content`, use `github-pr-workflow ensure`, and start one durable monitor outside the worktree.
- [ ] Set `IMPROVE-46` to `wait_customer`; process every changed check/review/head result on the same branch.
- [ ] After observed merge, cancel/remove the monitor, fast-forward local `main`, run the distinct post-merge `make check`, remove worktree/branch, post the consolidated task result, and complete `IMPROVE-46`.
- [ ] Complete parent `IMPROVE-43` only after both child PRs have merged and both post-merge checks and cleanups have succeeded.
