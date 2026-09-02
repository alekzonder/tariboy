#!/bin/sh
exec "${TARIBOY_PYTHON3:-python3}" -B "$(dirname "$0")/whoami.py" "$@"
