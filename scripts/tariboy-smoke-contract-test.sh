#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
script="$ROOT/scripts/tariboy-smoke.sh"
makefile="$ROOT/Makefile"
env_helper="$ROOT/scripts/smoke-codex-env.sh"

grep -q 'TARIBOY_SMOKE_REAL_HARNESS=codex TARIBOY_SMOKE_REQUIRE_REAL=1' "$makefile"
grep -q 'TARIBOY_SMOKE_REAL_HARNESS.*:-codex' "$script"
grep -q 'run_codex_case.*-codex-interactive.*true' "$script"
grep -q 'smoke_codex_env_setup.*CODEX_BIN' "$script"
grep -q 'trust_level = "trusted"' "$script"
! grep -q -- '--harness claude' "$script"
! grep -q 'command -v claude' "$script"

tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
mkdir -p "$tmp/source" "$tmp/bin" "$tmp/temps"
cat >"$tmp/bin/custom-codex" <<'SH'
#!/bin/sh
printf 'custom-codex-runtime\n'
SH
chmod +x "$tmp/bin/custom-codex"
output="$(TMPDIR="$tmp/temps" bash -c '
  set -euo pipefail
  source "$1"
  smoke_codex_env_setup "$2" "$3"
  test "$(readlink "$SMOKE_CODEX_BIN_DIR/codex")" = "$3"
  codex
' _ "$env_helper" "$tmp/source" "$tmp/bin/custom-codex")"
[ "$output" = "custom-codex-runtime" ]

# A failure while copying credentials happens after mktemp but before the main
# smoke cleanup is installed. The helper's immediate trap must still reap it.
printf '{}\n' >"$tmp/source/auth.json"
if TMPDIR="$tmp/temps" bash -c '
  set -euo pipefail
  cp() { return 42; }
  source "$1"
  smoke_codex_env_setup "$2" "$3"
' _ "$env_helper" "$tmp/source" "$tmp/bin/custom-codex"; then
  echo "FAIL: forced credential-copy failure unexpectedly succeeded" >&2
  exit 1
fi
[ -z "$(find "$tmp/temps" -mindepth 1 -print -quit)" ]

echo "OK: smoke harness contract requires Codex for full-smoke"
