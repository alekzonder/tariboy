#!/usr/bin/env python3
import json
import sys
from pathlib import Path

import http.client
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


def required(flags, name, command):
    value = flags.get(name, "").strip()
    if not value:
        raise UsageError(f"tools judge {command}: --{name} is required")
    return value


def positive(text, message):
    try:
        value = int(text)
    except ValueError as error:
        raise UsageError(message) from error
    if value <= 0:
        raise UsageError(message)
    return value


def json_object(text, message):
    try:
        value = json.loads(text)
    except json.JSONDecodeError as error:
        raise UsageError(f"{message}: {error}") from error
    if not isinstance(value, dict):
        raise UsageError(message)
    return value


def json_file(flags, command):
    file = required(flags, "file", command)
    try:
        raw = Path(file).read_text()
    except OSError as error:
        raise UsageError(f"tools judge {command}: read file: {error}") from error
    return json_object(raw, f"tools judge {command}: {file} is not valid JSON object"), raw


def positional(pos, command):
    if not pos or not pos[0].strip():
        raise UsageError(f"tools judge {command}: run id is required")
    return pos[0]


def build(args):
    if len(args) < 2:
        raise UsageError("tools judge: a command is required")
    command = " ".join(args[:2])
    allowed = {
        "automation begin": {"revision", "delivery", "limit"},
        "iterations search": {"agent", "judge-group", "group", "since", "until", "status", "limit"},
        "run create": {"request-file", "selector", "judges", "summary-agent", "judges-per-iteration", "judge-group"},
        "run inspect": set(), "run cancel": set(), "work retry": set(), "summary claim": set(),
        "work claim": {"run"},
        "evidence search": {"assignment", "artifact", "query", "cursor"},
        "evidence get": {"assignment", "artifact", "locator"},
        "analysis submit": {"assignment", "file"},
        "summary inputs": {"cursor"},
        "summary submit": {"file"},
        "improvement submit": {"file"},
    }
    if command not in allowed:
        raise UsageError(f'tools judge: unknown command "{command}"')
    flags, pos = parse_flags(args, 2, allowed[command])

    if command == "automation begin":
        revision = positive(required(flags, "revision", command), "tools judge automation begin: --revision must be a positive integer")
        delivery = required(flags, "delivery", command)
        limit = positive(flags.get("limit", "100"), "tools judge automation begin: --limit must be a positive integer")
        return "automation.begin", {"config_revision": revision, "delivery_id": delivery, "limit": limit}
    if command == "iterations search":
        selector = {"agents": [required(flags, "agent", command)]}
        for name in ("group", "since", "until"):
            if flags.get(name):
                selector[name] = flags[name]
        if flags.get("status"):
            selector["statuses"] = [item.strip() for item in flags["status"].split(",") if item.strip()]
        if flags.get("limit"):
            try:
                selector["limit"] = int(flags["limit"])
            except ValueError as error:
                raise UsageError("tools judge iterations search: --limit must be an integer") from error
        return "iterations.search", {"judge_group": required(flags, "judge-group", command), "selector": selector}
    if command == "run create":
        request_file = required(flags, "request-file", command)
        try:
            original = Path(request_file).read_text()
        except OSError as error:
            raise UsageError(f"tools judge run create: read request file: {error}") from error
        selector = json_object(required(flags, "selector", command), "tools judge run create: --selector is not valid JSON object")
        body = {
            "original_request": original,
            "selector": selector,
            "judge_agents": [item.strip() for item in required(flags, "judges", command).split(",") if item.strip()],
            "summary_agent": required(flags, "summary-agent", command),
        }
        if flags.get("judge-group"):
            body["judge_group"] = flags["judge-group"]
        if flags.get("judges-per-iteration"):
            try:
                body["judges_per_iteration"] = int(flags["judges-per-iteration"])
            except ValueError as error:
                raise UsageError("tools judge run create: --judges-per-iteration must be an integer") from error
        return "run.create", body
    if command in {"run inspect", "run cancel", "work retry", "summary claim"}:
        return command.replace(" ", "."), {"run_id": positional(pos, command)}
    if command == "work claim":
        return "work.claim", ({"run_id": flags["run"]} if flags.get("run") else {})
    if command == "evidence search":
        body = {"assignment_id": required(flags, "assignment", command), "artifacts": [required(flags, "artifact", command)]}
        for name in ("query", "cursor"):
            if flags.get(name):
                body[name] = flags[name]
        return "evidence.search", body
    if command == "evidence get":
        locator = json_object(required(flags, "locator", command), "tools judge evidence get: --locator is not valid JSON object")
        return "evidence.get", {"assignment_id": required(flags, "assignment", command), "artifact": required(flags, "artifact", command), "locator": locator}
    if command == "analysis submit":
        result, raw = json_file(flags, command)
        return "analysis.submit", {"assignment_id": required(flags, "assignment", command), "result": result, "raw_submission": raw}
    if command == "summary inputs":
        body = {"run_id": positional(pos, command)}
        if flags.get("cursor"):
            body["cursor"] = flags["cursor"]
        return "summary.inputs", body
    if command in {"summary submit", "improvement submit"}:
        result, raw = json_file(flags, command)
        return command.replace(" ", "."), {"run_id": positional(pos, command), "result": result, "raw_submission": raw}
    raise AssertionError(command)


def run(args):
    action, body = build(args)
    print_result(call("POST", "/tools/judge/action/" + action, body))


if __name__ == "__main__":
    raise SystemExit(execute(lambda: run(sys.argv[1:])))
