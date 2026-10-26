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
for name in tariboyd tariboy tariboy-tasks tariboy-mail tariboy-shim tariboy-plugin-telegram; do
  go build -trimpath -o "$BIN/$name" "./cmd/$name"
done

[ -x "$BIN/tariboy-tasks" ] || fail "missing $BIN/tariboy-tasks"
ttasks() { "$BIN/tariboy-tasks" "$@"; }
[ -x "$BIN/tariboy-mail" ] || fail "missing $BIN/tariboy-mail"
tmail() { "$BIN/tariboy-mail" "$@"; }
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
ttasks queue trigger create OPS --pattern 'user:*' --action create_task >/dev/null
ttasks queue trigger list OPS --json | grep -q '"pattern":"user:\*"' || fail "queue trigger was not created"
if ttasks queue trigger create OPS --pattern external:incidents --action create_task >/dev/null 2>&1; then
  fail "queue trigger accepted a channel pattern"
fi

echo "--- a letter to queue:OPS from the customer creates a task through the trigger"
tmail send --to queue:OPS --subject "incident on api" --text "the api returns 500" >/dev/null \
  || fail "operator could not mail queue:OPS"
INGRESS=""
for _ in $(seq 1 100); do
  INGRESS="$(ttasks list --queue OPS --json)"
  printf '%s' "$INGRESS" | grep -q '"title":"incident on api"' && break
  sleep 0.1
done
printf '%s' "$INGRESS" | grep -q '"title":"incident on api"' || { printf '%s\n' "$INGRESS" >&2; fail "trigger did not create the task from the letter"; }
tmail sent --json | grep -q '"to":"queue:OPS"' || fail "the queue letter is missing from the customer sent box"
if ttasks workflows versions cli-flow >/dev/null 2>&1; then
  fail "removed workflows command still runs"
fi
# Task keys are random, so every later reference reads the key the daemon minted.
task_key() { grep -o '"key":"[^"]*"' | head -1 | cut -d'"' -f4; }
OPS_KEY="$(ttasks create --queue OPS --title "operator task" --json | task_key)"
AGT_KEY="$(ttasks create --queue AGT --title "agent task" --json | task_key)"
[ -n "$OPS_KEY" ] && [ -n "$AGT_KEY" ] || fail "task creation did not report a key"
OPERATOR="$(ttasks mine --json)"
printf '%s' "$OPERATOR" | grep -q '"queue":"OPS"' || fail "operator did not see OPS"
printf '%s' "$OPERATOR" | grep -q '"queue":"AGT"' || fail "operator did not see AGT"
ttasks update "$OPS_KEY" --title "operator updated" >/dev/null
ttasks update "$AGT_KEY" --title "agent updated" >/dev/null
ttasks show "$OPS_KEY" --json | grep -q '"title":"operator updated"' || fail "operator did not update OPS"
ttasks show "$AGT_KEY" --json | grep -q '"title":"agent updated"' || fail "operator did not update AGT"
CHILD_KEY="$(ttasks create --parent "$OPS_KEY" --title "operator child" --json | task_key)"
SUBTREE="$(ttasks list --subtree "$OPS_KEY" --json)"
printf '%s' "$SUBTREE" | grep -q "\"key\":\"$CHILD_KEY\"" || fail "list --subtree missed the child"
if printf '%s' "$SUBTREE" | grep -q "\"key\":\"$OPS_KEY\""; then
  fail "list --subtree included its root"
fi
ttasks list --text "OPERATOR UPDATED" --sort updated | grep -q "$OPS_KEY" || fail "list --text table missed the task"
ttasks list --limit 1 | grep -q '^next: ttasks list --limit 1 --after ' || fail "list did not print the next page command"

echo "--- agent mode is identity-bound to its real tools socket"
"$BIN/tariboy" --socket "$SOCK" agent run tasks-test:latest --name worker --harness stub --interactive false \
  --env "STUB_SLEEP=300,STUB_CALL_DONE=0,STUB_TASKS_MINE=$RUNTIME/agent-tasks.json" >/dev/null
ttasks assign "$AGT_KEY" worker >/dev/null
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
TARIBOY_TOOLS_SOCKET="$RUNTIME/worker.sock" ttasks list --queue AGT --sort priority | grep -q "$AGT_KEY" || fail "agent list missed its AGT task"
[ "$(TARIBOY_TOOLS_SOCKET="$RUNTIME/worker.sock" ttasks list --subtree "$OPS_KEY")" = "no tasks match" ] || fail "agent list --subtree exposed an invisible tree"

echo "--- unreachable agent socket never falls back to operator mode"
set +e
TARIBOY_TOOLS_SOCKET="$RUNTIME/missing.sock" ttasks mine --json >"$RUNTIME/missing.stdout" 2>"$RUNTIME/missing.stderr"
CODE=$?
set -e
[ "$CODE" -eq 2 ] || fail "missing agent socket exit=$CODE, want 2"
[ ! -s "$RUNTIME/missing.stdout" ] || fail "missing agent socket returned operator data"
unset TARIBOY_TOOLS_SOCKET
ttasks mine --json | grep -q '"queue":"OPS"' || fail "operator daemon was not still available"

echo "--- the daemon serves the committed OpenAPI snapshot and matches its schemas"
# -v and the PASS line prove the test ran: without its variables it skips.
OPENAPI_E2E_SOCKET="$SOCK" OPENAPI_E2E_AGENT=worker OPENAPI_E2E_QUEUE=OPS OPENAPI_E2E_TASK="$OPS_KEY" \
  go test ./internal/commands -run '^TestDaemonOpenAPIContract$' -count=1 -v >"$RUNTIME/openapi.out" 2>&1 \
  || { cat "$RUNTIME/openapi.out" >&2; fail "daemon OpenAPI contract"; }
grep -q -- '--- PASS: TestDaemonOpenAPIContract' "$RUNTIME/openapi.out" || { cat "$RUNTIME/openapi.out" >&2; fail "daemon OpenAPI contract did not run"; }

echo "PASS: isolated operator and identity-bound agent Tasks modes"
