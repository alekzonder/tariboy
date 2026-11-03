#!/usr/bin/env bash
# Contract of scripts/docs-build-cached.sh: a change to any input of the
# documentation site rebuilds it, no change restores docs/dist from the cache,
# and TARIBOY_DOCS_REBUILD=1 always rebuilds. Fake npm and node commands stand
# in for Blume and record each doctor and build call.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT
repo="$work/repo"
docs="$repo/docs"
cache="$work/cache"
calls="$work/calls"
out="$work/out"

mkdir -p "$repo/scripts" "$work/bin" "$docs/docs" "$docs/pages" "$docs/public" "$docs/superpowers/specs"
cp "$repo_root/scripts/docs-build-cached.sh" "$repo/scripts/"
printf '# Home\n' >"$docs/docs/index.mdx"
printf '# Old\n' >"$docs/docs/old.mdx"
printf 'export default {};\n' >"$docs/docs/meta.ts"
printf 'page\n' >"$docs/pages/extra.astro"
printf 'logo\n' >"$docs/public/logo.svg"
printf '# Spec\n' >"$docs/superpowers/specs/one.md"
printf 'export default {};\n' >"$docs/blume.config.ts"
printf '{"name":"docs"}\n' >"$docs/package.json"
printf '{"lockfileVersion":3}\n' >"$docs/package-lock.json"
printf '.blume/\ndist/\n' >"$docs/.gitignore"

cat >"$work/bin/npm" <<'EOF'
#!/bin/sh
if [ "$*" = --version ]; then
  printf '%s\n' "${TEST_NPM_VERSION:-10.0.0}"
  exit 0
fi
printf '%s\n' "$*" >>"$TEST_CALLS"
case "$*" in
  'run doctor') [ "${TEST_DOCTOR_FAIL:-0}" != 1 ] || exit 3 ;;
  'run build')
    [ "${TEST_BUILD_FAIL:-0}" != 1 ] || exit 4
    rm -rf dist
    mkdir -p dist/assets
    n=$(grep -c 'run build' "$TEST_CALLS")
    printf 'build %s\n' "$n" >dist/index.html
    printf 'asset\n' >dist/assets/app.js
    ;;
esac
EOF
cat >"$work/bin/node" <<'EOF'
#!/bin/sh
[ "$*" = --version ] || exit 1
printf '%s\n' "${TEST_NODE_VERSION:-v22.0.0}"
EOF
chmod +x "$work/bin/npm" "$work/bin/node"
: >"$calls"

fail() {
  echo "FAIL: $1" >&2
  cat "$out" >&2 || true
  exit 1
}

# run VAR=VALUE... runs the script once with a fresh call log; $code, $out and
# $log hold the exit code, the combined output and the npm calls of that run.
run() {
  : >"$work/run-calls"
  code=0
  env -u TARIBOY_DOCS_REBUILD PATH="$work/bin:$PATH" TEST_CALLS="$work/run-calls" \
    TARIBOY_DOCS_CACHE_DIR="$cache" "$@" bash "$repo/scripts/docs-build-cached.sh" >"$out" 2>&1 || code=$?
  log=$(cat "$work/run-calls")
  cat "$work/run-calls" >>"$calls"
}

entries() {
  find "$cache" -mindepth 1 -maxdepth 1 -type d ! -name '.*' | wc -l
}

expect_build() {
  [ "$code" -eq 0 ] || fail "$1: exited $code"
  [ "$log" = "$(printf 'run doctor\nrun build')" ] || fail "$1: expected doctor and build, got: $log"
  [ -f "$docs/dist/index.html" ] || fail "$1: no docs/dist/index.html"
}

expect_hit() {
  [ "$code" -eq 0 ] || fail "$1: exited $code"
  [ -z "$log" ] || fail "$1: expected a cache hit, npm ran: $log"
  grep -Eq '^docs: cache hit [0-9a-f]{12}, doctor and build skipped$' "$out" || fail "$1: no cache hit line"
  [ -f "$docs/dist/index.html" ] || fail "$1: dist not restored"
  [ -f "$docs/dist/assets/app.js" ] || fail "$1: dist assets not restored"
}

# expect_rebuild NAME: the run after a change builds, the next run hits.
expect_rebuild() {
  run "${@:2}"
  expect_build "$1"
  run "${@:2}"
  expect_hit "$1 (second run)"
}

# First run: doctor, build, one cache entry.
run
expect_build "first run"
[ "$(entries)" -eq 1 ] || fail "first run left $(entries) cache entries"
first=$(cat "$docs/dist/index.html")

# No change: the cache restores the same output, also after dist is removed.
rm -rf "$docs/dist"
run
expect_hit "unchanged inputs"
[ "$(cat "$docs/dist/index.html")" = "$first" ] || fail "restored dist differs from the built one"
printf 'stale\n' >"$docs/dist/index.html"
touch "$docs/dist/stale-file"
run
expect_hit "unchanged inputs over a stale dist"
[ "$(cat "$docs/dist/index.html")" = "$first" ] || fail "stale dist was not replaced"
[ ! -e "$docs/dist/stale-file" ] || fail "stale dist file was kept"

# Changes outside the site inputs do not rebuild.
mkdir -p "$docs/node_modules/blume" "$docs/.blume"
printf 'x\n' >"$docs/node_modules/blume/index.js"
printf 'x\n' >"$docs/.blume/state"
printf 'x\n' >"$docs/dist/extra.html"
run
expect_hit "node_modules, .blume and dist changes"

# Each change of a site input rebuilds.
printf 'more\n' >>"$docs/docs/index.mdx"
expect_rebuild "edited MDX"
printf '# New\n' >"$docs/docs/new.mdx"
expect_rebuild "new file"
rm "$docs/docs/old.mdx"
expect_rebuild "removed file"
mv "$docs/docs/meta.ts" "$docs/docs/meta2.ts"
expect_rebuild "renamed file"
printf 'more\n' >>"$docs/pages/extra.astro"
expect_rebuild "pages file"
printf 'more\n' >>"$docs/public/logo.svg"
expect_rebuild "public file"
printf '// more\n' >>"$docs/blume.config.ts"
expect_rebuild "blume.config.ts"
printf '{"name":"docs2"}\n' >"$docs/package.json"
expect_rebuild "package.json"
printf '{"lockfileVersion":3,"x":1}\n' >"$docs/package-lock.json"
expect_rebuild "package-lock.json"
printf 'more\n' >>"$docs/superpowers/specs/one.md"
expect_rebuild "superpowers file"
expect_rebuild "Node.js version" TEST_NODE_VERSION=v24.0.0
expect_rebuild "npm version" TEST_NPM_VERSION=11.0.0

# A forced rebuild runs doctor and build although the entry exists.
run TARIBOY_DOCS_REBUILD=1
expect_build "TARIBOY_DOCS_REBUILD=1"
run
expect_hit "after a forced rebuild"

# A failed doctor skips build, fails, and stores nothing.
printf 'doctor\n' >>"$docs/docs/index.mdx"
before=$(entries)
run TEST_DOCTOR_FAIL=1
[ "$code" -eq 3 ] || fail "failed doctor exited $code"
[ "$log" = 'run doctor' ] || fail "failed doctor: npm calls were: $log"
[ "$(entries)" -eq "$before" ] || fail "failed doctor stored an entry"

# A failed build fails and stores nothing; the next run builds again.
run TEST_BUILD_FAIL=1
[ "$code" -eq 4 ] || fail "failed build exited $code"
[ "$(entries)" -eq "$before" ] || fail "failed build stored an entry"
run
expect_build "after a failed build"

# An entry without index.html is a miss.
key_dir=$(ls -1dt "$cache"/*/ | head -1)
rm -f "$key_dir/dist/index.html"
run
expect_build "entry without index.html"

# lastModified reads git history outside docs/: no cache.
printf 'export default { lastModified: true };\n' >"$docs/blume.config.ts"
before=$(entries)
run
expect_build "lastModified first run"
grep -Fq 'docs: lastModified is set, cache disabled' "$out" || fail "no lastModified line"
run
expect_build "lastModified second run"
[ "$(entries)" -eq "$before" ] || fail "lastModified stored an entry"
printf 'export default {};\n' >"$docs/blume.config.ts"

# A cache directory that cannot be created: build passes with a warning.
touch "$work/not-a-dir"
: >"$work/run-calls"
code=0
env -u TARIBOY_DOCS_REBUILD PATH="$work/bin:$PATH" TEST_CALLS="$work/run-calls" \
  TARIBOY_DOCS_CACHE_DIR="$work/not-a-dir/cache" bash "$repo/scripts/docs-build-cached.sh" >"$out" 2>&1 || code=$?
log=$(cat "$work/run-calls")
expect_build "unwritable cache"
grep -Fq 'docs: cannot write the cache' "$out" || fail "no unwritable cache warning"

# Only the 8 newest entries stay.
rm -rf "$cache"
for i in 1 2 3 4 5 6 7 8 9 10; do
  run TEST_NODE_VERSION="v$i.0.0"
  expect_build "entry $i"
done
[ "$(entries)" -eq 8 ] || fail "cache kept $(entries) entries, expected 8"
run TEST_NODE_VERSION=v10.0.0
expect_hit "newest entry"
run TEST_NODE_VERSION=v1.0.0
expect_build "oldest entry was removed"

echo "docs build cache contract ok"
