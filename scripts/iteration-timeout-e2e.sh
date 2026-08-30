#!/usr/bin/env bash
# Isolated end-to-end coverage for extending a running iteration timeout.  It
# owns its base/runtime directories and a non-default web port, so it never
# reaches a user's daemon or ~/.tariboy.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin"
BASE="$(mktemp -d)"
RUNTIME="$(mktemp -d)"
WEB_PORT="${TARIBOY_TIMEOUT_E2E_WEB_PORT:-18765}"
export TARIBOY_BASE_DIR="$BASE"
export TARIBOY_RUNTIME_DIR="$RUNTIME"
export TARIBOY_SHIM_BIN="$BIN/tariboy-shim"
export TARIBOY_STUB_HARNESS="$ROOT/scripts/stub-harness.sh"
chmod +x "$TARIBOY_STUB_HARNESS"

SOCK="$RUNTIME/tariboyd.sock"
DPID=""
trap 'kill "${DPID:-}" 2>/dev/null || true; rm -rf "$BASE" "$RUNTIME"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
start_daemon() {
  TARIBOY_SHELL_ENV=1 "$BIN/tariboyd" --base-dir "$BASE" --log-level error --web-addr "127.0.0.1:$WEB_PORT" &
  DPID=$!
  for _ in $(seq 1 100); do
    [ -S "$SOCK" ] && curl -fsS "http://127.0.0.1:$WEB_PORT/api/daemon/status" >/dev/null && return
    sleep 0.1
  done
  fail "isolated daemon never became ready (base=$BASE runtime=$RUNTIME web=$WEB_PORT)"
}
stop_daemon() {
  if [ "${1:-}" = "crash" ]; then kill -KILL "$DPID" 2>/dev/null || true
  else kill "$DPID" 2>/dev/null || true
  fi
  wait "$DPID" 2>/dev/null || true
  DPID=""
}
sa() { "$BIN/tariboy" --socket "$SOCK" "$@"; }
status() { curl -fsS "http://127.0.0.1:$WEB_PORT/api/agents/timer/status"; }
extend() { curl -sS -X POST "http://127.0.0.1:$WEB_PORT/api/agents/timer/iterations/$ITER/extend-timeout"; }

echo "--- start isolated daemon (base=$BASE runtime=$RUNTIME web=127.0.0.1:$WEB_PORT)"
start_daemon

echo "--- build isolated test image and start a deliberately long stub iteration"
sa image build --name timeout-e2e --tag latest --path "$ROOT/internal/builtinimages/source" | grep -q 'digest:' || fail "image build"
sa agent run timeout-e2e:latest --name timer --harness stub --loop false \
  --env 'STUB_SLEEP=75,STUB_CALL_DONE=0' | grep -q 'name: timer' || fail "agent run"
# Startup does real prompt/tool preparation, so leave enough room to make the
# first API call even on a cold CI machine. Three extensions still keep this a
# short wall-clock E2E while proving the final deadline is enforced.
sa loop timeout timer --value 15 | grep -q 'timeout_s: 15' || fail "set timeout"
# Run/loop configuration is intentionally snapshotted by the engine. Restart
# this isolated daemon before the first exec so the test starts with the
# configured timeout rather than the agent-run default.
stop_daemon
start_daemon
sa agent exec timer >/dev/null || fail "start iteration"

echo "--- wait for persisted running timeout snapshot"
for _ in $(seq 1 100); do
  LIVE="$(status)"
  ITER="$(printf '%s' "$LIVE" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
  printf '%s' "$LIVE" | grep -q '"timeout_period_s":15' && [ -n "$ITER" ] && break
  sleep 0.1
done
[ -n "${ITER:-}" ] || fail "running iteration snapshot never appeared"
INITIAL="$(status)"
printf '%s' "$INITIAL" | grep -q '"timeout_extensions":0' || fail "initial extension count"

echo "--- extend twice before daemon restart"
FIRST="$(extend)"; printf '%s' "$FIRST" | grep -q '"timeout_extensions":1' || fail "first extension: $FIRST"
SECOND="$(extend)"; printf '%s' "$SECOND" | grep -q '"timeout_extensions":2' || fail "second extension: $SECOND"
SECOND_DEADLINE="$(printf '%s' "$SECOND" | sed -n 's/.*"timeout_deadline":"\([^"]*\)".*/\1/p')"
[ -n "$SECOND_DEADLINE" ] || fail "second extension deadline missing"

echo "--- restart only the isolated daemon and wait for live iteration adoption"
stop_daemon crash
start_daemon
ADOPTED=0
for _ in $(seq 1 100); do
  LIVE="$(status)"
  if printf '%s' "$LIVE" | grep -q "\"id\":\"$ITER\"" \
    && printf '%s' "$LIVE" | grep -q '"timeout_extensions":2' \
    && printf '%s' "$LIVE" | grep -q "\"timeout_deadline\":\"$SECOND_DEADLINE\""; then ADOPTED=1; break; fi
  sleep 0.1
done
[ "$ADOPTED" = 1 ] || fail "daemon did not adopt persisted deadline/count"

echo "--- extend after adoption; persisted count and deadline remain authoritative"
THIRD="$(extend)"; printf '%s' "$THIRD" | grep -q '"timeout_extensions":3' || fail "post-restart extension: $THIRD"
THIRD_DEADLINE="$(printf '%s' "$THIRD" | sed -n 's/.*"timeout_deadline":"\([^"]*\)".*/\1/p')"
[ -n "$THIRD_DEADLINE" ] || fail "post-restart deadline missing"

echo "--- wait for the extended deadline to enforce timeout"
TIMED_OUT=0
for _ in $(seq 1 800); do
  ROWS="$(sa --json iteration ls timer)"
  if printf '%s' "$ROWS" | grep -q '"status":"timeout"'; then TIMED_OUT=1; break; fi
  sleep 0.1
done
[ "$TIMED_OUT" = 1 ] || fail "extended deadline was not enforced"

echo "PASS: timeout extensions 0→1→2 survived isolated daemon restart; 3rd extension applied; timeout enforced (iteration=$ITER deadline=$THIRD_DEADLINE base=$BASE runtime=$RUNTIME web=$WEB_PORT)"
