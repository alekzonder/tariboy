#!/usr/bin/env bash
# Isolated vertical coverage for managed Native Task workflows. The suite drives
# operator mutations through REST and agent mutations through each agent's
# identity-bound tools socket; it never opens the live 9990 listener or shares
# ~/.tariboy state.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin"
SANDBOX="$(mktemp -d)"
BASE="$SANDBOX/base"
RUNTIME="$SANDBOX/runtime"
mkdir -p "$BASE" "$RUNTIME"
export TARIBOY_BASE_DIR="$BASE"
export TARIBOY_RUNTIME_DIR="$RUNTIME"
export TARIBOY_SHIM_BIN="$BIN/tariboy-shim"
export TARIBOY_TOOLS_BIN="$BIN/tariboy-tools"
export TARIBOY_STUB_HARNESS="$ROOT/scripts/stub-harness.sh"

WEB_PORT="${TARIBOY_WORKFLOW_E2E_PORT:-$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(('127.0.0.1', 0))
print(s.getsockname()[1])
s.close()
PY
)}"
[ "$WEB_PORT" != 9990 ] || { echo "FAIL: workflow E2E may not use live port 9990" >&2; exit 1; }
API="http://127.0.0.1:$WEB_PORT"
SOCK="$RUNTIME/tariboyd.sock"
DPID=""

cleanup() {
  kill "${DPID:-}" 2>/dev/null || true
  wait "${DPID:-}" 2>/dev/null || true
  rm -rf "$SANDBOX"
}
trap cleanup EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

start_daemon() {
  TARIBOY_BASE_DIR="$BASE" TARIBOY_RUNTIME_DIR="$RUNTIME" TARIBOY_SHELL_ENV=1 \
    "$BIN/tariboyd" --base-dir "$BASE" --log-level error --http-addr "127.0.0.1:$WEB_PORT" &
  DPID=$!
  for _ in $(seq 1 200); do
    if [ -S "$SOCK" ] && curl -fsS "$API/api/daemon/status" >/dev/null; then return; fi
    sleep 0.05
  done
  fail "isolated daemon did not become ready"
}

stop_daemon() {
  kill "$DPID" 2>/dev/null || true
  wait "$DPID" 2>/dev/null || true
  DPID=""
}

sa() { "$BIN/tariboy" --socket "$SOCK" "$@"; }
api() {
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -fsS -X "$method" -H 'content-type: application/json' --data "$body" "$API$path"
  else
    curl -fsS -X "$method" "$API$path"
  fi
}
expect_api_error() {
  local method="$1" path="$2" body="$3" want="$4" response code
  response="$(curl -sS -X "$method" -H 'content-type: application/json' --data "$body" "$API$path")"
  code="$(printf '%s' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["error"]["code"])')"
  [ "$code" = "$want" ] || fail "$method $path error=$code, want $want; payload=$response"
}
db_scalar() {
  python3 -c 'import sqlite3,sys; db=sqlite3.connect(sys.argv[1]); print(db.execute(sys.argv[2]).fetchone()[0])' "$BASE/tariboyd.db" "$1"
}
result() {
  python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)["result"], separators=(",",":")))'
}
field() {
  local path="$1"
  python3 -c 'import json,sys
x=json.load(sys.stdin)
for part in sys.argv[1].split("."):
    x=x[int(part)] if isinstance(x,list) else x[part]
print(json.dumps(x,separators=(",",":")) if isinstance(x,(dict,list)) else str(x).lower() if isinstance(x,bool) else x)' "$path"
}
assert_field() {
  local json="$1" path="$2" want="$3" got
  got="$(printf '%s' "$json" | field "$path")"
  [ "$got" = "$want" ] || fail "$path=$got, want $want; payload=$json"
}
tools() {
  local agent="$1"; shift
  TARIBOY_TOOLS_SOCKET="$RUNTIME/$agent.sock" "$BIN/tariboy-tools" --json "$@"
}
packet() { tools "$1" tasks work show "$2"; }
fresh_revisions() {
  local p="$1"
  TASK_REV="$(printf '%s' "$p" | field task_revision)"
  ASSIGN_REV="$(printf '%s' "$p" | field assignment.revision)"
}
add_artifact() {
  local agent="$1" assignment="$2" name="$3" content="$4" key="$5" p
  p="$(packet "$agent" "$assignment")"; fresh_revisions "$p"
  tools "$agent" tasks artifacts add "$assignment" --name "$name" --type markdown \
    --content "$content" --task-revision "$TASK_REV" --assignment-revision "$ASSIGN_REV" \
    --idempotency-key "$key" >/dev/null
}
complete_work() {
  local agent="$1" assignment="$2" outcome="$3" key="$4" p
  p="$(packet "$agent" "$assignment")"; fresh_revisions "$p"
  tools "$agent" tasks work complete "$assignment" --outcome "$outcome" \
    --task-revision "$TASK_REV" --assignment-revision "$ASSIGN_REV" \
    --idempotency-key "$key" >/dev/null
}
claim_work() {
  local agent="$1" key="$2" p
  p="$(tools "$agent" tasks work next --queue DEV --idempotency-key "$key")"
  [ "$p" != "[]" ] || fail "$agent has no claimable workflow work"
  printf '%s' "$p"
}
workflow_view() { api GET "/api/tasks/$1/workflow" | result; }

echo "--- start isolated daemon (base=$BASE runtime=$RUNTIME http=127.0.0.1:$WEB_PORT)"
start_daemon

echo "--- create four explicit workflow agents with long-lived test iterations"
for agent in manager developer reviewer qa; do
  sa agent run basic:latest --name "$agent" --harness stub --loop false \
    --plugins tasks --env 'STUB_SLEEP=300,STUB_CALL_DONE=0' >/dev/null || fail "create $agent"
  sa agent exec "$agent" >/dev/null || fail "start $agent iteration"
done
for agent in manager developer reviewer qa; do
  for _ in $(seq 1 200); do [ -S "$RUNTIME/$agent.sock" ] && break; sleep 0.05; done
  [ -S "$RUNTIME/$agent.sock" ] || fail "$agent tools socket missing"
done

echo "--- publish development workflow and bind explicit queue pools"
DEFINITION='{
  "name":"workflow-e2e","version":1,"initial_status":"intake",
  "budgets":{"max_cycles":12,"max_assignments":24,"on_exhausted":"failed"},
  "timeouts":{"assignment":"10m","question":"10m","on_timeout":"retry"},
  "retries":{"max_attempts":2,"backoff":"immediate","on_exhausted":"failed"},
  "questions":{"route_to":"managers","allowed_holds":["assignment","requirement"],"max_open_per_assignment":2,"timeout":"10m"},
  "observations":{"on_late_event":"record_only","allowed_reactions":["record_only","wake_current"]},
  "permissions":{"tools":["tasks.artifacts.add"],"channels":{"subscribe":["metrics:*"],"reactions":["record_only","wake_current"]}},
  "statuses":[
    {"id":"intake","requirements":[{"id":"decomposition","pool":"managers","dispatch":"claim_one","inputs":["request"],"produces":["implementation_plan"],"outcomes":["ready","rejected"]}],"transitions":[{"when":"decomposition.ready","to":"implementation"},{"when":"decomposition.rejected","to":"failed"}]},
    {"id":"implementation","requirements":[{"id":"implementation","pool":"developers","dispatch":"claim_one","inputs":["implementation_plan","rework_plan","previous_outputs"],"produces":["implementation_result","test_evidence"],"outcomes":["implemented","failed"]}],"transitions":[{"when":"implementation.implemented","to":"verification"},{"when":"implementation.failed","to":"failed"}]},
    {"id":"verification","join":"require_all","requirements":[{"id":"code_review","pool":"reviewers","dispatch":"claim_one","inputs":["implementation_plan","implementation_result","test_evidence"],"produces":["review_report"],"outcomes":["approved","changes_requested","failed"]},{"id":"acceptance","pool":"qa","dispatch":"claim_one","inputs":["implementation_plan","implementation_result","test_evidence"],"produces":["qa_report"],"outcomes":["passed","changes_requested","failed"]}],"transitions":[{"when":"code_review.approved && acceptance.passed","to":"integration"},{"when":"(code_review.changes_requested && (acceptance.passed || acceptance.changes_requested)) || (acceptance.changes_requested && code_review.approved)","to":"rework"},{"when":"code_review.failed || acceptance.failed","to":"failed"}]},
    {"id":"rework","requirements":[{"id":"rework_decision","pool":"managers","dispatch":"claim_one","inputs":["review_report","qa_report"],"produces":["rework_plan"],"outcomes":["retry","abandon"]}],"transitions":[{"when":"rework_decision.retry","to":"implementation"},{"when":"rework_decision.abandon","to":"failed"}]},
    {"id":"integration","requirements":[{"id":"integration","pool":"managers","dispatch":"claim_one","inputs":["implementation_plan","implementation_result","test_evidence","review_report","qa_report","previous_outputs"],"produces":["integration_report"],"outcomes":["integrated","rework","failed"]}],"transitions":[{"when":"integration.integrated","to":"done"},{"when":"integration.rework","to":"rework"},{"when":"integration.failed","to":"failed"}]},
    {"id":"done","terminal":true,"requirements":[],"transitions":[]},
    {"id":"failed","terminal":true,"requirements":[],"transitions":[]}
  ]
}'
CREATED="$(api POST /api/workflows "$(python3 -c 'import json,sys; print(json.dumps({"definition":json.loads(sys.argv[1])}))' "$DEFINITION")" | result)"
WF_ID="$(printf '%s' "$CREATED" | field id)"
api POST /api/workflows/workflow-e2e/versions/1/publish >/dev/null
api POST /api/task-queues '{"prefix":"DEV","name":"Workflow E2E","owners":["user:customer"],"responsible_agent":"manager"}' >/dev/null
for spec in 'managers manager' 'developers developer' 'reviewers reviewer' 'qa qa'; do
  pool="${spec%% *}"; agent="${spec#* }"
  api PATCH "/api/task-queues/DEV/pools/$pool" "{\"agents\":[\"$agent\"],\"revision\":0,\"idempotency_key\":\"pool-$pool\"}" >/dev/null
done
api PUT /api/task-queues/DEV/workflow "{\"workflow_version_id\":$WF_ID,\"revision\":0,\"idempotency_key\":\"bind-dev\"}" >/dev/null

echo "--- create managed task and complete intake"
TASK="$(api POST /api/tasks '{"queue":"DEV","title":"Managed workflow E2E","description":"exercise the durable development flow","priority":"P1","idempotency_key":"task-managed"}' | result)"
TASK_KEY="$(printf '%s' "$TASK" | field key)"
assert_field "$TASK" workflow_status intake
echo "--- managed tasks reject all legacy lifecycle mutations without state changes"
TASK_REVISION="$(printf '%s' "$TASK" | field revision)"
expect_api_error PATCH "/api/tasks/$TASK_KEY" "{\"status\":\"in_progress\",\"revision\":$TASK_REVISION}" workflow_managed
# Claim is agent-only at the HTTP authorization boundary; the service-level
# workflow test covers its workflow_managed guard with an agent actor.
expect_api_error POST "/api/tasks/$TASK_KEY/complete" "{\"revision\":$TASK_REVISION}" workflow_managed
UNCHANGED="$(workflow_view "$TASK_KEY")"
assert_field "$UNCHANGED" task.workflow_status intake
assert_field "$UNCHANGED" task.status open
assert_field "$UNCHANGED" task.revision "$TASK_REVISION"
MGR="$(claim_work manager claim-intake)"; MGR_ID="$(printf '%s' "$MGR" | field assignment.id)"
add_artifact manager "$MGR_ID" implementation_plan 'Implement and verify the isolated workflow.' intake-plan
complete_work manager "$MGR_ID" ready intake-ready
assert_field "$(workflow_view "$TASK_KEY")" task.workflow_status implementation

echo "--- claim implementation, restart daemon with active lease, and recover it"
DEV="$(claim_work developer claim-implementation-1)"; DEV_ID="$(printf '%s' "$DEV" | field assignment.id)"
DEV_ITER="$(printf '%s' "$DEV" | field assignment.lease_iteration)"
stop_daemon
start_daemon
for _ in $(seq 1 200); do
  [ -S "$RUNTIME/developer.sock" ] && RECOVERED="$(tools developer tasks work show "$DEV_ID" 2>/dev/null || true)" && [ -n "$RECOVERED" ] && break
  sleep 0.05
done
[ -n "${RECOVERED:-}" ] || fail "active assignment lease/tools socket not recovered after daemon restart"
assert_field "$RECOVERED" assignment.lease_iteration "$DEV_ITER"

echo "--- record an allowed channel observation without changing workflow status"
P="$(packet developer "$DEV_ID")"; fresh_revisions "$P"
tools developer tasks observe subscribe "$DEV_ID" metrics:api --reaction record_only \
  --task-revision "$TASK_REV" --assignment-revision "$ASSIGN_REV" --idempotency-key observe-metrics >/dev/null
BEFORE_STATUS="$(workflow_view "$TASK_KEY" | field task.workflow_status)"
sa message send --channel metrics:api --type alert --text 'latency high' >/dev/null
OBSERVED=0
for _ in $(seq 1 200); do
  VIEW="$(workflow_view "$TASK_KEY")"
  if [ "$(printf '%s' "$VIEW" | field observations)" != "[]" ]; then OBSERVED=1; break; fi
  sleep 0.05
done
[ "$OBSERVED" = 1 ] || fail "channel event was not correlated as a workflow observation"
assert_field "$VIEW" task.workflow_status "$BEFORE_STATUS"

echo "--- blocking universal question routes to manager and resumes developer"
P="$(packet developer "$DEV_ID")"; fresh_revisions "$P"
QUESTION="$(tools developer tasks ask "$DEV_ID" --question 'Which retry limit?' --context 'The acceptance criteria omit the retry count.' \
  --blocking-scope assignment --task-revision "$TASK_REV" --assignment-revision "$ASSIGN_REV" --idempotency-key ask-retries)"
Q_ID="$(printf '%s' "$QUESTION" | field id)"
assert_field "$(workflow_view "$TASK_KEY")" task.workflow_status implementation
ANSWER="$(claim_work manager claim-answer)"; ANSWER_ID="$(printf '%s' "$ANSWER" | field assignment.id)"
P="$(packet manager "$ANSWER_ID")"; fresh_revisions "$P"
tools manager tasks answer "$Q_ID" --assignment "$ANSWER_ID" --answer 'Use three retries.' \
  --task-revision "$TASK_REV" --assignment-revision "$ASSIGN_REV" --idempotency-key answer-retries >/dev/null
RESUMED="$(packet developer "$DEV_ID")"
assert_field "$RESUMED" questions.0.answer 'Use three retries.'

echo "--- implementation outputs, parallel review/QA join, and rework cycle"
add_artifact developer "$DEV_ID" implementation_result 'commit abc123' implementation-result-1
add_artifact developer "$DEV_ID" test_evidence 'go test ./...' implementation-tests-1
complete_work developer "$DEV_ID" implemented implementation-done-1
assert_field "$(workflow_view "$TASK_KEY")" task.workflow_status verification
REVIEW="$(claim_work reviewer claim-review-1)"; REVIEW_ID="$(printf '%s' "$REVIEW" | field assignment.id)"
QA="$(claim_work qa claim-qa-1)"; QA_ID="$(printf '%s' "$QA" | field assignment.id)"
add_artifact reviewer "$REVIEW_ID" review_report 'One important correction is required.' review-report-1
complete_work reviewer "$REVIEW_ID" changes_requested review-changes-1
assert_field "$(workflow_view "$TASK_KEY")" task.workflow_status verification
add_artifact qa "$QA_ID" qa_report 'Acceptance checks pass.' qa-report-1
complete_work qa "$QA_ID" passed qa-passed-1
assert_field "$(workflow_view "$TASK_KEY")" task.workflow_status rework

REWORK="$(claim_work manager claim-rework)"; REWORK_ID="$(printf '%s' "$REWORK" | field assignment.id)"
add_artifact manager "$REWORK_ID" rework_plan 'Address the reviewer finding only.' rework-plan
complete_work manager "$REWORK_ID" retry rework-retry
assert_field "$(workflow_view "$TASK_KEY")" task.workflow_status implementation

DEV2="$(claim_work developer claim-implementation-2)"; DEV2_ID="$(printf '%s' "$DEV2" | field assignment.id)"
add_artifact developer "$DEV2_ID" implementation_result 'commit def456' implementation-result-2
add_artifact developer "$DEV2_ID" test_evidence 'go test ./... passes' implementation-tests-2
complete_work developer "$DEV2_ID" implemented implementation-done-2
REVIEW2="$(claim_work reviewer claim-review-2)"; REVIEW2_ID="$(printf '%s' "$REVIEW2" | field assignment.id)"
QA2="$(claim_work qa claim-qa-2)"; QA2_ID="$(printf '%s' "$QA2" | field assignment.id)"
add_artifact reviewer "$REVIEW2_ID" review_report 'Approved.' review-report-2
complete_work reviewer "$REVIEW2_ID" approved review-approved-2
add_artifact qa "$QA2_ID" qa_report 'Passed.' qa-report-2
complete_work qa "$QA2_ID" passed qa-passed-2
assert_field "$(workflow_view "$TASK_KEY")" task.workflow_status integration

echo "--- integrate and reach terminal done"
INTEGRATE="$(claim_work manager claim-integration)"; INTEGRATE_ID="$(printf '%s' "$INTEGRATE" | field assignment.id)"
add_artifact manager "$INTEGRATE_ID" integration_report 'Integrated and final checks passed.' integration-report
complete_work manager "$INTEGRATE_ID" integrated integration-done
FINAL="$(workflow_view "$TASK_KEY")"
assert_field "$FINAL" task.workflow_status done
assert_field "$FINAL" task.status done

echo "--- failed result wins over a simultaneous changes-requested result"
MIXED="$(api POST /api/tasks '{"queue":"DEV","title":"Mixed verification result","priority":"P2","idempotency_key":"task-mixed"}' | result)"
MIXED_KEY="$(printf '%s' "$MIXED" | field key)"
MIX_MGR="$(claim_work manager mixed-intake)"; MIX_MGR_ID="$(printf '%s' "$MIX_MGR" | field assignment.id)"
add_artifact manager "$MIX_MGR_ID" implementation_plan 'Exercise mixed terminal precedence.' mixed-plan
complete_work manager "$MIX_MGR_ID" ready mixed-ready
MIX_DEV="$(claim_work developer mixed-dev)"; MIX_DEV_ID="$(printf '%s' "$MIX_DEV" | field assignment.id)"
add_artifact developer "$MIX_DEV_ID" implementation_result 'mixed result' mixed-result
add_artifact developer "$MIX_DEV_ID" test_evidence 'mixed evidence' mixed-evidence
complete_work developer "$MIX_DEV_ID" implemented mixed-implemented
MIX_REVIEW="$(claim_work reviewer mixed-review)"; MIX_REVIEW_ID="$(printf '%s' "$MIX_REVIEW" | field assignment.id)"
MIX_QA="$(claim_work qa mixed-qa)"; MIX_QA_ID="$(printf '%s' "$MIX_QA" | field assignment.id)"
add_artifact reviewer "$MIX_REVIEW_ID" review_report 'Fatal review failure.' mixed-review-report
complete_work reviewer "$MIX_REVIEW_ID" failed mixed-review-failed
add_artifact qa "$MIX_QA_ID" qa_report 'QA requests a change.' mixed-qa-report
complete_work qa "$MIX_QA_ID" changes_requested mixed-qa-changes
MIXED_FINAL="$(workflow_view "$MIXED_KEY")"
assert_field "$MIXED_FINAL" task.workflow_status failed
assert_field "$MIXED_FINAL" task.status done

echo "--- workflow wake outbox remains pending across a daemon restart and then publishes"
# Domain setup stays on the public REST path. SQLite is read-only here and only
# inspects the durability invariant; batching beyond the publisher's fixed
# per-tick limit deterministically leaves committed rows pending before restart.
MARKER="$(api POST /api/tasks '{"queue":"DEV","title":"Outbox synchronization marker","priority":"P3","idempotency_key":"outbox-marker"}' | result)"
MARKER_KEY="$(printf '%s' "$MARKER" | field key)"
for _ in $(seq 1 100); do
  [ "$(db_scalar "SELECT COUNT(*) FROM task_workflow_outbox WHERE task_id=(SELECT id FROM tasks WHERE task_key='$MARKER_KEY') AND published_at<>''")" = 1 ] && break
  sleep 0.05
done
[ "$(db_scalar "SELECT COUNT(*) FROM task_workflow_outbox WHERE task_id=(SELECT id FROM tasks WHERE task_key='$MARKER_KEY') AND published_at<>''")" = 1 ] || fail "outbox marker was not published"
BATCH_PIDS=()
for n in $(seq 1 130); do
  api POST /api/tasks "{\"queue\":\"DEV\",\"title\":\"Restart outbox $n\",\"priority\":\"P3\",\"idempotency_key\":\"restart-outbox-$n\"}" >/dev/null &
  BATCH_PIDS+=("$!")
done
for pid in "${BATCH_PIDS[@]}"; do wait "$pid"; done
stop_daemon
PENDING="$(db_scalar "SELECT COUNT(*) FROM task_workflow_outbox WHERE published_at=''")"
[ "$PENDING" -gt 0 ] || fail "expected committed pending workflow wakes before restart"
MESSAGES_BEFORE="$(db_scalar "SELECT COUNT(*) FROM messages WHERE type='workflow.assignment_ready'")"
start_daemon
for _ in $(seq 1 200); do
  [ "$(db_scalar "SELECT COUNT(*) FROM task_workflow_outbox WHERE published_at=''")" = 0 ] && break
  sleep 0.05
done
[ "$(db_scalar "SELECT COUNT(*) FROM task_workflow_outbox WHERE published_at=''")" = 0 ] || fail "pending workflow wakes were not marked published after restart"
MESSAGES_AFTER="$(db_scalar "SELECT COUNT(*) FROM messages WHERE type='workflow.assignment_ready'")"
[ "$MESSAGES_AFTER" -ge $((MESSAGES_BEFORE + PENDING)) ] || fail "restart published $((MESSAGES_AFTER - MESSAGES_BEFORE)) messages, want at least $PENDING"

echo "--- legacy queue retains current task claim/complete commands"
api POST /api/task-queues '{"prefix":"LEG","name":"Legacy E2E","owners":["user:customer","agent:manager"]}' >/dev/null
LEGACY="$(api POST /api/tasks '{"queue":"LEG","title":"Legacy task","priority":"P2","idempotency_key":"legacy-task"}' | result)"
LEGACY_KEY="$(printf '%s' "$LEGACY" | field key)"
CLAIMED="$(tools manager tasks ready --queue LEG --claim --idempotency-key legacy-claim)"
assert_field "$CLAIMED" status in_progress
LEGACY_REV="$(printf '%s' "$CLAIMED" | field revision)"
DONE="$(tools manager tasks done "$LEGACY_KEY" --revision "$LEGACY_REV")"
assert_field "$DONE" status done

echo "workflow e2e ok"
