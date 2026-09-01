#!/usr/bin/env python3
import sys
from pathlib import Path

import http.client
import json
import os
import socket
class UsageError(Exception): pass
class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, path): super().__init__("localhost"); self.path = path
    def connect(self): self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); self.sock.connect(self.path)
def client_version(): return os.environ.get("TARIBOY_CLIENT_VERSION") or Path(__file__).resolve().parents[3].name
def call(method, route, body=None):
    c = UnixHTTPConnection(os.environ["TARIBOY_TOOLS_SOCKET"]); c.request(method, route, None if method == "GET" else json.dumps(body or {}).encode(), {"Content-Type": "application/json"}); r = c.getresponse(); daemon = r.getheader("X-Tariboy-Version", ""); envelope = json.load(r); c.close(); version = client_version()
    if daemon and daemon != version: print(f"warning: client version {version} does not match daemon version {daemon}; this client ({sys.argv[0]}) may not know the daemon's newer flags", file=sys.stderr)
    if not envelope.get("ok"): raise RuntimeError(envelope.get("error", {}).get("message", "daemon returned failure without error detail"))
    return envelope["result"]
def format_value(value):
    if value is None: return "<nil>"
    if isinstance(value, bool): return str(value).lower()
    if isinstance(value, list): return "[" + " ".join(format_value(item) for item in value) + "]"
    if isinstance(value, dict): return "map[" + " ".join(f"{key}:{format_value(value[key])}" for key in sorted(value)) + "]"
    return str(value)
def print_result(result):
    if os.environ.get("TARIBOY_TOOLS_JSON") == "1": print(json.dumps(result, separators=(",", ":")))
    elif isinstance(result, dict):
        for key in sorted(result): print(f"{key}: {format_value(result[key])}")
    elif isinstance(result, str): print(json.dumps(result)[1:-1])
    else: print(json.dumps(result, separators=(",", ":")))
def parse_flags(args, start=0, allowed=()):
    values = {}; positionals = []; allowed = set(allowed); index = start
    while index < len(args):
        arg = args[index]
        if arg.startswith("--"):
            name, separator, value = arg[2:].partition("=")
            if name not in allowed: raise UsageError(f"unknown flag --{name}")
            if not separator:
                if index + 1 < len(args) and not args[index + 1].startswith("--"): index += 1; value = args[index]
                else: value = "true"
            values[name] = value
        else: positionals.append(arg)
        index += 1
    return values, positionals
def execute(action):
    try: action(); return 0
    except KeyError: print("tools: TARIBOY_TOOLS_SOCKET is not set (are you running inside an agent?)", file=sys.stderr); return 2
    except UsageError as error: print(error, file=sys.stderr); return 2
    except OSError: print("tools: agent socket is not reachable", file=sys.stderr); return 2
    except (RuntimeError, ValueError) as error: print(error, file=sys.stderr); return 1


if "--json" in sys.argv[1:]:
    os.environ["TARIBOY_TOOLS_JSON"] = "1"
    sys.argv.remove("--json")


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
