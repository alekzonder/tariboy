#!/usr/bin/env python3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import UsageError, call, execute, parse_flags, print_result


def post(route, body):
    print_result(call("POST", route, body))


def create(args, scheduled):
    action = "schedule" if scheduled else "run"
    if len(args) < 3 or "--" not in args[2:]:
        raise UsageError(f"tools script {action}: NAME [options] -- COMMAND required")
    separator = args.index("--", 2)
    name = args[1]
    flags, pos = parse_flags(args[:separator], 2, {"description", "every", "quiet-exit"})
    if pos:
        raise UsageError(f'tools script {action}: unexpected argument "{pos[0]}" before --')
    command = " ".join(args[separator + 1:])
    if not command.strip():
        raise UsageError(f"tools script {action}: command is required after --")
    body = {"name": name, "description": flags.get("description", name), "command": command}
    if scheduled:
        try:
            every = int(flags.get("every", ""))
        except ValueError as error:
            raise UsageError("tools script schedule: --every must be a positive number of seconds") from error
        if every <= 0:
            raise UsageError("tools script schedule: --every must be a positive number of seconds")
        body["interval_seconds"] = every
        if "quiet-exit" in flags:
            try:
                quiet = int(flags["quiet-exit"])
            except ValueError as error:
                raise UsageError("tools script schedule: --quiet-exit must be between 0 and 255") from error
            if not 0 <= quiet <= 255:
                raise UsageError("tools script schedule: --quiet-exit must be between 0 and 255")
            body["quiet_exit"] = quiet
    post("/tools/script/" + action, body)


def run(args):
    if args[:1] == ["run"]:
        return create(args, False)
    if args[:1] == ["schedule"]:
        return create(args, True)
    if args == ["ls"]:
        print_result(call("GET", "/tools/script/ls"))
        return
    if args[:1] in (["rerun"], ["cancel"], ["rm"]):
        _, pos = parse_flags(args, 1)
        if not pos:
            raise UsageError(f"tools script {args[0]}: <id> is required")
        return post("/tools/script/" + args[0], {"id": pos[0]})
    if args[:1] in (["runs"], ["logs"]):
        _, pos = parse_flags(args, 1)
        if not pos:
            raise UsageError(f"tools script {args[0]}: <id> is required")
        print_result(call("GET", "/tools/script/" + args[0] + "/" + pos[0]))
        return
    raise UsageError("tools script: unknown command " + " ".join(args))


if __name__ == "__main__":
    raise SystemExit(execute(lambda: run(sys.argv[1:])))
