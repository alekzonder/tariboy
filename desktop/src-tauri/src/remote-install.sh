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

if test "$mode" != activate; then
  test -d "$stage"
  test "$(cat "$stage/VERSION")" = "$version"
  (cd "$stage" && sha256sum -c SHA256SUMS >&2)

  for name in tariboyd tariboy tariboy-shim tariboy-tools; do
    test -f "$stage/$name"
    chmod 0755 "$stage/$name"
  done
fi

rollback() {
  if test "$committed" != true; then
    for name in tariboy-tools tariboy-shim tariboy tariboyd; do
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
  for name in tariboyd tariboy tariboy-shim tariboy-tools; do
    rm -f "$bindir/.$name.new-$suffix"
    rm -f "$bindir/.$name.old-$suffix"
  done
}
trap rollback EXIT
trap 'exit 1' HUP INT TERM

command -v flock >/dev/null 2>&1 || {
  echo "flock is required for safe installation" >&2
  exit 69
}
exec 9>"$root/.install.lock"
flock -w 60 9 || {
  echo "another tariboy install holds $root/.install.lock" >&2
  exit 75
}

mkdir -p "$bindir"
umask 022
if test "$mode" = activate; then
  test -d "$release"
  test "$(cat "$release/VERSION")" = "$version"
  (cd "$release" && sha256sum -c SHA256SUMS >&2)
elif test -e "$release" || test -L "$release"; then
  test -d "$release"
  test "$(cat "$release/VERSION")" = "$version"
  (cd "$release" && sha256sum -c SHA256SUMS >&2)
  rm -rf "$stage"
else
  mv "$stage" "$release"
  release_created=true
fi

previous=
for name in tariboyd tariboy tariboy-shim tariboy-tools; do
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
    expected_target=$root/$old_version/$name
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

for name in tariboyd tariboy tariboy-shim tariboy-tools; do
  ln -s "$release/$name" "$bindir/.$name.new-$suffix"
done
for name in tariboyd tariboy tariboy-shim tariboy-tools; do
  target=$bindir/$name
  backup=$bindir/.$name.old-$suffix
  if test -L "$target"; then
    mv "$target" "$backup"
  fi
  mv "$bindir/.$name.new-$suffix" "$target"
  switched="$switched $name"
done
committed=true
for name in tariboyd tariboy tariboy-shim tariboy-tools; do
  rm -f "$bindir/.$name.old-$suffix" || true
done
rollback
trap - EXIT HUP INT TERM

if test "$mode" = activate; then
  printf '{"previous":"%s","version":"%s"}\n' "$previous" "$version"
else
  printf 'installed %s\n' "$version"
fi
