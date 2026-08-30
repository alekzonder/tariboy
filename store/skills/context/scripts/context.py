#!/usr/bin/env python3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import main


if __name__ == "__main__":
    args = sys.argv[1:]
    if args[:1] == ["get"]:
        raise SystemExit(main("GET", "/tools/context/get", text_key="text", cli_args=args[1:]))
    if args[:1] == ["set"]:
        raise SystemExit(main("POST", "/tools/context/set", {"text": " ".join(args[1:])}, cli_args=args[1:]))
    print("tools context: get or set is required", file=sys.stderr)
    raise SystemExit(2)
