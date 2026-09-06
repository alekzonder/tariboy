#!/usr/bin/env bash
# Isolated operator/agent contract for the dual-mode Native Tasks client.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin"
BASE="$(mktemp -d)"
RUNTIME="$(mktemp -d)"
export TARIBOY_BASE_DIR="$BASE"
export TARIBOY_RUNTIME_DIR="$RUNTIME"
export TARIBOY_SHIM_BIN="$BIN/tariboy-shim"
export TARIBOY_STUB_HARNESS="$ROOT/scripts/stub-harness.sh"
unset TARIBOY_TOOLS_SOCKET

SOCK="$RUNTIME/tariboyd.sock"
DPID=""
trap 'kill "${DPID:-}" 2>/dev/null || true; wait "${DPID:-}" 2>/dev/null || true; rm -rf "$BASE" "$RUNTIME"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

[ -x "$BIN/tariboy-tasks" ] || fail "missing $BIN/tariboy-tasks"
mkdir "$RUNTIME/bin"
ln -s "$BIN/tariboy-tasks" "$RUNTIME/bin/ttasks"
export PATH="$RUNTIME/bin:$PATH"

"$BIN/tariboyd" --base-dir "$BASE" --http-addr "" --log-level error &
DPID=$!
for _ in $(seq 1 200); do
  [ -S "$SOCK" ] && break
  sleep 0.05
done
[ -S "$SOCK" ] || fail "isolated daemon did not start"

echo "--- operator mode sees and updates both queues"
ttasks queue create --prefix OPS --name "Operator Queue" >/dev/null
ttasks queue create --prefix AGT --name "Agent Queue" >/dev/null
ttasks create --queue OPS --title "operator task" >/dev/null
ttasks create --queue AGT --title "agent task" >/dev/null
OPERATOR="$(ttasks mine --json)"
printf '%s' "$OPERATOR" | grep -q '"queue":"OPS"' || fail "operator did not see OPS"
printf '%s' "$OPERATOR" | grep -q '"queue":"AGT"' || fail "operator did not see AGT"
ttasks update OPS-1 --title "operator updated" >/dev/null
ttasks update AGT-1 --title "agent updated" >/dev/null
ttasks show OPS-1 --json | grep -q '"title":"operator updated"' || fail "operator did not update OPS"
ttasks show AGT-1 --json | grep -q '"title":"agent updated"' || fail "operator did not update AGT"

echo "--- agent mode is identity-bound to its real tools socket"
"$BIN/tariboy" --socket "$SOCK" agent run basic:latest --name worker --harness stub --loop false \
  --plugins tasks --env 'STUB_SLEEP=300,STUB_CALL_DONE=0' >/dev/null
ttasks assign AGT-1 worker >/dev/null
"$BIN/tariboy" --socket "$SOCK" agent exec worker >/dev/null
for _ in $(seq 1 200); do
  [ -S "$RUNTIME/worker.sock" ] && break
  sleep 0.05
done
[ -S "$RUNTIME/worker.sock" ] || fail "agent tools socket did not start"
AGENT="$(TARIBOY_TOOLS_SOCKET="$RUNTIME/worker.sock" ttasks mine --json)"
printf '%s' "$AGENT" | grep -q '"queue":"AGT"' || fail "agent did not see assigned AGT task"
if printf '%s' "$AGENT" | grep -q '"queue":"OPS"'; then
  fail "agent saw operator-only OPS task"
fi

echo "--- unreachable agent socket never falls back to operator mode"
set +e
TARIBOY_TOOLS_SOCKET="$RUNTIME/missing.sock" ttasks mine --json >"$RUNTIME/missing.stdout" 2>"$RUNTIME/missing.stderr"
CODE=$?
set -e
[ "$CODE" -eq 2 ] || fail "missing agent socket exit=$CODE, want 2"
[ ! -s "$RUNTIME/missing.stdout" ] || fail "missing agent socket returned operator data"
unset TARIBOY_TOOLS_SOCKET
ttasks mine --json | grep -q '"queue":"OPS"' || fail "operator daemon was not still available"

echo "PASS: isolated operator and identity-bound agent Tasks modes"
