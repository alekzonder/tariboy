#!/usr/bin/env bash
set -euo pipefail

# `make designer-check` is the designer's hand-off check: type check, oxlint on
# the files changed against origin/main, the format check, the five policy
# tests, Knip, and the Storybook build, under the host check slot. A dry run prints the recipe
# without running it, so this test pins the steps and their order.
repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
recipe="$(make --no-print-directory -n -C "$repo_root" designer-check)"

rest="$recipe"
for part in \
  'tariboy-check.lock' \
  'tsc -b' \
  'merge-base origin/main' \
  'oxlint' \
  'npm run fmt:check' \
  'TARIBOY_VITEST_MAX_WORKERS=2' \
  'viewLintPolicy.test.ts' \
  'storybookSafety.test.ts' \
  'storyNetwork.test.tsx' \
  'arbitraryPxPolicy.test.ts' \
  'textInputPolicy.test.ts' \
  'npm run knip' \
  'build-storybook'; do
  case "$rest" in
    *"$part"*) rest="${rest#*"$part"}" ;;
    *)
      echo "FAIL: designer-check recipe lacks '$part' after the previous steps" >&2
      exit 1
      ;;
  esac
done

# The target is not part of `check`; it must still be a declared phony target.
grep -Eq '^\.PHONY:.* designer-check( |$)' "$repo_root/Makefile"

echo "designer-check contract ok"
