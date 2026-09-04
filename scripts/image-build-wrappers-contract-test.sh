#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT

mkdir -p "$tmp/bin"
cat >"$tmp/bin/tariboy" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >"$TARIBOY_ARGS"
SH
chmod +x "$tmp/bin/tariboy"

for image in llm-as-judge tariboss; do
  script="$root/store/images/$image/build.sh"
  args="$tmp/$image.args"
  TARIBOY_ARGS="$args" PATH="$tmp/bin:$PATH" "$script" v1
  [ "$(cat "$args")" = "image build --name $image --path $root/store/images/$image --tag v1" ]
  if TARIBOY_ARGS="$args" PATH="$tmp/bin:$PATH" "$script" >/dev/null 2>&1; then
    echo "FAIL: $image wrapper accepted a missing tag" >&2
    exit 1
  fi
  if TARIBOY_ARGS="$args" PATH="$tmp/bin:$PATH" "$script" '' >/dev/null 2>&1; then
    echo "FAIL: $image wrapper accepted an empty tag" >&2
    exit 1
  fi
  if TARIBOY_ARGS="$args" PATH="$tmp/bin:$PATH" "$script" v1 v2 >/dev/null 2>&1; then
    echo "FAIL: $image wrapper accepted multiple tags" >&2
    exit 1
  fi
done

echo "OK: image build wrappers"
