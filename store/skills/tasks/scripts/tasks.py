#!/usr/bin/env python3
import json
import os
import sys
from pathlib import Path

import http.client
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


COMMON_WORK = {"task-revision", "assignment-revision", "idempotency-key"}


def csv(value):
    return [item.strip() for item in value.split(",") if item.strip()]


def require_pos(pos, action, index=0, label="task key"):
    if len(pos) <= index or not pos[index].strip():
        raise UsageError(f"tasks {action}: {label} is required")
    return pos[index]


def copy(payload, flags, name, target=None, present=False):
    if name in flags and (present or flags[name] != ""):
        payload[target or name.replace("-", "_")] = flags[name]


def parse(args):
    if not args:
        raise UsageError("tasks: a command is required")
    action = args[0]
    rest = args[1:]
    if action in {"work", "artifacts", "observe"}:
        if not rest:
            raise UsageError(f"tasks {action}: a command is required")
        action = ("artifact" if action == "artifacts" else action) + "_" + rest[0]
        rest = rest[1:]

    allowed = {
        "mine": {"queue", "status", "assignee", "text", "waiting-for"},
        "ready": {"queue", "limit", "idempotency-key", "claim"},
        "show": set(),
        "create": {"queue", "parent", "title", "description", "assignee", "group", "priority", "idempotency-key"},
        "update": {"title", "description", "status", "assignee", "manual-block-reason", "priority", "revision"},
        "assign": {"revision"},
        "comment": {"body", "idempotency-key"},
        "ask": {"question", "context", "blocking-scope", "anchor", "suggested-answer", "options", "artifacts", *COMMON_WORK},
        "move": {"parent", "before", "to-root", "revision"},
        "block": {"by", "revision", "idempotency-key"},
        "relate": {"revision", "idempotency-key"},
        "done": {"revision", "complete-anyway"},
        "work_next": {"queue", "idempotency-key"},
        "work_show": COMMON_WORK,
        "work_complete": {*COMMON_WORK, "outcome"},
        "work_release": COMMON_WORK,
        "artifact_add": {*COMMON_WORK, "name", "type", "content", "metadata"},
        "artifact_show": {"task"},
        "questions": set(),
        "answer": {*COMMON_WORK, "assignment", "answer"},
        "observe_subscribe": {*COMMON_WORK, "correlation-key", "reaction"},
        "observe_list": set(),
        "observe_cancel": COMMON_WORK,
    }
    if action not in allowed:
        raise UsageError(f'tasks: unknown command "{action}"')
    flags, pos = parse_flags(rest, 0, allowed[action])
    payload = {}

    if action == "work_next":
        copy(payload, flags, "queue")
        copy(payload, flags, "idempotency-key")
        if "idempotency_key" not in payload:
            raise UsageError("tasks work next: --idempotency-key is required")
    elif action in {"work_show", "work_complete", "work_release"}:
        payload["assignment_id"] = require_pos(pos, action.replace("_", " "), label="task key")
        for flag in COMMON_WORK:
            copy(payload, flags, flag)
        if action == "work_complete":
            copy(payload, flags, "outcome")
    elif action == "artifact_add":
        payload["assignment_id"] = require_pos(pos, "artifacts add")
        for flag in COMMON_WORK | {"name", "type"}:
            copy(payload, flags, flag)
        copy(payload, flags, "content", present=True)
        if "metadata" in flags:
            try:
                payload["metadata"] = json.loads(flags["metadata"])
            except json.JSONDecodeError as error:
                raise UsageError(f"tasks artifacts add: --metadata is not valid JSON: {error}") from error
    elif action == "artifact_show":
        payload["assignment_id"] = require_pos(pos, "artifacts show")
        payload["artifact_id"] = require_pos(pos, "artifacts show", 1, "artifact id")
        copy(payload, flags, "task", "task_key")
    elif action == "questions":
        payload["assignment_id"] = require_pos(pos, "questions")
    elif action == "answer":
        payload["question_id"] = require_pos(pos, "answer")
        copy(payload, flags, "assignment", "assignment_id")
        copy(payload, flags, "answer")
        for flag in COMMON_WORK:
            copy(payload, flags, flag)
        action = "workflow_answer"
    elif action == "observe_subscribe":
        payload["assignment_id"] = require_pos(pos, "observe subscribe")
        payload["pattern"] = require_pos(pos, "observe subscribe", 1, "pattern")
        for flag in COMMON_WORK | {"correlation-key", "reaction"}:
            copy(payload, flags, flag)
    elif action == "observe_list":
        payload["assignment_id"] = require_pos(pos, "observe list")
    elif action == "observe_cancel":
        payload["assignment_id"] = require_pos(pos, "observe cancel")
        payload["subscription_id"] = require_pos(pos, "observe cancel", 1, "subscription id")
        for flag in COMMON_WORK:
            copy(payload, flags, flag)
    elif action == "mine":
        for flag in allowed[action]:
            copy(payload, flags, flag)
    elif action == "ready":
        for flag in {"queue", "limit", "idempotency-key"}:
            copy(payload, flags, flag)
        if "claim" in flags:
            payload["claim"] = True
    elif action == "show":
        payload["key"] = require_pos(pos, "show")
    elif action == "create":
        for flag in allowed[action]:
            copy(payload, flags, flag, "parent_key" if flag == "parent" else None)
        if "title" not in payload:
            raise UsageError("tasks create: --title is required")
        if "queue" not in payload and "parent_key" not in payload:
            raise UsageError("tasks create: --queue or --parent is required")
    elif action == "update":
        payload["key"] = require_pos(pos, "update")
        for flag in allowed[action] - {"manual-block-reason"}:
            copy(payload, flags, flag)
        copy(payload, flags, "manual-block-reason", present=True)
    elif action == "assign":
        payload["key"] = require_pos(pos, "assign")
        payload["assignee"] = require_pos(pos, "assign", 1, "assignee")
        copy(payload, flags, "revision")
    elif action == "comment":
        payload["key"] = require_pos(pos, "comment")
        body = flags.get("body", " ".join(pos[1:])).strip()
        if not body:
            raise UsageError("tasks comment: comment text is required")
        payload["body"] = body
        copy(payload, flags, "idempotency-key")
    elif action == "ask":
        if "question" in flags:
            if len(pos) > 1:
                raise UsageError("tasks ask: workflow ask does not accept a positional principal or question")
            payload["assignment_id"] = require_pos(pos, "ask")
            for flag in {"question", "context", "blocking-scope", "anchor", "suggested-answer", *COMMON_WORK}:
                copy(payload, flags, flag)
            if "options" in flags:
                payload["options"] = csv(flags["options"])
            if "artifacts" in flags:
                try:
                    payload["artifact_attachments"] = [int(item) for item in csv(flags["artifacts"])]
                except ValueError as error:
                    raise UsageError("tasks ask: --artifacts must contain numeric ids") from error
            action = "workflow_ask"
        else:
            workflow_flags = sorted(set(flags) - {"idempotency-key"})
            if workflow_flags:
                raise UsageError(f"tasks ask: --{workflow_flags[0]} is a workflow-only flag")
            payload["key"] = require_pos(pos, "ask")
            payload["principal"] = require_pos(pos, "ask", 1, "principal")
            if len(pos) < 3:
                raise UsageError("tasks ask: principal and question are required")
            payload["body"] = " ".join(pos[2:])
            copy(payload, flags, "idempotency-key")
    elif action == "move":
        payload["key"] = require_pos(pos, "move")
        copy(payload, flags, "parent", "parent_key")
        copy(payload, flags, "before", "before_key")
        copy(payload, flags, "revision")
        if "to-root" in flags:
            if "parent_key" in payload or "before_key" in payload:
                raise UsageError("tasks move: --to-root cannot be combined with --parent or --before")
            payload["parent_key"] = ""
        elif "parent_key" not in payload and "before_key" not in payload:
            raise UsageError("tasks move: pass --parent, --before, or --to-root to detach")
    elif action == "block":
        payload["key"] = require_pos(pos, "block")
        if not flags.get("by"):
            raise UsageError("tasks block: --by is required")
        payload["blocker_key"] = flags["by"]
        for flag in {"revision", "idempotency-key"}:
            copy(payload, flags, flag)
    elif action == "relate":
        payload["key"] = require_pos(pos, "relate")
        payload["target_key"] = require_pos(pos, "relate", 1, "related task key")
        for flag in {"revision", "idempotency-key"}:
            copy(payload, flags, flag)
    elif action == "done":
        payload["key"] = require_pos(pos, "done")
        copy(payload, flags, "revision")
        if "complete-anyway" in flags:
            payload["complete_anyway"] = True
    return action, payload


def run(args):
    action, payload = parse(args)
    result = call("POST", "/tools/tasks/" + action, payload)
    print_result(result)
    if action == "create" and result.get("filed"):
        print(f"\n{result['key']} is filed into queue {result['queue']}: no assignee, and no longer visible to you.")
        print("Whoever runs that queue triages it. It is recorded — do not file it again.")


if __name__ == "__main__":
    args = sys.argv[1:]
    if args in (["-h"], ["--help"]):
        print("usage: tasks.sh <mine|ready|show|create|update|assign|comment|ask|move|block|relate|done|work|artifacts|questions|answer|observe> ... [--json]")
        raise SystemExit(0)
    if "--json" in args:
        os.environ["TARIBOY_TOOLS_JSON"] = "1"
        args = [arg for arg in args if arg != "--json"]
    raise SystemExit(execute(lambda: run(args)))
