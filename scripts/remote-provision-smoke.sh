#!/bin/sh
set -eu

: "${TARIBOY_SSH_TEST_HOST:?set TARIBOY_SSH_TEST_HOST to a disposable Linux x86_64 SSH target}"

case "$TARIBOY_SSH_TEST_HOST" in
  -*|*[!A-Za-z0-9._@-]*) echo "unsafe TARIBOY_SSH_TEST_HOST" >&2; exit 64 ;;
esac

version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p; s/^var Version = "\(.*\)"$/\1/p' internal/version/version.go)
test -n "$version"
stamp="$(date +%s)-$$"
remote_root="/tmp/tariboy-provision-smoke-$stamp"
remote_home="$remote_root/home"
remote_base="$remote_root/base"
remote_runtime="$remote_root/runtime"
staging=".stage-smoke-$stamp"
remote_stage="$remote_home/.local/lib/tariboy/$staging"
remote_release="$remote_home/.local/lib/tariboy/$version"
remote_port=$((20000 + ($$ % 20000)))

test -n "$remote_base"
test -n "$remote_runtime"
case "$remote_base:$remote_runtime" in
  /tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*) ;;
  *) echo "refusing unsafe isolated directories" >&2; exit 64 ;;
esac

cleanup() {
  ssh "$TARIBOY_SSH_TEST_HOST" sh -s -- "$remote_root" "$remote_home" "$remote_base" "$remote_runtime" <<'REMOTE' || true
set -eu
root=$1
home=$2
base=$3
runtime=$4
case "$root:$home:$base:$runtime" in
  /tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*) ;;
  *) exit 64 ;;
esac
if test -x "$home/.local/bin/tariboy"; then
  HOME=$home TARIBOY_BASE_DIR=$base TARIBOY_RUNTIME_DIR=$runtime \
    "$home/.local/bin/tariboy" daemon stop >/dev/null 2>&1 || true
fi
rm -rf "$root"
REMOTE
}
trap cleanup EXIT HUP INT TERM

for name in tariboyd tariboy tariboy-shim tariboy-tools tariboy-plugin-telegram; do
  test -f "desktop/src-tauri/resources/bin/linux-x86_64/$name"
done
test -f desktop/src-tauri/resources/bin/linux-x86_64/SHA256SUMS
test -f desktop/src-tauri/resources/bin/linux-x86_64/VERSION
test -f desktop/src-tauri/resources/bin/linux-x86_64/remote-install.sh

ssh "$TARIBOY_SSH_TEST_HOST" mkdir -p "$remote_stage"
scp desktop/src-tauri/resources/bin/linux-x86_64/tariboyd \
  desktop/src-tauri/resources/bin/linux-x86_64/tariboy \
  desktop/src-tauri/resources/bin/linux-x86_64/tariboy-shim \
  desktop/src-tauri/resources/bin/linux-x86_64/tariboy-tools \
  desktop/src-tauri/resources/bin/linux-x86_64/tariboy-plugin-telegram \
  desktop/src-tauri/resources/bin/linux-x86_64/SHA256SUMS \
  desktop/src-tauri/resources/bin/linux-x86_64/VERSION \
  desktop/src-tauri/resources/bin/linux-x86_64/remote-install.sh \
  "$TARIBOY_SSH_TEST_HOST:$remote_stage/"

ssh "$TARIBOY_SSH_TEST_HOST" sh -s -- \
  "$remote_root" "$remote_home" "$remote_base" "$remote_runtime" "$remote_stage" \
  "$remote_release" "$version" "$staging" "$remote_port" <<'REMOTE'
set -eu
root=$1
home=$2
base=$3
runtime=$4
stage=$5
release=$6
version=$7
staging=$8
port=$9
case "$root:$home:$base:$runtime:$stage:$release" in
  /tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*:/tmp/tariboy-provision-smoke-*) ;;
  *) exit 64 ;;
esac
HOME=$home sh "$stage/remote-install.sh" "$version" "$staging"
test -d "$release"
for name in tariboyd tariboy tariboy-shim tariboy-tools tariboy-plugin-telegram; do
  test "$("$release/$name" --version)" = "$version"
  test "$(readlink "$home/.local/bin/$name")" = "$release/$name"
done
mkdir -p "$base" "$runtime"
HOME=$home TARIBOY_BASE_DIR=$base TARIBOY_RUNTIME_DIR=$runtime \
  TARIBOY_HTTP_ADDR="127.0.0.1:$port" \
  "$home/.local/bin/tariboy" daemon start >"$root/daemon.log"
i=0
while test "$i" -lt 100; do
  if HOME=$home TARIBOY_BASE_DIR=$base TARIBOY_RUNTIME_DIR=$runtime \
      "$home/.local/bin/tariboy" daemon status --json >"$root/status.json" 2>/dev/null; then
    break
  fi
  i=$((i + 1))
  sleep 0.1
done
grep "\"version\":\"$version\"" "$root/status.json"
grep "\"http_addr\":\"127.0.0.1:$port\"" "$root/status.json"
case "$(sed -n 's/.*"http_addr":"\\([^"]*\\)".*/\\1/p' "$root/status.json")" in
  127.0.0.1:*) ;;
  *) echo "daemon is not loopback-bound" >&2; exit 1 ;;
esac
REMOTE

echo "remote provision smoke passed on $TARIBOY_SSH_TEST_HOST"
