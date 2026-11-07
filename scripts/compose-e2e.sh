#!/usr/bin/env bash
# Isolated end-to-end proof of `tariboy compose`: a full version 2 file with
# every section is validated, previewed, applied, applied again, changed,
# broken by an unavailable Store, refused by its tariboy range, packed, and
# unpacked on a second daemon. Two git Stores are local bare repositories that
# git reaches through url.insteadOf, so nothing uses the network. Each daemon
# owns its base and runtime directories and has no HTTP listener: the script
# never touches ~/.tariboy, ~/.tariboyd, or 127.0.0.1:9990.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
source "$ROOT/scripts/test-image-fixture.sh"
BIN="$ROOT/bin"
FIXTURE="$ROOT/scripts/testdata/compose-e2e"
SANDBOX="$(mktemp -d)"
BASE="$SANDBOX/base"
RUNTIME="$SANDBOX/runtime"
BASE2="$SANDBOX/base2"
RUNTIME2="$SANDBOX/runtime2"
SOCK="$RUNTIME/tariboyd.sock"
SOCK2="$RUNTIME2/tariboyd.sock"
PROJECT="$SANDBOX/project"
CLI_LOG="$SANDBOX/cli.log"
DAEMON_LOG="$SANDBOX/daemon.log"
DAEMON2_LOG="$SANDBOX/daemon2.log"
mkdir -p "$BASE" "$RUNTIME" "$BASE2" "$RUNTIME2" "$SANDBOX/bin"
: >"$CLI_LOG"
DPID=""
DPID2=""
STARTED_AT=$SECONDS

export TARIBOY_ONBOARDING=off
export TARIBOY_NO_AUTOSTART=1
unset TARIBOY_TOOLS_SOCKET TARIBOY_DAEMON_SOCKET TARIBOY_BASE_DIR TARIBOY_RUNTIME_DIR
# git of the daemons and of this script reads only this config: it maps the
# https Store URLs to the local bare repositories.
export GIT_CONFIG_GLOBAL="$SANDBOX/gitconfig"
export GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=compose-e2e GIT_AUTHOR_EMAIL=compose-e2e@example.invalid
export GIT_COMMITTER_NAME=compose-e2e GIT_COMMITTER_EMAIL=compose-e2e@example.invalid
TEAM_URL="https://store.e2e.invalid/team.git"
EXTRA_URL="https://store.e2e.invalid/extra.git"
printf '[url "file://%s/team.git"]\n\tinsteadOf = %s\n[url "file://%s/extra.git"]\n\tinsteadOf = %s\n' \
  "$SANDBOX" "$TEAM_URL" "$SANDBOX" "$EXTRA_URL" >"$GIT_CONFIG_GLOBAL"
# The image skill bridge probes `claude --version`; the host may have no claude.
printf '%s\n' '#!/bin/sh' "echo '9.9.9 (Claude Code)'" >"$SANDBOX/bin/claude"
chmod 0700 "$SANDBOX/bin/claude"
export PATH="$SANDBOX/bin:$PATH"

# The secret values the file reads with from_env. Step 9 searches every CLI
# output and the archive for them.
export COMPOSE_E2E_AGENT_TOKEN="compose-e2e-agent-secret-$$-$RANDOM"
export COMPOSE_E2E_QUEUE_TOKEN="compose-e2e-queue-secret-$$-$RANDOM"

# kill_sandbox_processes kills, as a last resort, every process that still
# belongs to this sandbox: the PIDs recorded under the runtime directories and
# every process whose command line or environment names the sandbox.
kill_sandbox_processes() {
  local pids=() pid file
  while IFS= read -r file; do
    pid="$(tr -dc '0-9' <"$file" 2>/dev/null)"
    [ -n "$pid" ] && pids+=("$pid")
  done < <(find "$RUNTIME" "$RUNTIME2" -name '*.pid' -type f 2>/dev/null)
  while IFS= read -r pid; do
    pids+=("$pid")
  done < <(pgrep -f "$SANDBOX" 2>/dev/null)
  for file in /proc/[0-9]*/environ; do
    [ -r "$file" ] && grep -qsF "$SANDBOX" "$file" && pids+=("$(basename "$(dirname "$file")")")
  done
  for pid in "${pids[@]}"; do
    [ "$pid" = "$$" ] || [ "$pid" = "${BASHPID:-}" ] && continue
    kill -9 "$pid" 2>/dev/null
  done
  return 0
}

stop_pid() {
  local pid="$1"
  [ -n "$pid" ] || return 0
  kill "$pid" 2>/dev/null
  for _ in $(seq 1 100); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill -9 "$pid" 2>/dev/null
  wait "$pid" 2>/dev/null
  return 0
}

cleanup() {
  local code=$?
  set +e
  stop_pid "$DPID"
  stop_pid "$DPID2"
  kill_sandbox_processes
  if [ "$code" -ne 0 ]; then
    for log in "$DAEMON_LOG" "$DAEMON2_LOG"; do
      [ -s "$log" ] || continue
      echo "--- last lines of $(basename "$log")" >&2
      tail -n 40 "$log" >&2
    done
  fi
  chmod -R u+w "$SANDBOX" 2>/dev/null
  rm -rf "$SANDBOX"
}
trap cleanup EXIT
trap 'exit 130' INT TERM
fail() { echo "FAIL [$STEP]: $*" >&2; exit 1; }
STEP="setup"
step() { STEP="$1"; echo "--- $1"; }

# start_daemon BASE SOCK LOG starts an isolated daemon and prints its PID.
start_daemon() {
  local base="$1" sock="$2" log="$3" pid
  TARIBOY_BASE_DIR="$base" TARIBOY_RUNTIME_DIR="$(dirname "$sock")" \
    "$BIN/tariboyd" --base-dir "$base" --http-addr "" --log-level warn >>"$log" 2>&1 &
  pid=$!
  for _ in $(seq 1 200); do
    if [ -S "$sock" ] && "$BIN/tariboy" --socket "$sock" agent ps >/dev/null 2>&1; then
      START_PID="$pid"
      return
    fi
    sleep 0.05
  done
  fail "isolated daemon at $base did not become ready"
}

# capture CMD... runs a command that may fail, keeps OUT, ERR and CODE, and
# appends both streams to the CLI log that step 9 searches.
capture() {
  local out="$SANDBOX/capture.out" err="$SANDBOX/capture.err"
  set +e
  "$@" >"$out" 2>"$err"
  CODE=$?
  set -e
  OUT="$(cat "$out")"
  ERR="$(cat "$err")"
  { printf '$ %s\n' "$*"; cat "$out" "$err"; } >>"$CLI_LOG"
}
# t1 and t2 run the tariboy CLI against the first and the second daemon.
t1() { TARIBOY_BASE_DIR="$BASE" TARIBOY_RUNTIME_DIR="$RUNTIME" "$BIN/tariboy" --socket "$SOCK" "$@"; }
t2() { TARIBOY_BASE_DIR="$BASE2" TARIBOY_RUNTIME_DIR="$RUNTIME2" "$BIN/tariboy" --socket "$SOCK2" "$@"; }
contains() { [[ "$1" == *"$2"* ]]; }
# has_line TEXT PREFIX succeeds when a line of TEXT starts with PREFIX.
has_line() {
  local line
  while IFS= read -r line; do
    [[ "$line" == "$2"* ]] && return 0
  done <<<"$1"
  return 1
}

tt1() { TARIBOY_BASE_DIR="$BASE" TARIBOY_RUNTIME_DIR="$RUNTIME" "$BIN/tariboy-tasks" "$@"; }
tt2() { TARIBOY_BASE_DIR="$BASE2" TARIBOY_RUNTIME_DIR="$RUNTIME2" "$BIN/tariboy-tasks" "$@"; }
# field PATH reads a dotted path (list indexes allowed) from JSON on stdin.
field() {
  python3 -I -c 'import json,sys
x=json.load(sys.stdin)
for part in sys.argv[1].split("."):
    x=x[int(part)] if isinstance(x,list) else x[part]
print(json.dumps(x,separators=(",",":"),sort_keys=True) if isinstance(x,(dict,list)) else str(x).lower() if isinstance(x,bool) else x)' "$1"
}
# use VARIANT makes the project's Tariboycompose.yaml the fixture variant
# (base, changed, extra or range).
use() {
  local src="$FIXTURE/Tariboycompose.yaml"
  [ "$1" = base ] || src="$FIXTURE/Tariboycompose.$1.yaml"
  if [ "$1" = range ]; then
    # The working extra file with a minimum no daemon satisfies.
    sed 's/^tariboy: .*/tariboy: {min: 999.0.0}/' "$FIXTURE/Tariboycompose.extra.yaml" >"$PROJECT/Tariboycompose.yaml"
  else
    cp "$src" "$PROJECT/Tariboycompose.yaml"
  fi
}
# snapshot NAME writes the state of the first daemon that compose manages to
# $SANDBOX/NAME.json as sorted JSON, without the fields that change by
# themselves (times, PIDs, counters). Equal snapshots before and after a step
# prove that the step changed nothing.
snapshot() {
  local dir="$SANDBOX/snap"
  rm -rf "$dir"
  mkdir -p "$dir"
  t1 --json store list >"$dir/stores.json" || fail "snapshot: store list"
  t1 --json image ls >"$dir/images.json" || fail "snapshot: image ls"
  t1 --json workflow ls >"$dir/workflows.json" || fail "snapshot: workflow ls"
  t1 --json agent ps >"$dir/agents.json" || fail "snapshot: agent ps"
  t1 --json group ls >"$dir/groups.json" || fail "snapshot: group ls"
  tt1 queue list --json >"$dir/queues.json" || fail "snapshot: queue list"
  # DEV and its binding exist only after the first up.
  tt1 queue get DEV --json >"$dir/queue-DEV.json" 2>/dev/null || rm -f "$dir/queue-DEV.json"
  tt1 queue workflow get DEV --json >"$dir/queue-DEV-workflow.json" 2>/dev/null || rm -f "$dir/queue-DEV-workflow.json"
  python3 -I - "$dir" >"$SANDBOX/$1.json" <<'PY' || fail "snapshot: unreadable JSON"
import json, os, re, sys
volatile = re.compile(r"(_at|_ago|^at$|time|uptime|pid|^last|^next|elapsed|duration|iteration|revision|seq)", re.I)
def clean(x):
    if isinstance(x, dict):
        return {k: clean(v) for k, v in sorted(x.items()) if not volatile.search(k)}
    if isinstance(x, list):
        return [clean(v) for v in x]
    return x
d = sys.argv[1]
out = {}
for name in sorted(os.listdir(d)):
    with open(os.path.join(d, name)) as f:
        out[name] = clean(json.load(f))
print(json.dumps(out, sort_keys=True, indent=1))
PY
}
# report_lines prints the outcome lines of the up report in OUT; the progress
# lines before the report are left out.
report_lines() { grep -E '^(ok|changed|failed|skipped) ' <<<"$OUT" || true; }
# report_has OUTCOME RESOURCE succeeds when the up report has that line.
report_has() { has_line "$OUT" "$(printf '%-8s %s' "$1" "$2")"; }
# report_order prints the resource kinds of the report lines in order, once
# per change of kind.
report_kinds() {
  awk '$1 ~ /^(ok|changed|failed|skipped)$/ { split($2, a, ":"); if (a[1] != last) { printf "%s ", a[1]; last = a[1] } }' <<<"$OUT"
}

# make_store_image DIR VERSION writes the test image with an image_version.
make_store_image() {
  make_test_image_fixture "$1"
  printf 'image_version: %s\n' "$2" >>"$1/Tariboyfile.yaml"
}

[ -x "$BIN/tariboyd" ] && [ -x "$BIN/tariboy" ] && [ -x "$BIN/tariboy-tasks" ] || fail "build the binaries first: make build"

step "prepare the Stores and the project, start an isolated daemon (base=$BASE, no HTTP listener)"
mkdir -p "$PROJECT"
cp -R "$FIXTURE/." "$PROJECT/"
rm -rf "$PROJECT/store"
# The git Store "team": tag v1 gives developer 1.0.0 and development 1.0.0,
# tag v2 gives both at 1.1.0.
SEED="$SANDBOX/seed-team"
mkdir -p "$SEED"
cp -R "$FIXTURE/store/." "$SEED/"
make_store_image "$SEED/images/developer" 1.0.0
git -C "$SEED" init -q -b main
git -C "$SEED" add -A
git -C "$SEED" commit -q -m v1
git -C "$SEED" tag v1
sed -i 's/^image_version: 1.0.0$/image_version: 1.1.0/' "$SEED/images/developer/Tariboyfile.yaml"
sed -i 's/^workflow_version: 1.0.0$/workflow_version: 1.1.0/' "$SEED/workflows/development/Workflowfile.yaml"
git -C "$SEED" commit -q -am v2
git -C "$SEED" tag v2
git clone -q --bare "$SEED" "$SANDBOX/team.git"
# The path Store "local" next to the compose file.
make_store_image "$PROJECT/local-store/images/reviewer" 1.0.0
start_daemon "$BASE" "$SOCK" "$DAEMON_LOG"
DPID="$START_PID"

FILE="$PROJECT/Tariboycompose.yaml"

step "1. validate the full file"
use base
capture t1 compose validate -f "$FILE"
[ "$CODE" -eq 0 ] || fail "validate exited $CODE: $OUT $ERR"
contains "$OUT" "$FILE: valid" || fail "validate did not print valid: $OUT"
capture t1 compose validate -f "$FILE" --daemon
[ "$CODE" -eq 0 ] || fail "validate --daemon exited $CODE: $OUT $ERR"

step "2. up --dry-run prints the plan by phase and changes nothing"
snapshot before
capture t1 compose up -f "$FILE" --dry-run
[ "$CODE" -eq 0 ] || fail "dry-run exited $CODE: $OUT $ERR"
for phase in stores workflows images queues agents groups "queue bindings"; do
  has_line "$OUT" "phase $phase" || fail "the plan has no phase $phase: $OUT"
done
contains "$OUT" "nothing applied" || fail "the plan does not say nothing applied: $OUT"
snapshot after; cmp -s "$SANDBOX/before.json" "$SANDBOX/after.json" || fail "dry-run changed the daemon"

step "3. up creates everything in phase order"
capture t1 compose up -f "$FILE"
[ "$CODE" -eq 0 ] || fail "up exited $CODE: $OUT $ERR"
KINDS="$(report_kinds)"
[ "$KINDS" = "store workflow image queue agent group budget " ] || fail "report kinds out of phase order: '$KINDS': $OUT"
for res in store:team store:local workflow:development workflow:review image:developer image:reviewer queue:DEV agent:dev agent:rev group:team budget:group:team; do
  report_has changed "$res" || fail "$res is not changed: $OUT"
done
[ "$(t1 --json store show team | field ref)" = v1 ] || fail "Store team is not at v1"
[ "$(t1 --json image inspect developer:latest | field image_version)" = 1.0.0 ] || fail "developer:latest is not 1.0.0"
t1 --json image inspect reviewer:latest >/dev/null || fail "reviewer:latest is missing"
[ "$(tt1 queue workflow get DEV --json | field version)" = 1.0.0 ] || fail "DEV is not bound to development 1.0.0"
[ "$(t1 --json group inspect team | field lead)" = dev ] || fail "the lead of team is not dev"
[ "$(t1 --json agent inspect dev | field group)" = team ] || fail "dev is not in team"
[ "$(t1 --json agent inspect dev | field notes)" = "first notes" ] || fail "dev has other notes"

step "4. a second up changes nothing and reports every resource ok"
snapshot before
capture t1 compose up -f "$FILE"
[ "$CODE" -eq 0 ] || fail "second up exited $CODE: $OUT $ERR"
REPORT="$(report_lines)"
[ "$(grep -c '^ok ' <<<"$REPORT")" -ge 11 ] || fail "second up reported fewer than 11 ok lines: $OUT"
! grep -qv '^ok ' <<<"$REPORT" || fail "second up reported a resource that is not ok: $REPORT"
has_line "$OUT" "up: " && contains "$OUT" " 0 changed, 0 failed, 0 skipped" || fail "second up summary: $OUT"
snapshot after; cmp -s "$SANDBOX/before.json" "$SANDBOX/after.json" || fail "second up changed the daemon"

step "5. a changed file updates the Store ref, the image and workflow versions, the queue, the agent and the group"
use changed
capture t1 compose up -f "$FILE"
[ "$CODE" -eq 0 ] || fail "up of the changed file exited $CODE: $OUT $ERR"
for res in store:team workflow:development image:developer queue:DEV agent:dev group:team budget:group:team; do
  report_has changed "$res" || fail "$res is not changed: $OUT"
done
for res in store:local workflow:review image:reviewer; do
  report_has ok "$res" || fail "$res is not ok: $OUT"
done
[ "$(t1 --json store show team | field ref)" = v2 ] || fail "Store team is not at v2"
[ "$(t1 --json image inspect developer:latest | field image_version)" = 1.1.0 ] || fail "developer:latest is not 1.1.0"
[ "$(tt1 queue workflow get DEV --json | field version)" = 1.1.0 ] || fail "DEV is not bound to development 1.1.0"
[ "$(tt1 queue get DEV --json | field description)" = "Compose E2E queue, changed" ] || fail "the DEV description did not change"
[ "$(t1 --json agent inspect dev | field notes)" = "second notes" ] || fail "dev has old notes"
[ "$(t1 --json group inspect team | field lead)" = rev ] || fail "the lead of team is not rev"
capture t1 compose status -f "$FILE"
contains "$OUT" "drift: 0" || fail "status after the update reports drift: $OUT $ERR"

step "6. an unavailable Store fails, its dependents are skipped, the rest applies; up after the fix converges"
use extra
capture t1 compose up -f "$FILE"
[ "$CODE" -ne 0 ] || fail "up with an unavailable Store exited 0: $OUT"
contains "$ERR" "compose up: 1 resource failed" || fail "up did not name the failed resource count: $ERR"
report_has failed store:extra || fail "store:extra did not fail: $OUT"
report_has skipped image:extra || fail "image:extra was not skipped: $OUT"
report_has skipped agent:ext || fail "agent:ext was not skipped: $OUT"
report_has changed queue:DEV || fail "queue:DEV was not applied: $OUT"
for res in store:team store:local workflow:development workflow:review image:developer image:reviewer group:team; do
  report_has ok "$res" || fail "$res is not ok: $OUT"
done
[ "$(tt1 queue get DEV --json | field description)" = "Compose E2E queue, extra" ] || fail "the DEV description did not change"
SEED_EXTRA="$SANDBOX/seed-extra"
make_store_image "$SEED_EXTRA/images/extra" 1.0.0
git -C "$SEED_EXTRA" init -q -b main
git -C "$SEED_EXTRA" add -A
git -C "$SEED_EXTRA" commit -q -m extra
git clone -q --bare "$SEED_EXTRA" "$SANDBOX/extra.git"
capture t1 compose up -f "$FILE"
[ "$CODE" -eq 0 ] || fail "up after the Store fix exited $CODE: $OUT $ERR"
for res in store:extra image:extra agent:ext; do
  report_has changed "$res" || fail "$res is not changed: $OUT"
done
[ "$(t1 --json agent inspect ext | field image)" = extra:latest ] || fail "ext is not on extra:latest"

step "7. a tariboy range the daemon does not satisfy stops up, status and validate before any change"
DAEMON_VERSION="$(t1 --json daemon status | field version)"
use range
snapshot before
for verb in "up" "status" "validate --daemon"; do
  # shellcheck disable=SC2086 # the verb has its flag
  capture t1 compose $verb -f "$FILE"
  [ "$CODE" -ne 0 ] || fail "compose $verb with min 999.0.0 exited 0: $OUT"
  contains "$ERR" "$DAEMON_VERSION" && contains "$ERR" ">= 999.0.0" && contains "$ERR" "$FILE" \
    || fail "compose $verb error does not name the version, the range and the file: $ERR"
done
snapshot after
cmp -s "$SANDBOX/before.json" "$SANDBOX/after.json" || fail "a refused up changed the daemon"

step "8. pack, validate --archive offline, unpack on a second isolated daemon"
use extra
ARCHIVE="$SANDBOX/project.tariboy.tar.gz"
capture t1 compose pack -f "$FILE" -o "$ARCHIVE"
[ "$CODE" -eq 0 ] && [ -s "$ARCHIVE" ] || fail "pack exited $CODE: $OUT $ERR"
# No second daemon runs yet: validate --archive works offline.
capture t2 compose validate --archive "$ARCHIVE"
[ "$CODE" -eq 0 ] || fail "validate --archive exited $CODE: $OUT $ERR"
start_daemon "$BASE2" "$SOCK2" "$DAEMON2_LOG"
DPID2="$START_PID"
capture t2 compose validate --archive "$ARCHIVE" --daemon
[ "$CODE" -eq 0 ] || fail "validate --archive --daemon exited $CODE: $OUT $ERR"
capture t2 compose unpack "$ARCHIVE" --yes
[ "$CODE" -eq 0 ] || fail "unpack exited $CODE: $OUT $ERR"
contains "$OUT" "skipped (archive)" || fail "unpack did not skip the Stores: $OUT"
[ "$(t2 --json store list)" = "[]" ] || fail "unpack registered a Store"
for ref in developer:latest reviewer:latest extra:latest; do
  [ "$(t2 --json image inspect "$ref" | field digest)" = "$(t1 --json image inspect "$ref" | field digest)" ] \
    || fail "$ref on the second daemon has another digest"
done
[ "$(tt2 queue workflow get DEV --json | field digest)" = "$(tt1 queue workflow get DEV --json | field digest)" ] \
  || fail "DEV on the second daemon is bound to another workflow"
[ "$(t2 --json workflow inspect review --tag latest | field digest)" = "$(t1 --json workflow inspect review --tag latest | field digest)" ] || fail "workflow review is missing on the second daemon"
[ "$(t2 --json group inspect team | field lead)" = rev ] || fail "the lead of team on the second daemon is not rev"
for agent in dev rev ext; do
  t2 --json agent inspect "$agent" >/dev/null || fail "agent $agent is missing on the second daemon"
done
[ "$(tt2 queue get DEV --json | field description)" = "Compose E2E queue, extra" ] || fail "DEV differs on the second daemon"

step "9. the secret values appear in no CLI output and in no byte of the archive"
python3 -I - "$CLI_LOG" "$ARCHIVE" "$COMPOSE_E2E_AGENT_TOKEN" "$COMPOSE_E2E_QUEUE_TOKEN" <<'PY' || fail "a secret value leaked"
import base64, gzip, io, sys, tarfile
log, archive, *secrets = sys.argv[1:]
def expand(data):
    """Yield data and, for a gzipped tar, every member, recursively."""
    yield data
    try:
        plain = gzip.decompress(data)
    except OSError:
        return
    yield plain
    try:
        tar = tarfile.open(fileobj=io.BytesIO(plain))
    except tarfile.TarError:
        return
    for member in tar.getmembers():
        f = tar.extractfile(member)
        if f is not None:
            yield from expand(f.read())
blobs = [("cli output", open(log, "rb").read())]
blobs += [("archive", b) for b in expand(open(archive, "rb").read())]
leaks = 0
for secret in secrets:
    for form in (secret.encode(), base64.b64encode(secret.encode())):
        for where, blob in blobs:
            if form in blob:
                print(f"secret found in {where}", file=sys.stderr)
                leaks += 1
print(f"searched {len(blobs)} blobs for {len(secrets)} secrets")
sys.exit(1 if leaks else 0)
PY

echo "compose-e2e: ok ($((SECONDS - STARTED_AT))s)"
