#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
home="$tmp/home"
tools="$tmp/tools"
mkdir -p "$home/.local/lib/tariboy/old" "$home/.local/bin" "$tools"

cat >"$tools/go" <<'SH'
#!/bin/sh
set -eu
if test "${1-}" = run; then exit 0; fi
output=
while test "$#" -gt 0; do
  if test "$1" = -o; then output=$2; shift 2; continue; fi
  shift
done
test -n "$output"
version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' internal/version/version.go)
mkdir -p "$(dirname "$output")"
printf '#!/bin/sh\nprintf '\''%%s\\n'\'' '\''%s'\''\n' "$version" >"$output"
chmod 0755 "$output"
SH
chmod 0755 "$tools/go"

binaries="tariboyd tariboy tariboy-shim tariboy-tools tariboy-store tariboy-plugin-telegram"
for name in $binaries; do
  printf 'old\n' >"$home/.local/lib/tariboy/old/$name"
  ln -s "$home/.local/lib/tariboy/old/$name" "$home/.local/bin/$name"
done

HOME="$home" PATH="$tools:$PATH" make --no-print-directory -C "$ROOT" \
  GO="$tools/go" BINDIR="$tmp/build" server-install

version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' "$ROOT/internal/version/version.go")
for name in $binaries; do
  expected="$home/.local/lib/tariboy/$version/$name"
  test "$(readlink "$home/.local/bin/$name")" = "$expected"
  test "$("$home/.local/bin/$name" --version)" = "$version"
done
test "$(cat "$home/.local/lib/tariboy/old/tariboy")" = old

echo "server install contract passed"
