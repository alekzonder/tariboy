#!/usr/bin/env bash
# E2E smoke: start a real tariboyd in a temp base dir, drive it with the
# real tariboy CLI, assert on output, shut down cleanly.
set -euo pipefail

BIN="$(cd "$(dirname "$0")/.." && pwd)/bin"
BASE="$(mktemp -d)"
RUNTIME="$(mktemp -d)"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export TARIBOY_BASE_DIR="$BASE"
export TARIBOY_RUNTIME_DIR="$RUNTIME"
trap 'kill "$DPID" 2>/dev/null || true; kill "${FAKE_PID:-}" 2>/dev/null || true; kill "${OTLP_PID:-}" 2>/dev/null || true; kill "${STORE_PID:-}" 2>/dev/null || true; rm -rf "${STORE_PULL_BASE:-}" "$BASE" "$RUNTIME"' EXIT

export TARIBOY_SHIM_BIN="$BIN/tariboy-shim"
export TARIBOY_STUB_HARNESS="$ROOT/scripts/stub-harness.sh"
chmod +x "$TARIBOY_STUB_HARNESS"

# Steerable fake-upstream reply text (read per-request by fake-upstream.py). Must
# be exported BEFORE the fake upstream launches so the subprocess inherits it; the
# file itself need not exist yet (absent => the fake serves its default "ok").
export FAKE_UPSTREAM_TEXTFILE="$BASE/fake_text.txt"

echo "--- ai proxy: start the fake upstream"
FAKE_PORT="$( { python3 "$ROOT/scripts/fake-upstream.py" & echo "FAKE_PID=$!" >&3; } 3>"$BASE/fakepid.env" | head -n1 )"
. "$BASE/fakepid.env"
export TARIBOY_UPSTREAM_ANTHROPIC_BASE_URL="http://127.0.0.1:${FAKE_PORT}"
export ANTHROPIC_API_KEY="e2e-dummy-real-key"   # the "real" key the proxy forwards; the fake ignores it

if command -v python3 >/dev/null; then
  echo "--- telemetry: start the fake OTLP collector"
  export TARIBOY_OTLP_HITFILE="$BASE/otlp-hits.txt"
  OTLP_PORT="$( { python3 "$ROOT/scripts/fake-otlp.py" & echo "OTLP_PID=$!" >&3; } 3>"$BASE/otlppid.env" | head -n1 )"
  . "$BASE/otlppid.env"
  export OTEL_EXPORTER_OTLP_ENDPOINT="127.0.0.1:${OTLP_PORT}"
fi

# Disable the embedded web listener for the smoke: it defaults to the fixed
# global port 127.0.0.1:8765, and a bind failure there is now fatal, so an
# isolated smoke must not depend on that port being free. The web UI is covered
# end-to-end by TestWebUIEndToEnd (internal/cli), on its own loopback port.
TARIBOY_SHELL_ENV=1 "$BIN/tariboyd" --base-dir "$BASE" --log-level error --web-addr "" &
DPID=$!

SOCK="$RUNTIME/tariboyd.sock"
for _ in $(seq 1 100); do
  [ -S "$SOCK" ] && break
  sleep 0.05
done
[ -S "$SOCK" ] || { echo "FAIL: socket never appeared"; exit 1; }

sa() { "$BIN/tariboy" --socket "$SOCK" "$@"; }

echo "--- daemon status"
sa daemon status | grep -q "version " || { echo "FAIL: status"; exit 1; }

echo "--- config set/get"
sa daemon config set log_level debug >/dev/null
sa daemon config get --key log_level | grep -q "log_level: debug" || { echo "FAIL: config"; exit 1; }

echo "--- json output"
sa daemon status --json | grep -q '"version"' || { echo "FAIL: --json"; exit 1; }

echo "--- help surfaces"
grep -q '"daemon"' <<<"$(sa --help-json)" || { echo "FAIL: --help-json"; exit 1; }
sa daemon config set --help | grep -q "Usage:" || { echo "FAIL: --help"; exit 1; }

echo "--- image build (CLI-local, no daemon needed)"
sa image build --name basic-example --tag latest --path "$ROOT/internal/builtinimages/source" | grep -q "digest:" \
  || { echo "FAIL: image build"; exit 1; }

echo "--- image ls"
sa image ls | grep -q "basic-example" || { echo "FAIL: image ls"; exit 1; }

echo "--- image prompt (i-am-done tail must be last)"
PROMPT="$(sa image prompt basic-example:latest)"
echo "$PROMPT" | grep -q "i-am-done" || { echo "FAIL: no i-am-done"; exit 1; }
echo "$PROMPT" | tail -n 6 | grep -q "i-am-done" || { echo "FAIL: tail not last"; exit 1; }
echo "$PROMPT" | grep -q "Update it before finishing each iteration" || { echo "FAIL: body missing"; exit 1; }

echo "--- image rm"
sa image rm basic-example:latest >/dev/null
if sa image ls | grep -q "basic-example"; then echo "FAIL: image not removed"; exit 1; fi

echo "--- image build for run"
sa image build --name basic-example --tag latest --path "$ROOT/internal/builtinimages/source" | grep -q "digest:" \
  || { echo "FAIL: image build"; exit 1; }
sa image build --name demo --tag latest --path "$ROOT/internal/builtinimages/source" | grep -q "digest:" \
  || { echo "FAIL: demo image build"; exit 1; }

echo "--- run agent with the stub harness"
sa agent run basic-example:latest --name smoke --harness stub | grep -q "name: smoke" \
  || { echo "FAIL: agent run"; exit 1; }

echo "--- agent ps shows the agent"
sa agent ps | grep -q "smoke" || { echo "FAIL: agent ps"; exit 1; }

echo "--- exec one manual iteration"
sa agent exec smoke >/dev/null || { echo "FAIL: agent exec"; exit 1; }

echo "--- wait for the iteration to reach done (stub calls i-am-done)"
DONE=0
for _ in $(seq 1 100); do
  if sa --json iteration ls smoke | grep -q '"status":"done"'; then DONE=1; break; fi
  sleep 0.1
done
[ "$DONE" = 1 ] || { echo "FAIL: iteration never reached done"; sa --json iteration ls smoke; exit 1; }

echo "--- iteration ls / inspect"
ITER_ID="$(sa --json iteration ls smoke | grep -o 'smoke-[0-9]*-[0-9]*' | head -n1)"
sa --json iteration inspect smoke "$ITER_ID" | grep -q '"done":true' \
  || { echo "FAIL: iteration inspect done flag"; exit 1; }

echo "--- bus: create agent B subscribed to a shared channel"
sa agent run basic-example:latest --name bob --harness stub --loop true \
  --env "STUB_SUBSCRIBE=chat:team,STUB_CALL_DONE=1" | grep -q "name: bob" \
  || { echo "FAIL: run bob"; exit 1; }
sa loop enable bob >/dev/null || { echo "FAIL: enable bob loop"; exit 1; }
sa agent start bob >/dev/null || { echo "FAIL: start bob"; exit 1; }
# First iteration subscribes bob to chat:team.
sa agent exec bob >/dev/null || { echo "FAIL: exec bob subscribe"; exit 1; }
SUBBED=0
for _ in $(seq 1 100); do
  if sa --json iteration ls bob | grep -q '"status":"done"'; then SUBBED=1; break; fi
  sleep 0.1
done
[ "$SUBBED" = 1 ] || { echo "FAIL: bob subscribe iteration"; exit 1; }

echo "--- bus: agent A publishes to chat:team"
sa agent run basic-example:latest --name alice --harness stub \
  --env "STUB_SEND=chat:team|hello-from-alice,STUB_CALL_DONE=1" | grep -q "name: alice" \
  || { echo "FAIL: run alice"; exit 1; }
BOB_BEFORE="$(sa --json iteration ls bob | grep -o '"id"' | wc -l)"
sa agent exec alice >/dev/null || { echo "FAIL: exec alice publish"; exit 1; }

echo "--- bus: channel tail shows the message with attribution"
TAILED=0
for _ in $(seq 1 100); do
  if sa --json channel tail chat:team | grep -q "hello-from-alice"; then TAILED=1; break; fi
  sleep 0.1
done
[ "$TAILED" = 1 ] || { echo "FAIL: channel tail"; sa --json channel tail chat:team; exit 1; }
sa --json channel tail chat:team | grep -q '"produced_by_agent":"alice"' \
  || { echo "FAIL: attribution missing"; exit 1; }

echo "--- bus: bob runs a message-triggered iteration"
WOKE=0
for _ in $(seq 1 100); do
  BOB_NOW="$(sa --json iteration ls bob | grep -o '"id"' | wc -l)"
  if [ "$BOB_NOW" -gt "$BOB_BEFORE" ]; then WOKE=1; break; fi
  sleep 0.1
done
[ "$WOKE" = 1 ] || { echo "FAIL: bob not woken by message"; sa --json iteration ls bob >&2; sa --json agent subscriptions bob >&2; sa --json agent ps >&2; exit 1; }
sa --json iteration ls bob | grep -q '"trigger":"message"' \
  || { echo "FAIL: bob iteration not message-triggered"; sa --json iteration ls bob; exit 1; }

echo "--- bus: channel ls lists chat:team and the agent channels"
sa channel ls | grep -q "chat:team" || { echo "FAIL: channel ls"; exit 1; }

echo "--- clean up bus agents"
sa agent rm alice --force --purge >/dev/null || { echo "FAIL: rm alice"; exit 1; }
sa agent rm bob --force --purge >/dev/null || { echo "FAIL: rm bob"; exit 1; }

# Opt-in wall-clock schedule check (timing-sensitive; off by default). A one-shot
# ~2s out fires as an inbox message that triggers an extra iteration.
if [ -n "${E2E_SLOW:-}" ]; then
  echo "--- schedule: agent arms a one-shot that wakes it (E2E_SLOW)"
  sa agent run basic-example:latest --name sched --harness stub --loop true \
    --plugins context,status,schedule \
    --env "STUB_SUBSCRIBE=agent:sched:inbox,STUB_SCHEDULE=2,STUB_CALL_DONE=1" | grep -q "name: sched" \
    || { echo "FAIL: run sched"; exit 1; }
  sa loop enable sched >/dev/null || { echo "FAIL: enable sched loop"; exit 1; }
  sa agent start sched >/dev/null || { echo "FAIL: start sched"; exit 1; }
  # First iteration subscribes to its own inbox and arms the one-shot.
  sa agent exec sched >/dev/null || { echo "FAIL: exec sched"; exit 1; }
  SCHED_ARMED=0
  for _ in $(seq 1 100); do
    if sa --json iteration ls sched | grep -q '"status":"done"'; then SCHED_ARMED=1; break; fi
    sleep 0.1
  done
  [ "$SCHED_ARMED" = 1 ] || { echo "FAIL: sched arm iteration"; exit 1; }
  SCHED_BEFORE="$(sa --json iteration ls sched | grep -o '"id"' | wc -l)"
  # Wait up to 5s for the schedule to fire and trigger an extra iteration.
  SCHED_FIRED=0
  for _ in $(seq 1 50); do
    SCHED_NOW="$(sa --json iteration ls sched | grep -o '"id"' | wc -l)"
    if [ "$SCHED_NOW" -gt "$SCHED_BEFORE" ]; then SCHED_FIRED=1; break; fi
    sleep 0.1
  done
  [ "$SCHED_FIRED" = 1 ] || { echo "FAIL: schedule did not wake sched"; sa --json iteration ls sched; exit 1; }
  sa --json iteration ls sched | grep -q '"trigger":"message"' \
    || { echo "FAIL: schedule iteration not message-triggered"; exit 1; }
  sa agent rm sched --force --purge >/dev/null || { echo "FAIL: rm sched"; exit 1; }
fi

echo "--- ai proxy: an iteration drives agent -> proxy -> fake upstream"
sa agent run basic-example:latest --name usagebot --harness stub \
  --env "STUB_AI=1,STUB_CALL_DONE=1" | grep -q "name: usagebot" \
  || { echo "FAIL: run usagebot"; exit 1; }
sa agent exec usagebot >/dev/null || { echo "FAIL: exec usagebot"; exit 1; }

echo "--- ai proxy: usage row recorded with attribution + tokens + cost"
RECORDED=0
for _ in $(seq 1 100); do
  if sa --json usage --agent usagebot | grep -q '"requests":1'; then RECORDED=1; break; fi
  sleep 0.1
done
[ "$RECORDED" = 1 ] || { echo "FAIL: usage not recorded"; sa --json usage --agent usagebot; exit 1; }
sa --json usage --agent usagebot | grep -q '"input_tokens":100' \
  || { echo "FAIL: token counts wrong"; sa --json usage --agent usagebot; exit 1; }
# cost = 100*5/1e6 (input) + 50*25/1e6 (output) = 0.0005 + 0.00125 = 0.00175
sa --json usage --agent usagebot | grep -q '"cost_usd":0.00175' \
  || { echo "FAIL: cost wrong (want 100*5/1e6 + 50*25/1e6 = 0.00175)"; sa --json usage --agent usagebot; exit 1; }

echo "--- ai proxy: proxy-transcript.jsonl exists for the iteration"
ITER_DIR="$(ls -d "${TARIBOY_BASE_DIR}/agents/usagebot/iterations/"*/ | head -n1)"
[ -f "${ITER_DIR}proxy-transcript.jsonl" ] || [ -f "${ITER_DIR}proxy-transcript.jsonl.gz" ] \
  || { echo "FAIL: proxy transcript missing in ${ITER_DIR}"; exit 1; }

echo "--- ai proxy: a tight block budget makes the next AI call fail"
sa budget set --scope agent:usagebot --limit-usd 0 --period 24h --mode block >/dev/null \
  || { echo "FAIL: budget set"; exit 1; }
sa agent exec usagebot >/dev/null || { echo "FAIL: exec usagebot 2"; exit 1; }
BLOCKED=0
for _ in $(seq 1 200); do
  if sa --json usage --agent usagebot | grep -q '"budget_block"' \
     || sa --json budget status | grep -q '"over":true' 2>/dev/null; then BLOCKED=1; break; fi
  sleep 0.1
done
[ "$BLOCKED" = 1 ] || { echo "FAIL: budget block not observed"; sa budget status; exit 1; }

echo "--- clean up ai proxy agent"
sa agent rm usagebot --force --purge >/dev/null || { echo "FAIL: rm usagebot"; exit 1; }

echo "--- telemetry: an iteration produced an OTLP trace export"
if command -v python3 >/dev/null; then
  TRACED=0
  for _ in $(seq 1 100); do
    if [ -f "$TARIBOY_OTLP_HITFILE" ] && grep -q '/v1/traces' "$TARIBOY_OTLP_HITFILE"; then TRACED=1; break; fi
    sleep 0.1
  done
  [ "$TRACED" = 1 ] || { echo "FAIL: no OTLP trace export captured"; cat "$TARIBOY_OTLP_HITFILE" 2>/dev/null; exit 1; }
fi

echo "--- retention: run an agent through two iterations under keep-1"
sa agent run basic-example:latest --name retbot --harness stub | grep -q "name: retbot" \
  || { echo "FAIL: run retbot"; exit 1; }
sa agent exec retbot >/dev/null || { echo "FAIL: exec retbot 1"; exit 1; }
for _ in $(seq 1 100); do sa --json iteration ls retbot | grep -q '"status":"done"' && break; sleep 0.1; done
sa agent exec retbot >/dev/null || { echo "FAIL: exec retbot 2"; exit 1; }
TWO=0
for _ in $(seq 1 100); do
  if [ "$(sa --json iteration ls retbot | grep -o '"id"' | wc -l)" -ge 2 ]; then TWO=1; break; fi
  sleep 0.1
done
[ "$TWO" = 1 ] || { echo "FAIL: retbot never had 2 iterations"; exit 1; }
OLD_ITER="$(sa --json iteration ls retbot | grep -o 'retbot-[0-9]*-[0-9]*' | head -n1)"

echo "--- retention: set keep 1 and prune"
sa retention set retbot --keep-iterations 1 --archive true >/dev/null || { echo "FAIL: retention set"; exit 1; }
sa --json retention get retbot | grep -q '"keep_iterations":1' || { echo "FAIL: retention get"; exit 1; }
sa --json prune retbot | grep -q "\"pruned\"" || { echo "FAIL: prune"; exit 1; }
REMAIN="$(sa --json iteration ls retbot | grep -o '"id"' | wc -l | tr -d ' ')"
[ "$REMAIN" = 1 ] || { echo "FAIL: prune did not reduce to 1 iteration (got $REMAIN)"; exit 1; }
[ -f "${TARIBOY_BASE_DIR}/agents/retbot/archive/${OLD_ITER}.tar.gz" ] \
  || { echo "FAIL: pruned iteration not archived"; ls "${TARIBOY_BASE_DIR}/agents/retbot/archive/" 2>/dev/null; exit 1; }
if [ -d "${TARIBOY_BASE_DIR}/agents/retbot/iterations/${OLD_ITER}" ]; then echo "FAIL: pruned iteration dir still present"; exit 1; fi

echo "--- backup: set a secret then back up (values must NOT travel)"
sa secret set retbot API_KEY e2e-sentinel-secret >/dev/null || { echo "FAIL: secret set"; exit 1; }
BK="$BASE/retbot.tar.gz"
sa backup retbot -o "$BK" | grep -q "sha256:" || { echo "FAIL: backup"; exit 1; }
[ -f "$BK" ] || { echo "FAIL: backup file missing"; exit 1; }
if zcat "$BK" 2>/dev/null | grep -a -q "e2e-sentinel-secret"; then echo "FAIL: secret value leaked into backup"; exit 1; fi

echo "--- restore: recreate under a new name"
sa restore "$BK" --name retclone >/dev/null || { echo "FAIL: restore"; exit 1; }
sa agent ps | grep -q "retclone" || { echo "FAIL: restored agent missing"; sa agent ps; exit 1; }

echo "--- clean up retention/backup agents"
sa agent rm retbot --force --purge >/dev/null || { echo "FAIL: rm retbot"; exit 1; }
sa agent rm retclone --force --purge >/dev/null || { echo "FAIL: rm retclone"; exit 1; }

echo "--- compose: groups end to end"
# A real image the compose agents run.
sa image build --name analyst --tag latest --path "$ROOT/internal/builtinimages/source" | grep -q "digest:" \
  || { echo "FAIL: compose image build"; exit 1; }

# The two members run the stub harness so message-triggered iterations complete
# with no real API key (delivery is observable as an iteration; see below). The
# non-lead (writer) also drives one AI call per iteration (STUB_AI) so the
# per-group budget can be enforced end to end against the fake upstream.
mkdir -p "$BASE/test-bin"
ln -s "$TARIBOY_STUB_HARNESS" "$BASE/test-bin/claude"
COMPOSE="$BASE/tariboy-compose.yaml"
cat > "$COMPOSE" <<YAML
version: 1
groups:
  research-team:
    lead: scout
    budget: { limit_usd: 50, period: 24h, mode: enforce }
agents:
  scout:
    image: analyst:latest
    group: research-team
    harness: { type: claude }
    env: { PATH: "$BASE/test-bin:$PATH", STUB_CALL_DONE: "1" }
  writer:
    image: analyst:latest
    group: research-team
    harness: { type: claude }
    env: { PATH: "$BASE/test-bin:$PATH", STUB_CALL_DONE: "1", STUB_AI: "1" }
YAML

# up: reconcile desired -> actual.
"$BIN/tariboy" --socket "$SOCK" compose -f "$COMPOSE" up \
  || { echo "FAIL: compose up"; exit 1; }

# Group exists with the right lead and both members.
sa --json group inspect research-team | grep -q '"lead":"scout"' \
  || { echo "FAIL: group lead"; exit 1; }
sa --json group inspect research-team | grep -q '"scout"' \
  || { echo "FAIL: group member scout"; exit 1; }
sa --json group inspect research-team | grep -q '"writer"' \
  || { echo "FAIL: group member writer"; exit 1; }

# The group broadcast + inbox channels exist.
sa --json channel ls | grep -q 'group:research-team:broadcast' \
  || { echo "FAIL: broadcast channel missing"; exit 1; }
sa --json channel ls | grep -q 'group:research-team:inbox' \
  || { echo "FAIL: inbox channel missing"; exit 1; }

# The group budget is applied (compose group budget -> group:<name> scope).
sa --json budget ls | grep -q 'group:research-team' \
  || { echo "FAIL: group budget not applied"; exit 1; }

# status shows no drift after up (checked before we mutate any budget below).
"$BIN/tariboy" --socket "$SOCK" compose -f "$COMPOSE" status | grep -q "drift: 0" \
  || { echo "FAIL: compose status drift"; exit 1; }

# --- Discriminant lead routing.
# Both members are loop-enabled and event-only (no interval), so an iteration
# can ONLY appear from a message delivery: iteration counts are a faithful proxy
# for "did this member receive the message". The daemon wakes exactly the agents
# a publish was delivered to, so these counts distinguish correct routing from a
# mis-subscription. (`|| true` masks grep's no-match exit under pipefail so an
# empty count reads as 0 instead of aborting the script.)
witers() { sa --json iteration ls writer | { grep -o '"id"' || true; } | wc -l | tr -d ' '; }
sciters() { sa --json iteration ls scout | { grep -o '"id"' || true; } | wc -l | tr -d ' '; }

# Baseline: nobody has run.
[ "$(witers)" = 0 ] || { echo "FAIL: writer ran before any publish"; sa --json iteration ls writer; exit 1; }
[ "$(sciters)" = 0 ] || { echo "FAIL: scout ran before any publish"; sa --json iteration ls scout; exit 1; }

# Publish to the group INBOX: only the lead (scout) subscribes to it.
sa message send -c group:research-team:inbox --type task --text "triage this" >/dev/null \
  || { echo "FAIL: send to group inbox"; exit 1; }
# The lead wakes with a message-triggered iteration.
LEAD_WOKE=0
for _ in $(seq 1 100); do
  if sa --json iteration ls scout | grep -q '"trigger":"message"'; then LEAD_WOKE=1; break; fi
  sleep 0.1
done
[ "$LEAD_WOKE" = 1 ] || { echo "FAIL: lead did not receive the group inbox"; sa --json iteration ls scout; exit 1; }
# DISCRIMINANT: the non-lead must NOT receive the inbox. Waking is delivery-
# scoped, so once the lead has processed the message a non-lead subscribed by
# mistake would already have woken too. Grace, then assert writer is untouched.
sleep 0.5
[ "$(witers)" = 0 ] \
  || { echo "FAIL: non-lead received the group inbox (lead routing broken)"; sa --json iteration ls writer; exit 1; }

# Publish to BROADCAST: both members subscribe, so both must wake.
SC_BEFORE="$(sciters)"
sa message send -c group:research-team:broadcast --type note --text "hello team" >/dev/null \
  || { echo "FAIL: send to broadcast"; exit 1; }
# DISCRIMINANT: the non-lead (writer), which had ZERO iterations, now wakes.
W_WOKE=0
for _ in $(seq 1 100); do
  if [ "$(witers)" -ge 1 ] && sa --json iteration ls writer | grep -q '"trigger":"message"'; then W_WOKE=1; break; fi
  sleep 0.1
done
[ "$W_WOKE" = 1 ] || { echo "FAIL: broadcast did not reach the non-lead"; sa --json iteration ls writer; exit 1; }
# The lead also receives broadcast (its iteration count strictly increases).
SC_UP=0
for _ in $(seq 1 100); do
  if [ "$(sciters)" -gt "$SC_BEFORE" ]; then SC_UP=1; break; fi
  sleep 0.1
done
[ "$SC_UP" = 1 ] || { echo "FAIL: broadcast did not reach the lead"; sa --json iteration ls scout; exit 1; }

# --- Discriminant per-group budget enforcement (red/green) via the fake upstream.
# GREEN: under the group's 50 USD budget, a member's AI call reaches the upstream
# (usage request count strictly increases) and is NOT blocked.
wreq() {
  local r
  r="$(sa --json usage --agent writer 2>/dev/null | python3 -c '
import json
import sys

try:
    print(int(json.load(sys.stdin).get("total_requests", 0)))
except (json.JSONDecodeError, TypeError, ValueError):
    print(0)
')" || r=0
  echo "${r:-0}"
}
WREQ_BEFORE="$(wreq)"
sa agent exec writer >/dev/null || { echo "FAIL: exec writer (under budget)"; exit 1; }
PASSED=0
for _ in $(seq 1 100); do
  if [ "$(wreq)" -gt "$WREQ_BEFORE" ]; then PASSED=1; break; fi
  sleep 0.1
done
[ "$PASSED" = 1 ] || { echo "FAIL: under-budget member AI call did not reach the upstream"; sa --json usage --agent writer; exit 1; }
if sa --json logs writer | grep -q 'budget_block'; then
  echo "FAIL: member was blocked while under the group budget"; sa --json logs writer; exit 1
fi

# The enforced group budget is now visible in `budget status` too: with spend
# incurred, the group:research-team scope appears (member-aggregate spend), so an
# operator can see the group's spend-vs-limit that member requests are checked
# against -- not just `budget ls`.
sa --json budget status | grep -q 'group:research-team' \
  || { echo "FAIL: group budget scope not shown in budget status"; sa --json budget status; exit 1; }

# RED: tighten the GROUP budget to 0 (block). Enforcement aggregates member
# spend across the group and rejects the member's request at the proxy, recording
# a budget_block audit event -- the faithful signal for the *enforced* block,
# which lags the live `budget status` view by the ~15s budget-cache refresh. Drive
# the member's AI call until that refresh picks up the new limit.
sa budget set --scope group:research-team --limit-usd 0 --period 24h --mode block >/dev/null \
  || { echo "FAIL: set tight group budget"; exit 1; }
BLOCKED=0
for _ in $(seq 1 25); do
  sa agent exec writer >/dev/null 2>&1 || true
  sleep 0.8
  if sa --json logs writer | grep -q 'budget_block'; then BLOCKED=1; break; fi
done
[ "$BLOCKED" = 1 ] || { echo "FAIL: over-budget member request was NOT blocked"; sa --json logs writer; exit 1; }
# The block was attributed to the GROUP scope (not an agent/global budget).
sa --json logs writer | grep 'budget_block' | grep -q 'group:research-team' \
  || { echo "FAIL: block not attributed to the group scope"; sa --json logs writer; exit 1; }

# down --volumes removes agents, the group, and the shared dir.
"$BIN/tariboy" --socket "$SOCK" compose -f "$COMPOSE" down --volumes \
  || { echo "FAIL: compose down"; exit 1; }
sa --json group ls | grep -q '"count":0' \
  || { echo "FAIL: group not removed by down"; sa --json group ls; exit 1; }
[ ! -d "$BASE/groups/research-team" ] \
  || { echo "FAIL: shared dir survived down --volumes"; exit 1; }
if sa --json agent ps | grep -q '"scout"'; then echo "FAIL: agent scout survived down"; sa --json agent ps; exit 1; fi
if sa --json agent ps | grep -q '"writer"'; then echo "FAIL: agent writer survived down"; sa --json agent ps; exit 1; fi

echo "--- compose: groups OK"

if command -v python3 >/dev/null; then
  echo "--- rules: model-policy deny returns 403 (model_denied audit)"
  sa rule set --scope agent:denybot --kind model-policy --deny 'claude-opus-*' >/dev/null \
    || { echo "FAIL: rule set deny"; exit 1; }
  sa agent run basic-example:latest --name denybot --harness stub \
    --env "STUB_AI=1,STUB_CALL_DONE=1" | grep -q "name: denybot" \
    || { echo "FAIL: run denybot"; exit 1; }
  DENIED=0
  for _ in $(seq 1 100); do
    sa agent exec denybot >/dev/null 2>&1 || true
    if sa --json logs denybot | grep -q 'model_denied'; then DENIED=1; break; fi
    sleep 0.2
  done
  [ "$DENIED" = 1 ] || { echo "FAIL: model-policy deny not enforced"; sa --json logs denybot; exit 1; }
  # DISCRIMINANT: an allowed model (no deny match) still reaches the upstream and
  # is accounted -- proves the rule blocks the denied model specifically, not all
  # traffic. A different scope's request under a non-matching model must pass.
  sa agent run basic-example:latest --name allowbot --harness stub \
    --env "STUB_AI=1,STUB_CALL_DONE=1" | grep -q "name: allowbot" \
    || { echo "FAIL: run allowbot"; exit 1; }
  sa agent exec allowbot >/dev/null || { echo "FAIL: exec allowbot"; exit 1; }
  ALLOWED=0
  for _ in $(seq 1 100); do
    if sa --json usage --agent allowbot | grep -q '"requests":1'; then ALLOWED=1; break; fi
    sleep 0.1
  done
  [ "$ALLOWED" = 1 ] || { echo "FAIL: allowed model did not reach upstream"; sa --json usage --agent allowbot; exit 1; }
  sa --json logs allowbot | grep -q 'model_denied' \
    && { echo "FAIL: allowed model was wrongly denied"; sa --json logs allowbot; exit 1; } || true
  sa agent stop denybot >/dev/null 2>&1 || true
  sa agent stop allowbot >/dev/null 2>&1 || true

  echo "--- rules: model-policy route rewrites the requested model"
  sa rule set --scope agent:routebot --kind model-policy --route 'claude-sonnet-4' >/dev/null \
    || { echo "FAIL: rule set route"; exit 1; }
  sa agent run basic-example:latest --name routebot --harness stub \
    --env "STUB_AI=1,STUB_CALL_DONE=1" | grep -q "name: routebot" \
    || { echo "FAIL: run routebot"; exit 1; }
  sa agent exec routebot >/dev/null || { echo "FAIL: exec routebot"; exit 1; }
  # DISCRIMINANT: the stub requests claude-opus-4-8; the route rule rewrites it to
  # claude-sonnet-4 before forward, so the transcript's recorded request body (and
  # the fake's echo) carry claude-sonnet-4. Without the rewrite, claude-sonnet-4
  # never appears anywhere (only claude-opus-4-8 would) -- so this grep proves the
  # USED model actually changed, not merely that the request succeeded.
  ROUTED=0
  for _ in $(seq 1 100); do
    ID="$(ls -d "${TARIBOY_BASE_DIR}/agents/routebot/iterations/"*/ 2>/dev/null | head -n1)"
    if [ -n "$ID" ]; then
      TF="${ID}proxy-transcript.jsonl"
      if [ -f "$TF.gz" ]; then
        gunzip -c "$TF.gz" 2>/dev/null | grep -q 'claude-sonnet-4' && { ROUTED=1; break; }
      elif [ -f "$TF" ]; then
        grep -q 'claude-sonnet-4' "$TF" && { ROUTED=1; break; }
      fi
    fi
    sleep 0.2
  done
  [ "$ROUTED" = 1 ] || { echo "FAIL: model-policy route did not rewrite the model"; exit 1; }
  sa agent stop routebot >/dev/null 2>&1 || true

  echo "--- rules: rate-limit returns 429 once the window is exceeded"
  sa rule set --scope agent:rlbot --kind rate-limit --max-requests 1 --window-s 3600 >/dev/null \
    || { echo "FAIL: rule set rate-limit"; exit 1; }
  sa agent run basic-example:latest --name rlbot --harness stub \
    --env "STUB_AI=1,STUB_CALL_DONE=1" | grep -q "name: rlbot" \
    || { echo "FAIL: run rlbot"; exit 1; }
  # GREEN: the first call is under the limit; drive it and confirm it reached the
  # upstream (was accounted). A rate-limit that rejected from the first request
  # would fail this -- it asserts the under-limit direction succeeds.
  sa agent exec rlbot >/dev/null || { echo "FAIL: exec rlbot (first)"; exit 1; }
  FIRST=0
  for _ in $(seq 1 100); do
    if sa --json usage --agent rlbot | grep -q '"requests":1'; then FIRST=1; break; fi
    sleep 0.1
  done
  [ "$FIRST" = 1 ] || { echo "FAIL: first rlbot call not accounted"; sa --json usage --agent rlbot; exit 1; }
  sa --json logs rlbot | grep -q 'rate_limited' \
    && { echo "FAIL: rlbot rate-limited while still under the limit"; sa --json logs rlbot; exit 1; } || true
  # RED: the 15s policy refresh then recounts (count>=1 >= max 1) and the next
  # calls 429 with a rate_limited audit. Bounded polling (not a fixed sleep) drives
  # the call until the refresh picks up the count -- matches the budget-block timing.
  LIMITED=0
  for _ in $(seq 1 30); do
    sa agent exec rlbot >/dev/null 2>&1 || true
    sleep 0.8
    if sa --json logs rlbot | grep -q 'rate_limited'; then LIMITED=1; break; fi
  done
  [ "$LIMITED" = 1 ] || { echo "FAIL: rate-limit not enforced"; sa --json logs rlbot; exit 1; }
  sa agent stop rlbot >/dev/null 2>&1 || true

  echo "--- rules: rule ls shows the surviving rules, rm removes one"
  RID="$(sa --json rule ls | python3 -c 'import json,sys; rows=json.load(sys.stdin).get("rules",[]); print(rows[0]["id"] if rows else "")')"
  [ -n "$RID" ] || { echo "FAIL: no rule id from rule ls"; sa --json rule ls; exit 1; }
  sa rule rm "$RID" >/dev/null || { echo "FAIL: rule rm"; exit 1; }
  sa --json rule ls | grep -q "\"id\":\"$RID\"" \
    && { echo "FAIL: rule survived rm"; exit 1; } || true
  for agent in denybot allowbot routebot rlbot; do
    sa agent rm "$agent" --force --purge >/dev/null || { echo "FAIL: purge $agent"; exit 1; }
  done
  echo "--- rules: OK"
fi


echo "--- stop and remove the agent"
sa agent stop smoke >/dev/null || { echo "FAIL: agent stop"; exit 1; }
sa agent rm smoke --force --purge >/dev/null || { echo "FAIL: agent rm"; exit 1; }
if sa --json agent ps | grep -q '"name":"smoke"'; then echo "FAIL: agent not removed"; sa --json agent ps >&2; exit 1; fi

echo "--- graceful shutdown"
kill -TERM "$DPID"
wait "$DPID" || { echo "FAIL: daemon exited non-zero"; exit 1; }

echo "--- tariboy-store: TLS push/pull round-trip"
echo "--- store: use the previously built demo:latest for the round-trip"

STORE_DIR="$BASE/store-data"
STORE_CERT="$BASE/store-cert"
STORE_PULL_BASE="$(mktemp -d)"
mkdir -p "$STORE_DIR" "$STORE_CERT"
# Self-signed cert (SAN 127.0.0.1) via the stdlib helper (no openssl dependency).
( cd "$ROOT" && export PATH="$HOME/.local/go-toolchain/bin:$PATH" && go run ./scripts/gen-cert "$STORE_CERT" )
# Bootstrap readwrite token.
STORE_TOKEN="store-e2e-token"
printf '%s' "$STORE_TOKEN" > "$BASE/store-token"

"$BIN/tariboy-store" \
  --addr 127.0.0.1:8444 \
  --data-dir "$STORE_DIR" \
  --tls-cert "$STORE_CERT/cert.pem" \
  --tls-key "$STORE_CERT/key.pem" \
  --token-file "$BASE/store-token" &
STORE_PID=$!

# Wait until the store answers TLS (bounded).
STORE_UP=0
for _ in $(seq 1 100); do
  if curl -s --cacert "$STORE_CERT/cert.pem" "https://127.0.0.1:8444/v1/images" >/dev/null 2>&1; then STORE_UP=1; break; fi
  sleep 0.1
done
[ "$STORE_UP" = 1 ] || { echo "FAIL: tariboy-store never came up"; kill "$STORE_PID" 2>/dev/null; exit 1; }

# The image built earlier in the e2e (demo:latest) lives under $TARIBOY_BASE_DIR/images.
sa() { "$BIN/tariboy" "$@"; }

echo "--- store: login (token via stdin, never argv)"
printf '%s' "$STORE_TOKEN" | sa login https://127.0.0.1:8444 --ca "$STORE_CERT/cert.pem" \
  | grep -q "logged_in: true" || { echo "FAIL: store login"; kill "$STORE_PID" 2>/dev/null; exit 1; }

echo "--- store: push demo:latest"
sa push demo:latest --registry https://127.0.0.1:8444 | grep -Eq "pushed: true|skipped: true" \
  || { echo "FAIL: store push"; kill "$STORE_PID" 2>/dev/null; exit 1; }

echo "--- store: pull into a fresh base dir and verify digest"
SRC_DIGEST="$(cat "$TARIBOY_BASE_DIR/images/demo/latest.digest" | tr -d '[:space:]')"
# The login credentials live under the original base dir; the pull base dir has no
# registries.json, so copy the credentials across for the fresh-dir pull to auth.
mkdir -p "$STORE_PULL_BASE"
cp "$TARIBOY_BASE_DIR/registries.json" "$STORE_PULL_BASE/registries.json"
TARIBOY_BASE_DIR="$STORE_PULL_BASE" sa pull demo:latest --registry https://127.0.0.1:8444 \
  | grep -q "pulled: true" || { echo "FAIL: store pull"; kill "$STORE_PID" 2>/dev/null; exit 1; }

PULLED_DIGEST="$(cat "$STORE_PULL_BASE/images/demo/latest.digest" | tr -d '[:space:]')"
[ "$PULLED_DIGEST" = "$SRC_DIGEST" ] || { echo "FAIL: pulled digest $PULLED_DIGEST != source $SRC_DIGEST"; kill "$STORE_PID" 2>/dev/null; exit 1; }
echo "--- store: digest matches end to end ($SRC_DIGEST)"
kill "$STORE_PID" 2>/dev/null || true
rm -rf "$STORE_PULL_BASE"

echo "E2E OK"
