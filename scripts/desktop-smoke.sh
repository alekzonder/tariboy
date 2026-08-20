#!/usr/bin/env bash
# Product smoke for the packaged macOS app. It deliberately exercises the
# public daemon API used by the WebView while the Desktop process owns daemon
# discovery/startup. No state is allowed outside SMOKE_ROOT.
set -euo pipefail
trap 'echo "FAIL: command failed at ${BASH_SOURCE[0]}:${LINENO}" >&2' ERR

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"; }

[ "$(uname -s)" = Darwin ] || fail "desktop smoke requires macOS"
need curl
need python3
need tmux

APP="${TARIBOY_DESKTOP_APP:-$ROOT/desktop/src-tauri/target/release/bundle/macos/Tariboy.app}"
EXE_NAME="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$APP/Contents/Info.plist" 2>/dev/null || true)"
EXE="$APP/Contents/MacOS/${EXE_NAME:-tariboy-desktop}"
CLI="$APP/Contents/Resources/bin/darwin-arm64/tariboy"
[ -x "$EXE" ] || fail "no app executable at $EXE — run \`make desktop\` first"
[ -x "$CLI" ] || fail "bundled CLI missing at $CLI"

SMOKE_ROOT="$(mktemp -d "/tmp/sa-desktop.XXXXXX")"
case "$SMOKE_ROOT" in
  /tmp/sa-desktop.*) ;;
  *) fail "mktemp returned unsafe root: $SMOKE_ROOT" ;;
esac
export TARIBOY_BASE_DIR="$SMOKE_ROOT/base"
export TARIBOY_RUNTIME_DIR="$SMOKE_ROOT/runtime"
export TARIBOY_DESKTOP_APP_DATA_DIR="$SMOKE_ROOT/app-data"
export TARIBOY_STUB_HARNESS="$ROOT/scripts/stub-harness.sh"
export TMUX_TMPDIR="$SMOKE_ROOT/tmux"
unset TMUX
REMOTE_ALIAS="${TARIBOY_DESKTOP_SSH_ALIAS:-}"
REMOTE_PORT="${TARIBOY_DESKTOP_SSH_PORT:-9990}"
if [ -n "$REMOTE_ALIAS" ]; then
  case "$REMOTE_ALIAS" in -*|*[!A-Za-z0-9._@-]*) fail "unsafe TARIBOY_DESKTOP_SSH_ALIAS" ;; esac
  case "$REMOTE_PORT" in ''|*[!0-9]*) fail "unsafe TARIBOY_DESKTOP_SSH_PORT" ;; esac
  [ "$REMOTE_PORT" -ge 1 ] && [ "$REMOTE_PORT" -le 65535 ] || fail "remote SSH port is out of range"
  need ssh
fi
mkdir -p "$TARIBOY_BASE_DIR" "$TARIBOY_RUNTIME_DIR" \
  "$TARIBOY_DESKTOP_APP_DATA_DIR" "$TMUX_TMPDIR"
chmod 700 "$SMOKE_ROOT" "$TARIBOY_BASE_DIR" "$TARIBOY_RUNTIME_DIR" \
  "$TARIBOY_DESKTOP_APP_DATA_DIR" "$TMUX_TMPDIR"

# Start with a valid empty native registry. This makes Desktop read the same
# path that later host saves replace atomically and lets the smoke verify the
# owner-only storage contract without adding a real external host.
(
  umask 077
  REMOTE_ALIAS="$REMOTE_ALIAS" REMOTE_PORT="$REMOTE_PORT" python3 - "$TARIBOY_DESKTOP_APP_DATA_DIR/hosts.json" <<'PY'
import json,os,sys
hosts=[]
if os.environ["REMOTE_ALIAS"]:
    hosts.append({
      "id":"alpha-smoke-remote","label":"Alpha Smoke Remote","kind":"ssh",
      "ssh_alias":os.environ["REMOTE_ALIAS"],
      "remote_install_dir":"~/.local/lib/tariboy",
      "remote_port":int(os.environ["REMOTE_PORT"]),
      "https_base_url":"","last_daemon_version":""
    })
with open(sys.argv[1],"w") as out:
    json.dump({"schema_version":1,"hosts":hosts},out)
PY
)

SOCK="$TARIBOY_RUNTIME_DIR/tariboyd.sock"
APP_PID=""
DAEMON_PID=""
stop_isolated_daemon() {
  local pid="$DAEMON_PID" command
  if [ -z "$pid" ] && [ -f "$TARIBOY_RUNTIME_DIR/tariboyd.pid" ]; then
    pid="$(tr -d '[:space:]' <"$TARIBOY_RUNTIME_DIR/tariboyd.pid")"
  fi
  if [ -z "$pid" ]; then
    echo "FAIL: isolated daemon PID is unknown; refusing cleanup" >&2
    return 1
  fi
  case "$pid" in *[!0-9]*) echo "FAIL: invalid isolated daemon pid: $pid" >&2; return 1 ;; esac
  kill -0 "$pid" 2>/dev/null || return 0
  "$CLI" daemon stop >/dev/null 2>&1 || true
  for _ in $(seq 1 50); do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 0.1
  done
  command="$(ps -p "$pid" -o command= 2>/dev/null || true)"
  case "$command" in
    *"$APP/Contents/Resources/bin/darwin-arm64/tariboyd"*)
      kill "$pid" 2>/dev/null || true
      for _ in $(seq 1 50); do
        kill -0 "$pid" 2>/dev/null || return 0
        sleep 0.1
      done
      kill -KILL "$pid" 2>/dev/null || true
      sleep 0.1
      kill -0 "$pid" 2>/dev/null && return 1
      ;;
    *)
      echo "FAIL: refusing to signal unverified daemon pid $pid: $command" >&2
      return 1
      ;;
  esac
  return 0
}

cleanup() {
  local exit_code=$? cleanup_code=0
  trap - EXIT HUP INT TERM
  set +e
  if [ -n "$APP_PID" ]; then
    kill "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
  fi
  stop_isolated_daemon || cleanup_code=1
  if [ "$cleanup_code" -eq 0 ]; then
    case "$SMOKE_ROOT" in
      /tmp/sa-desktop.*) rm -rf "$SMOKE_ROOT" ;;
    esac
  else
    echo "FAIL: preserved isolated state because daemon cleanup failed: $SMOKE_ROOT" >&2
  fi
  [ "$exit_code" -ne 0 ] && exit "$exit_code"
  exit "$cleanup_code"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM

api() {
  local method=$1 path=$2 body=${3:-} response
  local args=(-fsS --unix-socket "$SOCK" -X "$method" "http://localhost$path")
  if [ -n "$body" ]; then
    args+=(-H 'content-type: application/json' --data "$body")
  fi
  response="$(curl "${args[@]}")"
  printf '%s' "$response" | python3 -c '
import json,sys
value=json.load(sys.stdin)
if not value.get("ok"):
    raise SystemExit("API error: "+json.dumps(value.get("error")))
'
  printf '%s' "$response"
}

wait_for_daemon() {
  local status=""
  for _ in $(seq 1 100); do
    if status="$(curl -fsS --unix-socket "$SOCK" http://localhost/api/daemon/status 2>/dev/null)"; then
      break
    fi
    status=""
    sleep 0.2
  done
  [ -n "$status" ] || {
    echo "--- app log:" >&2
    cat "$TARIBOY_RUNTIME_DIR/app.log" >&2 2>/dev/null || true
    echo "--- daemon log:" >&2
    cat "$TARIBOY_RUNTIME_DIR/tariboyd.log" >&2 2>/dev/null || true
    fail "daemon never answered on $SOCK"
  }
  printf '%s' "$status"
}

launch_app() {
  "$EXE" >"$TARIBOY_RUNTIME_DIR/app.log" 2>&1 &
  APP_PID=$!
}

tunnel_pids() {
  [ -n "$REMOTE_ALIAS" ] || return 0
  ps -axo pid=,command= | awk -v alias="$REMOTE_ALIAS" '
    /[/]usr\/bin\/ssh/ && / -N -L / && $NF == alias { print $1 }
  '
}

normal_tunnel_pids() {
  tunnel_pids | sort -n | tr '\n' ' '
}

wait_for_new_tunnel() {
  local before=$1 pid
  for _ in $(seq 1 100); do
    for pid in $(tunnel_pids); do
      case " $before " in *" $pid "*) ;; *) printf '%s' "$pid"; return 0 ;; esac
    done
    sleep 0.1
  done
  return 1
}

wait_for_tunnel_baseline() {
  local baseline=$1 current
  for _ in $(seq 1 100); do
    current="$(normal_tunnel_pids)"
    [ "$current" = "$baseline" ] && return 0
    sleep 0.1
  done
  return 1
}

quit_app() {
  kill "$APP_PID" 2>/dev/null || true
  wait "$APP_PID" 2>/dev/null || true
  APP_PID=""
}

echo "--- Desktop starts an isolated daemon"
TUNNELS_BEFORE="$(normal_tunnel_pids)"
launch_app
STATUS="$(wait_for_daemon)"
BASE="$(printf '%s' "$STATUS" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["base_dir"])')"
ADDR="$(printf '%s' "$STATUS" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["http_addr"])')"
DAEMON_PID="$(printf '%s' "$STATUS" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["pid"])')"
[ "$BASE" = "$TARIBOY_BASE_DIR" ] || fail "daemon base_dir=$BASE, expected $TARIBOY_BASE_DIR"
case "$ADDR" in 127.0.0.1:*) ;; *) fail "daemon is not loopback-bound: $ADDR" ;; esac
curl -fsS "http://$ADDR/api/daemon/status" | grep -q '"ok":true' || fail "loopback API unavailable"
ACAO="$(curl -fsS -o /dev/null -D - -H 'Origin: tauri://localhost' \
  "http://$ADDR/api/daemon/status" | tr -d '\r' | awk -F': ' '/^[Aa]ccess-[Cc]ontrol-[Aa]llow-[Oo]rigin/{print $2}')"
[ "$ACAO" = "tauri://localhost" ] || fail "desktop CORS origin rejected: $ACAO"
[ "$(curl -s -o /dev/null -w '%{http_code}' "http://$ADDR/")" = 404 ] || fail "daemon unexpectedly serves UI"
echo "ok: isolated daemon started on $ADDR (pid $DAEMON_PID)"
REMOTE_TUNNEL_PID=""
if [ -n "$REMOTE_ALIAS" ]; then
  REMOTE_TUNNEL_PID="$(wait_for_new_tunnel "$TUNNELS_BEFORE")" ||
    fail "Desktop did not establish the saved-host tunnel"
  ssh "$REMOTE_ALIAS" "curl -fsS http://127.0.0.1:$REMOTE_PORT/api/daemon/status >/dev/null" ||
    fail "remote daemon is not reachable independently of the tunnel"
  echo "ok: saved remote host tunnel is live (pid $REMOTE_TUNNEL_PID)"
fi

echo "--- native host state stays owner-only in isolated data/runtime roots"
[ "$(stat -f '%Lp' "$TARIBOY_DESKTOP_APP_DATA_DIR")" = 700 ] || fail "app-data mode is not 0700"
[ "$(stat -f '%Lp' "$TARIBOY_DESKTOP_APP_DATA_DIR/hosts.json")" = 600 ] || fail "hosts.json mode is not 0600"
[ -d "$TARIBOY_RUNTIME_DIR/ssh" ] || fail "SSH control directory was not created"
[ "$(stat -f '%Lp' "$TARIBOY_RUNTIME_DIR/ssh")" = 700 ] || fail "SSH control mode is not 0700"

echo "--- source create, edit, validate, and immutable build"
api POST /api/image-sources \
  '{"name":"alpha-smoke","harness":"stub","interactive":true,"prompt":"Initial smoke prompt."}' >/dev/null
api PUT /api/image-sources/alpha-smoke/files/PROMPT.md \
  '{"content":"Updated smoke prompt."}' >/dev/null
VALIDATION="$(api POST /api/image-sources/alpha-smoke/validate '{}')"
printf '%s' "$VALIDATION" | python3 -c '
import json,sys
r=json.load(sys.stdin)["result"]
assert r["valid"] is True and r["diagnostics"] == []
'
BUILT="$(api POST /api/image-sources/alpha-smoke/build '{"tag":"latest"}')"
printf '%s' "$BUILT" | python3 -c '
import json,sys
r=json.load(sys.stdin)["result"]
assert r["ref"] == "alpha-smoke:latest" and r["digest"] and r["layers"] > 0
'
IMAGES="$(api GET /api/images)"
printf '%s' "$IMAGES" | python3 -c '
import json,sys
rows=json.load(sys.stdin)["result"]["images"]
assert any(x["name"] == "alpha-smoke" and x["tag"] == "latest" and not x.get("bare",False) for x in rows)
assert any(x["name"] == "bare" and x["tag"] == "latest" and x.get("bare") is True for x in rows)
'

echo "--- built image selection creates a stopped agent"
api POST /api/agents \
  '{"image":"alpha-smoke:latest","name":"built-smoke","harness":"stub","interactive":false,"loop":false}' >/dev/null
BUILT_AGENT="$(api GET /api/agents/built-smoke)"
printf '%s' "$BUILT_AGENT" | python3 -c '
import json,sys
r=json.load(sys.stdin)["result"]
assert r["image"] == "alpha-smoke:latest" and r["state"] == "stopped"
'

echo "--- bare:latest starts an ordinary interactive terminal"
api POST /api/agents \
  '{"image":"bare:latest","name":"bare-smoke","harness":"stub","interactive":true,"loop":false,"env":"STUB_SLEEP=30"}' >/dev/null
api POST /api/agents/bare-smoke/start '{}' >/dev/null
SCREEN=""
for _ in $(seq 1 100); do
  if SCREEN="$(api GET /api/agents/bare-smoke/screen 2>/dev/null)"; then
    break
  fi
  SCREEN=""
  sleep 0.1
done
[ -n "$SCREEN" ] || fail "interactive terminal never became attachable"
BARE_AGENT="$(api GET /api/agents/bare-smoke)"
printf '%s' "$BARE_AGENT" | python3 -c '
import json,sys
r=json.load(sys.stdin)["result"]
assert r["image"] == "bare:latest" and r["interactive"] is True
assert r["loop_enabled"] is False and r["state"] == "running"
'
api POST /api/agents/bare-smoke/stop '{}' >/dev/null
echo "ok: bare terminal is interactive and Autopilot is disabled"

echo "--- quitting Desktop leaves its daemon alive"
quit_app
sleep 1
kill -0 "$DAEMON_PID"
curl -fsS --unix-socket "$SOCK" http://localhost/api/daemon/status >/dev/null
if [ -n "$REMOTE_TUNNEL_PID" ]; then
  wait_for_tunnel_baseline "$TUNNELS_BEFORE" ||
    fail "Desktop exit did not restore the SSH tunnel process baseline"
  ssh "$REMOTE_ALIAS" "curl -fsS http://127.0.0.1:$REMOTE_PORT/api/daemon/status >/dev/null" ||
    fail "remote daemon stopped with Desktop tunnel"
fi

echo "--- relaunch adopts the same daemon instead of starting another"
TUNNELS_BEFORE="$(normal_tunnel_pids)"
launch_app
STATUS="$(wait_for_daemon)"
ADOPTED_PID="$(printf '%s' "$STATUS" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["pid"])')"
[ "$ADOPTED_PID" = "$DAEMON_PID" ] || fail "Desktop did not adopt daemon $DAEMON_PID (got $ADOPTED_PID)"
REMOTE_TUNNEL_PID=""
if [ -n "$REMOTE_ALIAS" ]; then
  REMOTE_TUNNEL_PID="$(wait_for_new_tunnel "$TUNNELS_BEFORE")" ||
    fail "Desktop did not restore the saved-host tunnel"
fi
quit_app
sleep 1
kill -0 "$DAEMON_PID"
curl -fsS --unix-socket "$SOCK" http://localhost/api/daemon/status >/dev/null
if [ -n "$REMOTE_TUNNEL_PID" ]; then
  wait_for_tunnel_baseline "$TUNNELS_BEFORE" ||
    fail "Desktop exit did not restore the adopted tunnel process baseline"
  ssh "$REMOTE_ALIAS" "curl -fsS http://127.0.0.1:$REMOTE_PORT/api/daemon/status >/dev/null" ||
    fail "remote daemon did not outlive the adopted tunnel"
fi
if find "$TARIBOY_RUNTIME_DIR/ssh" -type s -print -quit | grep -q .; then
  fail "Desktop exit left an SSH control socket behind"
fi
echo "PASS: packaged Desktop product smoke"
