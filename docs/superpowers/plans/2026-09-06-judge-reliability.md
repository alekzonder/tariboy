# Judge Reliability Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development for implementation and review. The controller owns real operational analysis.

**Goal:** Deliver evidence-grounded iteration reviews on the existing judge system.
**Architecture:** Reuse immutable bundles and v1 analyses; deterministic validation and a frozen rubric surround ordinary Codex judge workers. Add a thin operator entry point to existing run creation.
**Tech Stack:** Go, SQLite, existing CLI registry, Markdown image prompts.
**Spec:** docs/superpowers/specs/2026-09-06-judge-reliability-design.md

## Global Constraints

- No live daemon in tests; isolated temporary base/runtime and disabled HTTP.
- No new dependency or changes to developer agents. Deployment permission was
  subsequently expanded; the version/basic-image approval remains pending as
  recorded in the spec.
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

- [x] Read owning docs and callers; write focused failing snapshot/evidence tests.
- [x] Run `go test ./internal/judge -run 'Snapshot|Evidence'` and record RED.
- [x] Implement minimal changes; run the focused tests and full judge package.
- [x] Commit only owned files; report RED/GREEN and compatibility risks.

### Task 1b: Readable immutable transcript evidence

Production calibration exposed a prerequisite: transcript request/response bytes
are serialized as base64, so plaintext evidence searches miss recorded actions.
Sol decoded the payload itself; Terra returned uncertain on the same evidence.
Reuse the existing session parser to expose readable transcript evidence with
stable request locators, without rewriting old bundles or their hashes. Bound
the presentation to individual calls and avoid repeated full request history.
Decode before applying secret redaction in new snapshots. Test search/get parity,
legacy bundles, malformed payload handling, and redaction using synthetic data.
No new transcript engine or production daemon deployment.

Completed in c9fecb0; focused and full judge tests passed, task review approved.

### Task 2: Grounded rubric and submission contract

**Ownership:** judge validation/consensus and tests; store/images/llm-as-judge
instructions and rubric; automatic run criteria; sanitized calibration cases.

Canonical rubric source: `store/prompts/judge-rubric.md`, exposed using existing
`storeassets.ReadBundled` (store/assets.go) to a small `judge.ReviewCriteria`
helper, and referenced by the judge image as an ordered prompt layer. Persist
the rubric text and SHA-256 in OriginalRequest at creation; no migration is
needed. Use `.superpowers/sdd/2026-09-06-judge-reliability/calibration-criteria.md`
as the reviewed wording seed, generalized beyond the four production cases.
The helper must be usable by Task 3 for identical operator criteria.

Operational evidence also shows workers guessing `work submit` and `--help`.
Ensure image instructions name `analysis submit --assignment ID --file FILE
--json`, show exact evidence search syntax and stable string locators, and
provide useful per-command help in the packaged judge script if necessary.
The existing `evidence get` script parses a JSON object for locator while the
service expects a string: fix this API mismatch with a focused script test.

- [x] Add table tests for missing citations, fail without violations, uncertain
  without gaps, nonfinite scores/confidence, contradictory pass and violations,
  and all-uncertain consensus. Observe RED, implement minimal validation, update
  existing valid fixtures to provide real citations where needed.
- [x] Add a fixed rubric addressing applicable instructions, task progress,
  verification, safety, efficiency, infrastructure and evidence gaps. Embed the
  same source into automatic OriginalRequest so it is persisted with the run;
  workers must apply those criteria and cite requirement plus action.
- [x] Make summary/improvement instruction ordering consistent and prevent
  mandatory proposals when no actionable image defect is evidenced.
- [x] Run judge and Store skill tests; record actual production calibration
  separately from deterministic schema tests; commit.

### Task 3: Simple operator review and handoff

**Ownership:** existing command registry/operator judge route, tests, current
product documentation, sanitized operational report.

Shared helper from Task 2: `judge.ReviewCriteria() (text, sha256 string, err error)`.
Use repeated `--iteration ID` flags following registry Repeatable patterns.
Keep selection strictly explicit: combining ExplicitIDs with automatic selector
filters currently unions selections in Store.selectIterations. Do not widen the
operator request to the automation's target set. Manual review uses configured
roles even when the cron schedule is disabled, and must not enable agents or
loops. Automatic JUDGE task workflows remain unchanged; document manual review
as a bounded run, not a new task workflow.

- [x] Locate existing operator run actions and CreateRun. Add `judge review`
  taking explicit terminal iteration IDs and configured judge roles; permit an
  explicit bounded judges-per-iteration for independent review. Reuse normal
  authorization and run creation; do not invoke daemon lifecycle.
- [x] Test invalid selection/roles without side effects and successful frozen
  criteria/selection through the real service and command adapter.
- [x] Update product docs with one-command usage, evidence limitations and
  calibrated confidence guidance. Run appropriate checks and full diff review.
- [x] Report actual live Codex runs, model assignments, findings and limitations;
  retain worktree/branch for the user without merging or replacing live binaries.

## Final review and verification

Final review found two important edge cases: transcript projection ignored a
JSON serialization error, and the shared parser trimmed changed histories by
message count. Commit 377822b fixes both with focused RED/GREEN tests; the parser
now trims only a genuinely matching prefix. Commit 850220a makes the rubric
require main-task evidence rather than auxiliary title-generation evidence.
The follow-up review has no remaining findings.

At 850220a, `make check` passed (backend 147 seconds; frontend 114 seconds).
`make build` passed; both local CLI version forms report the unchanged canonical
0.47.0. These builds do not replace installed binaries or restart the daemon.
