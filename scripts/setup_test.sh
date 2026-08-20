#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "$fixture_dir"' EXIT

mkdir -p "$fixture_dir/bin"
log_file="$fixture_dir/npx.log"

printf '%s\n' '#!/usr/bin/env bash' 'printf "PWD=%s\\n" "$PWD" > "$SETUP_TEST_NPX_LOG"' 'printf "%s\\n" "$@" >> "$SETUP_TEST_NPX_LOG"' > "$fixture_dir/bin/npx"
chmod +x "$fixture_dir/bin/npx"

(
  cd "$fixture_dir"
  PATH="$fixture_dir/bin:$PATH" SETUP_TEST_NPX_LOG="$log_file" make -C "$repo_root" setup
)

expected_file="$fixture_dir/expected.log"
printf '%s\n' \
  "PWD=$repo_root" \
  --yes \
  skills \
  add \
  "$repo_root/ai/skills/write-docs" \
  --skill \
  write-docs \
  --agent \
  codex \
  -y > "$expected_file"

diff -u "$expected_file" "$log_file"
