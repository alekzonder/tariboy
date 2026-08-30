#!/usr/bin/env python3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import main


if __name__ == "__main__":
    args = sys.argv[1:]
    if args[:1] == ["set"]:
        if len(args) < 2:
            print("tools status set: <message...> required", file=sys.stderr)
            raise SystemExit(2)
        raise SystemExit(main("POST", "/tools/status/set", {"message": " ".join(args[1:])}, cli_args=args[1:]))
    raise SystemExit(main("GET", "/tools/status", cli_args=args))
