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
def client_version():
    version = os.environ.get("TARIBOY_CLIENT_VERSION")
    if version:
        return version
    script = Path(__file__).resolve()
    try:
        skills = json.loads((script.parents[3] / "bridge-manifest.json").read_text()).get("skills", [])
    except (OSError, json.JSONDecodeError):
        skills = []
    for skill in skills:
        version = skill.get("client_version") if isinstance(skill, dict) and skill.get("name") == script.parents[1].name else None
        if isinstance(version, str) and version:
            return version
    return script.parents[3].name
def call(method, route, body=None):
    connection = UnixHTTPConnection(os.environ["TARIBOY_TOOLS_SOCKET"]); payload = None if method == "GET" else json.dumps(body or {}).encode(); connection.request(method, route, payload, {"Content-Type": "application/json"}); response = connection.getresponse(); daemon_version = response.getheader("X-Tariboy-Version", ""); envelope = json.load(response); connection.close(); version = client_version()
    if daemon_version and daemon_version != version: print(f"warning: client version {version} does not match daemon version {daemon_version}; this client ({sys.argv[0]}) may not know the daemon's newer flags", file=sys.stderr)
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
    if args in (["-h"], ["--help"]):
        print("usage: context.sh get | set <text...> [--json]")
        raise SystemExit(0)
    if "--json" in args:
        os.environ["TARIBOY_TOOLS_JSON"] = "1"
        args = [arg for arg in args if arg != "--json"]
    if args[:1] == ["get"]:
        raise SystemExit(main("GET", "/tools/context/get", text_key="text", cli_args=args[1:]))
    if args[:1] == ["set"]:
        raise SystemExit(main("POST", "/tools/context/set", {"text": " ".join(args[1:])}, cli_args=args[1:]))
    print("tools context: get or set is required", file=sys.stderr)
    raise SystemExit(2)
