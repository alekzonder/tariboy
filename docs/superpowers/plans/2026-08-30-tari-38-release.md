# TARI-38 Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut the next minor Tariboy release, 0.46.0, and tag the merged release commit.

**Architecture:** The repository-owned `scripts/set-version.sh` updates the explicit version allowlist and regenerates Cargo.lock. Release gates validate version consistency; the final committed release is merged to `main` and receives an annotated `v0.46.0` tag.

**Tech Stack:** Bash, Go, Rust/Cargo, Git.

**Spec:** `TARI-38` and `docs/internal-alpha-release-runbook.md`

## Global Constraints

- Use only `scripts/set-version.sh` to change version pins.
- Do not start, stop, or test against a live Tariboy daemon.
- Run the documented release gates, `make check`, `make full-check`, and post-build CLI version checks.

---

### Task 1: Cut and verify version 0.46.0

**Files:**
- Modify: allowlisted version files and `desktop/src-tauri/Cargo.lock` via `scripts/set-version.sh`
- Create: `scripts/release-version.txt` via `scripts/set-version.sh`

**Interfaces:**
- Consumes: `scripts/set-version.sh NEW_VERSION`
- Produces: canonical `internal/version.Version == "0.46.0"` and release tag `v0.46.0`

- [ ] **Step 1: Run the canonical version tool**

Run: `scripts/set-version.sh 0.46.0`
Expected: `PASS: moved canonical version 0.45.1 -> 0.46.0`

- [ ] **Step 2: Run release gates**

Run: `. "$HOME/.cargo/env" && make desktop-version-check && make desktop-lock-check && go test ./scripts/...`
Expected: all commands exit 0.

- [ ] **Step 3: Build and verify CLI version**

Run: `make build && test "$(./bin/tariboy version)" = "0.46.0" && test "$(./bin/tariboy --version)" = "0.46.0"`
Expected: all commands exit 0.

- [ ] **Step 4: Run repository checks and inspect the release diff**

Run: `make check && make full-check && git diff --check && git diff -- docs/superpowers/plans/2026-08-30-tari-38-release.md`
Expected: checks exit 0; the plan-only diff is inspected before committing.

- [ ] **Step 5: Commit, merge, retest main, and tag**

Run: `git add -A && git commit -m "release: 0.46.0"`, merge into `main`, re-run release checks on `main`, then `git tag -a v0.46.0 -m "Tariboy 0.46.0"`.
Expected: the tag points to the merged release commit.
