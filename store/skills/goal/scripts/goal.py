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
def call(body):
    c = UnixHTTPConnection(os.environ["TARIBOY_TOOLS_SOCKET"]); c.request("POST", "/tools/goal/set", json.dumps(body).encode(), {"Content-Type": "application/json"}); r = c.getresponse(); daemon = r.getheader("X-Tariboy-Version", ""); envelope = json.load(r); c.close(); version = client_version()
    if daemon and daemon != version: print(f"warning: client version {version} does not match daemon version {daemon}; this client ({sys.argv[0]}) may not know the daemon's newer flags", file=sys.stderr)
    if not envelope.get("ok"): raise RuntimeError(envelope.get("error", {}).get("message", "daemon returned failure without error detail"))
    return envelope["result"]
def format_value(value):
    if value is None: return "<nil>"
    if isinstance(value, bool): return str(value).lower()
    return str(value)
def main():
    args = sys.argv[1:]
    if args in (["-h"], ["--help"]):
        print("usage: goal.sh set <task-key> [--json]")
        return 0
    as_json = "--json" in args
    args = [arg for arg in args if arg != "--json"]
    if len(args) != 2 or args[0] != "set" or args[1].startswith("--"):
        raise UsageError("usage: goal.sh set <task-key> [--json]")
    result = call({"key": args[1]})
    if as_json:
        print(json.dumps(result, separators=(",", ":")))
    else:
        for key in sorted(result): print(f"{key}: {format_value(result[key])}")
    return 0

if __name__ == "__main__":
    try: raise SystemExit(main())
    except KeyError: print("tools: TARIBOY_TOOLS_SOCKET is not set (are you running inside an agent?)", file=sys.stderr); raise SystemExit(2)
    except UsageError as error: print(error, file=sys.stderr); raise SystemExit(2)
    except OSError: print("tools: agent socket is not reachable", file=sys.stderr); raise SystemExit(2)
    except (RuntimeError, ValueError) as error: print(error, file=sys.stderr); raise SystemExit(1)
