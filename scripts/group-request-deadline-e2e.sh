#!/usr/bin/env bash
# E2E for the request primitive behind `tools group request` (spec §4.2, EPIC R
# R3). Brings up a fully ISOLATED tariboyd (own base/runtime dirs, web off,
# never touches the user's live daemon), creates a dev group, and drives the
# lead through the stub harness to send a group request with a short --deadline.
# With NO reply the daemon's deadline seam must arm a one-shot schedule that
# publishes a type=timeout event into the lead's inbox once the deadline passes.
#
# This is the wall-clock leg (scheduler ticks once a second); the reply-cancels
# leg and the arming/threading are covered deterministically by the unit +
# integration tests (internal/schedule, internal/loop, internal/bus).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin"
BASE="$(mktemp -d)"
RUNTIME="$(mktemp -d)"
export TARIBOY_BASE_DIR="$BASE"
export TARIBOY_RUNTIME_DIR="$RUNTIME"
export TARIBOY_SHIM_BIN="$BIN/tariboy-shim"
export TARIBOY_STUB_HARNESS="$ROOT/scripts/stub-harness.sh"
chmod +x "$TARIBOY_STUB_HARNESS"

SOCK="$RUNTIME/tariboyd.sock"
trap 'kill "${DPID:-}" 2>/dev/null || true; rm -rf "$BASE" "$RUNTIME"' EXIT

echo "--- start isolated daemon (web off)"
TARIBOY_SHELL_ENV=1 "$BIN/tariboyd" --base-dir "$BASE" --log-level error --web-addr "" &
DPID=$!
for _ in $(seq 1 100); do [ -S "$SOCK" ] && break; sleep 0.1; done
[ -S "$SOCK" ] || { echo "FAIL: socket never appeared"; exit 1; }

sa() { "$BIN/tariboy" --socket "$SOCK" "$@"; }

echo "--- image build"
sa image build --name basic-example --tag latest --path "$ROOT/internal/builtinimages/source" | grep -q "digest" \
  || { echo "FAIL: image build"; exit 1; }

echo "--- group create (lead=manager)"
sa group create dev-team --lead manager | grep -q "dev-team" \
  || { echo "FAIL: group create"; exit 1; }

echo "--- run lead + member into the group (stub harness)"
sa agent run basic-example:latest --name manager --harness stub --group dev-team \
  --env "STUB_GROUP_REQUEST=worker|what is blocking you?|3s,STUB_CALL_DONE=1" \
  | grep -q "name: manager" || { echo "FAIL: run manager"; exit 1; }
sa agent run basic-example:latest --name worker --harness stub --group dev-team \
  --env "STUB_CALL_DONE=1" | grep -q "name: worker" || { echo "FAIL: run worker"; exit 1; }

echo "--- lead iteration sends the group request"
sa agent exec manager >/dev/null || { echo "FAIL: exec manager"; exit 1; }
DONE=0
for _ in $(seq 1 100); do
  if sa --json iteration ls manager | grep -q '"status":"done"'; then DONE=1; break; fi
  sleep 0.1
done
[ "$DONE" = 1 ] || { echo "FAIL: manager iteration did not finish"; exit 1; }

echo "--- request landed on the member's group direct channel as kind=request"
sa --json channel tail "group:dev-team:direct:worker" | tee "$BASE/direct.json" | grep -q '"kind":"request"' \
  || { echo "FAIL: request not on direct channel"; cat "$BASE/direct.json"; exit 1; }
CORR="$(grep -o '"correlation_id":"[^"]*"' "$BASE/direct.json" | head -n1 | cut -d'"' -f4)"
[ -n "$CORR" ] || { echo "FAIL: request has no correlation id"; exit 1; }
echo "    correlation_id=$CORR"

echo "--- no reply: waiting for the deadline timeout to fire into the lead inbox"
FIRED=0
for _ in $(seq 1 80); do   # up to ~8s (deadline 3s + scheduler tick)
  if sa --json channel tail "agent:manager:inbox" | grep -q '"type":"timeout"'; then FIRED=1; break; fi
  sleep 0.1
done
sa --json channel tail "agent:manager:inbox" | tee "$BASE/inbox.json" >/dev/null
[ "$FIRED" = 1 ] || { echo "FAIL: timeout never arrived"; cat "$BASE/inbox.json"; exit 1; }

echo "--- timeout event carries the request's correlation id"
grep -q "$CORR" "$BASE/inbox.json" || { echo "FAIL: timeout missing correlation id"; cat "$BASE/inbox.json"; exit 1; }

echo "PASS: group request armed a deadline; no reply -> type=timeout delivered to lead inbox (corr=$CORR)"
