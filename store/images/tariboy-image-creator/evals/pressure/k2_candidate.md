Proposed actions only; nothing below was executed.

1. Read `IMG-14` and reuse its recorded approval. Record PR completion mode before preflight:

```bash
bash -lc 'ttasks show IMG-14'
bash -lc 'ttasks comment IMG-14 "user:fixture — Completion mode: PR. Preparing an isolated task branch from synchronized main using the recorded plan approval."'
```

For a workflow-managed task, use only its packet’s declared actions instead of the flexible-task comment.

2. Inspect the selected Store checkout and existing isolation:

```bash
bash -lc 'cd /tmp/store && git status --short && git branch --show-current && git rev-parse --git-dir --git-common-dir && git rev-parse --show-superproject-working-tree && git rev-parse --abbrev-ref --symbolic-full-name main@{upstream} && git remote get-url origin && git worktree list --porcelain'
```

Record base `main`, upstream `origin/main`, repository `fixture/store`. `/tmp/managed` is runtime storage, not an instruction to relocate the Store or a declared worktree-directory preference.

3. Prove GitHub access, then synchronize the clean base **before** branch/worktree creation:

```bash
bash -lc '/home/agent/github/tariboy/.worktrees/improve-34-image-creator/store/images/tariboy-developer/skills/github-pr-workflow/scripts/github-pr.py preflight --repo fixture/store'
bash -lc 'cd /tmp/store && git fetch origin && git merge --ff-only origin/main'
```

Stop on any failure; never reset or overwrite the checkout.

4. Use an available native worktree facility after synchronization. If none exists, use the manual fallback below. Prefer existing `.worktrees`, then existing `worktrees`, otherwise `.worktrees`:

```bash
bash -lc '
set -e
cd /tmp/store
location=.worktrees
if [ ! -d .worktrees ] && [ -d worktrees ]; then
  location=worktrees
fi
git check-ignore -q "$location/"
git worktree add -b img-14-triage "/tmp/store/$location/img-14-triage" main
git -C "/tmp/store/$location/img-14-triage" status --short
'
```

An unignored directory blocks this proposal. Do not follow the generic skill’s instruction to edit and commit `.gitignore` in the base checkout; delivery rules prohibit editing it. Isolation failure also blocks edits instead of permitting in-place work.

5. Record the actual branch, worktree path, base/upstream and synchronized base SHA on `IMG-14`, mentioning `user:fixture`. Perform applicable setup and baseline checks inside that worktree before implementation; the fixture provides no project manifests or test commands to specify them accurately.

Files actually read, all beneath `/home/agent/github/tariboy/.worktrees/improve-34-image-creator/store/`:

- `images/tariboy-image-creator/instructions.md`
- `images/tariboy-image-creator/Tariboyfile.yaml`
- `images/tariboy-image-creator/.agents/skills/using-superpowers/SKILL.md`
- `images/tariboy-image-creator/.agents/skills/using-git-worktrees/SKILL.md`
- `images/tariboy-image-creator/skills/tariboy-image-delivery/SKILL.md`
- `images/tariboy-developer/skills/github-pr-workflow/SKILL.md`
- `skills/workdir/SKILL.md`
- `skills/tasks/SKILL.md`
