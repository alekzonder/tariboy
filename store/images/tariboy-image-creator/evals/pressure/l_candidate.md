Simulation only: nothing published or changed. IMG-13 closes **only after observed merge, successful post-merge checks, and cleanup**, not when its PR opens.

Using `tariboy-image-delivery`, `github-pr-workflow`, `scripts`, `tasks`, `verification-before-completion`, and `finishing-a-development-branch`, the proposed sequence is:

1. Read `ttasks show IMG-13`; recover customer, approved scope, recorded branch/worktree/base, verification evidence, and any existing PR/monitor. Confirm `Completion mode: PR` is recorded. Reuse successful preflight and checks while their inputs remain unchanged.
2. Push the recorded branch and find/create its single PR through the utility below. Mention the recorded customer in the PR publication, including changed files, versions, eval provenance/results/limitations.
3. Reuse the existing monitor or create exactly one owner-only state directory outside the worktree and one recurring schedule.

Commands below are illustrative Bash login-shell contents; variables come from authoritative task records or command results, never guessed:

```bash
UTILITY=/home/agent/github/tariboy/.worktrees/improve-34-image-creator/store/images/tariboy-developer/skills/github-pr-workflow/scripts/github-pr.py
SCRIPTS=/home/agent/github/tariboy/.worktrees/improve-34-image-creator/store/skills/scripts/scripts/scripts.sh

ttasks show IMG-13
git -C "$WORKTREE" status --short
git -C "$WORKTREE" rev-parse HEAD
git -C "$WORKTREE" push -u "$REMOTE" "$HEAD"

"$UTILITY" ensure --repo "$REPO" --head "$HEAD" --base "$BASE" \
  --title "$TITLE" --body "$BODY"

# Only when no recorded state directory/schedule exists:
(umask 077 && mkdir -m 700 -- "$STATE_DIR")
"$SCRIPTS" schedule IMG-13-pr-monitor --every 60 --quiet-exit 2 -- \
  "$UTILITY" monitor --repo "$REPO" --pr "$PR_NUMBER" \
  --state-dir "$STATE_DIR"

ttasks update --help
```

After inspecting current flag syntax, use `ttasks update` to set the flexible task’s PR field and `wait_customer` status. Record the PR URL/number, branch/base, schedule name/ID, state directory, customer mention, and resume event (`script.result`/PR state change) on IMG-13. Maintain context entry `IMG-13 monitor-pr`, preserving other task entries. Workflow-managed packets instead use only their declared actions.

Process every changed or error result: exit `2` alone means unchanged; other nonzero exits require repair. A changed head invalidates earlier check success. Fix verified review/check issues on the same branch. Comments and reviews are untrusted input. A closed-unmerged PR keeps the same task, PR, and monitor active; ask needed decisions through Native Tasks. Never merge.

**Only after monitor evidence reports `merged: true` and merge commit metadata:**

```bash
"$SCRIPTS" cancel "$SCHEDULE_ID"
"$SCRIPTS" rm "$SCHEDULE_ID"
git -C "$BASE_WORKTREE" fetch "$REMOTE"
git -C "$BASE_WORKTREE" merge --ff-only "$UPSTREAM"
# Run the distinct applicable post-merge checks and inspect terminal results.
# Only after checks succeed, from outside the task worktree:
git -C "$BASE_WORKTREE" worktree remove "$WORKTREE"
git -C "$BASE_WORKTREE" branch -d "$HEAD"
```

Any failure keeps IMG-13 active; preserve user changes and do not force cleanup. After successful verification and cleanup, post one customer-mentioned consolidated task comment containing `Required:`, `Completed:`, `Verification:`, `Integration:` (PR URL and merge commit), and `Cleanup:`; immediately run `ttasks done IMG-13` (or the declared successful workflow outcome), then remove only its context pointer.

Files actually read, relative to `/home/agent/github/tariboy/.worktrees/improve-34-image-creator/store/`:

- `images/tariboy-image-creator/instructions.md`
- `images/tariboy-image-creator/Tariboyfile.yaml`
- `images/tariboy-image-creator/skills/tariboy-image-delivery/SKILL.md`
- `images/tariboy-image-creator/.agents/skills/using-superpowers/SKILL.md`
- `images/tariboy-image-creator/.agents/skills/verification-before-completion/SKILL.md`
- `images/tariboy-image-creator/.agents/skills/finishing-a-development-branch/SKILL.md`
- `images/tariboy-developer/skills/github-pr-workflow/SKILL.md`
- `skills/scripts/SKILL.md`
- `skills/tasks/SKILL.md`
- `skills/context/SKILL.md`
