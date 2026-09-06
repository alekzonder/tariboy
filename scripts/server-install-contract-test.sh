#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
home="$tmp/home"
tools="$tmp/tools"
prefix="$tmp/prefix"
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

cat >"$tools/mv" <<'SH'
#!/bin/sh
if test "${TARIBOY_TEST_FAIL_MV_TO-}" = "${2-}"; then exit 1; fi
exec /bin/mv "$@"
SH
chmod 0755 "$tools/mv"

binaries=(tariboyd tariboy tariboy-shim tariboy-store tariboy-plugin-telegram tariboy-tasks)
links=(
  tariboyd:tariboyd
  tariboy:tariboy
  tariboy-tasks:tariboy-tasks
  ttasks:tariboy-tasks
  tariboy-shim:tariboy-shim
  tariboy-plugin-telegram:tariboy-plugin-telegram
  tariboy-store:tariboy-store
)

seed_old() {
  rm -rf -- "$home/.local"
  mkdir -p "$home/.local/lib/tariboy/old" "$home/.local/bin"
  for name in "${binaries[@]}"; do
    printf 'old\n' >"$home/.local/lib/tariboy/old/$name"
  done
  for pair in "${links[@]}"; do
    name=${pair%%:*}
    source=${pair#*:}
    ln -s "$home/.local/lib/tariboy/old/$source" "$home/.local/bin/$name"
  done
}

install_server() {
  HOME="$home" PATH="$tools:$PATH" make --no-print-directory -C "$ROOT" \
    GO="$tools/go" BINDIR="$tmp/build" server-install
}

install_ordinary() {
  HOME="$home" PATH="$tools:$PATH" make --no-print-directory -C "$ROOT" \
    GO="$tools/go" BINDIR="$tmp/build" PREFIX="$prefix" install
}

assert_old_links() {
  for pair in "${links[@]}"; do
    name=${pair%%:*}
    source=${pair#*:}
    test "$(readlink "$home/.local/bin/$name")" = "$home/.local/lib/tariboy/old/$source"
  done
}

assert_old_links_except_ttasks() {
  for pair in "${links[@]}"; do
    name=${pair%%:*}
    source=${pair#*:}
    test "$name" = ttasks || test "$(readlink "$home/.local/bin/$name")" = "$home/.local/lib/tariboy/old/$source"
  done
}

version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' "$ROOT/internal/version/version.go")

mkdir -p "$prefix/bin" "$tmp/foreign-ttasks"
ln -s "$tmp/foreign-ttasks" "$prefix/bin/ttasks"
install_ordinary
test "$(readlink "$prefix/bin/ttasks")" = tariboy-tasks
test ! -e "$tmp/foreign-ttasks/tariboy-tasks"

seed_old
install_server

for pair in "${links[@]}"; do
  name=${pair%%:*}
  source=${pair#*:}
  expected="$home/.local/lib/tariboy/$version/$source"
  test "$(readlink "$home/.local/bin/$name")" = "$expected"
  test "$("$home/.local/bin/$name" --version)" = "$version"
done
test "$(cat "$home/.local/lib/tariboy/old/tariboy")" = old
test -f "$home/.local/lib/tariboy/$version/tariboy-tasks"
test ! -e "$home/.local/lib/tariboy/$version/ttasks"

# A pre-Tasks release has neither payload nor alias. Reactivation removes the
# newer managed names, and selecting the new release again restores them.
old_release="$home/.local/lib/tariboy/old"
rm "$old_release/tariboy-tasks"
printf 'old\n' >"$old_release/VERSION"
(cd "$old_release" && sha256sum tariboyd tariboy tariboy-shim tariboy-store tariboy-plugin-telegram >SHA256SUMS)
if HOME="$home" PATH="$tools:$PATH" TARIBOY_TEST_FAIL_MV_TO="$home/.local/bin/tariboy-shim" \
  sh "$ROOT/desktop/src-tauri/src/remote-install.sh" old .stage-rollback-fail activate "$version"; then
  echo "forced pre-Tasks activation failure was accepted" >&2
  exit 1
fi
for pair in "${links[@]}"; do
  test "$(readlink "$home/.local/bin/${pair%%:*}")" = "$home/.local/lib/tariboy/$version/${pair#*:}"
done
HOME="$home" sh "$ROOT/desktop/src-tauri/src/remote-install.sh" old .stage-rollback activate "$version"
for name in tariboy-tasks ttasks; do
  if test -e "$home/.local/bin/$name" || test -L "$home/.local/bin/$name"; then
    echo "pre-Tasks activation left a Tasks link: $name" >&2
    exit 1
  fi
done
for name in tariboyd tariboy tariboy-shim tariboy-store tariboy-plugin-telegram; do
  test "$(readlink "$home/.local/bin/$name")" = "$old_release/$name"
done
HOME="$home" sh "$ROOT/desktop/src-tauri/src/remote-install.sh" "$version" .stage-reactivate activate old
test "$("$home/.local/bin/ttasks" --version)" = "$version"

# An incomplete checksum list must not allow absent mandatory payloads.
rm "$old_release/tariboy-shim"
(cd "$old_release" && sha256sum tariboyd tariboy tariboy-store tariboy-plugin-telegram >SHA256SUMS)
if HOME="$home" sh "$ROOT/desktop/src-tauri/src/remote-install.sh" old .stage-incomplete activate "$version"; then
  echo "activation accepted an incomplete release" >&2
  exit 1
fi
for pair in "${links[@]}"; do
  test "$(readlink "$home/.local/bin/${pair%%:*}")" = "$home/.local/lib/tariboy/$version/${pair#*:}"
done

seed_old
rm "$home/.local/bin/ttasks"
printf 'foreign\n' >"$home/.local/bin/ttasks"
if install_server; then
  echo "foreign ttasks regular file was accepted" >&2
  exit 1
fi
assert_old_links_except_ttasks
test "$(cat "$home/.local/bin/ttasks")" = foreign

seed_old
rm "$home/.local/bin/ttasks"
printf 'foreign\n' >"$home/.local/lib/tariboy/old/ttasks"
ln -s "$home/.local/lib/tariboy/old/ttasks" "$home/.local/bin/ttasks"
if install_server; then
  echo "foreign ttasks symlink was accepted" >&2
  exit 1
fi
assert_old_links_except_ttasks
test "$(readlink "$home/.local/bin/ttasks")" = "$home/.local/lib/tariboy/old/ttasks"

seed_old
if TARIBOY_TEST_FAIL_MV_TO="$home/.local/bin/ttasks" install_server; then
  echo "forced ttasks switch failure was accepted" >&2
  exit 1
fi
assert_old_links

echo "server install contract passed"
