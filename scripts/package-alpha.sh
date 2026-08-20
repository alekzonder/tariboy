#!/usr/bin/env bash
# Build and stage the ad-hoc-signed Tariboy internal alpha.
set -euo pipefail
trap 'echo "FAIL: command failed at ${BASH_SOURCE[0]}:${LINENO}" >&2' ERR

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' "$ROOT/internal/version/version.go")"
NUMERIC_VERSION="${VERSION%%-*}"
RELEASE_VERSION_FILE="$ROOT/scripts/release-version.txt"
[ -s "$RELEASE_VERSION_FILE" ] || {
  echo "FAIL: declared release version is missing or empty: $RELEASE_VERSION_FILE" >&2
  exit 1
}
EXPECTED_VERSION="$(cat "$RELEASE_VERSION_FILE")"
EXPECTED_NUMERIC_VERSION="${EXPECTED_VERSION%%-*}"
APP="$ROOT/desktop/src-tauri/target/release/bundle/macos/Tariboy.app"
BUILT_DMG="$ROOT/desktop/src-tauri/target/release/bundle/dmg/Tariboy_${NUMERIC_VERSION}_aarch64.dmg"
RELEASE_ROOT="$ROOT/dist/releases"
RELEASE_DIR="$RELEASE_ROOT/$VERSION"
STAGE=""
BACKUP=""

cleanup() {
  [ -z "$STAGE" ] || [ ! -e "$STAGE" ] || rm -rf "$STAGE"
  if [ -n "$BACKUP" ] && [ -e "$BACKUP" ] && [ ! -e "$RELEASE_DIR" ]; then
    mv "$BACKUP" "$RELEASE_DIR"
  fi
}
trap cleanup EXIT

[ "$VERSION" = "$EXPECTED_VERSION" ] || {
  echo "FAIL: release script is pinned to $EXPECTED_VERSION, found $VERSION" >&2
  exit 1
}
[ "$NUMERIC_VERSION" = "$EXPECTED_NUMERIC_VERSION" ] || {
  echo "FAIL: expected numeric metadata $EXPECTED_NUMERIC_VERSION, found $NUMERIC_VERSION" >&2
  exit 1
}
[ "$(uname -s)" = "Darwin" ] && [ "$(uname -m)" = "arm64" ] || {
  echo "FAIL: alpha DMG packaging requires macOS Apple Silicon" >&2
  exit 1
}
for command in codesign git hdiutil make python3 shasum tmux; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "FAIL: missing required command: $command" >&2
    exit 1
  }
done
if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=normal)" ]; then
  echo "FAIL: release packaging requires a clean worktree" >&2
  exit 1
fi

make -C "$ROOT" desktop-version-check
make -C "$ROOT" desktop

[ -d "$APP" ] || { echo "FAIL: missing app bundle: $APP" >&2; exit 1; }
[ -f "$BUILT_DMG" ] || { echo "FAIL: missing DMG: $BUILT_DMG" >&2; exit 1; }
codesign --verify --deep --strict --verbose=2 "$APP"
codesign --force --sign - "$BUILT_DMG"
codesign --verify --strict --verbose=2 "$BUILT_DMG"
TARIBOY_DESKTOP_APP="$APP" "$ROOT/scripts/desktop-smoke.sh"

mkdir -p "$RELEASE_ROOT"
STAGE="$(mktemp -d "$RELEASE_ROOT/.${VERSION}.stage.XXXXXX")"
ARTIFACT="Tariboy_${VERSION}_aarch64.dmg"
install -m 0644 "$BUILT_DMG" "$STAGE/$ARTIFACT"
ARTIFACT_SHA="$(shasum -a 256 "$STAGE/$ARTIFACT" | awk '{print $1}')"
COMMIT="$(git -C "$ROOT" rev-parse HEAD)"
BUILT_AT="$(python3 - <<'PY'
import datetime
import os
epoch = os.environ.get("SOURCE_DATE_EPOCH")
moment = (
    datetime.datetime.fromtimestamp(int(epoch), datetime.timezone.utc)
    if epoch
    else datetime.datetime.now(datetime.timezone.utc)
)
print(moment.replace(microsecond=0).isoformat().replace("+00:00", "Z"))
PY
)"
python3 - "$STAGE/release.json" "$VERSION" "$NUMERIC_VERSION" "$ARTIFACT" \
  "$ARTIFACT_SHA" "$COMMIT" "$BUILT_AT" <<'PY'
import json
import sys

path, version, numeric, artifact, digest, commit, built_at = sys.argv[1:]
document = {
    "schema_version": 1,
    "product": "Tariboy",
    "version": version,
    "bundle_version": numeric,
    "architecture": "aarch64",
    "signing": "ad-hoc",
    "artifact": artifact,
    "artifact_sha256": digest,
    "git_commit": commit,
    "built_at": built_at,
    "publication": "manual-reviewed-internal-https",
}
with open(path, "w", encoding="utf-8", newline="\n") as output:
    json.dump(document, output, indent=2, sort_keys=True)
    output.write("\n")
PY
(cd "$STAGE" && shasum -a 256 "$ARTIFACT" release.json > SHA256SUMS)
"$ROOT/scripts/check-alpha-artifacts.sh" "$STAGE"

if [ -e "$RELEASE_DIR" ]; then
  BACKUP="$RELEASE_ROOT/.${VERSION}.previous.$$"
  mv "$RELEASE_DIR" "$BACKUP"
fi
mv "$STAGE" "$RELEASE_DIR"
STAGE=""
if [ -n "$BACKUP" ] && [ -e "$BACKUP" ]; then
  rm -rf "$BACKUP"
  BACKUP=""
fi
echo "PASS: alpha release staged at $RELEASE_DIR"
