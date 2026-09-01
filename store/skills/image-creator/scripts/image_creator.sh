#!/bin/sh
exec python3 -B "$(dirname "$0")/image_creator.py" "$@"
