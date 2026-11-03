#!/usr/bin/env bash
# Contract of scripts/ui-test-memory-gate.sh: it waits for MemAvailable before
# ui-test, gives up after its time limit without skipping the step, and is off
# with a zero threshold or without /proc/meminfo. A fake meminfo file stands in
# for the kernel.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
gate="$repo_root/scripts/ui-test-memory-gate.sh"
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT
meminfo="$work/meminfo"
out="$work/out"

fail() {
  echo "FAIL: $1" >&2
  cat "$out" >&2 || true
  exit 1
}

set_mem() {
  printf 'MemTotal:        6066176 kB\nMemAvailable:    %s kB\n' "$1" >"$meminfo.new"
  mv -f -- "$meminfo.new" "$meminfo"
}

# run_gate VAR=VALUE... runs the gate with the fake meminfo and records the exit
# code in $code and the combined output in $out.
run_gate() {
  code=0
  env -u TARIBOY_UI_TEST_MIN_MEM_MB -u TARIBOY_UI_TEST_MEM_WAIT_TIMEOUT -u TARIBOY_UI_TEST_MEM_POLL \
    TARIBOY_MEMINFO="$meminfo" "$@" sh "$gate" >"$out" 2>&1 || code=$?
}

# Enough memory: no wait and no output.
set_mem 4096000
run_gate
[ "$code" -eq 0 ] || fail "enough memory exited $code"
[ ! -s "$out" ] || fail "enough memory printed output"

# Low memory, then enough: the gate waits, says why, and starts ui-test.
set_mem 1024000
(sleep 2; set_mem 4096000) &
start=$(date +%s)
run_gate TARIBOY_UI_TEST_MEM_POLL=1 TARIBOY_UI_TEST_MEM_WAIT_TIMEOUT=30
elapsed=$(($(date +%s) - start))
wait
[ "$code" -eq 0 ] || fail "low memory exited $code"
grep -Fq 'memory gate: waiting for MemAvailable >= 3000 MB before ui-test (now 1000 MB, limit 30s)' "$out" || fail "no waiting line"
grep -Eq 'memory gate: MemAvailable 4000 MB after [0-9]+s, starting ui-test' "$out" || fail "no starting line"
[ "$elapsed" -ge 2 ] || fail "low memory did not wait (${elapsed}s)"

# A zero threshold turns the gate off.
set_mem 1024000
run_gate TARIBOY_UI_TEST_MIN_MEM_MB=0
[ "$code" -eq 0 ] || fail "threshold 0 exited $code"
[ ! -s "$out" ] || fail "threshold 0 printed output"

# The time limit runs out: a warning, then ui-test runs anyway.
set_mem 1024000
run_gate TARIBOY_UI_TEST_MEM_POLL=1 TARIBOY_UI_TEST_MEM_WAIT_TIMEOUT=2
[ "$code" -eq 0 ] || fail "time limit exited $code"
grep -Fq 'memory gate: MemAvailable 1000 MB is still below 3000 MB after 2s; running ui-test anyway' "$out" || fail "no time limit warning"

# No MemAvailable (macOS has no /proc/meminfo): one line, no wait.
rm -f -- "$meminfo"
run_gate
[ "$code" -eq 0 ] || fail "missing meminfo exited $code"
[ "$(wc -l <"$out")" -eq 1 ] || fail "missing meminfo printed more than one line"
grep -Fq 'has no MemAvailable, not waiting for memory' "$out" || fail "no missing meminfo line"

# A threshold that is not a number is an error.
set_mem 4096000
run_gate TARIBOY_UI_TEST_MIN_MEM_MB=abc
[ "$code" -eq 2 ] || fail "non-numeric threshold exited $code"
grep -Fq 'TARIBOY_UI_TEST_MIN_MEM_MB' "$out" || fail "non-numeric threshold message does not name the variable"

echo "ui-test memory gate contract ok"
