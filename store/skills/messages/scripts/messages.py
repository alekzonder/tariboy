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


def required(values, name, command):
    value = values.get(name, "")
    if not value:
        raise UsageError(f"tools {command}: --{name} is required")
    return value


def json_value(values, name, command, object_only=False):
    if name not in values:
        return None
    try:
        value = json.loads(values[name])
    except json.JSONDecodeError as error:
        raise UsageError(f"tools {command}: --{name} is not valid JSON: {error}") from error
    if object_only and not isinstance(value, dict):
        raise UsageError(f"tools {command}: --{name} is not valid JSON object")
    return value


def send(method, route, body=None):
    print_result(call(method, route, body))


def run(args):
    if args == ["sources"]:
        result = call("GET", "/tools/sources")
        channels = result.get("channels", [])
        if not channels:
            print("no channels")
            return
        for channel in channels:
            parts = [channel["name"], channel["kind"]]
            if channel.get("provider"):
                parts.append("provider=" + channel["provider"])
            if channel.get("params"):
                parts.append("params: {" + ",".join(channel["params"]) + "}")
            if channel.get("help"):
                parts.append(channel["help"])
            print("  ".join(parts))
        return

    if args[:2] == ["message", "send"]:
        flags, _ = parse_flags(args, 2, {"channel", "type", "subject", "text", "data"})
        body = {"channel": required(flags, "channel", "message send"), "type": flags.get("type", ""), "text": flags.get("text", "")}
        if "subject" in flags:
            body["subject"] = dict(item.split("=", 1) for item in flags["subject"].split(",") if "=" in item)
        if "data" in flags:
            body["data"] = json_value(flags, "data", "message send")
        return send("POST", "/tools/message/send", body)
    if args[:2] == ["message", "ls"]:
        flags, _ = parse_flags(args, 2, {"all"})
        return send("GET", "/tools/message/ls" + ("?all=true" if "all" in flags else ""))
    if args[:2] == ["message", "processed"]:
        flags, pos = parse_flags(args, 2, {"result"})
        if not pos:
            raise UsageError("tools message processed: <id> is required")
        result = flags.get("result", " ".join(pos[1:]))
        if not result.strip():
            raise UsageError("tools message processed: a result is required")
        return send("POST", "/tools/message/processed", {"id": pos[0], "result": result})
    if args[:2] == ["message", "reply"]:
        flags, pos = parse_flags(args, 2, {"text", "type", "data"})
        if not pos:
            raise UsageError("tools message reply: <id> is required")
        body = {"id": pos[0], "text": flags.get("text", " ".join(pos[1:])), "type": flags.get("type", "")}
        if "data" in flags:
            body["data"] = json_value(flags, "data", "message reply")
        return send("POST", "/tools/message/reply", body)
    if args[:3] == ["message", "dlq", "requeue"]:
        _, pos = parse_flags(args, 3)
        if not pos:
            raise UsageError("tools message dlq requeue: <id> is required")
        return send("POST", "/tools/message/dlq/requeue", {"id": pos[0]})
    if args[:2] == ["message", "dlq"]:
        parse_flags(args, 2)
        return send("GET", "/tools/message/dlq")
    if args[:1] == ["request"]:
        flags, _ = parse_flags(args, 1, {"channel", "text", "deadline"})
        body = {"channel": required(flags, "channel", "request"), "text": flags.get("text", "")}
        if flags.get("deadline"):
            body["deadline"] = flags["deadline"]
        return send("POST", "/tools/request", body)
    if args[:2] == ["channel", "subscribe"]:
        flags, pos = parse_flags(args, 2, {"matcher", "type", "params"})
        if not pos:
            raise UsageError("tools channel subscribe: <channel> is required")
        body = {"channel": pos[0], "type": flags.get("type", "")}
        if "matcher" in flags:
            body["matcher"] = json_value(flags, "matcher", "channel subscribe", True)
        if "params" in flags:
            body["params"] = json_value(flags, "params", "channel subscribe", True)
        return send("POST", "/tools/channel/subscribe", body)
    if args[:2] == ["channel", "unsubscribe"]:
        _, pos = parse_flags(args, 2)
        if not pos:
            raise UsageError("tools channel unsubscribe: <id> is required")
        return send("POST", "/tools/channel/unsubscribe", {"id": pos[0]})
    if args[:2] == ["channel", "ls"]:
        parse_flags(args, 2)
        return send("GET", "/tools/channel/ls")
    if args[:2] in (["group", "info"], ["group", "status"]):
        _, pos = parse_flags(args, 2)
        member = pos[0] if pos else ""
        return send("GET", "/tools/group/" + args[1] + ("/" + member if member else ""))
    if args[:2] in (["group", "send"], ["group", "request"]):
        flags, pos = parse_flags(args, 2, {"text", "deadline"})
        if not pos:
            raise UsageError(f"tools group {args[1]}: <member> is required")
        text = required(flags, "text", "group " + args[1])
        body = {"member": pos[0], "text": text}
        if flags.get("deadline"):
            body["deadline"] = flags["deadline"]
        return send("POST", "/tools/group/" + args[1], body)
    if args[:2] == ["group", "observe"]:
        flags, pos = parse_flags(args, 2, {"tail"})
        if not pos:
            raise UsageError("tools group observe: <member> is required")
        route = "/tools/group/observe/" + pos[0]
        if flags.get("tail"):
            route += "?tail=" + flags["tail"]
        return send("GET", route)
    if len(args) >= 3 and args[:2] == ["group", "loop"] and args[2] in {"start", "stop"}:
        _, pos = parse_flags(args, 3)
        if not pos:
            raise UsageError(f"tools group loop {args[2]}: <member> is required")
        return send("POST", "/tools/group/loop", {"member": pos[0], "action": args[2]})
    raise UsageError("tools messages: unknown command " + " ".join(args))


if __name__ == "__main__":
    if sys.argv[1:] in (["-h"], ["--help"]):
        print("usage: messages.sh <sources|message|request|channel|group> ... [--json]")
        raise SystemExit(0)
    raise SystemExit(execute(lambda: run(sys.argv[1:])))
