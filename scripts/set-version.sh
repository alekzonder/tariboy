#!/usr/bin/env bash
# Move every active version literal through the shared explicit allowlist.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ALLOWLIST="$ROOT/scripts/version-pinned-files.txt"
CANONICAL="$ROOT/internal/version/version.go"
LOCKFILE="desktop/src-tauri/Cargo.lock"
RELEASE_DECLARATION="scripts/release-version.txt"
TEMPORARY=""

cleanup() {
  [ -z "$TEMPORARY" ] || [ ! -e "$TEMPORARY" ] || rm -f "$TEMPORARY"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

[ "$#" -eq 1 ] || fail "usage: $0 NEW_VERSION"
NEW_VERSION="$1"
[[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || \
  fail "NEW_VERSION must be MAJOR.MINOR.PATCH with an optional prerelease suffix"

OLD_VERSION="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' "$CANONICAL")"
[ -n "$OLD_VERSION" ] || fail "cannot read canonical version from $CANONICAL"
[ "$NEW_VERSION" != "$OLD_VERSION" ] || fail "NEW_VERSION is already the canonical version: $OLD_VERSION"
[ -f "$ALLOWLIST" ] || fail "missing allowlist: $ALLOWLIST"
[ -f "$ROOT/$RELEASE_DECLARATION" ] || fail "missing release declaration: $RELEASE_DECLARATION"
command -v cargo >/dev/null 2>&1 || fail "cargo is required to regenerate desktop/src-tauri/Cargo.lock"

paths=()
declare -A allowed=()
while IFS= read -r line || [ -n "$line" ]; do
  path="${line#"${line%%[![:space:]]*}"}"
  path="${path%"${path##*[![:space:]]}"}"
  [ -z "$path" ] || [[ "$path" == \#* ]] && continue
  [[ "$path" != /* && "$path" != *".."* ]] || fail "allowlist path is not repository-relative: $path"
  [ -z "${allowed[$path]+x}" ] || fail "allowlist contains duplicate path: $path"
  [ -f "$ROOT/$path" ] || fail "allowlisted file is missing: $path"
  grep -F -q -- "$OLD_VERSION" "$ROOT/$path" || \
    fail "allowlisted file does not contain current version $OLD_VERSION: $path"
  paths+=("$path")
  allowed["$path"]=1
done < "$ALLOWLIST"
[ "${#paths[@]}" -gt 0 ] || fail "allowlist contains no paths"

declare -A modified_before=()
while IFS= read -r path; do
  [ -n "$path" ] || continue
  modified_before["$path"]=1
done < <({ git -C "$ROOT" diff --name-only; git -C "$ROOT" diff --cached --name-only; } | sort -u)

rewrite_file() {
  local path="$1"
  TEMPORARY="$(mktemp "$ROOT/.set-version.XXXXXX")"
  sed "s/${OLD_VERSION//./\\.}/${NEW_VERSION//&/\\&}/g" "$ROOT/$path" > "$TEMPORARY"
  mv "$TEMPORARY" "$ROOT/$path"
  TEMPORARY=""
}

for path in "${paths[@]}"; do
  count="$(grep -oF -- "$OLD_VERSION" "$ROOT/$path" | wc -l | tr -d '[:space:]' || true)"
  echo "INFO: replacing $count occurrence(s) in $path"
  rewrite_file "$path"
done

printf '%s\n' "$NEW_VERSION" > "$ROOT/$RELEASE_DECLARATION"

for path in "${paths[@]}"; do
  grep -F -q -- "$OLD_VERSION" "$ROOT/$path" && fail "old version remains in allowlisted file: $path"
  grep -F -q -- "$NEW_VERSION" "$ROOT/$path" || fail "new version is absent from allowlisted file: $path"
done

release_version="$(tr -d '\r\n' < "$ROOT/$RELEASE_DECLARATION")"
[ "$release_version" = "$NEW_VERSION" ] || \
  fail "release declaration does not match new version: $RELEASE_DECLARATION"

while IFS= read -r path; do
  [ -n "$path" ] || continue
  [ -n "${modified_before[$path]+x}" ] && continue
  [ -n "${allowed[$path]+x}" ] || [ "$path" = "$LOCKFILE" ] || [ "$path" = "$RELEASE_DECLARATION" ] || \
    fail "rewrite modified file outside allowlist: $path"
done < <({ git -C "$ROOT" diff --name-only; git -C "$ROOT" diff --cached --name-only; } | sort -u)

(cd "$ROOT/desktop/src-tauri" && cargo update --offline -p tariboy-desktop)

while IFS= read -r path; do
  [ -n "$path" ] || continue
  [ -n "${modified_before[$path]+x}" ] && continue
  [ -n "${allowed[$path]+x}" ] || [ "$path" = "$LOCKFILE" ] || [ "$path" = "$RELEASE_DECLARATION" ] || \
    fail "rewrite modified unexpected file: $path"
done < <({ git -C "$ROOT" diff --name-only; git -C "$ROOT" diff --cached --name-only; } | sort -u)

echo "PASS: moved canonical version $OLD_VERSION -> $NEW_VERSION"
echo "NEXT: run make desktop-version-check and make desktop-lock-check."
