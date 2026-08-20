#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
publisher="$repo_root/scripts/publish-docs.sh"
fixture_root="$(mktemp -d)"
trap 'rm -rf -- "$fixture_root"' EXIT

source_repo="$fixture_root/source"
remote_repo="$fixture_root/remote.git"
fake_bin="$fixture_root/bin"
real_git="$(command -v git)"

git init -q --bare "$remote_repo"
git init -q -b main "$source_repo"
git -C "$source_repo" config user.email contract-test@example.invalid
git -C "$source_repo" config user.name "Contract Test"
git -C "$source_repo" remote add origin "$remote_repo"

mkdir -p "$source_repo/docs/node_modules" "$source_repo/docs/publish-fixture/assets" "$fake_bin"
printf 'source content\n' >"$source_repo/README.md"
printf '%s\n' \
  '<link rel="canonical" href="https://alekzonder.github.io/tariboy">' \
  '<link href="/tariboy/_astro/app.css">' \
  'first build' >"$source_repo/docs/publish-fixture/index.html"
printf 'asset\n' >"$source_repo/docs/publish-fixture/assets/app.js"
printf '<url><loc>https://alekzonder.github.io/tariboy/architecture</loc></url>\n' \
  >"$source_repo/docs/publish-fixture/sitemap.xml"
git -C "$source_repo" add README.md
git -C "$source_repo" commit -qm base
git -C "$source_repo" config --unset user.email
git -C "$source_repo" config --unset user.name

cat >"$fake_bin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "$*" in
  "run doctor") ;;
  "run build")
    rm -rf dist
    mkdir -p dist
    cp -a publish-fixture/. dist/
    ;;
  *)
    echo "unexpected npm invocation: $*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$fake_bin/npm"

cat >"$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ " $* " == *" worktree add "*" --orphan "* ]]; then
  echo "git worktree add --orphan is unavailable in this compatibility fixture" >&2
  exit 129
fi
exec "$REAL_GIT" "$@"
EOF
chmod +x "$fake_bin/git"

run_publisher() {
  (
    cd "$source_repo"
    GIT_CONFIG_NOSYSTEM=1 \
      GIT_CONFIG_GLOBAL=/dev/null \
      REAL_GIT="$real_git" \
      PATH="$fake_bin:$PATH" \
      "$publisher"
  )
}

run_publisher

if [ "$(git -C "$source_repo" branch --show-current)" != "main" ]; then
  echo "FAIL: publishing switched the source worktree away from main" >&2
  exit 1
fi

published_files="$(git --git-dir="$remote_repo" ls-tree -r --name-only docs | LC_ALL=C sort)"
expected_files=$'.nojekyll\nassets/app.js\nindex.html\nsitemap.xml'
if [ "$published_files" != "$expected_files" ]; then
  echo "FAIL: docs branch contains the wrong files" >&2
  printf 'expected:\n%s\nactual:\n%s\n' "$expected_files" "$published_files" >&2
  exit 1
fi

published_index="$(git --git-dir="$remote_repo" show docs:index.html)"
if [[ "$published_index" != *"first build"* ]]; then
  echo "FAIL: docs branch does not contain the first build" >&2
  exit 1
fi

published_author="$(git --git-dir="$remote_repo" show -s --format='%an <%ae>' docs)"
if [ "$published_author" != "Contract Test <contract-test@example.invalid>" ]; then
  echo "FAIL: docs commit did not fall back to the source commit identity" >&2
  exit 1
fi

# A fresh or single-branch clone may know nothing about an existing remote
# publication branch. The publisher must discover and build on that remote ref
# instead of creating an unrelated orphan history.
git -C "$source_repo" branch -D docs >/dev/null
git -C "$source_repo" update-ref -d refs/remotes/origin/docs

printf '%s\n' \
  '<link rel="canonical" href="https://alekzonder.github.io/tariboy">' \
  '<link href="/tariboy/_astro/app.css">' \
  'second build' >"$source_repo/docs/publish-fixture/index.html"
run_publisher

published_index="$(git --git-dir="$remote_repo" show docs:index.html)"
if [[ "$published_index" != *"second build"* ]]; then
  echo "FAIL: docs branch was not updated by the second build" >&2
  exit 1
fi

commit_count_before="$(git --git-dir="$remote_repo" rev-list --count docs)"
run_publisher
commit_count_after="$(git --git-dir="$remote_repo" rev-list --count docs)"
if [ "$commit_count_after" != "$commit_count_before" ]; then
  echo "FAIL: unchanged output created another docs commit" >&2
  exit 1
fi

# Another publisher can advance origin/docs while the local generated branch
# is stale. The next run must base its replacement commit on that remote tip.
external_repo="$fixture_root/external"
git clone -q --branch docs "$remote_repo" "$external_repo"
git -C "$external_repo" config user.email external@example.invalid
git -C "$external_repo" config user.name "External Publisher"
printf 'external drift\n' >>"$external_repo/index.html"
git -C "$external_repo" add index.html
git -C "$external_repo" commit -qm "external docs update"
git -C "$external_repo" push -q origin docs
external_tip="$(git -C "$external_repo" rev-parse HEAD)"

run_publisher

published_parent="$(git --git-dir="$remote_repo" rev-parse docs^)"
if [ "$published_parent" != "$external_tip" ]; then
  echo "FAIL: docs publication was not based on the latest remote tip" >&2
  exit 1
fi
if git --git-dir="$remote_repo" show docs:index.html | grep -Fq 'external drift'; then
  echo "FAIL: docs publication did not replace stale remote output" >&2
  exit 1
fi

echo "docs publishing contract ok"
