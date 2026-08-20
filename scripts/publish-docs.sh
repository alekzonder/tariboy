#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
script_root="$(cd "$(dirname "$0")" && pwd -P)"
docs_root="$repo_root/docs"
build_root="$docs_root/dist"
source_commit="$(git -C "$repo_root" rev-parse --short HEAD)"
publisher_name="$(git -C "$repo_root" config user.name || true)"
publisher_email="$(git -C "$repo_root" config user.email || true)"

if [ -z "$publisher_name" ]; then
  publisher_name="$(git -C "$repo_root" show -s --format=%an HEAD)"
fi
if [ -z "$publisher_email" ]; then
  publisher_email="$(git -C "$repo_root" show -s --format=%ae HEAD)"
fi

remote_docs=0
if git -C "$repo_root" ls-remote --exit-code --heads origin refs/heads/docs >/dev/null; then
  remote_docs=1
else
  remote_status=$?
  if [ "$remote_status" -ne 2 ]; then
    echo "cannot read origin/docs; verify the origin remote and Git credentials" >&2
    exit "$remote_status"
  fi
fi

if [ "$remote_docs" -eq 1 ]; then
  git -C "$repo_root" fetch --no-tags origin \
    +refs/heads/docs:refs/remotes/origin/docs
fi

if [ ! -d "$docs_root/node_modules" ]; then
  echo "docs/node_modules is missing; installing the locked dependencies"
  (cd "$docs_root" && npm ci)
fi

(cd "$docs_root" && npm run doctor && npm run build)

if [ ! -f "$build_root/index.html" ]; then
  echo "documentation build did not produce docs/dist/index.html" >&2
  exit 1
fi
"$script_root/docs-build-contract-test.sh" "$build_root"

publish_root="$(mktemp -d)"
publish_worktree="$publish_root/worktree"
worktree_added=0

cleanup() {
  if [ "$worktree_added" -eq 1 ]; then
    git -C "$repo_root" worktree remove --force "$publish_worktree" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$publish_root"
}
trap cleanup EXIT

if [ "$remote_docs" -eq 1 ]; then
  git -C "$repo_root" worktree add -q -B docs "$publish_worktree" origin/docs
elif git -C "$repo_root" show-ref --verify --quiet refs/heads/docs; then
  git -C "$repo_root" worktree add -q "$publish_worktree" docs
else
  git -C "$repo_root" worktree add -q --detach "$publish_worktree" HEAD
  worktree_added=1
  git -C "$publish_worktree" checkout -q --orphan docs
fi
worktree_added=1

git -C "$publish_worktree" rm -rf --ignore-unmatch . >/dev/null
git -C "$publish_worktree" clean -dfx >/dev/null
cp -a "$build_root/." "$publish_worktree/"
touch "$publish_worktree/.nojekyll"
git -C "$publish_worktree" add -A

if git -C "$publish_worktree" diff --cached --quiet; then
  echo "documentation output is unchanged"
else
  git -C "$publish_worktree" \
    -c user.name="$publisher_name" \
    -c user.email="$publisher_email" \
    commit -m "docs: publish $source_commit"
fi

git -C "$publish_worktree" push origin docs
