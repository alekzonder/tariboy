#!/usr/bin/env bash

SMOKE_CODEX_HOME=""
SMOKE_CODEX_BIN_DIR=""

smoke_codex_env_cleanup() {
  [ -z "$SMOKE_CODEX_HOME" ] || rm -rf -- "$SMOKE_CODEX_HOME"
}

smoke_codex_env_setup() {
  local source_home="$1"
  local configured_bin="$2"
  local resolved_bin
  resolved_bin="$(command -v "$configured_bin" 2>/dev/null || true)"
  if [ -n "$resolved_bin" ] && [ "${resolved_bin#/}" = "$resolved_bin" ]; then
    resolved_bin="$(cd "$(dirname "$resolved_bin")" && pwd)/$(basename "$resolved_bin")"
  fi

  SMOKE_CODEX_HOME="$(mktemp -d)"
  # This narrow trap is installed before credentials are copied. The caller may
  # replace it only with a broader cleanup that calls smoke_codex_env_cleanup.
  trap smoke_codex_env_cleanup EXIT
  SMOKE_CODEX_BIN_DIR="$SMOKE_CODEX_HOME/bin"
  mkdir -p "$SMOKE_CODEX_BIN_DIR"
  if [ -n "$resolved_bin" ]; then
    ln -s "$resolved_bin" "$SMOKE_CODEX_BIN_DIR/codex"
  fi

  local codex_file
  for codex_file in auth.json config.toml; do
    if [ -f "$source_home/$codex_file" ]; then
      cp "$source_home/$codex_file" "$SMOKE_CODEX_HOME/$codex_file"
    fi
  done
  export CODEX_HOME="$SMOKE_CODEX_HOME"
  export PATH="$SMOKE_CODEX_BIN_DIR:$PATH"
}
