#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
output="$(mktemp)"
trap 'rm -f -- "$output"' EXIT

if make --no-print-directory -C "$repo_root" check-output-contract-fixture >"$output" 2>&1; then
  echo "FAIL: output fixture unexpectedly passed" >&2
  exit 1
fi

if grep -Fq 'SUCCESS_DETAIL' "$output" || grep -Fq 'AFTER_DETAIL' "$output"; then
  echo "FAIL: successful step output was not suppressed" >&2
  cat "$output" >&2
  exit 1
fi

grep -Fq 'success-step' "$output"
grep -Fq '==> success-step' "$output"
grep -Eq 'success-step +ok +[0-9]+s' "$output"
grep -Fq 'failure-step' "$output"
grep -Eq 'failure-step +FAIL +[0-9]+s' "$output"
grep -Fq 'command: echo FAILURE_DETAIL; exit 7' "$output"
grep -Fq 'FAILURE_DETAIL' "$output"
grep -Fq 'after-step' "$output"
grep -Eq 'after-step +ok +[0-9]+s' "$output"
grep -Fq 'check-output-contract-fixture FAILED' "$output"

echo "check output contract ok"

# Fast checks delegate Tasks E2E without invoking the workspace build target.
# Its script owns isolated builds; here a fixture observes the Make dependency.
fixture="$(mktemp -d)"
trap 'rm -f -- "$output"; rm -rf -- "$fixture"' EXIT
cp "$repo_root/Makefile" "$fixture/Makefile"
mkdir "$fixture/scripts"
mkdir -p "$fixture/internal/version"
cp "$repo_root/internal/version/version.go" "$fixture/internal/version/version.go"
printf '#!/bin/sh\ntouch ran-e2e\n' >"$fixture/scripts/tariboy-tasks-e2e.sh"
chmod +x "$fixture/scripts/tariboy-tasks-e2e.sh"
make --no-print-directory -C "$fixture" GO=false VERSION=test tariboy-tasks-e2e
test -f "$fixture/ran-e2e"
test ! -e "$fixture/bin"
echo "Tasks check build isolation contract ok"
