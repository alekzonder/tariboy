#!/usr/bin/env python3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import main


if __name__ == "__main__":
    args = sys.argv[1:]
    if args == ["--clear"]:
        raise SystemExit(main("POST", "/tools/task/current", {"clear": True}))
    if not args:
        print("tools task current: <task-key> is required (or pass --clear)", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(main("POST", "/tools/task/current", {"id": args[0]}, cli_args=args, allowed_flags={"clear"}))
