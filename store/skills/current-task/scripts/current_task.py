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
def reject_unknown_flags(args, allowed=()):
    for arg in args:
        if arg.startswith("--") and arg[2:].split("=", 1)[0] not in set(allowed): raise UsageError(f"unknown flag --{arg[2:].split('=', 1)[0]}")
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
def execute(action):
    try: action(); return 0
    except KeyError: print("tools: TARIBOY_TOOLS_SOCKET is not set (are you running inside an agent?)", file=sys.stderr); return 2
    except UsageError as error: print(error, file=sys.stderr); return 2
    except OSError: print("tools: agent socket is not reachable", file=sys.stderr); return 2
    except (RuntimeError, ValueError) as error: print(error, file=sys.stderr); return 1
def main(method, route, body=None, text_key=None, cli_args=(), allowed_flags=()):
    if "--json" in cli_args:
        os.environ["TARIBOY_TOOLS_JSON"] = "1"
        cli_args = tuple(arg for arg in cli_args if arg != "--json")
    def action():
        reject_unknown_flags(cli_args, allowed_flags); result = call(method, route, body)
        if route == "/tools/whoami" and isinstance(result, dict): result.setdefault("client_version", client_version())
        print(result[text_key]) if text_key else print_result(result)
    return execute(action)


if __name__ == "__main__":
    args = sys.argv[1:]
    if "--json" in args:
        os.environ["TARIBOY_TOOLS_JSON"] = "1"
        args = [arg for arg in args if arg != "--json"]
    if args == ["--clear"]:
        raise SystemExit(main("POST", "/tools/task/current", {"clear": True}))
    if not args:
        print("tools task current: <task-key> is required (or pass --clear)", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(main("POST", "/tools/task/current", {"id": args[0]}, cli_args=args, allowed_flags={"clear"}))
