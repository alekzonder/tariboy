#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
legacy=$(printf '\163\165\160\145\162\141\147\145\156\164')

if git -C "$root" grep -Iin -e "$legacy" -- .; then
  echo "legacy product token remains in tracked text" >&2
  exit 1
fi

bad_paths=$(git -C "$root" ls-files | awk -v token="$legacy" '
  BEGIN { token = tolower(token) }
  index(tolower($0), token) { print }
')
if test -n "$bad_paths"; then
  printf '%s\n' "$bad_paths" >&2
  echo "legacy product token remains in tracked paths" >&2
  exit 1
fi
