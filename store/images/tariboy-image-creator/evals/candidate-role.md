Simulation only. I read the candidate instructions, manifest, cases, and the skill bodies named below. No simulated command was executed.

Shared intake for I–L: use `using-superpowers`, `tariboy-image-delivery`, `tasks`, `goal`, `context`, `messages`, and `workdir`. Proposed first command is `ttasks show IMG-10` / `IMG-11` / `IMG-12` / `IMG-13`, respectively. Recover customer, workflow mode, approval, artifacts and assignment; read existing context before replacing it. Use only packet-declared actions for managed workflows.

All four fixtures omit the literal runtime `workdir: /absolute/path` required by `workdir`. Request that runtime fact through the task before workdir-dependent operations; `/tmp/store` identifies the selected Store but does not prove the configured or managed workdir. Commands below describe the subsequent conditional sequence, with identifiers resolved from authoritative task/runtime data.

**I — IMG-10: image-local review skill**

Additional skills read: `tariboy-image-authoring`, `writing-skills`, `test-driven-development`, `systematic-debugging`, `tariboy-image-evals`, `using-git-worktrees`, `image-creator`, and `verification-before-completion`. Also read writing-skills’ testing guidance.

1. Reuse the recorded plan approval. Inspect `/tmp/store/images/reviewer/skills/review/SKILL.md`, its consumers, manifest, locks and supplied failure evidence. Establish the actual failing decision before choosing wording.
2. Inspect Store Git state. Reuse IMG-10’s recorded isolated branch/worktree, or establish one through the delivery process; if no Git, approved in-place work is allowed.
3. Create the missing **skill** suite under `images/reviewer/skills/review/evals/` before editing the skill. Run fresh-context no-guidance controls and the current skill; record actual failures and rationalizations. For behavior-shaping wording, use at least five samples per variant and inspect every flagged result.
4. Apply the smallest approved skill correction, rerun the identical scenarios, and address demonstrated failures. Keep skill evidence separate from image evidence.
5. For image routing/composition, create any missing image suite under `images/reviewer/evals/`, observe the current composed image baseline before changing its inputs, and rerun against the candidate. Give actors the actual role/catalog and selected skill access, without the evaluator rubric.
6. Subject to IMG-10’s version policy, from the isolated Store root:

   ```bash
   tariboy image version get --path images/reviewer
   tariboy image version update patch --path images/reviewer
   ```

   Build through the identity-bound `image-creator` launcher only after confirming its source boundary; packaging success remains separate from behavioral results.

Unresolved: exact approved correction, failing fixtures, Git/integration type, configured workdir, version policy and authorized build access.

**J — IMG-11: independent Store review skill**

Additional skills read: `tariboy-image-authoring`, `writing-skills`, `test-driven-development`, `tariboy-image-evals`, `using-git-worktrees`, and `verification-before-completion`.

1. Reuse approval and inspect `/tmp/store/skills/review/SKILL.md`, its references and consumers.
2. Recover or establish task isolation as in I.
3. Create missing **independent skill** scenarios under `skills/review/evals/`; use `writing-skills`’ RED/GREEN/REFACTOR process. Observe no-guidance and current-skill behavior before changing the skill, then rerun the same cases with the minimal candidate.
4. Preserve exact actor responses, source provenance and limitations. Inspect any affected consumer composition using the separate image-eval method only where that boundary is involved.
5. Publish skill changes and evidence through IMG-11 using its actual integration type.

No image-local skill is required for this route. An independent skill change does not itself justify inventing an image or bumping an image version.

Unresolved: improvement criteria and current skill text, consumers, Git/integration type, workdir and evaluation harness availability.

**K — IMG-12: new triage image, five-minute deadline**

Additional skills read: `tariboy-image-authoring`, `brainstorming`, `writing-plans`, `using-git-worktrees`, and `github-pr-workflow`.

Classify this as architectural because it creates a new workflow. Read the task and existing Store sources; establish triage inputs, decisions, permitted actions and delivery requirements. Reuse existing skills where they fit.

Record PR completion mode before any GitHub preflight:

```bash
ttasks comment IMG-12 'Completion mode: PR'
```

Prepare a concrete Native Task proposal covering `images/triage/Tariboyfile.yaml`, `instructions.md`, required skills, initial `0.1.0` version, behavioral scenarios, packaging checks, and PR delivery. Resolve material missing requirements through a task question. For a flexible task, the approval request is:

```bash
ttasks ask IMG-12 user:<recorded-customer> '<concrete proposal and implementation plan; request approval>'
```

Use the packet’s assignment-scoped question instead if workflow-managed. Record the returned question identifier and resume event on the task, preserve other context entries, and retain `IMG-12 await-plan-approval`.

Do not edit image sources, scaffold, build, or treat the deadline as approval. Continue available investigation; otherwise wait on the recorded unanswered question. After its approved answer, perform GitHub preflight and base synchronization before task worktree creation, then baseline evaluations before implementation.

Unresolved: customer identity, workflow packet, configured workdir, exact triage behavior, repository/base and recorded approval.

**L — IMG-13: publish committed, tested GitHub changes**

Additional skills read: `verification-before-completion`, `finishing-a-development-branch`, `github-pr-workflow`, `scripts`, `requesting-code-review`, and `loop`.

Recover the recorded worktree, branch/base, customer, commit, verification evidence and any PR/monitor. Confirm `Completion mode: PR` was recorded before the successful preflight. Preserve valid verification for unchanged inputs; run missing or invalidated checks, including required review where applicable. The GitHub route overrides the finishing skill’s integration menu.

With authoritative values substituted, the publication sequence is:

```bash
UTILITY=/home/agent/github/tariboy/.worktrees/improve-34-image-creator/store/images/tariboy-developer/skills/github-pr-workflow/scripts/github-pr.py
SCRIPTS=/home/agent/github/tariboy/.worktrees/improve-34-image-creator/store/skills/scripts/scripts/scripts.sh

git -C "$TASK_WORKTREE" push -u "$REMOTE" "$HEAD"
"$UTILITY" ensure --repo "$REPO" --head "$HEAD" --base "$BASE" \
  --title "$TITLE" --body "$BODY"
```

`BODY` includes the customer mention, concrete change, versions, behavioral/packaging evidence and limitations. Recover or record the returned PR number/URL; use `ensure` again after an uncertain result rather than another creation mechanism.

If no monitor exists, choose one task-scoped persistent directory outside the worktree and create one schedule:

```bash
(umask 077 && mkdir -m 700 -- "$STATE_DIR")
"$SCRIPTS" schedule img-13-pr --every 60 --quiet-exit 2 -- \
  "$UTILITY" monitor --repo "$REPO" --pr "$PR_NUMBER" \
  --state-dir "$STATE_DIR"
ttasks update --help
```

Use the returned update-help flags to set the flexible task’s PR field and `wait_customer`. Record PR URL/number, branch/base, schedule name/ID, state directory, customer-mentioned results and the `script.result` resume event on IMG-13. Workflow-managed publication uses only declared actions. Preserve context entries for other tasks and add `IMG-13 monitor-pr`.

Process changed/error results, fixing substantive reviews or failed checks on the same branch. A new head invalidates previous check success. Exit `2` alone is unchanged/quiet. A closed-unmerged PR remains the identified PR with task and monitor active; neither it nor a maintainer’s request to close proves completion.

**Only after monitor evidence reports `merged: true` with merge commit metadata:**

```bash
"$SCRIPTS" cancel "$SCHEDULE_ID"
"$SCRIPTS" rm "$SCHEDULE_ID"
git -C "$BASE_CHECKOUT" fetch "$REMOTE"
git -C "$BASE_CHECKOUT" merge --ff-only "$UPSTREAM"
```

Then run the distinct applicable post-merge checks, remove the task worktree and local branch without forcing away customer changes, and post one consolidated customer-mentioned task comment containing `Required:`, `Completed:`, `Verification:`, `Integration:` with PR and merge commit, and `Cleanup:`. Immediately run `ttasks done IMG-13` or the declared successful workflow outcome, then remove its context pointer.

Any synchronization, verification or cleanup failure keeps the task active. No merge command is proposed. Closure occurs after observed merge, successful post-merge verification and cleanup—not when the PR is published.

Unresolved: concrete repository/branch/base, customer, commit/evidence, packet mode, workdir, persistent state location and any existing PR/schedule identifiers.
