#!/usr/bin/env bash
# Describe the macOS disk image state that Tauri's bundle_dmg.sh depends on.
#
# The bundler reports a DMG failure as a bare "failed to run bundle_dmg.sh"
# without that script's own output, so a failed release build has to describe
# its environment here: a leftover attached image, an exhausted disk, a stuck
# diskimages-helper, or an earlier partial bundle.
#
# This reporter runs after a failure and never fails: a missing tool is
# reported, not fatal, so it cannot mask the failure that called it.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUNDLE_DIR="${1:-$ROOT/desktop/src-tauri/target/release/bundle}"

section() {
  printf '\n--- desktop bundle diagnostics: %s ---\n' "$1"
}

report() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'unavailable: %s is not installed on this host\n' "$1"
    return 0
  fi
  "$@" 2>&1 || printf 'command failed: %s (status %d)\n' "$1" "$?"
  return 0
}

printf '=== desktop bundle diagnostics: %s ===\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

section "host"
report uname -a
report sw_vers

section "disk space"
report df -h "$ROOT" /tmp

section "attached disk images"
report hdiutil info

section "mounted volumes"
if [ -d /Volumes ]; then
  report ls -la /Volumes
else
  printf 'unavailable: /Volumes does not exist on this host\n'
fi

section "disk image processes"
# A bundling run that is retried after a stuck attach leaves one of these
# behind; anything listed here still holds the image the bundler needs.
if ! command -v ps >/dev/null 2>&1; then
  printf 'unavailable: ps is not installed on this host\n'
else
  matches="$(ps ax -o pid,stat,command 2>/dev/null |
    grep -E 'hdiutil|diskimages-helper|DiskImages|bundle_dmg' |
    grep -v grep || true)"
  if [ -n "$matches" ]; then
    printf '%s\n' "$matches"
  else
    printf 'none: no disk image process is running\n'
  fi
fi

section "bundle output"
if [ -d "$BUNDLE_DIR" ]; then
  report ls -la "$BUNDLE_DIR"
  for directory in dmg macos; do
    if [ -d "$BUNDLE_DIR/$directory" ]; then
      printf '\n%s:\n' "$BUNDLE_DIR/$directory"
      report ls -la "$BUNDLE_DIR/$directory"
    fi
  done
else
  printf 'missing: %s was never created\n' "$BUNDLE_DIR"
fi

printf '\n=== end of desktop bundle diagnostics ===\n'
exit 0
