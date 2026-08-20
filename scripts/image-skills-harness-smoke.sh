#!/usr/bin/env bash
set -euo pipefail
trap 'echo "FAIL: image skills smoke at ${BASH_SOURCE[0]}:${LINENO}" >&2' ERR

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ORIGINAL_PATH="$PATH"
SMOKE_ROOT="$(mktemp -d)"
export TARIBOY_BASE_DIR="$SMOKE_ROOT/base"
export TARIBOY_RUNTIME_DIR="$SMOKE_ROOT/runtime"
export TARIBOY_HTTP_ADDR="127.0.0.1:0"
export HOME="$SMOKE_ROOT/home"
FIXTURE_CWD="$HOME/project"
FIXTURE_SOURCE="$SMOKE_ROOT/image"
RECORD_DIR="$SMOKE_ROOT/records"
FAKE_BIN="$SMOKE_ROOT/fake-bin"
mkdir -p "$TARIBOY_BASE_DIR" "$TARIBOY_RUNTIME_DIR" "$FIXTURE_CWD" \
  "$FIXTURE_SOURCE/skills/inventory-proof" "$RECORD_DIR" "$FAKE_BIN"

cleanup() {
  "$ROOT/bin/tariboy" daemon stop >/dev/null 2>&1 || true
  if [ "${TARIBOY_IMAGE_SKILLS_KEEP:-0}" = "1" ]; then
    echo "DEBUG: retained smoke root $SMOKE_ROOT" >&2
    return
  fi
  rm -rf "$SMOKE_ROOT"
}
trap cleanup EXIT

cat >"$FIXTURE_SOURCE/Tariboyfile.yaml" <<'YAML'
schema_version: 2
plugins:
  - name: loop
skills:
  - dir: ./skills/inventory-proof
prompts: []
YAML
cat >"$FIXTURE_SOURCE/skills/inventory-proof/SKILL.md" <<'SKILL'
---
name: inventory-proof
description: Proves that image skills reach the harness inventory.
---
# Inventory proof

When asked to prove this skill was used, respond with the exact phrase
TARIBOY_IMAGE_SKILL_PROOF_7C91.
SKILL

FAKE_HARNESS="$SMOKE_ROOT/fake-harness.sh"
cat >"$FAKE_HARNESS" <<'HARNESS'
#!/bin/sh
set -eu
harness="$(basename "$0")"
if [ "$harness" = "claude" ] && [ "${1:-}" = "--version" ]; then
  echo "2.1.227 (Claude Code)"
  exit 0
fi
if [ "$harness" = "codex" ] && [ "${1:-}" = "--version" ]; then
  echo "codex-cli 0.147.0"
  exit 0
fi
if [ "$harness" = "opencode" ] && [ "${1:-}" = "debug" ] && [ "${2:-}" = "config" ]; then
  cat "$OPENCODE_CONFIG_DIR/opencode.json"
  exit 0
fi
record="$SMOKE_RECORD_DIR/$harness-$TARIBOY_ITERATION"
printf '%s\n' "$@" >"$record.args"
printf '%s\n' "$HOME" >"$record.home"
pwd >"$record.cwd"
printf '%s\n' "${OPENCODE_CONFIG_DIR:-}" >"$record.config-dir"
cat >"$record.prompt"
exit 0
HARNESS
chmod 0700 "$FAKE_HARNESS"
for harness in claude codex opencode; do
  cp "$FAKE_HARNESS" "$FAKE_BIN/$harness"
done

DAEMON_WRAPPER="$TARIBOY_RUNTIME_DIR/tariboyd-no-http.sh"
cat >"$DAEMON_WRAPPER" <<WRAPPER
#!/bin/sh
exec env TARIBOY_SHELL_ENV=1 "$ROOT/bin/tariboyd" --web-addr "127.0.0.1:0" "\$@"
WRAPPER
chmod 0700 "$DAEMON_WRAPPER"
export TARIBOY_DAEMON_BIN="$DAEMON_WRAPPER"

tb() { "$ROOT/bin/tariboy" "$@"; }
tb daemon start >/dev/null
for _ in $(seq 1 100); do
  tb daemon status >/dev/null 2>&1 && break
  sleep 0.1
done
tb daemon status >/dev/null
tb image build --path "$FIXTURE_SOURCE" --name image-skills-smoke >/dev/null

wait_iteration() {
  local agent="$1"
  for _ in $(seq 1 200); do
    local status
    status="$(tb --json iteration ls "$agent" | python3 -c 'import json,sys; rows=json.load(sys.stdin).get("iterations") or []; print(rows[-1].get("status", "") if rows else "")')"
    case "$status" in
      done|no_i_am_done) return 0 ;;
      harness_error|timeout|killed)
        echo "FAIL: $agent iteration ended $status" >&2
        tb --json iteration ls "$agent" >&2 || true
        tail -n 80 "$TARIBOY_RUNTIME_DIR/tariboyd.log" >&2 || true
        return 1
        ;;
    esac
    sleep 0.1
  done
  echo "FAIL: timed out waiting for $agent" >&2
  return 1
}

bridge_manifest() {
  local agent="$1"
  find "$TARIBOY_BASE_DIR/agents/$agent/image-bridges" -name bridge-manifest.json -type f -print -quit
}

file_stat() {
  python3 -c 'import os,sys; s=os.stat(sys.argv[1]); print(f"{s.st_ino}:{s.st_mtime_ns}")' "$1"
}

run_iteration() {
  local agent="$1"
  tb agent exec "$agent" "inventory smoke" >/dev/null
  wait_iteration "$agent"
}

for harness in claude codex opencode; do
  agent="image-skills-$harness"
  tb agent run image-skills-smoke:latest --name "$agent" --harness "$harness" \
    --cwd "$FIXTURE_CWD" --loop false --interactive false \
    --env "PATH=$FAKE_BIN:$ORIGINAL_PATH,SMOKE_RECORD_DIR=$RECORD_DIR" >/dev/null
  run_iteration "$agent"
  manifest="$(bridge_manifest "$agent")"
  [ -n "$manifest" ]
  bridge_dir="$(dirname "$manifest")"
  record_home="$(find "$RECORD_DIR" -name "$harness-*.home" -type f -print | sort | head -n 1)"
  [ -n "$record_home" ]
  record="${record_home%.home}"
  case "$harness" in
    claude) grep -Fx -- "$bridge_dir" "$record.args" >/dev/null ;;
    codex)
      grep -F -- "## Image skills" "$record.prompt" >/dev/null
      grep -F -- "$bridge_dir/skills/inventory-proof/SKILL.md" "$record.prompt" >/dev/null
      ! grep -F -- "marketplaces." "$record.args" >/dev/null
      test ! -e "$bridge_dir/marketplace"
      ;;
    opencode) grep -Fx -- "$bridge_dir" "$record.config-dir" >/dev/null ;;
  esac
  grep -Fx -- "$HOME" "$record.home" >/dev/null
  grep -Fx -- "$FIXTURE_CWD" "$record.cwd" >/dev/null
  before="$(file_stat "$manifest")"
  run_iteration "$agent"
  after="$(file_stat "$manifest")"
  [ "$before" = "$after" ]
done

for hidden in .claude .agents .codex .opencode; do
  test ! -e "$FIXTURE_CWD/$hidden"
done

claude_bridge="$(dirname "$(bridge_manifest image-skills-claude)")"
codex_bridge="$(dirname "$(bridge_manifest image-skills-codex)")"
opencode_bridge="$(dirname "$(bridge_manifest image-skills-opencode)")"
[ "$claude_bridge" != "$codex_bridge" ]
[ "$codex_bridge" != "$opencode_bridge" ]
case "$claude_bridge" in */2/claude) ;; *) echo "FAIL: contract version missing from $claude_bridge" >&2; exit 1 ;; esac

tb image build --path "$FIXTURE_SOURCE" --name image-skills-smoke-next >/dev/null
tb agent run image-skills-smoke-next:latest --name image-skills-new-digest --harness claude \
  --cwd "$FIXTURE_CWD" --loop false --interactive false \
  --env "PATH=$FAKE_BIN:$ORIGINAL_PATH,SMOKE_RECORD_DIR=$RECORD_DIR" >/dev/null
run_iteration image-skills-new-digest
next_bridge="$(dirname "$(bridge_manifest image-skills-new-digest)")"
[ "$next_bridge" != "$claude_bridge" ]

REAL_HARNESS="${TARIBOY_IMAGE_SKILLS_REAL_HARNESS:-}"
if [ -n "$REAL_HARNESS" ]; then
  case "$REAL_HARNESS" in claude|codex|opencode|all) ;; *) echo "FAIL: TARIBOY_IMAGE_SKILLS_REAL_HARNESS must be claude, codex, opencode, or all" >&2; exit 1 ;; esac
  if [ "$REAL_HARNESS" = "codex" ] || [ "$REAL_HARNESS" = "all" ]; then
    CODEX_TEST_AUTH="${TARIBOY_IMAGE_SKILLS_CODEX_AUTH_JSON:-}"
    if [ -z "$CODEX_TEST_AUTH" ] || [ ! -f "$CODEX_TEST_AUTH" ]; then
      echo "FAIL: real Codex image-skill smoke requires TARIBOY_IMAGE_SKILLS_CODEX_AUTH_JSON to name an explicit test auth.json" >&2
      exit 1
    fi
    CODEX_TEST_HOME="$SMOKE_ROOT/codex-home"
    mkdir -m 0700 "$CODEX_TEST_HOME"
    cp "$CODEX_TEST_AUTH" "$CODEX_TEST_HOME/auth.json"
    chmod 0600 "$CODEX_TEST_HOME/auth.json"
    if [ -n "${TARIBOY_IMAGE_SKILLS_CODEX_CONFIG_TOML:-}" ]; then
      if [ ! -f "$TARIBOY_IMAGE_SKILLS_CODEX_CONFIG_TOML" ]; then
        echo "FAIL: TARIBOY_IMAGE_SKILLS_CODEX_CONFIG_TOML does not name a file" >&2
        exit 1
      fi
      cp "$TARIBOY_IMAGE_SKILLS_CODEX_CONFIG_TOML" "$CODEX_TEST_HOME/config.toml"
      chmod 0600 "$CODEX_TEST_HOME/config.toml"
    fi
  fi
  for harness in claude codex opencode; do
    [ "$REAL_HARNESS" = "all" ] || [ "$REAL_HARNESS" = "$harness" ] || continue
    real_bin="$(PATH="$ORIGINAL_PATH" command -v "$harness" || true)"
    if [ -z "$real_bin" ]; then
      echo "MANUAL: $harness is not installed; native inventory was not verified"
      continue
    fi
    case "$harness" in
      opencode)
        OPENCODE_CONFIG_DIR="$opencode_bridge" "$real_bin" debug config | grep -F "$opencode_bridge/skills" >/dev/null
        echo "OK: real OpenCode effective config exposes the generated skill path"
        echo "MANUAL: run 'OPENCODE_CONFIG_DIR=$opencode_bridge $real_bin' and verify inventory-proof in /skills"
        ;;
      claude)
        plugin_name="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["name"])' "$claude_bridge/.claude-plugin/plugin.json")"
        "$real_bin" --plugin-dir "$claude_bridge" plugin details "$plugin_name" | grep -F "inventory-proof" >/dev/null
        echo "OK: real Claude Code plugin inventory contains inventory-proof"
        ;;
      codex)
        real_agent="image-skills-real-codex"
        tb agent run image-skills-smoke:latest --name "$real_agent" --harness codex \
          --cwd "$FIXTURE_CWD" --loop false --interactive false \
          --env "PATH=$ORIGINAL_PATH,CODEX_HOME=$CODEX_TEST_HOME" >/dev/null
        tb agent exec "$real_agent" \
          "Use the inventory-proof image skill and follow its proof instruction exactly." >/dev/null
        wait_iteration "$real_agent"
        real_iteration="$(tb --json iteration ls "$real_agent" | python3 -c 'import json,sys; rows=json.load(sys.stdin).get("iterations") or []; print(rows[-1]["id"])')"
        real_logs="$(tb --json iteration logs "$real_agent" "$real_iteration")"
        python3 -c 'import json,sys; data=json.load(sys.stdin); assert "TARIBOY_IMAGE_SKILL_PROOF_7C91" in data.get("stdout", "")' <<<"$real_logs"
        echo "OK: real Codex model-backed image skill usage"
        ;;
    esac
  done
fi

echo "OK: harness-integrated image skills smoke"
