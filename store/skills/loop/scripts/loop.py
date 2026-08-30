#!/usr/bin/env python3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import main


if __name__ == "__main__":
    args = sys.argv[1:]
    if args[:1] == ["done"]:
        body = None
        for index, arg in enumerate(args[1:], 1):
            if arg == "--idle":
                value = args[index + 1] if index + 1 < len(args) and not args[index + 1].startswith("--") else "true"
                body = {"idle": value.lower() not in {"0", "false"}}
            elif arg.startswith("--idle="):
                body = {"idle": arg.split("=", 1)[1].lower() not in {"0", "false"}}
        raise SystemExit(main("POST", "/tools/loop/done", body, cli_args=args[1:], allowed_flags={"idle"}))
    if args[:1] and args[0] in {"start", "stop"}:
        raise SystemExit(main("POST", "/tools/loop/control", {"action": args[0]}, cli_args=args[1:]))
    print("tools loop: done, start, or stop is required", file=sys.stderr)
    raise SystemExit(2)
