#!/usr/bin/env bash
set -euo pipefail

directory="${1:?usage: npm-ci-locked.sh DIR}"
cd -- "$directory"
stamp=node_modules/.tariboy-npm-ci.sha256
node_version=$(node --version)
npm_version=$(npm --version)
if command -v sha256sum >/dev/null 2>&1; then
  hash=(sha256sum)
else
  hash=(shasum -a 256)
fi
fingerprint=$({
  "${hash[@]}" package.json package-lock.json || exit "$?"
  printf '%s\n' "$node_version" "$npm_version"
} | "${hash[@]}")

# shortcut: manual node_modules or npm configuration changes need the force flag.
if [[ ${TARIBOY_NPM_CI_FORCE:-0} != 1 && -f "$stamp" && $(cat "$stamp") == "$fingerprint" ]]; then
  printf '%s: dependencies up to date\n' "$directory"
  exit 0
fi

rm -f -- "$stamp"
npm ci --no-audit --no-fund
printf '%s\n' "$fingerprint" >"$stamp"
