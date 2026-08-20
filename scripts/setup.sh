#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
skills_root="$repo_root/ai/skills"

if ! command -v npx >/dev/null 2>&1; then
  echo "npx is required to install project skills." >&2
  exit 1
fi

if [[ ! -d "$skills_root" ]]; then
  echo "Skills directory not found: $skills_root" >&2
  exit 1
fi

installed=0
while IFS= read -r -d '' manifest; do
  skill_dir="${manifest%/SKILL.md}"
  skill_name="${skill_dir##*/}"

  echo "Installing project skill: $skill_name"
  (
    cd "$repo_root"
    npx --yes skills add "$skill_dir" --skill "$skill_name" --agent codex -y
  )
  installed=$((installed + 1))
done < <(find "$skills_root" -mindepth 2 -type f -name SKILL.md -print0 | sort -z)

if (( installed == 0 )); then
  echo "No skill packages found under $skills_root." >&2
  exit 1
fi
