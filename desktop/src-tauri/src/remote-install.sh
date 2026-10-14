#!/bin/sh
set -eu
: "${HOME:?HOME is required}"

version=${1-}
staging=${2-}
mode=${3-install}
expected_previous=${4-}

case "$version" in
  ""|*[!A-Za-z0-9._-]*|.*|*/*) echo "invalid version" >&2; exit 64 ;;
esac
case "$staging" in
  .stage-*) ;;
  *) echo "invalid staging basename" >&2; exit 64 ;;
esac
case "${staging#.stage-}" in
  ""|*[!A-Za-z0-9-]*) echo "invalid staging basename" >&2; exit 64 ;;
esac
case "$mode" in install|stage|activate) ;; *) echo "invalid install mode" >&2; exit 64 ;; esac
if test "$mode" = activate; then
  case "$expected_previous" in
    *[!A-Za-z0-9._-]*|.*|*/*) echo "invalid expected version" >&2; exit 64 ;;
  esac
elif test -n "$expected_previous"; then
  echo "expected version is only valid in activate mode" >&2
  exit 64
fi

root=$HOME/.local/lib/tariboy
stage=$root/$staging
release=$root/$version
bindir=$HOME/.local/bin
suffix=${staging#.stage-}
switched=
release_created=false
committed=false
sources="tariboyd tariboy tariboy-shim tariboy-plugin-telegram"
managed_links="tariboyd:tariboyd tariboy:tariboy tariboy-tasks:tariboy-tasks ttasks:tariboy-tasks tariboy-shim:tariboy-shim tariboy-plugin-telegram:tariboy-plugin-telegram"
# macOS ships `shasum` instead of coreutils `sha256sum`; both read SHA256SUMS.
verify_checksums() {
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$1" && sha256sum -c SHA256SUMS >&2)
  else
    (cd "$1" && shasum -a 256 -c SHA256SUMS >&2)
  fi
}
binary_source=$stage
test "$mode" != activate || binary_source=$release
if test "$mode" != activate || test -e "$binary_source/tariboy-tasks" || test -L "$binary_source/tariboy-tasks"; then
  sources="$sources tariboy-tasks"
fi
if test -f "$binary_source/tariboy-store"; then
  sources="$sources tariboy-store"
fi
if test -f "$binary_source/tariboy-store" || test -L "$bindir/tariboy-store"; then
  managed_links="$managed_links tariboy-store:tariboy-store"
fi

if test "$mode" != activate; then
  test -d "$stage"
  test "$(cat "$stage/VERSION")" = "$version"
  verify_checksums "$stage"

  for source in $sources; do
    test -f "$stage/$source"
    chmod 0755 "$stage/$source"
  done
fi

rollback() {
  if test "$committed" != true; then
    for entry in $managed_links; do
      name=${entry%%:*}
      target=$bindir/$name
      backup=$bindir/.$name.old-$suffix
      case " $switched " in
        *" $name "*) rm -f "$target" ;;
      esac
      if test -e "$backup" || test -L "$backup"; then
        mv -f "$backup" "$target" || true
      fi
    done
    if test "$release_created" = true; then
      rm -rf "$release"
    fi
  fi
  for entry in $managed_links; do
    name=${entry%%:*}
    rm -f "$bindir/.$name.new-$suffix"
    rm -f "$bindir/.$name.old-$suffix"
  done
}
trap rollback EXIT
trap 'exit 1' HUP INT TERM

# Tests shorten the lock wait; Desktop and `tariboy update` never set it.
lock_wait=${TARIBOY_INSTALL_LOCK_WAIT:-60}
case "$lock_wait" in ""|*[!0-9]*) lock_wait=60 ;; esac
exec 9>"$root/.install.lock"
if command -v flock >/dev/null 2>&1; then
  flock -w "$lock_wait" 9 || {
    echo "another tariboy install holds $root/.install.lock" >&2
    exit 75
  }
elif command -v python3 >/dev/null 2>&1; then
  # macOS has no flock(1). The lock belongs to the open file description that
  # fd 9 shares with this shell, so it outlives python3 just like flock 9.
  python3 -c '
import fcntl, sys, time
deadline = time.monotonic() + int(sys.argv[1])
while True:
    try:
        fcntl.flock(9, fcntl.LOCK_EX | fcntl.LOCK_NB)
        break
    except BlockingIOError:
        if time.monotonic() >= deadline:
            sys.exit(75)
        time.sleep(0.1)
' "$lock_wait" || {
    echo "another tariboy install holds $root/.install.lock" >&2
    exit 75
  }
else
  echo "flock or python3 is required for safe installation" >&2
  exit 69
fi

mkdir -p "$bindir"
umask 022
previous=
legacy_tools=$bindir/tariboy-tools
remove_legacy_tools=false
if test -L "$legacy_tools"; then
  legacy_target=$(readlink "$legacy_tools")
  legacy_version=$(basename "$(dirname "$legacy_target")")
  case "$legacy_version" in
    ""|*[!A-Za-z0-9._-]*|.*|*/*) ;;
    *) test "$legacy_target" != "$root/$legacy_version/tariboy-tools" || remove_legacy_tools=true ;;
  esac
fi
for entry in $managed_links; do
  name=${entry%%:*}
  source=${entry#*:}
  target=$bindir/$name
  if test -e "$target" && ! test -L "$target"; then
    echo "refusing to replace non-symlink $target" >&2
    exit 1
  fi
  if test -L "$target"; then
    old_target=$(readlink "$target")
    old_version=$(basename "$(dirname "$old_target")")
    case "$old_version" in
      ""|*[!A-Za-z0-9._-]*|.*|*/*) echo "invalid current release link" >&2; exit 1 ;;
    esac
    expected_target=$root/$old_version/$source
    if test "$old_target" != "$expected_target"; then
      echo "refusing to replace foreign symlink $target -> $old_target" >&2
      exit 1
    fi
    if test -z "$previous"; then
      previous=$old_version
    elif test "$previous" != "$old_version"; then
      echo "current tariboy links are inconsistent" >&2
      exit 1
    fi
  fi
done

if test "$mode" = activate; then
  test -d "$release"
  test "$(cat "$release/VERSION")" = "$version"
  verify_checksums "$release"
elif test -e "$release" || test -L "$release"; then
  test -d "$release"
  test "$(cat "$release/VERSION")" = "$version"
  verify_checksums "$release"
  rm -rf "$stage"
else
  mv "$stage" "$release"
  release_created=true
fi

for source in $sources; do
  test -f "$release/$source" || { echo "missing release payload: $source" >&2; exit 1; }
done

if test "$mode" = stage; then
  committed=true
  rollback
  trap - EXIT HUP INT TERM
  printf '{"previous":"%s","version":"%s"}\n' "$previous" "$version"
  exit 0
fi

if test "$mode" = activate && test "$previous" != "$expected_previous"; then
  echo "release conflict: current $previous, expected $expected_previous" >&2
  exit 76
fi

for entry in $managed_links; do
  name=${entry%%:*}
  source=${entry#*:}
  # Releases predating an optional payload must restore its absence too.
  if test -f "$release/$source"; then
    ln -s "$release/$source" "$bindir/.$name.new-$suffix"
  fi
done
for entry in $managed_links; do
  name=${entry%%:*}
  target=$bindir/$name
  backup=$bindir/.$name.old-$suffix
  if test -L "$target"; then
    mv "$target" "$backup"
  fi
  if test -L "$bindir/.$name.new-$suffix"; then
    mv "$bindir/.$name.new-$suffix" "$target"
  fi
  switched="$switched $name"
done
committed=true
test "$remove_legacy_tools" != true || rm -f "$legacy_tools"
for entry in $managed_links; do
  name=${entry%%:*}
  rm -f "$bindir/.$name.old-$suffix" || true
done
rollback
trap - EXIT HUP INT TERM

if test "$mode" = activate; then
  printf '{"previous":"%s","version":"%s"}\n' "$previous" "$version"
else
  printf 'installed %s\n' "$version"
fi
