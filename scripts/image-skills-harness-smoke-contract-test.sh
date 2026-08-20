#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
test -x "$ROOT/bin/tariboy"
test -x "$ROOT/bin/tariboyd"
output="$("$ROOT/scripts/image-skills-harness-smoke.sh")"
printf '%s\n' "$output"
grep -F "OK: harness-integrated image skills smoke" <<<"$output" >/dev/null

if missing_auth_output="$(TARIBOY_IMAGE_SKILLS_REAL_HARNESS=codex "$ROOT/scripts/image-skills-harness-smoke.sh" 2>&1)"; then
  echo "FAIL: real Codex smoke accepted a missing explicit test auth.json" >&2
  exit 1
fi
grep -F "requires TARIBOY_IMAGE_SKILLS_CODEX_AUTH_JSON to name an explicit test auth.json" <<<"$missing_auth_output" >/dev/null
