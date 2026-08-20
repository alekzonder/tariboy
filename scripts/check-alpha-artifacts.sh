#!/usr/bin/env bash
# Verify a static Tariboy internal-alpha directory before manual publication.
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
RELEASE_DIR="${1:-}"
EXPECTED_DMG="Tariboy_${VERSION}_aarch64.dmg"
MOUNT=""
STRING_DUMP=""

cleanup() {
  if [ -n "$MOUNT" ]; then
    mounts="$(mount 2>/dev/null || true)"
    if grep -F " on $MOUNT " <<<"$mounts" >/dev/null; then
      hdiutil detach "$MOUNT" -quiet || true
    fi
  fi
  [ -z "$MOUNT" ] || [ ! -d "$MOUNT" ] || rmdir "$MOUNT" 2>/dev/null || true
  [ -z "$STRING_DUMP" ] || [ ! -f "$STRING_DUMP" ] || rm -f "$STRING_DUMP"
}
trap cleanup EXIT

binary_contains() {
  strings "$1" > "$STRING_DUMP"
  grep -F "$2" "$STRING_DUMP" >/dev/null
}

scan_old_version() {
  local status
  if rg -a -uuu -F -n "0.9.0-dev" "$@"; then
    return 0
  else
    status=$?
  fi
  [ "$status" -eq 1 ] || {
    echo "FAIL: stale-version scan could not read the release" >&2
    exit 1
  }
  return 1
}

scan_secrets() {
  local status
  if rg -a -uuu -n \
    -e '-----BEGIN (OPENSSH |RSA |EC |DSA )?PRIVATE KEY-----' \
    -e 'ghp_[A-Za-z0-9]{20,}' \
    -e 'github_pat_[A-Za-z0-9_]{20,}' \
    -e 'sk-[A-Za-z0-9_-]{20,}' \
    -e 'AKIA[0-9A-Z]{16}' \
    "$@"; then
    return 0
  else
    status=$?
  fi
  [ "$status" -eq 1 ] || {
    echo "FAIL: secret scan could not read the release" >&2
    exit 1
  }
  return 1
}

run_linux_version() {
  local path="$1"
  if [ -n "${TARIBOY_LINUX_AMD64_RUNNER:-}" ]; then
    [ -x "$TARIBOY_LINUX_AMD64_RUNNER" ] || {
      echo "FAIL: TARIBOY_LINUX_AMD64_RUNNER is not executable" >&2
      return 1
    }
    "$TARIBOY_LINUX_AMD64_RUNNER" "$path" --version
  elif command -v qemu-x86_64 >/dev/null 2>&1; then
    qemu-x86_64 "$path" --version
  elif command -v docker >/dev/null 2>&1 \
    && docker image inspect alpine:3.22 >/dev/null 2>&1; then
    docker run --rm --pull=never --platform linux/amd64 \
      --volume "$(dirname "$path"):/bundle:ro" \
      alpine:3.22 "/bundle/$(basename "$path")" --version
  else
    echo "FAIL: execute Linux x86_64 version checks with qemu-x86_64, a local" >&2
    echo "      alpine:3.22 Docker image, or TARIBOY_LINUX_AMD64_RUNNER" >&2
    return 1
  fi
}

[ -n "$RELEASE_DIR" ] && [ -d "$RELEASE_DIR" ] || {
  echo "usage: $0 <release-directory>" >&2
  exit 2
}
[ "$VERSION" = "$EXPECTED_VERSION" ] && [ "$NUMERIC_VERSION" = "$EXPECTED_NUMERIC_VERSION" ] || {
  echo "FAIL: checker contract does not match declared release version $EXPECTED_VERSION: $VERSION" >&2
  exit 1
}
[ "$(uname -s)" = "Darwin" ] && [ "$(uname -m)" = "arm64" ] || {
  echo "FAIL: DMG verification requires macOS Apple Silicon" >&2
  exit 1
}
for command in codesign file hdiutil lipo mount python3 rg shasum strings; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "FAIL: missing required command: $command" >&2
    exit 1
  }
done

ACTUAL_FILES="$(find "$RELEASE_DIR" -mindepth 1 -maxdepth 1 -exec basename {} \; | LC_ALL=C sort)"
EXPECTED_FILES="$(printf '%s\n' "$EXPECTED_DMG" SHA256SUMS release.json | LC_ALL=C sort)"
[ "$ACTUAL_FILES" = "$EXPECTED_FILES" ] || {
  echo "FAIL: unexpected release files; found:" >&2
  printf '%s\n' "$ACTUAL_FILES" >&2
  exit 1
}
for artifact in "$EXPECTED_DMG" SHA256SUMS release.json; do
  [ -f "$RELEASE_DIR/$artifact" ] && [ ! -L "$RELEASE_DIR/$artifact" ] || {
    echo "FAIL: release entry is not a regular file: $artifact" >&2
    exit 1
  }
done
(cd "$RELEASE_DIR" && shasum -a 256 -c SHA256SUMS)
codesign --verify --strict --verbose=2 "$RELEASE_DIR/$EXPECTED_DMG"
python3 - "$RELEASE_DIR/release.json" "$VERSION" "$NUMERIC_VERSION" "$EXPECTED_DMG" <<'PY'
import hashlib
import json
import pathlib
import sys

metadata_path, version, numeric, artifact = sys.argv[1:]
metadata = json.loads(pathlib.Path(metadata_path).read_text(encoding="utf-8"))
expected = {
    "schema_version": 1,
    "product": "Tariboy",
    "version": version,
    "bundle_version": numeric,
    "architecture": "aarch64",
    "signing": "ad-hoc",
    "artifact": artifact,
    "publication": "manual-reviewed-internal-https",
}
for key, value in expected.items():
    if metadata.get(key) != value:
        raise SystemExit(f"release metadata {key}={metadata.get(key)!r}, expected {value!r}")
release = pathlib.Path(metadata_path).parent
digest = hashlib.sha256((release / artifact).read_bytes()).hexdigest()
if metadata.get("artifact_sha256") != digest:
    raise SystemExit("release metadata artifact_sha256 does not match DMG")
commit = metadata.get("git_commit", "")
if len(commit) != 40 or any(ch not in "0123456789abcdef" for ch in commit):
    raise SystemExit("release metadata git_commit is not a full SHA-1")
PY

MOUNT="$(mktemp -d "${TMPDIR:-/tmp}/tariboy-alpha-mount.XXXXXX")"
STRING_DUMP="$(mktemp "${TMPDIR:-/tmp}/tariboy-alpha-strings.XXXXXX")"
hdiutil attach -readonly -nobrowse -mountpoint "$MOUNT" "$RELEASE_DIR/$EXPECTED_DMG" >/dev/null
APP="$MOUNT/Tariboy.app"
[ -d "$APP" ] && [ ! -L "$APP" ] || {
  echo "FAIL: DMG does not contain a real Tariboy.app directory" >&2
  exit 1
}
while IFS= read -r entry; do
  case "$(basename "$entry")" in
    Tariboy.app|Applications|.background|.VolumeIcon.icns|.DS_Store) ;;
    *) echo "FAIL: unexpected DMG root entry: $entry" >&2; exit 1 ;;
  esac
done < <(find "$MOUNT" -mindepth 1 -maxdepth 1 -print)
[ -L "$MOUNT/Applications" ] && [ "$(readlink "$MOUNT/Applications")" = "/Applications" ] || {
  echo "FAIL: DMG Applications entry is not the standard /Applications symlink" >&2
  exit 1
}
if [ -e "$MOUNT/.background" ] || [ -L "$MOUNT/.background" ]; then
  [ -d "$MOUNT/.background" ] && [ ! -L "$MOUNT/.background" ] || {
    echo "FAIL: DMG .background has the wrong type" >&2
    exit 1
  }
fi
for optional_file in .VolumeIcon.icns .DS_Store; do
  if [ -e "$MOUNT/$optional_file" ] || [ -L "$MOUNT/$optional_file" ]; then
    [ -f "$MOUNT/$optional_file" ] && [ ! -L "$MOUNT/$optional_file" ] || {
      echo "FAIL: DMG $optional_file has the wrong type" >&2
      exit 1
    }
  fi
done
if find "$APP/Contents/Resources" -type l -print -quit | grep . >/dev/null; then
  echo "FAIL: app resources contain a symlink" >&2
  exit 1
fi
codesign --verify --deep --strict --verbose=2 "$APP"

INFO="$APP/Contents/Info.plist"
PLIST_BUDDY="/usr/libexec/PlistBuddy"
[ "$("$PLIST_BUDDY" -c 'Print :CFBundleName' "$INFO")" = "Tariboy" ] || {
  echo "FAIL: CFBundleName is not Tariboy" >&2; exit 1; }
[ "$("$PLIST_BUDDY" -c 'Print :CFBundleShortVersionString' "$INFO")" = "$NUMERIC_VERSION" ] || {
  echo "FAIL: bundle version is not $NUMERIC_VERSION" >&2; exit 1; }
[ "$("$PLIST_BUDDY" -c 'Print :CFBundleVersion' "$INFO")" = "$NUMERIC_VERSION" ] || {
  echo "FAIL: bundle build version is not $NUMERIC_VERSION" >&2; exit 1; }
EXE_NAME="$("$PLIST_BUDDY" -c 'Print :CFBundleExecutable' "$INFO")"
APP_EXE="$APP/Contents/MacOS/$EXE_NAME"
[ "$(lipo -archs "$APP_EXE")" = "arm64" ] || {
  echo "FAIL: desktop executable is not arm64-only" >&2; exit 1; }
binary_contains "$APP_EXE" "$VERSION" || {
  echo "FAIL: desktop executable does not embed $VERSION" >&2; exit 1; }

for platform in darwin-arm64 linux-x86_64; do
  if [ "$platform" = "darwin-arm64" ]; then
    expected_format="Mach-O 64-bit.*arm64"
  else
    expected_format="ELF 64-bit.*x86-64"
  fi
  for binary in tariboyd tariboy tariboy-shim tariboy-tools; do
    path="$APP/Contents/Resources/bin/$platform/$binary"
    [ -x "$path" ] || { echo "FAIL: missing bundled binary: $path" >&2; exit 1; }
    description="$(file "$path")"
    grep "$expected_format" <<<"$description" >/dev/null || {
      echo "FAIL: wrong architecture or format: $path" >&2; exit 1; }
    binary_contains "$path" "$VERSION" || {
      echo "FAIL: $path does not embed $VERSION" >&2; exit 1; }
    if [ "$platform" = "darwin-arm64" ]; then
      reported="$("$path" --version)"
    else
      reported="$(run_linux_version "$path")"
    fi
    [ "$reported" = "$VERSION" ] || {
      echo "FAIL: $path reports $reported, expected $VERSION" >&2; exit 1; }
  done
  [ "$(tr -d '\r\n' < "$APP/Contents/Resources/bin/$platform/VERSION")" = "$VERSION" ] || {
    echo "FAIL: $platform/VERSION does not equal $VERSION" >&2; exit 1; }
done

credential_names="$(find "$MOUNT" \( -name '*.pem' -o -name '*.key' -o -name 'id_rsa*' \
  -o -name 'id_ed25519*' -o -name '.env' \) -print -quit)"
if [ -n "$credential_names" ]; then
  echo "FAIL: app contains a credential-like filename" >&2
  exit 1
fi
if scan_old_version "$MOUNT" "$RELEASE_DIR"; then
  echo "FAIL: release contains stale version 0.9.0-dev" >&2
  exit 1
fi
if scan_secrets "$MOUNT" "$RELEASE_DIR"; then
  echo "FAIL: release contains private-key or token material" >&2
  exit 1
fi

echo "PASS: verified $RELEASE_DIR"
