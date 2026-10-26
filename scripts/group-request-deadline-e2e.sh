#!/usr/bin/env bash
# E2E for a mail request with a deadline between two group members (mailbox
# design §2). Brings up a fully ISOLATED tariboyd (own base/runtime dirs, web
# off, never touches the user's live daemon), creates a dev group, and drives
# the lead through the stub harness to mail the worker a request with a short
# deadline. With NO reply the deadline arms a one-shot schedule that mails the
# lead a kind=deadline notice from system:schedule once the deadline passes,
# with data.request_id naming the request.
#
# This is the wall-clock leg (scheduler ticks once a second); the reply-cancels
# leg and the arming are covered deterministically by the unit + integration
# tests (internal/schedule, internal/loop, internal/mail).
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
. "$ROOT/scripts/test-image-fixture.sh"
TEST_IMAGE_SOURCE="$BASE/test-image"
make_test_image_fixture "$TEST_IMAGE_SOURCE"

SOCK="$RUNTIME/tariboyd.sock"
trap 'kill "${DPID:-}" 2>/dev/null || true; rm -rf "$BASE" "$RUNTIME"' EXIT

echo "--- start isolated daemon (web off)"
TARIBOY_SHELL_ENV=1 "$BIN/tariboyd" --base-dir "$BASE" --log-level error --web-addr "" &
DPID=$!
for _ in $(seq 1 100); do [ -S "$SOCK" ] && break; sleep 0.1; done
[ -S "$SOCK" ] || { echo "FAIL: socket never appeared"; exit 1; }

sa() { "$BIN/tariboy" --socket "$SOCK" "$@"; }

echo "--- image build"
sa image build --name basic-example --tag latest --path "$TEST_IMAGE_SOURCE" | grep -q "digest" \
  || { echo "FAIL: image build"; exit 1; }

echo "--- group create (lead=manager)"
sa group create dev-team --lead manager | grep -q "dev-team" \
  || { echo "FAIL: group create"; exit 1; }

echo "--- run lead + member into the group (stub harness)"
sa agent run basic-example:latest --name manager --harness stub --interactive false --group dev-team \
  --env "STUB_GROUP_REQUEST=worker|what is blocking you?|3s,STUB_CALL_DONE=1" \
  | grep -q "name: manager" || { echo "FAIL: run manager"; exit 1; }
sa agent run basic-example:latest --name worker --harness stub --interactive false --group dev-team \
  --env "STUB_CALL_DONE=1" | grep -q "name: worker" || { echo "FAIL: run worker"; exit 1; }

echo "--- lead iteration mails the worker a request with a deadline"
sa agent exec manager >/dev/null || { echo "FAIL: exec manager"; exit 1; }
DONE=0
for _ in $(seq 1 100); do
  if sa --json iteration ls manager | grep -q '"status":"done"'; then DONE=1; break; fi
  sleep 0.1
done
[ "$DONE" = 1 ] || { echo "FAIL: manager iteration did not finish"; exit 1; }

echo "--- the request is in the worker's mailbox"
sa --json mail sent --as agent:manager | tee "$BASE/sent.json" | grep -q '"to":"agent:worker"' \
  || { echo "FAIL: request not sent to the worker"; cat "$BASE/sent.json"; exit 1; }
REQ="$(jq -r '[.letters[] | select(.to == "agent:worker" and .needs_reply and .deadline != "")][0].id // empty' "$BASE/sent.json")"
[ -n "$REQ" ] || { echo "FAIL: no request with a deadline in the lead's sent box"; cat "$BASE/sent.json"; exit 1; }
sa --json mail inbox --as agent:worker --all | grep -q "\"id\":\"$REQ\"" \
  || { echo "FAIL: request missing from the worker's mailbox"; sa --json mail inbox --as agent:worker --all; exit 1; }
echo "    request_id=$REQ"

deadline_notice() {
  sa --json mail inbox --as agent:manager --all \
    | jq -e --arg req "$REQ" '[.letters[] | select(.from == "system:schedule" and .kind == "deadline" and .data.request_id == $req)] | length > 0' >/dev/null
}

echo "--- no reply: waiting for the deadline notice in the lead's mailbox"
FIRED=0
for _ in $(seq 1 80); do   # up to ~8s (deadline 3s + scheduler tick)
  if deadline_notice; then FIRED=1; break; fi
  sleep 0.1
done
sa --json mail inbox --as agent:manager --all >"$BASE/inbox.json"
[ "$FIRED" = 1 ] || { echo "FAIL: deadline notice never arrived"; cat "$BASE/inbox.json"; exit 1; }

echo "PASS: request armed a deadline; no reply -> kind=deadline notice from system:schedule in the lead's mailbox (request=$REQ)"
