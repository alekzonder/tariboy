#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if grep -Fq 'desktop-smoke.sh' "$ROOT/scripts/package-alpha.sh"; then
  echo "FAIL: alpha packaging must not run desktop-smoke.sh" >&2
  exit 1
fi

echo "OK: alpha packaging does not run the desktop smoke suite"
