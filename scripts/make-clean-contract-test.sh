#!/usr/bin/env bash
set -eu

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
fixture_root="$(mktemp -d)"
trap 'rm -rf -- "$fixture_root"' EXIT

fixture_repo="$fixture_root/repo"
mkdir -p "$fixture_repo"
git -C "$fixture_repo" init -q

mkdir -p "$fixture_repo/internal/version"
printf 'package version\n\nconst Version = "test"\n' >"$fixture_repo/internal/version/version.go"
printf '/ignored-file\n/ignored-dir/\n' >"$fixture_repo/.gitignore"
printf 'tracked original\n' >"$fixture_repo/tracked-file"
git -C "$fixture_repo" add .gitignore internal/version/version.go tracked-file
printf 'tracked edit\n' >"$fixture_repo/tracked-file"

printf 'untracked\n' >"$fixture_repo/untracked-file"
mkdir -p "$fixture_repo/untracked-dir" "$fixture_repo/ignored-dir"
printf 'untracked nested\n' >"$fixture_repo/untracked-dir/file"
printf 'ignored\n' >"$fixture_repo/ignored-file"
printf 'ignored nested\n' >"$fixture_repo/ignored-dir/file"

mkdir -p "$fixture_repo/nested-repository"
git -C "$fixture_repo/nested-repository" init -q
printf 'nested repository content\n' >"$fixture_repo/nested-repository/content"

worktree_source="$fixture_root/worktree-source"
git -C "$fixture_root" init -q worktree-source
git -C "$worktree_source" config user.email contract-test@example.invalid
git -C "$worktree_source" config user.name "Contract Test"
printf 'linked worktree content\n' >"$worktree_source/content"
git -C "$worktree_source" add content
git -C "$worktree_source" commit -qm base
git -C "$worktree_source" worktree add -q "$fixture_repo/nested-worktree"

make --no-print-directory -C "$fixture_repo" -f "$repo_root/Makefile" clean

for path in untracked-file untracked-dir ignored-file ignored-dir; do
  if [ -e "$fixture_repo/$path" ]; then
    echo "FAIL: make clean left $path behind" >&2
    exit 1
  fi
done

if [ "$(cat "$fixture_repo/tracked-file")" != "tracked edit" ]; then
  echo "FAIL: make clean changed tracked content" >&2
  exit 1
fi

if [ ! -e "$fixture_repo/nested-repository/.git" ] || [ ! -e "$fixture_repo/nested-repository/content" ]; then
  echo "FAIL: make clean removed nested repository content" >&2
  exit 1
fi

if [ ! -f "$fixture_repo/nested-worktree/.git" ] || [ ! -e "$fixture_repo/nested-worktree/content" ]; then
  echo "FAIL: make clean removed nested worktree content" >&2
  exit 1
fi

echo "make clean contract ok"
