#!/usr/bin/env python3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import UsageError, call, execute, parse_flags, print_result


def run(args):
    if args[:1] != ["build"]:
        raise UsageError("tools image: build is required")
    flags, _ = parse_flags(args, 1, {"name", "tag", "path"})
    if not flags.get("name") or not flags.get("path"):
        raise UsageError("tools image build: --name and --path are required")
    body = {"name": flags["name"], "tag": flags.get("tag", "latest"), "path": flags["path"]}
    print_result(call("POST", "/tools/image/build", body))


if __name__ == "__main__":
    raise SystemExit(execute(lambda: run(sys.argv[1:])))
