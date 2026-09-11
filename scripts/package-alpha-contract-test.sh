#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCAN_TEST_DIR="$(mktemp -d)"
SCAN_BIN="$SCAN_TEST_DIR/bin"

cleanup() {
  rm -f "$SCAN_TEST_DIR/plain/metadata" "$SCAN_TEST_DIR/binary/payload" "$SCAN_BIN/grep"
  rmdir "$SCAN_TEST_DIR/plain" "$SCAN_TEST_DIR/binary" "$SCAN_BIN" "$SCAN_TEST_DIR" 2>/dev/null || true
}
trap cleanup EXIT

if grep -Fq 'desktop-smoke.sh' "$ROOT/scripts/package-alpha.sh"; then
  echo "FAIL: alpha packaging must not run desktop-smoke.sh" >&2
  exit 1
fi

if grep -Eq 'qemu|TARIBOY_LINUX_AMD64_RUNNER|run_linux_version' \
  "$ROOT/.github/workflows/desktop-release.yml" "$ROOT/scripts/check-alpha-artifacts.sh"; then
  echo "FAIL: macOS release verification must not require Linux execution" >&2
  exit 1
fi

eval "$(awk '
  /^scan_secrets\(\) \{$/ { capture = 1 }
  capture {
    print
    if ($0 ~ /\{$/) depth++
    if ($0 ~ /^[[:space:]]*}[[:space:]]*$/) depth--
    if (depth == 0) exit
  }
' "$ROOT/scripts/check-alpha-artifacts.sh")"

mkdir "$SCAN_TEST_DIR/plain" "$SCAN_TEST_DIR/binary" "$SCAN_BIN"
ln -s "$(command -v grep)" "$SCAN_BIN/grep"
printf 'ghp_12345678901234567890\n' > "$SCAN_TEST_DIR/plain/metadata"
printf '\0sk-tariboy-bad_requestbad_archivebad_composeTyped\0' > "$SCAN_TEST_DIR/binary/payload"

(PATH="$SCAN_BIN"; scan_secrets "$SCAN_TEST_DIR/plain") >/dev/null || {
  echo "FAIL: secret scan must reject plaintext credentials" >&2
  exit 1
}
if (PATH="$SCAN_BIN"; scan_secrets "$SCAN_TEST_DIR/binary"); then
  echo "FAIL: secret scan must ignore compiled binary lookalikes" >&2
  exit 1
fi

echo "OK: alpha packaging does not run the desktop smoke suite"
