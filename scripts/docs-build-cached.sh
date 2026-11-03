#!/usr/bin/env bash
# Runs `npm run doctor` and `npm run build` for the documentation site, or
# restores docs/dist from a host-wide cache when no input of the site changed.
# The key covers every file under docs/ except node_modules, dist and .blume,
# the Node.js and npm versions, and this script. TARIBOY_DOCS_REBUILD=1 always
# rebuilds. See "Documentation build cache" in docs/docs/development.mdx.
# Usage: scripts/docs-build-cached.sh [DOCS_DIR]
set -euo pipefail

self="$(cd "$(dirname "$0")" && pwd -P)/$(basename "$0")"
repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
docs="$(cd "${1:-$repo_root/docs}" && pwd -P)"
cache="${TARIBOY_DOCS_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/tariboy/docs-build}"
keep=8
restore_tmp="$docs/.dist-restore.$$"
store_tmp="$cache/.tmp.$$"
trap 'rm -rf -- "$restore_tmp" "$store_tmp"' EXIT

build() {
  (cd "$docs" && npm run doctor && npm run build)
}

# lastModified makes Blume read git history, which is outside the key.
if grep -q lastModified "$docs/blume.config.ts" 2>/dev/null; then
  echo "docs: lastModified is set, cache disabled"
  build
  exit 0
fi

key="$(
  {
    echo "tariboy-docs-build v1"
    sha256sum <"$self"
    node --version
    npm --version
    cd "$docs"
    find . \( -path ./node_modules -o -path ./dist -o -path ./.blume -o -name '.dist-restore.*' \) -prune \
      -o \( -type f -o -type l \) -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum --
  } | sha256sum | cut -c1-64
)"
entry="$cache/$key"

if [ "${TARIBOY_DOCS_REBUILD:-0}" != 1 ] && [ -f "$entry/dist/index.html" ]; then
  cp -R -- "$entry/dist" "$restore_tmp"
  rm -rf -- "$docs/dist"
  mv -- "$restore_tmp" "$docs/dist"
  touch -- "$entry"
  echo "docs: cache hit ${key:0:12}, doctor and build skipped"
  exit 0
fi

build

store() {
  mkdir -p -- "$cache" && rm -rf -- "$store_tmp" && mkdir -- "$store_tmp" &&
    cp -R -- "$docs/dist" "$store_tmp/dist" || return 1
  rm -rf -- "$entry"
  # Another check may store the same key first; its entry is as good as ours.
  [ -e "$entry" ] || mv -- "$store_tmp" "$entry"
}
if ! store 2>/dev/null; then
  echo "docs: cannot write the cache $cache, the build is not stored" >&2
  exit 0
fi
ls -1dt -- "$cache"/*/ | tail -n +$((keep + 1)) | while IFS= read -r old; do rm -rf -- "$old"; done
find "$cache" -mindepth 1 -maxdepth 1 -name '.tmp.*' -mtime +0 -exec rm -rf -- {} +
