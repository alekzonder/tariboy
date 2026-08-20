#!/usr/bin/env bash
set -euo pipefail

if test "$#" -ne 1 || test -z "$1"; then
  printf '%s\n' "usage: $0 <tag>" >&2
  exit 64
fi

script_dir=$(cd -- "$(dirname -- "$0")" && pwd -P)
exec tariboy image build --name tariboy-developer --path "$script_dir" --tag "$1"
