---
name: publish-release
description: Use when cutting and publishing a Tariboy Git release with a patch, minor, or major version bump directly from main.
---

# Publish a Release

Publish one reviewed `main` commit as one immutable annotated version tag, then publish its documentation after the GitHub Release succeeds. Never force-push, move a tag, or publish from another branch.

## Authorization and preflight

The current user request must explicitly authorize both pushing `main` and creating and pushing the version tag. A request to choose or bump a version alone is not push authorization.

From the repository root:

1. Require branch `main` and a clean worktree.
2. Read `branch.main.remote` and `branch.main.merge`; require a real remote and `refs/heads/main`.
3. Fetch that remote. Require local `HEAD` to equal the fetched remote `main`; stop on divergence instead of merging, rebasing, resetting, or overwriting work.
4. Inspect every commit since the latest release tag. Honor an explicit version choice; otherwise use the highest-impact change: patch for fixes/refactoring/tooling/docs, minor for user-facing capability or an intentional incompatible change on `0.x`, and major for an intentional incompatible change on `1.x+`.
5. Require the resulting `vMAJOR.MINOR.PATCH` tag to be absent remotely. An existing local tag is reusable only when it peels to the same commit already verified on remote `main`.

## Bump and verify

Load Rust, then use the repository-owned version command:

```bash
. "$HOME/.cargo/env"
scripts/set-version.sh MAJOR.MINOR.PATCH
make desktop-version-check
make desktop-lock-check
go test ./scripts/...
git diff --check
git diff
```

Inspect the complete diff and require only the version files allowed by `scripts/set-version.sh`, `scripts/release-version.txt`, and `desktop/src-tauri/Cargo.lock` to change.

Do not run `make check` or `make full-check` for this version bump. The focused commands above are the release-version checks.

## Commit and publish

With the worktree still clean apart from the inspected bump:

1. Stage the bump and commit it as `release: MAJOR.MINOR.PATCH`.
2. Push `main` to its configured remote. Stop if the push fails; do not create a tag for an unpushed commit.
3. Verify remote `main` resolves to the release commit.
4. Create annotated tag `vMAJOR.MINOR.PATCH` with message `Tariboy MAJOR.MINOR.PATCH`, pointing at that exact commit.
5. Push only that tag.
6. Use `git ls-remote` to verify remote `main` and the peeled annotated tag both resolve to the release commit.

If any check or push fails, report the exact local commit, remote ref, and tag state. Preserve successful immutable state and resume only from the failed step; never use `--force`, delete a published tag, or silently select a different version.

## Wait for the release, then publish documentation

Pushing the tag starts publication; it does not complete it.

1. Find the `Desktop release` run (`.github/workflows/desktop-release.yml`) for the exact pushed tag and release commit in the configured GitHub repository. Record its run ID and URL, wait for completion, and require conclusion `success`. A missing, pending, cancelled, or failed run does not permit documentation publication.
2. Verify the GitHub Release for that exact tag exists, is not a draft, and has a publication timestamp. Record its URL. If publication failed or remains incomplete, report the run and release state and stop before `make docs`.
3. Require a clean source worktree still at the release commit, and verify `origin` identifies the same GitHub repository: `make docs` pushes to `origin/docs`, regardless of the configured `main` remote. If the source or remote changed, stop instead of publishing unrelated documentation or resetting user work.
4. From that release source root, run:

   ```bash
   make docs
   ```

   This uses `scripts/publish-docs.sh` to build the site and push the generated `docs` branch through a temporary worktree. Do not commit generated site files to `main`.
5. Require a successful exit and use `git ls-remote origin refs/heads/docs` to confirm the remote branch matches the local `docs` commit produced or reused by the publisher. Report the release tag, release commit, workflow URL, GitHub Release URL, docs commit, and site URL `https://alekzonder.github.io/tariboy/`. A branch push alone does not prove GitHub Pages has finished deploying; report deployment as pending unless separately confirmed.

If documentation build or push fails, preserve the published release and tag, report documentation as incomplete, and resume only the documentation step after resolving the failure. Do not bump the version again or rerun release publication to retry documentation. An unchanged documentation build is successful when the existing `docs` commit is verified remotely.
