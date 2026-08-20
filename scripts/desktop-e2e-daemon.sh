#!/bin/sh
set -eu

: "${TARIBOY_REAL_DAEMON_BIN:?TARIBOY_REAL_DAEMON_BIN is required}"
: "${TARIBOY_BASE_DIR:?TARIBOY_BASE_DIR is required}"
: "${TARIBOY_RUNTIME_DIR:?TARIBOY_RUNTIME_DIR is required}"

canonical_path() {
  realpath -m -- "$1"
}

live_base=$(canonical_path "$HOME/.tariboy")
live_runtime=$(canonical_path "$HOME/.tariboyd")
test_base=$(canonical_path "$TARIBOY_BASE_DIR")
test_runtime=$(canonical_path "$TARIBOY_RUNTIME_DIR")

if [ "$test_base" = "$live_base" ] || [ "$test_runtime" = "$live_runtime" ]; then
  echo "refusing live Tariboy state directory" >&2
  exit 64
fi

previous=
for argument in "$@"; do
  if { [ "$previous" = "--http-addr" ] || [ "$previous" = "--web-addr" ]; } &&
     [ "$argument" = "127.0.0.1:9990" ]; then
    echo "refusing live Tariboy listener 127.0.0.1:9990" >&2
    exit 64
  fi
  case "$argument" in
    --http-addr=127.0.0.1:9990|--web-addr=127.0.0.1:9990)
      echo "refusing live Tariboy listener 127.0.0.1:9990" >&2
      exit 64
      ;;
  esac
  previous=$argument
done

exec env TARIBOY_SHELL_ENV=1 "$TARIBOY_REAL_DAEMON_BIN" "$@"
