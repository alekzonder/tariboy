# Judge Reliability Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development for implementation and review. The controller owns real operational analysis.

**Goal:** Deliver evidence-grounded iteration reviews on the existing judge system.
**Architecture:** Reuse immutable bundles and v1 analyses; deterministic validation and a frozen rubric surround ordinary Codex judge workers. Add a thin operator entry point to existing run creation.
**Tech Stack:** Go, SQLite, existing CLI registry, Markdown image prompts.
**Spec:** docs/superpowers/specs/2026-09-06-judge-reliability-design.md

## Global Constraints

- No live daemon in tests; isolated temporary base/runtime and disabled HTTP.
- No new dependency, version bump, deployment, or changes to developer agents.
- Keep v1 reports and old immutable bundles readable with unchanged hashes.
- All findings cite immutable evidence. Missing evidence is not an agent failure.
- Use apply_patch, TDD RED/GREEN, scoped tests, then relevant repository checks.

### Task 1: Honest evidence snapshots

**Ownership:** internal/judge/evidence.go, snapshot.go, snapshot_test.go and focused additional evidence tests only.

Read existing snapshot tests and image archive APIs before implementation.
Add focused failing tests proving: usage totals match persisted ai_requests;
empty transcript/audit do not claim complete evidence; completeness is queryable
through stable evidence locators; old bundle hashes remain valid after additive
fields. Populate Usage from the existing query/accounting pattern, expose
completeness in evidence search/get, and represent empty as missing/empty without
equating presence to complete coverage. Preserve redaction. Do not invent task
history or read live repositories. Task content and full skills are separate
follow-up work if no immutable capture contract can safely serve them here.

- [ ] Read owning docs and callers; write focused failing snapshot/evidence tests.
- [ ] Run `go test ./internal/judge -run 'Snapshot|Evidence'` and record RED.
- [ ] Implement minimal changes; run the focused tests and full judge package.
- [ ] Commit only owned files; report RED/GREEN and compatibility risks.

### Task 2: Grounded rubric and submission contract

**Ownership:** judge validation/consensus and tests; store/images/llm-as-judge
instructions and rubric; automatic run criteria; sanitized calibration cases.

- [ ] Add table tests for missing citations, fail without violations, uncertain
  without gaps, nonfinite scores/confidence, contradictory pass and violations,
  and all-uncertain consensus. Observe RED, implement minimal validation, update
  existing valid fixtures to provide real citations where needed.
- [ ] Add a fixed rubric addressing applicable instructions, task progress,
  verification, safety, efficiency, infrastructure and evidence gaps. Embed the
  same source into automatic OriginalRequest so it is persisted with the run;
  workers must apply those criteria and cite requirement plus action.
- [ ] Make summary/improvement instruction ordering consistent and prevent
  mandatory proposals when no actionable image defect is evidenced.
- [ ] Run judge and Store skill tests; record actual production calibration
  separately from deterministic schema tests; commit.

### Task 3: Simple operator review and handoff

**Ownership:** existing command registry/operator judge route, tests, current
product documentation, sanitized operational report.

- [ ] Locate existing operator run actions and CreateRun. Add `judge review`
  taking explicit terminal iteration IDs and configured judge roles; permit an
  explicit bounded judges-per-iteration for independent review. Reuse normal
  authorization and run creation; do not invoke daemon lifecycle.
- [ ] Test invalid selection/roles without side effects and successful frozen
  criteria/selection through the real service and command adapter.
- [ ] Update product docs with one-command usage, evidence limitations and
  calibrated confidence guidance. Run appropriate checks and full diff review.
- [ ] Report actual live Codex runs, model assignments, findings and limitations;
  retain worktree/branch for the user without merging or replacing live binaries.
