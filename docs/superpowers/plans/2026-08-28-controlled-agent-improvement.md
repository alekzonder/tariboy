# Controlled Agent Improvement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect production task evidence, LLM-as-Judge, Git-governed image sources, immutable publication, approvals, and safe single-agent or atomic-team rollout into one controlled improvement loop.

**Architecture:** Extend the existing Judge, image-source snapshot, agent pending-image, task, message, and audit subsystems instead of adding a workflow engine. Judge remains read-only and creates content-hashed proposals; operator approvals unlock a scoped Git Improver and later an immutable release rollout; a deterministic Publisher validates locked source and builds the image.

**Tech Stack:** Go, SQLite migrations, existing registry/CLI/HTTP command surfaces, React/TypeScript UI, existing message bus, existing schema-v2 image builder, Git CLI, YAML, Go and Vitest tests.

**Spec:** `docs/superpowers/specs/2026-08-28-controlled-agent-improvement-design.md`

## Global Constraints

- Review completed production tasks; do not add single-agent versus multi-agent experiments or synthetic benchmark work.
- Judge may read immutable redacted evidence and submit proposals but may not write Git, approve, publish, assign, or roll out.
- Plan and rollout approvals bind exact canonical content hashes and are append-only.
- External image inputs are locked and vendored; do not mutate `$CURRENT_VERSION_STORE` or resolve an unpinned current Store dependency for reproducible publication.
- Existing production image tags remain immutable; production rollout rejects `latest`.
- Team revisions activate as one complete mapping at a task boundary; no partial activation.
- Preserve redaction, filesystem confinement, symlink rejection, loopback boundaries, and repository credential isolation.
- Use isolated `TARIBOY_BASE_DIR` and `TARIBOY_RUNTIME_DIR` values for daemon and e2e checks; never touch the live daemon or live data.
- Do not move the product version during ordinary implementation.
- Changes limited to `docs/superpowers/specs/` or `docs/superpowers/plans/` do not run Blume documentation gates.

---

## File Structure

### New backend files

- `internal/store/migrations/0037_controlled_improvement.sql` — provenance, Judge subjects, proposals, approvals, releases, and team revisions.
- `internal/improvement/model.go` — proposal, approval, release, rollout, and team-revision types and state constants.
- `internal/improvement/validate.go` — canonical JSON hashing and transition validation.
- `internal/improvement/store.go` — transactional persistence and append-only approval operations.
- `internal/improvement/service.go` — operator and Judge-authorized lifecycle methods.
- `internal/improvement/git.go` — registered-repository checkout and approved path-scope validation.
- `internal/improvement/lock.go` — `tariboy.lock.yaml` parsing and vendored prompt hash validation.
- `internal/improvement/publisher.go` — deterministic source validation, image build, source snapshot, and release candidate creation.
- `internal/improvement/store_test.go`, `validate_test.go`, `lock_test.go`, `publisher_test.go` — focused backend tests.
- `internal/commands/improvement.go` — operator CLI and HTTP routes.
- `internal/commands/improvement_test.go` — command authorization and contract tests.

### Existing backend files

- `internal/imagesource/model.go`, `store.go`, and tests — repository, commit, and lock provenance in editable image-source metadata.
- `internal/imagesnapshot/store.go` and tests — persist and query source provenance by image digest.
- `internal/commands/image_source.go`, `team.go`, and tests — pass source provenance into snapshots.
- `internal/judge/model.go`, `store.go`, `select.go`, `snapshot.go`, `evidence.go`, `service.go`, `runner.go`, and tests — execution subjects, evidence v2, proposal submission, and proposal-ready event.
- `internal/registry/registry.go` — `ImprovementControl` and extended Judge control interfaces.
- `internal/daemon/daemon.go` — construct and wire the improvement service and publisher.
- `internal/commands/daemon.go` — register improvement and release command groups.
- `internal/toolscli/toolscli.go` and tests — Judge proposal agent commands.
- `internal/agent/agent.go` and tests — transactional atomic team pending/promote helpers.
- `internal/loop/image_activation.go` and tests — respect staged team revision barriers.
- `store/skills/llm-as-judge/prompt.md` — evidence and proposal submission procedure.
- `internal/plugincaps/plugincaps.go` and tests — advertise new Judge commands.

### UI and product documentation

- `ui/src/lib/types.ts`, `api.ts` — proposal, approval, release, and rollout contracts.
- `ui/src/pages/JudgeRunDetailPage.tsx` and tests — evidence-linked proposal view.
- `ui/src/pages/ImprovementDetailPage.tsx` and tests — approvals, diff/provenance, release, rollout, and rollback.
- `ui/src/App.tsx` — improvement detail route.
- `docs/docs/images/agent-skills.mdx`, `docs/docs/plugins/built-in/llm-as-judge.mdx`, `docs/docs/architecture/ai-proxy.mdx`, `docs/docs/architecture/state-model.mdx`, `docs/docs/images-and-groups/index.mdx`, and `docs/docs/security-controls.mdx` — current product behavior after implementation.

---

### Task 1: Persist Git and lock provenance for image sources

**Files:**
- Create: `internal/store/migrations/0037_controlled_improvement.sql`
- Modify: `internal/imagesource/model.go`
- Modify: `internal/imagesource/store.go`
- Modify: `internal/imagesource/store_test.go`
- Modify: `internal/imagesnapshot/store.go`
- Modify: `internal/imagesnapshot/store_test.go`
- Modify: `internal/commands/image_source.go`
- Modify: `internal/commands/team.go`

**Interfaces:**
- Produces: `imagesource.Provenance{RepositoryID, GitCommit, LockDigest string}`.
- Produces: `imagesnapshot.Store.LookupDigest(context.Context, string) (Snapshot, bool, error)`.
- Produces: source snapshots with `RepositoryID`, `GitCommit`, and `LockDigest`.

- [ ] **Step 1: Write failing metadata and snapshot tests**

Add a source round-trip test that creates a source with:

```go
Provenance: imagesource.Provenance{
    RepositoryID: "production-agent-images",
    GitCommit:    "91ab820",
    LockDigest:   "sha256:lock",
},
```

and asserts `Get` returns the same fields. Add a snapshot test that captures the
same provenance, calls `LookupDigest(ctx, "sha256:image")`, and asserts the
returned source and Git identities.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/imagesource ./internal/imagesnapshot`

Expected: compile failure because `Provenance` and `LookupDigest` do not exist.

- [ ] **Step 3: Add migration and minimal model/storage changes**

Add nullable-safe defaulted columns:

```sql
ALTER TABLE image_source_snapshots ADD COLUMN repository_id TEXT NOT NULL DEFAULT '';
ALTER TABLE image_source_snapshots ADD COLUMN git_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE image_source_snapshots ADD COLUMN lock_digest TEXT NOT NULL DEFAULT '';
CREATE INDEX image_source_snapshots_image_digest_idx ON image_source_snapshots(image_digest);
```

Extend `.tariboy-source.json`, `CreateRequest`, `Snapshot`, `Capture`, and query
scans. `LookupDigest` selects by exact image digest and returns an error if more
than one row has conflicting source provenance.

- [ ] **Step 4: Pass provenance through both image-source build paths**

Read the source metadata before capture in `imageSourceBuild` and team import
build, and pass its `Provenance` to `imagesnapshot.Store.Capture`. Do not infer a
Git commit from the current working directory.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/imagesource ./internal/imagesnapshot ./internal/commands`

Commit: `feat: record image source git provenance`

### Task 2: Add task-level Judge execution subjects

**Files:**
- Modify: `internal/store/migrations/0037_controlled_improvement.sql`
- Modify: `internal/judge/model.go`
- Modify: `internal/judge/store.go`
- Modify: `internal/judge/select.go`
- Modify: `internal/judge/store_test.go`
- Modify: `internal/judge/service_test.go`

**Interfaces:**
- Produces: `judge.Subject` with immutable task snapshot identity.
- Produces: `Target.SubjectID string` while preserving `Target.Iteration`.
- Consumes: `ai_requests.task_id`, native `tasks`, `task_artifacts`, and iteration image snapshots.

- [ ] **Step 1: Write failing subject grouping tests**

Create three terminal iterations: two with AI requests attributed to `TARI-42`
and one attributed to `TARI-43`. Create a Judge run selecting all three and
assert two durable subjects exist, the first subject owns two targets, and each
participant records its iteration image digest.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/judge -run 'Subject|TaskGrouping'`

Expected: compile or assertion failure because subjects are not persisted.

- [ ] **Step 3: Add subject tables and selection logic**

Add `judge_subjects` and `judge_subject_targets`; snapshot task key, task status,
group, participant mapping, and artifact IDs into canonical JSON and store its
SHA-256. An iteration without task attribution receives an `iteration` subject
so existing explicit Judge runs remain valid.

- [ ] **Step 4: Return subjects from operator inspect**

Add `subjects` to `OperatorInspect` without changing existing `targets`,
`analyses`, `summaries`, or usage fields.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/judge ./internal/commands`

Commit: `feat: group judge targets by production task`

### Task 3: Build Evidence Bundle v2 with runtime and task provenance

**Files:**
- Modify: `internal/judge/evidence.go`
- Modify: `internal/judge/snapshot.go`
- Modify: `internal/judge/evidence_test.go`
- Modify: `internal/judge/service_test.go`
- Modify: `internal/daemon/daemon.go`

**Interfaces:**
- Produces: `EvidenceBundle.SchemaVersion == 2` for new snapshots.
- Produces artifact kinds: `task`, `image`, and `source` in addition to v1 artifacts.
- Consumes: `imagesnapshot.Store.LookupDigest` from Task 1 and `judge.Subject` from Task 2.

- [ ] **Step 1: Write failing bundle and locator tests**

Assert a new bundle contains:

```go
Runtime: RuntimeEvidence{
    Agent: "reviewer", ImageRef: "reviewer:v7",
    ImageDigest: "sha256:image", GitCommit: "91ab820",
},
```

plus task key/status, prompt-template hash, skill/plugin manifests, source
digest, lock digest, and task artifact metadata. Assert `Search` and `Get` expose
stable `task`, `image`, and `source` locators without returning a host path.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/judge -run 'EvidenceV2|RuntimeEvidence|TaskEvidence'`

Expected: missing v2 fields and artifact kinds.

- [ ] **Step 3: Implement v2 snapshot construction**

Query `iterations.image_ref`, `image_digest`, and
`prompt_template_sha256`; resolve immutable image source provenance by digest;
load the subject snapshot; record bounded artifact metadata. Keep the existing
redaction pass and canonical CAS hash verification.

- [ ] **Step 4: Preserve v1 reads**

Make `EvidenceReader` accept schema 1 and 2 envelopes. Existing v1 searches must
return their original prompt, metadata, usage, audit, and transcript behavior.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/judge ./internal/daemon`

Commit: `feat: snapshot judge evidence with image provenance`

### Task 4: Persist and validate structured improvement proposals

**Files:**
- Modify: `internal/store/migrations/0037_controlled_improvement.sql`
- Create: `internal/improvement/model.go`
- Create: `internal/improvement/validate.go`
- Create: `internal/improvement/store.go`
- Create: `internal/improvement/validate_test.go`
- Create: `internal/improvement/store_test.go`

**Interfaces:**
- Produces: `improvement.CanonicalHash(any) (string, error)`.
- Produces: `Store.CreateProposal`, `GetProposal`, `ListProposals`, and `TransitionProposal`.
- Proposal statuses are exactly `draft`, `awaiting_plan_approval`, `approved`, `implementing`, `pull_request_open`, `merged`, `image_built`, `awaiting_rollout_approval`, `rollout_pending`, `rolled_out`, `rejected`, `cancelled`, and `failed`.

- [ ] **Step 1: Write failing canonical and proposal validation tests**

Test that reordered JSON object keys yield the same SHA-256, while changed
acceptance criteria change it. Reject a proposal with no evidence citation,
empty repository/base commit, no file scope, `latest` rollback image, or a path
that is absolute or escapes with `..`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/improvement`

Expected: package does not exist.

- [ ] **Step 3: Implement the minimal model and validator**

Use `encoding/json`, `crypto/sha256`, `path.Clean`, and existing ID conventions.
Do not introduce a schema library or generic workflow abstraction.

- [ ] **Step 4: Implement transactional persistence**

Create `improvement_proposals` with normalized JSON, revision hash, immutable
Judge/source linkage, state, branch/PR/build fields, and timestamps. Enforce
allowed state transitions in Go and compare the stored revision in the update
predicate.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/improvement`

Commit: `feat: persist evidence-linked improvement proposals`

### Task 5: Let the authorized Judge summary agent submit proposals

**Files:**
- Modify: `internal/judge/service.go`
- Modify: `internal/judge/service_test.go`
- Modify: `internal/judge/runner.go`
- Modify: `internal/judge/runner_test.go`
- Modify: `internal/toolscli/toolscli.go`
- Modify: `internal/toolscli/toolscli_test.go`
- Modify: `store/skills/llm-as-judge/prompt.md`
- Modify: `internal/plugincaps/plugincaps.go`
- Modify: `internal/daemon/daemon.go`

**Interfaces:**
- Consumes: `improvement.Service.SubmitFromJudge(agent, iteration, runID, ProposalDraft)`.
- Produces agent command: `tools judge improvement submit RUN_ID --file proposal.json`.
- Produces event: `improvement.plan.approval_requested` with proposal ID and revision hash.

- [ ] **Step 1: Write failing authorization and CLI tests**

Assert only the run's current summary agent in its active iteration may submit a
proposal while the run is summarizing or completed. Reject a worker, another
group member, a stale iteration, an uncited target, or a repository/base digest
not present in the run evidence. Assert CLI JSON maps to action
`improvement.submit`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/judge ./internal/toolscli -run Improvement`

- [ ] **Step 3: Implement submission and notification**

Bind caller identity in the existing `JudgeAction` bridge. Validate every
subject, bundle, image digest, repository, and base commit against the immutable
run. Persist the proposal as `awaiting_plan_approval`, then publish the bounded
event after commit.

- [ ] **Step 4: Update Judge instructions**

Document evidence classification, repository routing, required citations,
file scope, acceptance criteria, risk, rollback, and the prohibition on
approval/publication. Do not inject the prompt implicitly from the plugin.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/judge ./internal/toolscli ./internal/daemon ./internal/plugincaps`

Commit: `feat: accept improvement proposals from judge summaries`

### Task 6: Add append-only plan and rollout approvals

**Files:**
- Modify: `internal/store/migrations/0037_controlled_improvement.sql`
- Modify: `internal/improvement/model.go`
- Modify: `internal/improvement/store.go`
- Create: `internal/improvement/service.go`
- Modify: `internal/improvement/store_test.go`
- Create: `internal/improvement/service_test.go`
- Modify: `internal/registry/registry.go`
- Create: `internal/commands/improvement.go`
- Create: `internal/commands/improvement_test.go`
- Modify: `internal/commands/daemon.go`
- Modify: `internal/daemon/daemon.go`

**Interfaces:**
- Produces: `ApprovePlan(id, revisionHash, actor, reason)` and `ApproveRollout(id, releaseHash, actor, reason)`.
- Produces operator routes under `/api/improvements` and `/api/image-releases`.

- [ ] **Step 1: Write failing approval-binding tests**

Approve an exact proposal hash, mutate the proposal revision, and assert the
old approval no longer unlocks implementation. Assert approval rows cannot be
updated or deleted through the Store API. Reject Judge/agent actors and wrong
phase or object hashes.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/improvement ./internal/commands -run Approval`

- [ ] **Step 3: Implement append-only approvals and operator routes**

Insert `improvement_approvals` rows with phase, object hash, decision, actor,
reason, timestamp, and invalidation metadata. The command layer obtains the
authenticated operator actor; it does not accept an actor supplied in request
JSON.

- [ ] **Step 4: Publish bounded approval events**

After transaction commit publish `improvement.plan.approved`,
`improvement.plan.rejected`, and `image.rollout.approval_requested` using the
existing bus. Event data contains IDs and hashes only.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/improvement ./internal/commands ./internal/daemon`

Commit: `feat: require hashed improvement approvals`

### Task 7: Validate external image locks and scoped Git changes

**Files:**
- Create: `internal/improvement/lock.go`
- Create: `internal/improvement/lock_test.go`
- Create: `internal/improvement/git.go`
- Create: `internal/improvement/git_test.go`

**Interfaces:**
- Produces: `LoadLock(sourceDir string) (Lock, error)` and `ValidateLock(sourceDir string, Lock) error`.
- Produces: `ValidateChangedPaths(approved []string, changed []string) error`.
- Produces: `RepositoryRegistry.Resolve(id string) (Repository, error)` without credentials in returned JSON.

- [ ] **Step 1: Write failing lock and path-scope tests**

Cover exact upstream hashes, declared fork local hashes, missing vendor files,
symlinks, oversized files, duplicate paths, absolute paths, traversal, and an
unapproved Git diff path. Assert `mode: upstream` rejects different local bytes
and `mode: fork` requires both upstream and local hashes.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/improvement -run 'Lock|ChangedPaths|Repository'`

- [ ] **Step 3: Implement with standard library plus the existing YAML dependency**

Reuse the repository's YAML package and existing image path confinement rules.
Do not add Git libraries or submodule support. Git execution uses non-interactive
`git` commands with explicit repository and branch arguments and redacted
bounded errors.

- [ ] **Step 4: Add approved branch identity checks**

Require exact repository ID, base commit, branch prefix
`tariboy/improve/<image>/<proposal-id>`, and head commit. Compare
`git diff --name-only <base>...<head>` to the approved path allowlist before a
proposal may transition to `pull_request_open` or `merged`.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/improvement`

Commit: `feat: validate locked external image sources`

### Task 8: Build deterministic inert image releases

**Files:**
- Modify: `internal/store/migrations/0037_controlled_improvement.sql`
- Create: `internal/improvement/publisher.go`
- Create: `internal/improvement/publisher_test.go`
- Modify: `internal/improvement/model.go`
- Modify: `internal/improvement/store.go`
- Modify: `internal/commands/improvement.go`
- Modify: `internal/daemon/daemon.go`

**Interfaces:**
- Produces: `Publisher.Build(ctx, BuildRequest) (Release, error)`.
- Consumes: existing `imagefile.Parse`, `image.BuildV2`, `imagesnapshot.Store.Capture`, installed plugin resolver, and Task 7 lock validation.

- [ ] **Step 1: Write failing deterministic publication tests**

Build a merged fixture commit with a valid image and lock. Assert the release
records proposal, repository, commit, source, lock, prompt-template, skill-tree,
plugin, builder, tag, and image digests and remains inert. Reject `latest`, an
existing conflicting tag, an unmerged/unapproved commit, lock drift, source
mutation during build, and an unavailable plugin.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/improvement -run Publisher`

- [ ] **Step 3: Implement the minimal Publisher**

Validate the exact checkout and lock, parse the source, call the existing v2
builder, capture the source snapshot with Git provenance, and insert
`image_releases` in `image_built` state. If persistence fails, remove only the
newly created image ref. Never assign the image from this method.

- [ ] **Step 4: Add release inspect API**

Return immutable provenance, validation result, current status, prior release,
and rollout eligibility. Do not return repository credentials or host source
paths.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/improvement ./internal/commands ./internal/daemon`

Commit: `feat: publish inert immutable image releases`

### Task 9: Roll out approved single-agent releases and rollback

**Files:**
- Modify: `internal/improvement/service.go`
- Modify: `internal/improvement/service_test.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/agent_test.go`
- Modify: `internal/commands/improvement.go`
- Modify: `internal/loop/image_activation.go`
- Modify: `internal/loop/image_activation_test.go`

**Interfaces:**
- Produces: `StageSingleRollout(releaseID, agent, approvedHash)`.
- Consumes: existing `SetPendingImage` and next-iteration activation.

- [ ] **Step 1: Write failing rollout authorization tests**

Assert an inert release cannot stage without a rollout approval for its exact
release hash. Assert staging preserves the running iteration, updates only the
named agent's pending image, records the prior release, and is idempotent.
Assert rollback stages the exact previous immutable ref and digest.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/improvement ./internal/agent ./internal/loop -run 'Rollout|Rollback'`

- [ ] **Step 3: Implement transactional staging**

In one database transaction verify release, approval, target agent, current and
pending assignments, then write pending assignment and rollout state. Existing
loop activation remains the only code that promotes an agent image.

- [ ] **Step 4: Record activation outcome**

When pending promotion succeeds or fails, update the linked release rollout and
publish `image.rollout.completed` or `image.rollout.failed`. Failure leaves the
old active image unchanged.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/improvement ./internal/agent ./internal/loop ./internal/commands`

Commit: `feat: roll out approved agent image releases`

### Task 10: Stage and atomically activate complete team revisions

**Files:**
- Modify: `internal/store/migrations/0037_controlled_improvement.sql`
- Modify: `internal/improvement/model.go`
- Modify: `internal/improvement/store.go`
- Modify: `internal/improvement/service.go`
- Modify: `internal/improvement/service_test.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/agent_test.go`
- Modify: `internal/loop/manager.go`
- Modify: `internal/loop/image_activation.go`
- Modify: `internal/loop/image_activation_test.go`

**Interfaces:**
- Produces: `StageTeamRevision(group, revisionID, map[string]ImageAssignment)`.
- Produces: `PromoteTeamRevision(group, revisionID)` with all-or-nothing database promotion.

- [ ] **Step 1: Write failing atomicity tests**

Create three grouped agents, keep one iteration running, and stage a complete
revision. Assert no member promotes while the group is busy. End the iteration
and assert all three current refs/digests update in one transaction. Inject a
missing image and assert no member changes. Assert rollback restores the entire
prior mapping.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/improvement ./internal/agent ./internal/loop -run TeamRevision`

- [ ] **Step 3: Persist and validate complete mappings**

Add `team_revisions` and `team_revision_members`. Require the exact current
group membership, one immutable existing image per enabled member, one content
hash, one prior revision, and one rollout approval.

- [ ] **Step 4: Add the quiescent task-boundary barrier**

The loop manager checks for any running iteration or active task assignment in
the group before promotion. Promotion updates every agent image and clears every
member pending assignment in one SQLite transaction; it never invokes
single-agent promotion one member at a time.

- [ ] **Step 5: Run GREEN and commit**

Run: `go test ./internal/improvement ./internal/agent ./internal/loop`

Commit: `feat: activate immutable team revisions atomically`

### Task 11: Add operator UI for proposals, approvals, provenance, and rollout

**Files:**
- Modify: `ui/src/lib/types.ts`
- Modify: `ui/src/lib/api.ts`
- Modify: `ui/src/pages/JudgeRunDetailPage.tsx`
- Modify: `ui/src/pages/JudgeRunDetailPage.test.tsx`
- Create: `ui/src/pages/ImprovementDetailPage.tsx`
- Create: `ui/src/pages/ImprovementDetailPage.test.tsx`
- Modify: `ui/src/App.tsx`

**Interfaces:**
- Consumes: operator proposal, approval, release, rollout, evidence, and rollback routes from Tasks 6–10.

- [ ] **Step 1: Write failing UI tests**

Render a Judge run with an awaiting proposal and assert evidence citations,
repository/base commit, files, acceptance criteria, risk, and link to detail.
Render detail states and assert plan approval requires the displayed revision
hash, rollout approval shows exact image/team digests and prior revision, and
buttons disappear for unauthorized or invalid states.

- [ ] **Step 2: Verify RED**

Run: `npm test -- --run ui/src/pages/JudgeRunDetailPage.test.tsx ui/src/pages/ImprovementDetailPage.test.tsx`

- [ ] **Step 3: Implement the minimal screens**

Reuse existing cards, tables, buttons, error handling, and API helpers. Do not
add a generic workflow component or visual diff editor. Git remains the diff
viewer; the UI links to the recorded pull request and shows validated path and
provenance summaries.

- [ ] **Step 4: Run GREEN and commit**

Run: `npm test -- --run ui/src/pages/JudgeRunDetailPage.test.tsx ui/src/pages/ImprovementDetailPage.test.tsx && npm run typecheck && npm run lint`

Commit: `feat: review and approve agent improvements`

### Task 12: Migrate built-in instructions and document the operator workflow

**Files:**
- Modify: selected `store/skills/*/prompt.md`
- Create or modify: selected `store/skills/*/SKILL.md`
- Modify: built-in and Store `Tariboyfile.yaml` files that consume migrated instructions
- Modify: `docs/docs/images/agent-skills.mdx`
- Modify: `docs/docs/plugins/built-in/llm-as-judge.mdx`
- Modify: `docs/docs/architecture/ai-proxy.mdx`
- Modify: `docs/docs/architecture/state-model.mdx`
- Modify: `docs/docs/images-and-groups/index.mdx`
- Modify: `docs/docs/security-controls.mdx`

**Interfaces:**
- Consumes: the completed behavior from Tasks 1–11.

- [ ] **Step 1: Classify each current prompt fragment**

For each built-in fragment, record whether each instruction is capability help,
conditional procedure, role contract, runtime state, lifecycle invariant, or
security enforcement. Move only conditional procedures into a valid Agent
Skill; keep unconditional contracts explicitly ordered in the image.

- [ ] **Step 2: Write failing image contract tests**

Assert migrated images declare the expected plugins, skills, and mandatory
prompt layers; skill trees validate; no schema-v2 image relies on a plugin
manifest's legacy prompt field; and no mandatory finish/message/approval rule
exists only in an on-demand Skill.

- [ ] **Step 3: Verify RED, migrate minimally, and verify GREEN**

Run the focused built-in image and Agent Skill tests, make the smallest prompt
and image changes, then rerun them.

- [ ] **Step 4: Update current product documentation**

Document task-level evidence, repository ownership, locked external prompts,
Judge proposal authority, both approvals, immutable publication, atomic team
revision activation, and rollback. Do not copy the historical design spec into
product documentation.

- [ ] **Step 5: Run repository verification and commit**

Run: `make check`

For Desktop UI behavior also run the mandated production Desktop Playwright and
`tauri-driver` gates, then `make full-check` because the implementation reaches
e2e and Desktop behavior.

Run: `git diff --check` and inspect the complete diff.

Commit: `docs: document controlled agent improvement`

---

## Final Self-Review Checklist

- [ ] Every spec requirement maps to Tasks 1–12.
- [ ] Existing Judge v1 runs and evidence remain readable.
- [ ] Judge cannot mutate source or approve any phase.
- [ ] Approvals bind immutable hashes and are append-only.
- [ ] External source builds are reproducible and do not mutate Store content.
- [ ] Production tags are immutable and reject `latest`.
- [ ] Single-agent rollout does not interrupt a running iteration.
- [ ] Team rollout cannot partially activate.
- [ ] Rollback uses the exact prior immutable mapping.
- [ ] Audit and messages contain bounded IDs and hashes, not secrets or raw evidence.
- [ ] Focused RED→GREEN evidence exists for every production change.
- [ ] `make check`, required e2e/Desktop gates, `make full-check`, and `git diff --check` pass before completion.
