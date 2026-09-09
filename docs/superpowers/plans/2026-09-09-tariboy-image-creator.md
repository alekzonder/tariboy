# Tariboy Image Creator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Store image that creates and improves images through Native Tasks, with reusable image knowledge and existing writing-skills for skill work.

**Architecture:** One role prompt routes to three local domain skills and existing Store/Superpowers/PR skills. No runtime API changes or new eval framework. These steps share a single image composition and run inline; independent behavioral actors and final review use subagents.

**Tech Stack:** YAML, Markdown, existing Go image builder, existing Python PR utility.

**Spec:** `docs/superpowers/specs/2026-09-09-tariboy-image-creator-design.md`

## Global Constraints

- Schema v2; initial image version `0.1.0`; no product version change.
- Skill creation, improvement and skill evals MUST use existing `writing-skills`.
- Domain entrypoints and writing-skills precede the larger catalog.
- Reuse `../tariboy-developer/skills/github-pr-workflow`, without copying the utility.
- Native Tasks owns questions and approvals. One Git branch/worktree per task. Never merge GitHub PRs.
- Preserve non-GitHub/no-Git artifacts pending a recorded customer decision.
- Customer comment 1685 approves the revised spec and narrows verification to image-related tests/checks. Do not run `make check` or `make full-check`.
- Test builders use `t.TempDir()` stores, no live daemon or listener.

## Task 1: Protect image packaging

**Files:** Create `internal/builtinimages/image_creator_test.go`; extend `internal/builtinimages/builtinimages_test.go` actionable-runtime table.
**Consumes:** `imagefile.ParseV2`, `image.BuildV2`, `image.Store.Inspect` as used in `generate_test.go`.
**Produces:** Real image build contract; independently restored upstream dependencies exercised by opt-in full-source build.

- [x] Run existing `go test ./internal/builtinimages` baseline.
- [x] Add new image to runtime-order table; run `go test ./internal/builtinimages -run TestShippedImagesOrderActionableRuntimes` and observe missing manifest RED.
- [x] Add a build test: parse the manifest, use source Store roots and temporary image.Store, BuildV2, inspect required plugins/skills, and inspect archived executable PR utility. Offline test omits only lock-restored upstream directories; a required opt-in full-source test retains every directory after restoration. Assert lock entries cover each upstream declaration.

```go
parsed, err := imagefile.ParseV2(source)
if err != nil { t.Fatal(err) }
store := &image.Store{Dir: t.TempDir()}
ref := image.Ref{Name: "tariboy-image-creator", Tag: "0.1.0"}
_, err = image.BuildV2(parsed, imagefile.ResolveRoots{CurrentVersionStore: assets, Store: assets, CurrentStoreVersion: "test"}, ref, store, time.Now, nil)
if err != nil { t.Fatal(err) }
```

## Task 2: Author and evaluate domain guidance

**Files:** Create `store/images/tariboy-image-creator/skills/{tariboy-image-authoring,tariboy-image-evals,tariboy-image-delivery}/SKILL.md` and their `evals/cases.json`; create image `evals/cases.json` and `evals/README.md`.
**Consumes:** Approved spec; baseline actor given existing image-creator launcher only.
**Produces:** Tested domain skills and repeatable scenario inputs/rubrics with actual baseline/candidate evidence.

For EACH skill, finish its RED/GREEN cycle before authoring the next:

- [x] Store realistic requests and observable criteria separately; baseline actor receives requests and raw inputs only. Save actual response, instruction hash and failures.
- [x] Authoring: describe schema, paths, source dependencies, version get/update, build surfaces, log diagnosis. Require writing-skills for any skill work; do not duplicate its method.
- [x] Evals: describe whole-image routing/composition checks and evidence; route single-skill failures to writing-skills. Missing evals are created before changing instructions.
- [x] Delivery: cover task intake, approval, Git isolation, GitHub PR/monitor/wait_customer/merge verification, non-GitHub and no-Git customer questions, context and completion.
- [x] Run same cases with only candidate skill and raw inputs. Score actual actions, preserve verdicts and limitations; correct demonstrated failures and rerun affected cases.
- [x] Validate skill frontmatter through the real image builder in Task 3; commit intended skill/eval files after review.

## Task 3: Compose, verify and publish

**Files:** Create image `Tariboyfile.yaml`, `instructions.md`, `skills-lock.json`; update `docs/docs/images/agent-skills.mdx`; update spec verification to customer 1685.
**Consumes:** Task 2 domain skills, existing developer locked upstream skills and PR utility.
**Produces:** Buildable source, operator documentation and one task PR.

- [x] Declare tasks, goal, messages, scripts, context, whoami, workdir, status, loop and image-creator plugins and skills. Reuse developer Superpowers lock entries (exclude local github-pr-workflow lock entry). Put required domain/writing-skills/PR entrypoints first; retain runtime prompt ordering and finish asset.
- [x] Write short role process requiring using-superpowers, delivery, authoring, writing-skills for skills, image-evals for composition, and github-pr-workflow plus scripts for publication/monitoring.
- [x] Restore upstream with `npx skills experimental_install` in image directory. Commit lock only, never restored hidden skill directories.
- [x] Run composition behavioral cases with role prompt and catalog, including both image-local and Store skill work invoking writing-skills.
- [x] Run `go test ./internal/builtinimages ./internal/imagefile ./internal/agentskills ./internal/image`; run opt-in full-source build; run existing PR utility unit tests. These are packaging gates, not behavioral evals.
- [x] Update Agent Skills product docs: Store CWD, independent authoring+writing-skills usage, sibling dependency, restore/build commands and delivery outcomes.
- [x] Run docs `npm run doctor` and `npm run build`; inspect full diff and `git diff --check`.
- [x] Independent final review; resolve Important/Critical findings, rerun affected checks only.
- [ ] Commit, push, `github-pr-workflow ensure`, one Scripts monitor outside worktree. Record all identifiers and results, mention customer and set wait_customer. Retain task active until observed merge, post-merge focused checks and cleanup.

## Plan review

Coverage: composition, authoring, task state machine, skill/image eval separation and verification map to Tasks 1–3. Shared interface is the image source layout; no production function changes. Customer 1685 supersedes the spec original aggregate verification requirement. Prior unrelated UI timeout is recorded in task comment 1676 and excluded from this narrowed scope.

## Verification checkpoint

All implementation and branch checks passed. Review finding on repeated evals
was resolved with 19 fresh actors; strict K future-step omission remains
documented and K2 verifies the executable synchronization stage. The final
publication checkbox is a pre-publication snapshot; PR and monitor state are
tracked authoritatively on IMPROVE-34. No aggregate checks were run.
