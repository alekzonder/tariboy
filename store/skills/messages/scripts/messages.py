#!/usr/bin/env python3
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[2] / "agent-tools" / "scripts"))
from client import UsageError, call, execute, parse_flags, print_result


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
        body = {"id": pos[0], "text": flags.get("text", ""), "type": flags.get("type", "")}
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
    raise SystemExit(execute(lambda: run(sys.argv[1:])))
