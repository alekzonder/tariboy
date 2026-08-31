#!/usr/bin/env python3
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import UsageError, call, execute, parse_flags, print_result


def run(args):
    if args[:1] == ["add"]:
        flags, _ = parse_flags(args, 1, {"kind", "spec", "channel", "message"})
        body = {"kind": flags.get("kind", ""), "spec": flags.get("spec", ""), "channel": flags.get("channel", "")}
        if "message" in flags:
            try:
                body["message"] = json.loads(flags["message"])
            except json.JSONDecodeError as error:
                raise UsageError(f"tools schedule add: --message is not valid JSON: {error}") from error
        print_result(call("POST", "/tools/schedule/add", body))
        return
    if args == ["ls"]:
        print_result(call("GET", "/tools/schedule/ls"))
        return
    if args[:1] == ["cancel"] and len(args) > 1:
        _, pos = parse_flags(args, 1)
        print_result(call("POST", "/tools/schedule/cancel", {"id": pos[0]}))
        return
    raise UsageError("tools schedule: add, ls, or cancel is required")


if __name__ == "__main__":
    raise SystemExit(execute(lambda: run(sys.argv[1:])))
