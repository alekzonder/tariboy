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
