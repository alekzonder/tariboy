# Tariboy Store Image Content Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move all ordinary image, Agent Skill, and static prompt sources into a self-checking `tariboy-store` repository.

**Architecture:** Copy the existing source trees without rewriting their behavior, make every manifest repository-relative, and validate the resulting sources through the public Tariboy binaries in an isolated temporary daemon. The Store repository owns content and checks only; it does not gain daemon implementation code.

**Tech Stack:** YAML, Markdown, Bash, Python standard library, GNU Make, Git.

**Spec:** `docs/superpowers/specs/2026-09-09-store-owned-image-content-design.md`

## Global Constraints

- Repository: `alekzonder/tariboy-store`; Native Task: `IMPROVE-45`; completion mode: PR.
- No submodule, vendored tariboy checkout, new runtime dependency, or generated image archive.
- Use owner-only temporary `TARIBOY_BASE_DIR` and `TARIBOY_RUNTIME_DIR`; pass `--http-addr ""`.
- `make check` is the aggregate gate. Never run `make full-check`.
- Existing images increment `0.1.0` to `0.1.1`; new `basic` starts at `0.1.0`.

---

### Task 1: Move self-contained source trees

**Files:**
- Create: `images/**` from tariboy `store/images/**`
- Create: `skills/**` from tariboy `store/skills/**`
- Create: `skills/loop/finish-iteration.md` from tariboy `store/prompts/iteration-finish.md`
- Create: `images/basic/Tariboyfile.yaml` from tariboy `internal/builtinimages/source/Tariboyfile.yaml`
- Modify: every `images/*/Tariboyfile.yaml`

**Interfaces:**
- Consumes: schema-v2 relative path resolution rooted at each manifest directory.
- Produces: `images/<name>/Tariboyfile.yaml` sources whose skill and prompt inputs remain inside declared, frozen Store content.

- [ ] Copy the four source trees while preserving executable bits.
- [x] Replace every `$CURRENT_VERSION_STORE/skills/<name>` with `../../skills/<name>` and each finish prompt with `../../skills/loop/finish-iteration.md`; keep one shared prompt inside the already declared loop skill.
- [ ] Add `image_version: 0.1.0` to `images/basic/Tariboyfile.yaml`; update the four existing `image_version` values to `0.1.1`.
- [ ] Run `rg -n '\$STORE|\$CURRENT_VERSION_STORE|/github/tariboy' images skills prompts` and require no active source references. Historical eval transcripts may retain quoted fixture paths but must not be executable build inputs.
- [ ] Run `git diff --check` and inspect executable modes for `build.sh` and `skills/*/scripts/*.sh`.
- [ ] Commit the content move as `feat: add self-contained agent image sources`.

### Task 2: Add the failing repository check

**Files:**
- Create: `scripts/check-images.sh`
- Create: `Makefile`
- Modify: `skills/test_store_skills.py` only if its root-relative assumptions fail after the move.

**Interfaces:**
- Consumes: `TARIBOY_BIN` and `TARIBOYD_BIN`, defaulting to `tariboy` and `tariboyd` on `PATH`.
- Produces: `make check`, a read-only checkout gate that returns nonzero for invalid skills, forbidden paths, daemon startup failure, or any image validate/build failure.

- [x] Add shell contracts that reject `$CURRENT_VERSION_STORE` and image-local finish-prompt copies through `scripts/check-images.sh --paths-only`.
- [ ] Run that check and observe RED because `scripts/check-images.sh` does not exist.
- [ ] Implement `--paths-only` with `rg` and implement the full mode with `mktemp -d`, `umask 077`, cleanup traps, an isolated daemon socket, locked-skill restoration in a temporary Store copy, and one `image validate` plus `image build` call per `images/*` directory.
- [ ] Make `make check` run `python3 -B skills/test_store_skills.py`, the shell contract, `scripts/check-images.sh`, and `git diff --check`.
- [ ] Run the shell contract and `make check`; require exit 0 and an unchanged `git status --short`.
- [ ] Commit as `test: validate Store skills and images`.

### Task 3: Document Store operation and publish

**Files:**
- Create: `README.md`

**Interfaces:**
- Consumes: public `tariboy store add`, `store refresh`, `image validate`, and `image build` commands.
- Produces: exact prerequisites and commands for maintaining and building the Store.

- [ ] Document root layout, Git/local Store registration, refresh, versioned image build, `make check`, and the required Tariboy/Node/Python/Git tools.
- [ ] State that runnable images contain static prompt and skill bytes, while plugins and runtime placeholders must exist on the target daemon.
- [ ] Run `make check`, `git diff --check`, inspect the complete diff, and resolve Important/Critical review findings.
- [ ] Commit only intended files, push `improve-45-store-content`, create one PR with `github-pr-workflow ensure`, and start one durable monitor.
- [ ] After observed merge, refresh `main`, rerun `make check`, remove the monitor/worktree/branch, post the consolidated task result, and complete `IMPROVE-45`.
