# Controlled Git-Backed Agent Improvement Design

## Status

Approved by the user in the current design conversation on 2026-08-28. The
design evaluates agents and teams while they perform real production tasks. It
does not introduce single-agent versus multi-agent experiments, autonomous
A/B testing, or automatic prompt rollout.

The initial release has two mandatory operator approvals: one for the proposed
improvement plan and one for the built image or team revision before rollout.

## Problem

Tariboy can run agents and multi-agent groups from immutable agent images. The
AI proxy already records model transcripts, the audit log records iteration
activity, LLM-as-Judge can evaluate immutable evidence bundles, editable image
sources can be built into immutable images, and pending image assignment can
activate a new image without interrupting a running iteration.

Those mechanisms are not yet connected into one controlled improvement loop.
An operator needs to know whether a role or team:

- followed its assembled prompt and applicable Agent Skills;
- used tools and messages correctly;
- completed the actual task rather than merely producing plausible reasoning;
- coordinated correctly across agent and iteration boundaries;
- remained within security, approval, and lifecycle constraints.

After evaluation, the operator needs an evidence-backed improvement plan, a
reviewable Git change, an immutable image with complete provenance, an explicit
rollout decision, and a reliable rollback target.

Agent image source may live in the Tariboy repository or in another registered
Git repository. Improvements can also expose defects in shared Tariboy prompt
fragments or plugin behavior, so the workflow must route changes to the correct
repository without mutating an installed Store or a built image in place.

## Goals

- Evaluate completed real tasks for one agent or a team.
- Preserve the exact evidence, runtime image, prompt, skills, plugins, source,
  Git commit, and task outcome used by every verdict.
- Run a Judge group on an operator-defined recurring schedule.
- Produce structured findings and a concrete, evidence-linked improvement
  proposal.
- Require approval before any source change and before any production rollout.
- Apply approved changes through ordinary Git branches and pull requests.
- Build only immutable images from merged Git commits.
- Activate a single-agent image or a complete team revision at a safe boundary.
- Keep the previous image or team revision as an immediate rollback target.
- Support image repositories outside the Tariboy repository.
- Route role-specific, shared instruction, and plugin implementation changes to
  their correct owners.

## Non-Goals

- Comparing single-agent and multi-agent performance.
- Generating synthetic benchmark tasks as part of the production loop.
- Letting a Judge merge, publish, assign, or roll out its own changes.
- Automatically optimizing one scalar score.
- Editing installed `$CURRENT_VERSION_STORE` content in place.
- Rebuilding or replacing an existing image tag.
- Allowing a Judge to revise its own rubric in the same improvement chain.
- Creating a second scheduler, task system, message bus, image store, or general
  workflow engine.

## Design Principles

### Reuse existing durable mechanisms

The design extends existing Judge runs, evidence snapshots, schedules, tasks,
messages, image sources, source snapshots, immutable image builds, audit events,
and pending assignments. New state represents only concepts that do not exist
today: execution subjects, improvement proposals, approvals, releases, and team
revisions.

### Separate evidence from instructions

Task text, transcripts, messages, repository content, generated artifacts, and
Judge citations are untrusted evidence. They cannot grant authority, alter the
Judge rubric, select a repository credential, approve a proposal, or authorize
a rollout.

### Bind every decision to immutable content

An approval is valid for one content hash. Editing a proposal, changing a Git
head, rebuilding an image, or changing a team revision invalidates the previous
approval for the affected phase.

### Separate decision and execution authority

Judge, Improver, Publisher, Approver, and Tariboy daemon are distinct security
roles even if the UI presents Judge and Improver as one coaching workflow.

## System Roles

### Work agents

Work agents perform real tasks using their assigned immutable image. Their
capabilities are unchanged by this design. They cannot influence whether their
own work is selected for review.

### Judge group

The Judge group has read-only access to redacted immutable evidence and
registered source snapshots. It selects completed work according to an approved
review policy, submits evidence-linked analyses, produces a summary, and creates
an improvement proposal.

It cannot write Git, edit live image sources, build an image, approve a plan,
assign an image, or modify its own rubric.

### Approver

An authenticated operator approves or rejects the exact improvement proposal
revision and later approves or rejects the exact release candidate or team
revision. Approval identity, timestamp, phase, object ID, and content hash are
audited.

### Improver

After plan approval, an Improver runs in a new, task-scoped iteration. It
receives the approved proposal, one registered repository, one base commit, a
path allowlist, and a short-lived branch credential. It applies the smallest
approved change, runs the required checks, and opens or updates one pull
request.

It cannot alter the proposal, its acceptance criteria, Judge evidence, release
state, or assignments. A change outside the approved scope returns the proposal
to plan approval.

### Publisher

The Publisher is deterministic service code, not an LLM role. It validates a
merged commit, resolves locked dependencies, builds an immutable image, records
provenance, and creates a release candidate. It never interprets Judge prose or
chooses which agents receive an image.

### Tariboy daemon

After rollout approval, the daemon records pending single-agent assignments or
a pending team revision and activates them at the defined boundary. It retains
the prior assignment for rollback.

## Source and Repository Ownership

### Repository registry

An operator-managed repository registry identifies source owners without
exposing credentials to Judge evidence:

```yaml
repositories:
  - id: tariboy-core
    url: git@github.com:example/tariboy.git
    default_branch: main
    judge_access: read

  - id: production-agent-images
    url: git@github.com:example/agent-images.git
    default_branch: main
    judge_access: read
```

Registry entries contain repository identity and access policy. Credentials are
resolved by the host credential boundary and are never included in prompts,
transcripts, evidence bundles, proposals, or Git URLs returned to agents.

The Judge may read only registered repositories and exact commits referenced by
the execution evidence. The Improver receives write authority only after plan
approval and only for the proposal's target repository and branch.

### Change routing

Every finding is classified before a proposal is accepted:

| Finding scope | Change owner |
| --- | --- |
| Behavior unique to one role or team | Agent image repository |
| Reusable task procedure | Shared Agent Skill repository or directory |
| Generic guidance for a Tariboy plugin | Tariboy Store source |
| Tool API, authorization, safety, or runtime semantics | Tariboy plugin or daemon code |
| Judge rubric or Judge image | Separately governed Judge source and approval chain |

A proposal may declare dependencies on another proposal, but one approved
proposal modifies one repository. A cross-repository improvement therefore
uses linked proposals and pull requests rather than one credential spanning
multiple repositories.

## Agent Image Source Contract

An externally maintained image source is self-contained at build time:

```text
reviewer-image/
  Tariboyfile.yaml
  tariboy.lock.yaml
  role.md
  policies.md
  overlays/
    messages-reviewer.md
  skills/
    code-review/
      SKILL.md
  vendor/
    tariboy-prompts/
      messages.md
```

The image explicitly declares capabilities, packaged skills, and ordered prompt
layers. A plugin declaration never implicitly contributes a legacy plugin
prompt in schema v2.

```yaml
schema_version: 2

plugins:
  - name: messages
  - name: tasks
  - name: current-task

skills:
  - dir: ./skills/code-review

prompts:
  - runtime: identity
  - file: ./vendor/tariboy-prompts/messages.md
  - file: ./overlays/messages-reviewer.md
  - file: ./role.md
  - runtime: user-prompt
  - runtime: one-shot
```

Image source resolution remains confined: a source-relative file cannot escape
the source root, symlinks are rejected, and build inputs are copied into the
immutable image archive.

### Locked shared prompt dependencies

External image repositories must not depend on an unqualified
`$CURRENT_VERSION_STORE` path when reproducible rebuilds are required. The path
is resolved from the daemon's current Tariboy version at build time, so the same
external Git commit could otherwise acquire a different base prompt after a
Tariboy upgrade.

`tariboy.lock.yaml` records every vendored upstream instruction dependency:

```yaml
schema_version: 1
tariboy_version: 0.18.0

prompt_dependencies:
  messages:
    repository: tariboy-core
    upstream_commit: 82fd301
    upstream_path: store/skills/messages/prompt.md
    upstream_sha256: 96d2d9b2...
    local_path: vendor/tariboy-prompts/messages.md
    mode: upstream
```

The Publisher verifies the local file against the lock before building. The
build records the lock digest in release provenance.

Git submodules are not part of the initial design. A small vendored instruction
set plus hashes is easier to validate, snapshot, build offline, and authorize.

### Overlay and fork behavior

A role-specific addition is an ordered local overlay following the locked base
prompt. It is owned by the image repository and does not affect other images.

When the base instruction is unsuitable for one image, the image may replace it
with a declared fork. A fork lock entry records the upstream base and the local
content separately:

```yaml
prompt_dependencies:
  messages:
    repository: tariboy-core
    upstream_commit: 82fd301
    upstream_path: store/skills/messages/prompt.md
    upstream_sha256: 96d2d9b2...
    local_path: prompts/messages.md
    local_sha256: 5d1f6cf4...
    mode: fork
```

The Publisher permits the intentional hash difference only in `fork` mode.
The proposal must explain why an overlay is insufficient and state how future
upstream changes will be reconciled.

### Upstream shared prompt changes

A generic defect in a standard prompt produces a proposal targeting
`tariboy-core`. After the upstream change is merged and published, each external
image repository updates its lock in a separate pull request and builds a new
image tag. An urgent role-specific hotfix may use a temporary declared fork,
but it does not mutate an installed Store and does not silently become the new
global default.

## Plugins, Skills, and Prompt Layers

Migration of current plugin prompt fragments follows these ownership rules:

- A plugin owns capability metadata, commands, routes, and authorization.
- A Skill owns conditional procedures, detailed workflows, examples, and
  reusable operational knowledge.
- The role prompt owns the agent's responsibility and mandatory behavioral
  contract.
- Runtime prompt layers own iteration-specific identity, context, messages,
  task input, and other host-generated state.
- Security and data-integrity invariants are enforced by the daemon whenever
  possible and are not entrusted only to prose.

Not every current plugin prompt becomes a Skill. Mandatory lifecycle rules such
as processing delivered work, respecting approval boundaries, and completing
an iteration correctly must remain unconditional or become daemon enforcement.
An on-demand Skill is unsuitable for a rule that must hold before the model
chooses which Skill to read.

The long-term standard prompt package for a plugin may be split into a minimal
contract and an optional Agent Skill:

```text
store/skills/messages/
  contract.md
  SKILL.md
```

Images still include both explicitly. Schema v2 does not regain implicit prompt
injection as part of this design.

## Evaluation Unit: Execution Subject

The production evaluation unit is a completed execution subject, normally a
Native Task, rather than one model call or one isolated iteration.

```json
{
  "schema_version": 1,
  "type": "task",
  "task_id": "TASK-184",
  "group": "dev-team",
  "participants": [
    {"agent": "manager", "image_digest": "sha256:111"},
    {"agent": "developer", "image_digest": "sha256:222"},
    {"agent": "reviewer", "image_digest": "sha256:333"}
  ],
  "iteration_ids": ["it-101", "it-102", "it-103"],
  "outcome": {
    "status": "completed",
    "artifacts": ["commit", "pull_request", "test_results"]
  }
}
```

Existing Judge targets remain per iteration so current leases, replicas,
citations, and analyses continue to work. A new durable Judge subject groups
the targets that contributed to the same task. Judge summary and improvement
logic operate on subjects while preserving per-iteration evidence.

The subject snapshot records the task request, final state, relevant artifacts,
participant list, iteration IDs, group identity, and exact image digest used by
each participant. A later task mutation does not change an existing subject.

## Evidence Bundle v2

The current prompt, audit, transcript, usage, redaction, content addressing,
retention pinning, and stable locator behavior remain. Schema v2 adds references
required to judge actual task execution and reconstruct its configuration:

```json
{
  "schema_version": 2,
  "subject": {
    "type": "task",
    "id": "TASK-184",
    "snapshot_hash": "sha256:..."
  },
  "runtime": {
    "agent": "reviewer",
    "group": "dev-team",
    "image_ref": "reviewer:2026.08.28-91ab820",
    "image_digest": "sha256:...",
    "source_digest": "sha256:...",
    "repository_id": "production-agent-images",
    "git_commit": "91ab820"
  },
  "configuration": {
    "assembled_prompt_hash": "sha256:...",
    "lock_hash": "sha256:...",
    "skills": [
      {"name": "code-review", "tree_sha256": "sha256:..."}
    ],
    "plugins": ["messages", "tasks", "current-task"]
  }
}
```

Large task artifacts and source trees are not duplicated into every bundle.
The evidence manifest refers to immutable content-addressed task and image
source snapshots. Judge evidence commands expose bounded `search` and `get`
access using stable artifact names and locators. They never expose arbitrary
host filesystem paths.

Additional artifact kinds are:

- `task`: request, lifecycle, comments needed for the decision, and outcome;
- `messages`: relevant inter-agent delivery and acknowledgement metadata;
- `artifact`: bounded test, commit, pull-request, or generated-result metadata;
- `image`: manifest, prompt template, skill manifest, and lock metadata;
- `source`: files from the immutable source snapshot at the recorded digest.

Secrets continue to be redacted before content is written. Completeness records
which artifacts were present, absent, truncated, or unavailable. Judge must
report evidence gaps rather than infer missing results.

## Review Policy and Scheduling

An operator-approved review policy is versioned with the Judge configuration:

```yaml
schema_version: 1
schedule: "0 2 * * *"
subject: completed_tasks
group: dev-team
minimum_subjects: 5
maximum_subjects: 20
cooldown: 24h
rubric: task-quality/v1
judge_replicas: 2
```

The existing schedule capability wakes the Judge lead. No new scheduler is
introduced. The lead selects terminal subjects after the stored watermark,
creates an ordinary Judge run, and advances the watermark only after immutable
subject and evidence snapshots have succeeded.

Selection is idempotent by subject snapshot hash and rubric digest. A subject
may be deliberately reevaluated under a new rubric, but a recurring schedule
does not repeatedly evaluate the same subject with the same rubric.

The review schedule does not compare architectures or create test work. It
reviews only tasks already performed through the normal production workflow.

## Judge Rubric

The rubric is an immutable, versioned input to the Judge run. It evaluates
separate dimensions rather than optimizing one aggregate score:

- task outcome correctness and completeness;
- adherence to the assembled role prompt;
- correct discovery and use of applicable Agent Skills;
- tool use and error handling;
- inter-agent coordination and message handling;
- lifecycle and approval compliance;
- efficiency and avoidable repeated work;
- security and trust-boundary compliance.

Every violation and strength requires stable evidence citations. Each proposed
change must trace to one or more cited findings. A score may summarize a rubric
dimension for operators, but no score threshold can approve a plan, merge a
pull request, or roll out an image.

Judge rubric source and Judge image source are governed separately from work
images. A Judge may recommend a rubric change, but that recommendation starts a
new independently approved proposal evaluated by a different pinned Judge or a
human reviewer.

## Improvement Proposal

A Judge summary may create one or more durable improvement proposals. Each
proposal targets one repository, one base commit, and one logical release unit.

```yaml
schema_version: 1
proposal_id: imp-2026-0042
judge_run: judge-run-291
subject_ids: [TASK-184, TASK-191]

target:
  repository: production-agent-images
  base_commit: 91ab820
  image: reviewer
  image_digest: sha256:abc

findings:
  - severity: important
    criterion: review-completeness
    observation: Reviewer completed without checking the current CI state.
    evidence:
      - bundle: sha256:evidence
        artifact: transcript
        locator: req-17

changes:
  - file: skills/code-review/SKILL.md
    intent: Make current-head CI verification an explicit procedure.
  - file: images/reviewer/role.md
    intent: Prohibit final approval when CI state is unknown.

acceptance:
  - Reviewer records the reviewed commit SHA and concrete CI state.
  - Failed, pending, or unavailable CI cannot produce an approved verdict.
  - No new tool, filesystem, repository, or rollout authority is granted.

risk: medium
rollback_image: reviewer:2026.08.10-51ca012
```

Free-form recommendations such as “improve the prompt” are not actionable
proposals. A proposal requires file scope, intent, acceptance criteria, risk,
rollback target, source base, and evidence-linked findings.

### Lifecycle

```text
draft
  -> awaiting_plan_approval
  -> approved
  -> implementing
  -> pull_request_open
  -> merged
  -> image_built
  -> awaiting_rollout_approval
  -> rollout_pending
  -> rolled_out
```

Any mutable stage may also become `rejected`, `cancelled`, or `failed`. A source
scope change returns the proposal to `awaiting_plan_approval`. A merged commit
that differs from the approved pull-request head is a new release input and
must pass validation before rollout approval.

## Approval Contract

### Plan approval

The first approval binds:

- proposal ID and revision hash;
- target repository and base commit;
- exact file allowlist;
- intended changes and acceptance criteria;
- risk and rollback plan.

The approval opens the Improver phase. It does not authorize merge, build, or
rollout.

### Rollout approval

The second approval binds:

- merged Git commit;
- complete source and lock digests;
- validation results;
- image tags and digests or the complete team revision;
- target agents or group;
- activation boundary and rollback revision.

The built release candidate remains inert until this approval. A rebuild with
different bytes or provenance requires a new rollout approval.

Approvals are operator-only API and UI actions. They are not conveyed by a
free-form agent message, task comment, transcript statement, repository file,
or Judge recommendation.

## Git Improvement Workflow

After plan approval, Tariboy creates or records one branch based on the approved
commit:

```text
tariboy/improve/reviewer/imp-2026-0042
```

The Improver:

1. verifies repository identity, clean base commit, and approved path scope;
2. creates an isolated branch and worktree;
3. changes only approved files;
4. updates tests or validation fixtures required by the acceptance criteria;
5. runs repository-defined checks;
6. commits with the proposal ID;
7. opens or updates one pull request;
8. reports pull-request URL, head commit, diff hash, and checks to Tariboy.

Existing repository protection and human or automation review remain in force.
Tariboy does not bypass branch protection. A pull request may expand the
approved scope only by returning to plan approval.

For a shared upstream prompt defect, the first proposal and pull request target
`tariboy-core`. After the shared artifact is published, a linked proposal in
the external image repository updates `tariboy.lock.yaml`, vendored content,
and the image tag.

## Deterministic Publication

The Publisher accepts only a merged commit from the registered repository. It:

1. checks out the exact commit without mutable branch dependencies;
2. validates repository and approved source paths;
3. verifies `tariboy.lock.yaml` and every vendored prompt hash;
4. validates `Tariboyfile.yaml` and all Agent Skills;
5. resolves and records installed plugin capability versions;
6. assembles and hashes the ordered prompt template;
7. builds an immutable image through the existing image builder;
8. captures the immutable source snapshot;
9. records proposal, repository, commit, lock, prompt, skills, plugins, source,
   image digest, checks, builder version, and build time;
10. creates an inert release candidate.

Production tags are immutable and include a version or date plus short commit:

```text
reviewer:2026.08.28-91ab820
```

`latest` is not accepted for an approved production assignment. An existing tag
cannot be overwritten. Repeating a build of the same approved input may return
the existing identical release; different output is a failure requiring
investigation.

## Team Revisions and Activation

A team release is a versioned mapping of role assignments, not a mutable list
of independent tags:

```yaml
schema_version: 1
revision: dev-team-r12
repository: production-agent-images
git_commit: 91ab820

agents:
  manager:
    image_ref: manager:2026.08.28-91ab820
    image_digest: sha256:111
  developer:
    image_ref: developer:2026.08.28-91ab820
    image_digest: sha256:222
  reviewer:
    image_ref: reviewer:2026.08.28-91ab820
    image_digest: sha256:333
```

The daemon stages the entire revision. Activation occurs at a task boundary
after the group has no work from the prior subject in progress. New task intake
must not begin between staging and activation. If the boundary cannot be
reached, rollout remains pending and does not partially update the group.

Single-agent assignment continues to activate at the next safe iteration
boundary. A running iteration is never interrupted.

Rollback selects the previous immutable single-agent assignment or complete
team revision. It is an explicit operator action, is fully audited, and follows
the same safe activation boundary. Emergency rollback does not require a new
Judge proposal.

## Commands, API, Events, and Plugins

### Judge and proposal commands

The existing `llm-as-judge` capability is extended with authenticated commands
for structured proposals:

```text
tools judge improvement submit
tools judge improvement inspect
tools judge improvement list
```

Judge agents may submit and inspect proposals for their own authorized run.
They cannot approve them.

Operator-only API and CLI actions are separate from agent authority:

```text
tools improvement approve-plan
tools improvement reject
tools image release inspect
tools image rollout approve
tools image rollout rollback
```

Exact names may follow the final registry naming convention, but the authority
split is normative.

### Events

Durable state remains authoritative. Messages carry object IDs and bounded
summaries rather than full proposals, diffs, transcripts, or image content:

```text
judge.run.completed
improvement.plan.approval_requested
improvement.plan.approved
improvement.plan.rejected
improvement.pull_request.ready
image.release.ready
image.rollout.approval_requested
image.rollout.pending
image.rollout.completed
image.rollout.failed
```

The existing message bus and inbox delivery are reused. No improvement-specific
chat channel is required.

### Plugin assignments

A Judge image normally has:

```yaml
plugins:
  - name: llm-as-judge
  - name: schedule
  - name: messages
  - name: current-task
```

It does not receive `image-creator` or image-assignment authority. An Improver
image receives the repository and task tools required by the approved workflow
but no Judge approval or rollout command. Publisher behavior is service code
and is not packaged as an agent plugin.

## Durable Data Model

The implementation adds the minimum durable records required for the new
state transitions:

### Judge subjects

- ID, type, external subject ID, snapshot hash, created time;
- group and task identity;
- participant agent, iteration, image ref, and image digest mapping;
- outcome and artifact snapshot references;
- optional parent subject for nested work;
- uniqueness by subject snapshot hash.

Judge targets receive a subject ID while retaining their existing iteration
identity and evidence bundle.

### Improvement proposals

- proposal ID, Judge run and summary IDs;
- target repository, base commit, image or team identity;
- normalized proposal document and revision hash;
- status, risk, creator, created and updated times;
- branch, pull request, approved head, merged commit, and last error;
- linked upstream or downstream proposal IDs.

### Approvals

- proposal or release ID;
- phase (`plan` or `rollout`);
- approved object hash;
- decision, actor, timestamp, and bounded reason;
- invalidation timestamp and cause when the bound object changes.

Approval rows are append-only.

### Image releases

- proposal ID, repository, merged commit, builder version;
- image ref and digest;
- source, lock, prompt template, skill tree, and plugin manifest digests;
- validation result hash and creation time;
- release status and prior release ID.

### Team revisions

- revision ID, group, repository, commit, content hash, state;
- complete ordered agent-to-image mapping;
- prior revision ID, staged, activated, and rolled-back times.

## Security and Failure Handling

- Evidence snapshot failure leaves the Judge run non-runnable and does not
  advance its selection watermark.
- Missing task artifacts are reported as evidence gaps, not successful work.
- Source snapshots are content-addressed and exposed through bounded evidence
  locators, never raw host paths.
- Secrets and repository credentials are excluded from evidence and prompts.
- Repository content, pull-request comments, transcripts, and proposed patches
  are untrusted input.
- The Improver cannot change files outside the approved path set.
- The Publisher rejects unresolved dependencies, lock mismatches, symlinks,
  unsafe files, changed build inputs, mutable production tags, or unavailable
  plugin capabilities.
- Approval of one proposal revision does not approve a later revision.
- Approval of one image digest does not approve another build with the same
  human-readable tag.
- A failed or unavailable team member image prevents the entire team revision
  from activating.
- Judge, Improver, and work agents cannot approve their own phase through an
  agent-accessible command.
- Judge rubric or Judge image changes require an independent governance chain.
- Rollout failure preserves the currently active assignment and records the
  error for operator inspection.

## Approaches Explicitly Rejected

### Editing installed Store prompts

Changing `$CURRENT_VERSION_STORE` in place is host-global, unauditable as an
image-source change, and non-reproducible. Shared changes are made in the
Tariboy repository and published normally; local changes use an image-owned
overlay or declared fork.

### Moving every prompt into on-demand Skills

An agent may not load a Skill before violating a mandatory rule. Conditional
procedures belong in Skills; unconditional role, lifecycle, approval, and
security rules remain prompt layers or daemon enforcement.

### One autonomous Judge with all permissions

A model that selects evidence, changes prompts, edits its rubric, merges Git,
publishes images, and rolls them out can reward-hack its own evaluation and
turn prompt injection into production authority. Phase-specific authority and
operator approvals are mandatory.

### Judging only individual iterations

A team task crosses agents, messages, child tasks, and iterations. Per-iteration
evidence is retained, but verdict and improvement scope are based on the
completed execution subject.

### Judging only transcripts

Good reasoning text does not prove task completion. Verdicts also require task
state and outcome artifacts such as commits, tests, pull requests, generated
outputs, and delivery state.

### Mutable `latest` rollout

A mutable tag cannot identify which instructions ran and does not provide a
safe rollback target. Production rollout uses immutable tags and digests.

### Automatic changes from one failure or one score

One incident may be task-specific, tool-related, or an evidence gap. Judge must
show recurring evidence or justify a high-severity single incident, and an
operator approves concrete acceptance criteria. No scalar score authorizes an
automatic change.

### Partial team rollout

Mixed revisions can create incompatible role and message contracts. A team
revision is staged and activated as one unit at a task boundary.

## Migration Strategy

### Phase 1: provenance and evidence

- Add repository and Git provenance to successful image-source snapshots.
- Introduce execution subjects and Evidence Bundle v2.
- Link task outcomes, messages, runtime image digests, source snapshots, skills,
  plugins, prompt hashes, and lock hashes.
- Preserve v1 evidence reading for existing Judge runs.

Exit condition: an operator can reconstruct the exact task, runtime
configuration, and source behind every new verdict.

### Phase 2: review policy and proposals

- Version Judge policies and rubrics.
- Add scheduled subject selection, watermarking, and deduplication.
- Add normalized improvement proposals with evidence citations, source scope,
  acceptance criteria, risk, and rollback target.

Exit condition: Judge creates a reproducible proposal but has no mutation
authority.

### Phase 3: plan approval and Git improvement

- Add append-only plan approvals bound to proposal hashes.
- Add repository registry and scoped source checkout.
- Launch a separate Improver iteration after approval.
- Record branch, pull request, head commit, diff hash, and checks.

Exit condition: every source modification is an approved, reviewable Git diff
against an exact base commit.

### Phase 4: deterministic publication

- Add `tariboy.lock.yaml` validation for external image repositories.
- Build only merged commits into immutable tags.
- Record complete release provenance and create inert release candidates.

Exit condition: repeated approved input is reproducible and no release is
assigned automatically.

### Phase 5: rollout, team revisions, and rollback

- Add rollout approvals bound to release hashes.
- Stage single-agent pending assignments and complete team revisions.
- Activate at safe iteration or task boundaries.
- Retain and expose the previous release for audited rollback.

Exit condition: no unapproved or partial release can become active.

### Phase 6: prompt and Skill migration

- Classify current Store prompt fragments by capability, procedure, role,
  lifecycle, and security responsibility.
- Move conditional procedures into valid Agent Skills.
- Keep mandatory contracts explicit and ordered.
- Publish standard prompt dependencies with hashes for external image repos.
- Migrate built-in images away from duplicated or ambiguous instructions.

Exit condition: plugins provide capabilities, Skills provide reusable
procedures, and prompts provide only explicit mandatory and runtime contracts.

## Testing and Verification

Implementation uses focused failing tests before each behavior change and
isolated Tariboy base and runtime directories. Tests never use the live daemon,
live listener, or live user data.

Required coverage includes:

- evidence v1 compatibility and v2 canonical hashing;
- task-to-iteration grouping for single agents and teams;
- redaction, truncation, completeness, stable locators, and corrupt evidence;
- exact image, source, prompt, skill, plugin, lock, and Git provenance;
- schedule watermarking and duplicate selection prevention;
- proposal validation, citation validation, and repository routing;
- approval hash binding, append-only decisions, and invalidation;
- path-scoped Git changes and credential isolation;
- upstream, overlay, and declared-fork lock verification;
- rejection of mutable or conflicting production tags;
- deterministic release creation and provenance lookup;
- single-agent pending activation without iteration interruption;
- atomic team staging, activation, failure, and rollback;
- authorization failures for Judge, Improver, Publisher, and agent commands;
- prompt-injection-shaped transcript, source, task, and pull-request content;
- operator UI and API presentation of evidence, diff, provenance, approvals,
  release candidate, and rollback target.

Run `make check` for every implementation phase. Run the appropriate isolated
end-to-end suites for Judge, image build, assignment, and group lifecycle
changes. Any Desktop UI implementation also requires the repository-mandated
production Desktop Playwright and `tauri-driver` verification. Run
`make full-check` when the implementation reaches e2e, packaging, or Desktop
behavior.

## Acceptance Criteria

The design is implemented when all of the following are true:

1. A scheduled Judge can select completed real tasks without creating test
   tasks or comparing agent architectures.
2. A verdict identifies the exact task snapshot, participants, iterations,
   images, prompts, skills, plugins, source digests, Git commits, and outcome
   artifacts it evaluated.
3. Every actionable recommendation is a structured, evidence-linked proposal
   targeting one registered repository and exact base commit.
4. Judge cannot write Git, approve a plan, build an image, or authorize rollout.
5. No Improver runs before approval of the exact proposal revision.
6. Every source change is represented by an ordinary branch and pull request
   constrained to the approved scope.
7. An external image repository can use locked upstream prompts, local
   overlays, or declared forks without mutating the installed Tariboy Store.
8. A generic base prompt defect can flow through a separate Tariboy upstream
   proposal and a later external image dependency update.
9. Publisher builds only a merged commit and records complete immutable
   provenance.
10. No built release becomes active before approval of its exact digest and
    rollout mapping.
11. A team revision cannot partially activate.
12. The operator can restore the previous immutable image or team revision and
    inspect the complete audit trail.

## Documentation and Versioning

Implementation must update the Judge, Agent Skills, image source, image and
group, messaging, AI proxy, security controls, and operator workflow
documentation as their behavior changes.

This design-only change does not move the canonical product version. Future
implementation is a user-facing capability and follows the repository release
policy when a release is cut; ordinary implementation commits do not hand-edit
version files.
