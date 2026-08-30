#!/usr/bin/env python3
import os
from pathlib import Path
import sys

from client import client_version


HELP = """tools — the agent command surface

Usage: tools <group> <command> [args]

Commands: whoami, status, loop, context, task current, message, request,
channel, sources, group, schedule, script, image, judge, tasks, help

The socket is taken from $TARIBOY_TOOLS_SOCKET.
"""


def main(args):
    if args == ["--version"]:
        print(client_version())
        return 0
    if not args or args[0] in {"help", "--help"}:
        print(HELP)
        return 0
    json_output = "--json" in args
    args = [arg for arg in args if arg != "--json"]
    skills = Path(__file__).parents[2]
    group = args[0]
    mapping = {
        "whoami": ("whoami/scripts/whoami.py", args[1:]),
        "status": ("status/scripts/status.py", args[1:]),
        "loop": ("loop/scripts/loop.py", args[1:]),
        "context": ("context/scripts/context.py", args[1:]),
        "message": ("messages/scripts/messages.py", args),
        "request": ("messages/scripts/messages.py", args),
        "channel": ("messages/scripts/messages.py", args),
        "sources": ("messages/scripts/messages.py", args),
        "group": ("messages/scripts/messages.py", args),
        "schedule": ("schedule/scripts/schedule.py", args[1:]),
        "script": ("scripts/scripts/scripts.py", args[1:]),
        "image": ("image-creator/scripts/image_creator.py", args[1:]),
        "judge": ("llm-as-judge/scripts/judge.py", args[1:]),
        "tasks": ("tasks/scripts/tasks.py", args[1:]),
    }
    if group == "task" and args[1:2] == ["current"]:
        relative, forwarded = "current-task/scripts/current_task.py", args[2:]
    elif group in mapping:
        relative, forwarded = mapping[group]
    else:
        print(f'tools: unknown command "{" ".join(args)}" (try \'tools help\')', file=sys.stderr)
        return 2
    env = os.environ | ({"TARIBOY_TOOLS_JSON": "1"} if json_output else {})
    os.execvpe("python3", ["python3", "-B", str(skills / relative), *forwarded], env)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
