#!/bin/sh
exec python3 -B "$(dirname "$0")/status.py" "$@"
