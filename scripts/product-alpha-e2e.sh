#!/usr/bin/env bash
# Destructive product vertical for an explicitly disposable Linux x86_64 SSH
# target. The target owner must opt in twice: an environment acknowledgement
# here and a sentinel file already present on the remote host.
set -euo pipefail
trap 'echo "FAIL: command failed at ${BASH_SOURCE[0]}:${LINENO}" >&2' ERR

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit "${2:-1}"; }
need() { command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"; }

: "${TARIBOY_SSH_TEST_HOST:?set TARIBOY_SSH_TEST_HOST to a disposable Linux x86_64 SSH target}"
case "$TARIBOY_SSH_TEST_HOST" in
  -*|*[!A-Za-z0-9._@-]*) fail "unsafe TARIBOY_SSH_TEST_HOST" 64 ;;
esac
[ "${TARIBOY_SSH_TEST_DISPOSABLE:-}" = "I_ACCEPT_REMOTE_DATA_DELETION" ] ||
  fail "refusing target without TARIBOY_SSH_TEST_DISPOSABLE=I_ACCEPT_REMOTE_DATA_DELETION" 64
need ssh
need scp
need python3

# Environment acknowledgement alone is too easy to copy accidentally. The
# remote owner must have created this sentinel deliberately in advance.
ssh "$TARIBOY_SSH_TEST_HOST" \
  'test "$(cat "$HOME/.tariboy-disposable-test-host" 2>/dev/null)" = TARIBOY_DISPOSABLE_TEST_HOST' ||
  fail "remote disposable sentinel is absent or invalid" 64
REMOTE_PLATFORM="$(ssh "$TARIBOY_SSH_TEST_HOST" 'printf "%s/%s" "$(uname -s)" "$(uname -m)"')"
[ "$REMOTE_PLATFORM" = "Linux/x86_64" ] ||
  fail "target must be Linux/x86_64, got $REMOTE_PLATFORM" 64

VERSION="$(sed -n 's/^const Version = "\(.*\)"$/\1/p; s/^var Version = "\(.*\)"$/\1/p' internal/version/version.go)"
[ -n "$VERSION" ] || fail "cannot read product version"
make desktop-binaries

STAMP="$(date +%s)-$$"
REMOTE_ROOT="/tmp/tariboy-product-alpha-$STAMP"
REMOTE_HOME="$REMOTE_ROOT/home"
REMOTE_BASE="$REMOTE_ROOT/base"
REMOTE_RUNTIME="$REMOTE_ROOT/runtime"
REMOTE_STAGE="$REMOTE_HOME/.local/lib/tariboy/.stage-alpha-$STAMP"
REMOTE_RELEASE="$REMOTE_HOME/.local/lib/tariboy/$VERSION"
LOCAL_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/tariboy-product-alpha.XXXXXX")"
LOCAL_APP_DATA="$LOCAL_ROOT/app-data"
mkdir -p "$LOCAL_APP_DATA"
chmod 700 "$LOCAL_ROOT" "$LOCAL_APP_DATA"
(
  umask 077
  printf '%s\n' TARIBOY_ALPHA_SMOKE_ROOT >"$LOCAL_ROOT/.tariboy-alpha-smoke-root"
)

case "$REMOTE_ROOT:$REMOTE_HOME:$REMOTE_BASE:$REMOTE_RUNTIME:$LOCAL_ROOT" in
  /tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:"${TMPDIR:-/tmp}"/tariboy-product-alpha.*) ;;
  *) fail "refusing unsafe isolated paths" 64 ;;
esac

cleanup() {
  local exit_code=$? cleanup_code=0
  trap - EXIT HUP INT TERM
  set +e
  ssh "$TARIBOY_SSH_TEST_HOST" sh -s -- \
    "$REMOTE_ROOT" "$REMOTE_HOME" "$REMOTE_BASE" "$REMOTE_RUNTIME" "$REMOTE_RELEASE" <<'REMOTE_CLEANUP'
set -eu
root=$1
home=$2
base=$3
runtime=$4
release=$5
case "$root:$home:$base:$runtime" in
  /tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*) ;;
  *) exit 64 ;;
esac
if test -x "$home/.local/bin/tariboy"; then
  pid=$(HOME=$home TARIBOY_BASE_DIR=$base TARIBOY_RUNTIME_DIR=$runtime \
    "$home/.local/bin/tariboy" daemon status --json 2>/dev/null |
    sed -n 's/.*"pid":\([0-9][0-9]*\).*/\1/p')
  if test -z "$pid" && test -f "$runtime/tariboyd.pid"; then
    pid=$(tr -d '[:space:]' <"$runtime/tariboyd.pid")
  fi
  if test -z "$pid"; then
    for process in /proc/[0-9]*; do
      test -r "$process/environ" || continue
      test "$(readlink "$process/exe" 2>/dev/null || true)" = "$release/tariboyd" || continue
      if tr '\0' '\n' <"$process/environ" | grep -Fqx "TARIBOY_BASE_DIR=$base"; then
        pid=${process##*/}
        break
      fi
    done
  fi
  case "$pid" in *[!0-9]*) echo "invalid remote daemon pid: $pid" >&2; exit 1 ;; esac
  HOME=$home TARIBOY_BASE_DIR=$base TARIBOY_RUNTIME_DIR=$runtime \
    "$home/.local/bin/tariboy" daemon stop >/dev/null 2>&1 || true
  if test -n "$pid"; then
    i=0
    while test "$i" -lt 50 && kill -0 "$pid" 2>/dev/null; do
      i=$((i + 1))
      sleep 0.1
    done
    if kill -0 "$pid" 2>/dev/null; then
      exe=$(readlink "/proc/$pid/exe" 2>/dev/null || true)
      if test "$exe" = "$release/tariboyd"; then
        tr '\0' '\n' <"/proc/$pid/environ" |
          grep -Fqx "TARIBOY_BASE_DIR=$base" || {
            echo "refusing to signal unverified remote daemon $pid" >&2
            exit 1
          }
        kill "$pid" 2>/dev/null || true
      fi
      i=0
      while test "$i" -lt 50 && kill -0 "$pid" 2>/dev/null; do
        i=$((i + 1))
        sleep 0.1
      done
      if kill -0 "$pid" 2>/dev/null &&
          test "$(readlink "/proc/$pid/exe" 2>/dev/null || true)" = "$release/tariboyd"; then
        kill -KILL "$pid" 2>/dev/null || true
        sleep 0.1
      fi
      if kill -0 "$pid" 2>/dev/null &&
          test "$(readlink "/proc/$pid/exe" 2>/dev/null || true)" = "$release/tariboyd"; then
        echo "remote alpha daemon survived cleanup" >&2
        exit 1
      fi
    fi
  fi
fi
rm -rf "$root"
REMOTE_CLEANUP
  cleanup_code=$?
  case "$LOCAL_ROOT" in
    "${TMPDIR:-/tmp}"/tariboy-product-alpha.*) rm -rf "$LOCAL_ROOT" ;;
  esac
  [ "$exit_code" -ne 0 ] && exit "$exit_code"
  exit "$cleanup_code"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM

for name in tariboyd tariboy tariboy-shim tariboy-plugin-telegram; do
  [ -f "desktop/src-tauri/resources/bin/linux-x86_64/$name" ] ||
    fail "missing packaged Linux binary: $name"
done
for name in SHA256SUMS VERSION remote-install.sh; do
  [ -f "desktop/src-tauri/resources/bin/linux-x86_64/$name" ] ||
    fail "missing packaged Linux metadata: $name"
done

echo "--- provision version-matched Linux binaries"
ssh "$TARIBOY_SSH_TEST_HOST" mkdir -p "$REMOTE_STAGE"
scp desktop/src-tauri/resources/bin/linux-x86_64/tariboyd \
  desktop/src-tauri/resources/bin/linux-x86_64/tariboy \
  desktop/src-tauri/resources/bin/linux-x86_64/tariboy-shim \
  desktop/src-tauri/resources/bin/linux-x86_64/tariboy-plugin-telegram \
  desktop/src-tauri/resources/bin/linux-x86_64/SHA256SUMS \
  desktop/src-tauri/resources/bin/linux-x86_64/VERSION \
  desktop/src-tauri/resources/bin/linux-x86_64/remote-install.sh \
  "$TARIBOY_SSH_TEST_HOST:$REMOTE_STAGE/"

# This record is intentionally local and non-secret, just like Desktop's native
# registry. It is removed after the vertical while the remote daemon remains up.
(
  umask 077
  python3 - "$LOCAL_APP_DATA/hosts.json" "$TARIBOY_SSH_TEST_HOST" <<'PY'
import json,sys
path,alias=sys.argv[1:]
json.dump({"schema_version":1,"hosts":[{
  "id":"alpha-e2e","label":"Alpha E2E","kind":"ssh","ssh_alias":alias,
  "remote_install_dir":"~/.local/lib/tariboy","remote_port":9990,
  "https_base_url":"","last_daemon_version":""
}]},open(path,"w"),indent=2)
PY
)
[ "$(stat -c '%a' "$LOCAL_APP_DATA/hosts.json")" = 600 ] || fail "local host registry is not 0600"

echo "--- run image → interactive Autopilot → event vertical"
ssh "$TARIBOY_SSH_TEST_HOST" sh -s -- \
  "$REMOTE_ROOT" "$REMOTE_HOME" "$REMOTE_BASE" "$REMOTE_RUNTIME" \
  "$REMOTE_STAGE" "$REMOTE_RELEASE" "$VERSION" "$STAMP" <<'REMOTE_VERTICAL'
set -eu
root=$1
home=$2
base=$3
runtime=$4
stage=$5
release=$6
version=$7
stamp=$8
case "$root:$home:$base:$runtime:$stage:$release" in
  /tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*) ;;
  *) echo "unsafe remote paths" >&2; exit 64 ;;
esac
for command in curl python3 tmux; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "missing remote product-test prerequisite: $command" >&2
    exit 69
  }
done

HOME=$home sh "$stage/remote-install.sh" "$version" ".stage-alpha-$stamp"
for name in tariboyd tariboy tariboy-shim tariboy-plugin-telegram; do
  test "$("$release/$name" --version)" = "$version"
  test "$(readlink "$home/.local/bin/$name")" = "$release/$name"
done

mkdir -p "$base" "$runtime" "$root/tmux"
chmod 700 "$root/tmux"
harness="$root/alpha-harness.sh"
cat >"$harness" <<'HARNESS'
#!/bin/sh
set -eu
prompt=${1:-}
marker="$TARIBOY_ALPHA_E2E_ROOT/first-session"
if test ! -f "$marker"; then
  : >"$marker"
  echo "alpha interactive console ready"
  exec /bin/sh
fi
if test -n "$prompt" && test -f "$prompt"; then
  ids=$(sed -n 's/^- id \([^ ]*\).*/\1/p' "$prompt")
  for id in $ids; do
    "$TARIBOY_ALPHA_E2E_MESSAGES" message processed "$id" "alpha-e2e-consumed" >/dev/null
  done
fi
i-am-done >/dev/null
HARNESS
chmod 700 "$harness"

export HOME=$home
export TARIBOY_BASE_DIR=$base
export TARIBOY_RUNTIME_DIR=$runtime
export TARIBOY_STUB_HARNESS=$harness
export TARIBOY_ALPHA_E2E_ROOT=$root
export TARIBOY_ALPHA_E2E_MESSAGES=$base/store/versions/$version/skills/messages/scripts/messages.sh
export TARIBOY_HTTP_ADDR=127.0.0.1:9990
export TMUX_TMPDIR=$root/tmux
unset TMUX
cli="$home/.local/bin/tariboy"
"$cli" daemon start >/dev/null

i=0
while test "$i" -lt 100; do
  if "$cli" daemon status --json >"$root/status.json" 2>/dev/null; then break; fi
  i=$((i + 1))
  sleep 0.1
done
grep -q "\"version\":\"$version\"" "$root/status.json"
addr=$(sed -n 's/.*"http_addr":"\([^"]*\)".*/\1/p' "$root/status.json")
case "$addr" in 127.0.0.1:*) ;; *) echo "daemon is not loopback-only: $addr" >&2; exit 1 ;; esac
sock="$runtime/tariboyd.sock"

api() {
  method=$1
  path=$2
  body=${3:-}
  if test -n "$body"; then
    response=$(curl -fsS --unix-socket "$sock" -X "$method" \
      -H 'content-type: application/json' --data "$body" "http://localhost$path")
  else
    response=$(curl -fsS --unix-socket "$sock" -X "$method" "http://localhost$path")
  fi
  printf '%s' "$response" | grep -q '"ok":true'
  printf '%s' "$response"
}

api POST /api/image-sources \
  '{"name":"alpha-remote","harness":"stub","interactive":true,"prompt":"Handle the pending event."}' >/dev/null
api PUT /api/image-sources/alpha-remote/files/PROMPT.md \
  '{"content":"Handle exactly one pending event, then finish."}' >/dev/null
api POST /api/image-sources/alpha-remote/validate '{}' | grep -q '"valid":true'
api POST /api/image-sources/alpha-remote/build '{"tag":"latest"}' | grep -q '"ref":"alpha-remote:latest"'

api POST /api/agents \
  '{"image":"alpha-remote:latest","name":"vertical","harness":"stub","interactive":true,"loop":true}' >/dev/null
api POST /api/agents/vertical/subscriptions '{"channel":"alpha:e2e"}' >/dev/null
api POST /api/agents/vertical/start '{}' >/dev/null

i=0
while test "$i" -lt 100; do
  if "$cli" agent screen vertical >"$root/screen.txt" 2>/dev/null &&
      grep -q 'alpha interactive console ready' "$root/screen.txt"; then
    break
  fi
  i=$((i + 1))
  sleep 0.1
done
grep -q 'alpha interactive console ready' "$root/screen.txt"

# Exercise the exact websocket attach route used by Console in both directions.
# A handshake-only check is insufficient because attach failures deliberately
# upgrade and then close with 4404.
command -v python3 >/dev/null 2>&1 || {
  echo "python3 is required for the Console websocket probe" >&2
  exit 69
}
python3 - "$addr" <<'PY'
import os,socket,struct,sys,time
host,port=sys.argv[1].rsplit(":",1)
s=socket.create_connection((host,int(port)),timeout=5)
request=(
  "GET /api/agents/vertical/terminal?cols=80&rows=24 HTTP/1.1\r\n"
  f"Host: {host}:{port}\r\n"
  "Connection: Upgrade\r\nUpgrade: websocket\r\n"
  "Sec-WebSocket-Version: 13\r\n"
  "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
).encode()
s.sendall(request)
buf=b""
while b"\r\n\r\n" not in buf:
    chunk=s.recv(4096)
    if not chunk: raise SystemExit("Console websocket closed before handshake")
    buf+=chunk
headers,buf=buf.split(b"\r\n\r\n",1)
if b" 101 " not in headers.split(b"\r\n",1)[0]:
    raise SystemExit("Console websocket did not return 101")

payload=b"echo alpha-console-round-trip\r"
mask=os.urandom(4)
frame=bytes([0x82,0x80|len(payload)])+mask+bytes(
    value ^ mask[index % 4] for index,value in enumerate(payload)
)
s.sendall(frame)
s.settimeout(0.5)
deadline=time.time()+5
stream=buf
while time.time()<deadline and b"alpha-console-round-trip" not in stream:
    try:
        head=s.recv(2)
    except socket.timeout:
        continue
    if len(head)<2: raise SystemExit("Console websocket closed before terminal output")
    opcode=head[0]&0x0f
    length=head[1]&0x7f
    if length==126:
        length=struct.unpack("!H",s.recv(2))[0]
    elif length==127:
        length=struct.unpack("!Q",s.recv(8))[0]
    data=b""
    while len(data)<length:
        chunk=s.recv(length-len(data))
        if not chunk: raise SystemExit("truncated Console websocket frame")
        data+=chunk
    if opcode==8:
        code=struct.unpack("!H",data[:2])[0] if len(data)>=2 else 1005
        raise SystemExit(f"Console websocket closed before round trip: {code}")
    if opcode in (1,2,0):
        stream+=data
if b"alpha-console-round-trip" not in stream:
    raise SystemExit("Console websocket had no bidirectional terminal round trip")
s.close()
PY

"$cli" --json iteration ls vertical >"$root/before.json"
test "$(grep -o '"id"' "$root/before.json" | wc -l | tr -d ' ')" -eq 1
api POST /api/messages '{"channel":"alpha:e2e","type":"alpha.test","text":"wake after console closes"}' >/dev/null
api GET /api/agents/vertical/inbox?status=pending | grep -q '"count":1'

# Closing the live interactive iteration clears the session collision. The
# pending delivery must then cause one and only one message-triggered iteration.
api POST /api/agents/vertical/kill '{}' >/dev/null
i=0
while test "$i" -lt 200; do
  "$cli" --json iteration ls vertical >"$root/after.json"
  if test "$(grep -o '"id"' "$root/after.json" | wc -l | tr -d ' ')" -eq 2 &&
      grep -q '"trigger":"message"' "$root/after.json" &&
      grep -q '"status":"done"' "$root/after.json"; then
    break
  fi
  i=$((i + 1))
  sleep 0.1
done
test "$(grep -o '"id"' "$root/after.json" | wc -l | tr -d ' ')" -eq 2
test "$(grep -o '"trigger":"message"' "$root/after.json" | wc -l | tr -d ' ')" -eq 1
api GET /api/agents/vertical/inbox?status=pending | grep -q '"count":0'

# Activity surfaces used by the Desktop Activity tab plus both usage reports.
api GET /api/agents/vertical/iterations | grep -q '"count":2'
api GET /api/agents/vertical/logs?limit=20 | grep -q '"events"'
api GET /api/agents/vertical/status/history | grep -q '"events"'
api GET /api/agents/vertical/usage | grep -q '"rows"'
api GET /api/usage | grep -q '"rows"'

api POST /api/agents/vertical/stop '{}' >/dev/null
api GET /api/agents/vertical | grep -q '"state":"stopped"'
test -d "$base/agents/vertical"
echo "$addr" >"$root/http-addr"
REMOTE_VERTICAL

if [ "$(uname -s)" = Darwin ]; then
  echo "--- verify Desktop tunnel teardown while the remote daemon stays live"
  TARIBOY_DESKTOP_SSH_ALIAS="$TARIBOY_SSH_TEST_HOST" \
    TARIBOY_DESKTOP_SSH_PORT=9990 \
    "$ROOT/scripts/desktop-smoke.sh"
  APP="${TARIBOY_DESKTOP_APP:-$ROOT/desktop/src-tauri/target/release/bundle/macos/Tariboy.app}"
  EXE_NAME="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$APP/Contents/Info.plist")"
  TARIBOY_DESKTOP_APP_DATA_DIR="$LOCAL_APP_DATA" \
    "$APP/Contents/MacOS/$EXE_NAME" --alpha-smoke-remove-host alpha-e2e
  python3 - "$LOCAL_APP_DATA/hosts.json" <<'PY'
import json,sys
with open(sys.argv[1]) as source:
    assert json.load(source)["hosts"] == []
PY
  DESKTOP_GATES=passed
else
  echo "SKIP: packaged Desktop tunnel and host_remove require the macOS release host"
  DESKTOP_GATES=skipped
fi

echo "--- remote daemon and durable data remain available"
ssh "$TARIBOY_SSH_TEST_HOST" sh -s -- \
  "$REMOTE_ROOT" "$REMOTE_HOME" "$REMOTE_BASE" "$REMOTE_RUNTIME" <<'REMOTE_VERIFY'
set -eu
root=$1
home=$2
base=$3
runtime=$4
case "$root:$home:$base:$runtime" in
  /tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*:/tmp/tariboy-product-alpha-*) ;;
  *) exit 64 ;;
esac
HOME=$home TARIBOY_BASE_DIR=$base TARIBOY_RUNTIME_DIR=$runtime \
  "$home/.local/bin/tariboy" daemon status --json | grep -q '"version"'
test -d "$base/agents/vertical"
test -d "$base/image-sources/alpha-remote"
REMOTE_VERIFY

if [ "$DESKTOP_GATES" = passed ]; then
  echo "PASS: complete Tariboy product alpha vertical on $TARIBOY_SSH_TEST_HOST"
else
  echo "PASS: remote product vertical on $TARIBOY_SSH_TEST_HOST"
  echo "NOT ACCEPTED: packaged Desktop tunnel and host_remove gates were skipped"
fi
