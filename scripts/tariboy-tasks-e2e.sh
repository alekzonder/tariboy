#!/usr/bin/env bash
# Isolated operator/agent contract for the dual-mode Native Tasks client.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="$(mktemp -d)"
RUNTIME="$(mktemp -d)"
BIN="$RUNTIME/bundle"
export TARIBOY_BASE_DIR="$BASE"
export TARIBOY_RUNTIME_DIR="$RUNTIME"
export TARIBOY_SHIM_BIN="$BIN/tariboy-shim"
export TARIBOY_STUB_HARNESS="$ROOT/scripts/stub-harness.sh"
unset TARIBOY_TOOLS_SOCKET

SOCK="$RUNTIME/tariboyd.sock"
DPID=""
trap '"$BIN/tariboy" --socket "$SOCK" agent kill worker >/dev/null 2>&1 || true; kill "${DPID:-}" 2>/dev/null || true; wait "${DPID:-}" 2>/dev/null || true; rm -rf "$BASE" "$RUNTIME"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

# Compile the current source without modifying workspace artifacts.
cd "$ROOT"
for name in tariboyd tariboy tariboy-tasks tariboy-shim tariboy-plugin-telegram; do
  go build -trimpath -o "$BIN/$name" "./cmd/$name"
done

[ -x "$BIN/tariboy-tasks" ] || fail "missing $BIN/tariboy-tasks"
ttasks() { "$BIN/tariboy-tasks" "$@"; }
mkdir "$RUNTIME/prerequisites"
ln -s "$(command -v python3)" "$RUNTIME/prerequisites/python3"

# Model fresh Desktop startup: no optional CLI install or Tasks alias on PATH.
TARIBOY_SHELL_ENV=1 PATH="$RUNTIME/prerequisites:/usr/bin:/bin" "$BIN/tariboyd" --base-dir "$BASE" --http-addr "" --log-level error &
DPID=$!
for _ in $(seq 1 200); do
  [ -S "$SOCK" ] && break
  sleep 0.05
done
[ -S "$SOCK" ] || fail "isolated daemon did not start"

IMAGE_SOURCE="$RUNTIME/tasks-image"
mkdir -p "$IMAGE_SOURCE/skills/tasks/scripts"
cat >"$IMAGE_SOURCE/Tariboyfile.yaml" <<'YAML'
schema_version: 2
plugins:
  - name: tasks
skills:
  - dir: ./skills/tasks
prompts: []
YAML
cat >"$IMAGE_SOURCE/skills/tasks/SKILL.md" <<'MARKDOWN'
---
name: tasks
description: Exercise the identity-bound Tasks launcher.
---
MARKDOWN
cat >"$IMAGE_SOURCE/skills/tasks/scripts/tasks.sh" <<'SH'
#!/bin/sh
exec ttasks "$@"
SH
chmod 0700 "$IMAGE_SOURCE/skills/tasks/scripts/tasks.sh"
"$BIN/tariboy" --socket "$SOCK" image build --name tasks-test --tag latest --path "$IMAGE_SOURCE" >/dev/null

echo "--- operator mode sees and updates both queues"
ttasks queue create --prefix OPS --name "Operator Queue" >/dev/null
ttasks queue create --prefix AGT --name "Agent Queue" >/dev/null
ttasks queue get OPS --json | grep -q '"prefix":"OPS"' || fail "operator queue argument was lost"
ttasks workflows create --definition '{"name":"cli-flow","version":1,"initial_status":"implement","statuses":[{"id":"implement","requirements":[{"id":"code","pool":"developers","dispatch":"claim_one","produces":["implementation"],"outcomes":["done"]}],"transitions":[{"when":"code.done","to":"done"}]},{"id":"done","terminal":true}]}' >/dev/null
ttasks workflows publish cli-flow 1 >/dev/null
ttasks workflows get cli-flow 1 --json | grep -q '"state":"published"' || fail "workflow definition was not published"
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
"$BIN/tariboy" --socket "$SOCK" agent run tasks-test:latest --name worker --harness stub --loop false \
  --env "STUB_SLEEP=300,STUB_CALL_DONE=0,STUB_TASKS_MINE=$RUNTIME/agent-tasks.json" >/dev/null
ttasks assign AGT-1 worker >/dev/null
"$BIN/tariboy" --socket "$SOCK" agent exec worker >/dev/null
for _ in $(seq 1 200); do
  [ -S "$RUNTIME/worker.sock" ] && break
  sleep 0.05
done
[ -S "$RUNTIME/worker.sock" ] || fail "agent tools socket did not start"
for _ in $(seq 1 200); do
  [ -s "$RUNTIME/agent-tasks.json" ] && break
  sleep 0.05
done
[ -s "$RUNTIME/agent-tasks.json" ] || fail "daemon-launched agent could not invoke bundled ttasks"
grep -q '"queue":"AGT"' "$RUNTIME/agent-tasks.json" || fail "launched agent did not see assigned task"
test ! -e "$BASE/agents/worker/bin/ttasks" && test ! -L "$BASE/agents/worker/bin/ttasks" || fail "agent-local ttasks was created"
test "$(readlink "$RUNTIME/bin/ttasks")" = "$BIN/tariboy-tasks" || fail "runtime alias does not select the bundled payload"
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
