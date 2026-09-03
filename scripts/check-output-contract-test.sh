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
