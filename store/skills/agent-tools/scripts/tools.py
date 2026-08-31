#!/usr/bin/env python3
import os
from pathlib import Path
import sys

from client import client_version


HELP = """tools — the agent command surface

Usage: tools <group> <command> [args]

Commands:
  loop done            Signal this iteration is finished (i-am-done)
  loop start|stop      Enable or disable your own loop
  whoami               Print agent, cwd, current iteration and both versions
  context get          Print the durable working memory (CONTEXT.md)
  context set <text>   Overwrite the durable working memory
  status               Print agent state and your current status message
  status set <text>    Set your status message ("what I'm doing now")
  task current <id>    Tag this iteration with a native task (attributes AI usage)
  task current --clear Clear the current-task tag
  tasks mine [--status S] [--queue Q] [--waiting-for me]
  tasks ready [--queue Q] [--claim]
  tasks show KEY
  tasks create --queue Q|--parent KEY --title TEXT [--priority P0|P1|P2|P3]
  tasks update KEY [--status S] [--title TEXT] [--assignee AGENT] [--priority P0|P1|P2|P3]
  tasks assign KEY AGENT
  tasks comment KEY TEXT
  tasks ask KEY agent:NAME|user:LOGIN TEXT
  tasks move KEY [--parent KEY] [--before KEY] [--to-root]
  tasks block KEY --by BLOCKER
  tasks relate KEY OTHER
  tasks done KEY [--complete-anyway]
  tasks work next [--queue Q] --idempotency-key KEY
  tasks work show|complete|release ASSIGNMENT [workflow flags]
  tasks artifacts add ASSIGNMENT --name N --type T --content VALUE
  tasks artifacts show ASSIGNMENT ARTIFACT --task KEY
  tasks ask ASSIGNMENT --question Q --context C --blocking-scope none|assignment|requirement
  tasks questions ASSIGNMENT
  tasks answer QUESTION --assignment ASSIGNMENT --answer TEXT
  tasks observe subscribe|list|cancel ASSIGNMENT [PATTERN|SUBSCRIPTION]
  group info
  group status [member]
  group send <member> --text TEXT
  group request <member> --text TEXT [--deadline DURATION]
  group observe <member> [--tail N]
  group loop start|stop <member>
  message send --channel C [--type T] [--subject k=v,..] [--text .. | --data JSON]
  message ls [--all]   List your inbox (pending; --all adds archive+dlq)
  message processed ID <result...>   Ack a message with a mandatory result
  message reply ID [--text ..] [--data JSON]   Reply to a message (auto-processes)
  message dlq          List your dead-lettered messages
  message dlq requeue ID   Requeue a dead-lettered message
  request --channel C --text .. [--deadline DURATION]   Send a request (§4.2)
  channel subscribe C [--matcher JSON] [--type globs] [--params JSON]
  channel unsubscribe ID   By subscription id (or channel name to drop all own subs)
  channel ls           List your subscriptions
  sources              List available channels
  schedule add --kind cron|oneshot --spec S [--channel C] [--message JSON]
  schedule ls          List your schedules
  schedule cancel ID
  script run NAME [--description TEXT] -- COMMAND
  script schedule NAME --every SECONDS [--quiet-exit CODE] [--description TEXT] -- COMMAND
  script rerun SCRIPT_ID
  script ls
  script runs SCRIPT_ID
  script logs RUN_ID
  script cancel SCRIPT_OR_RUN_ID
  script rm SCRIPT_ID
  image build --name NAME [--tag TAG] --path DIR   Author+build a new image (image-creator only)
  judge automation begin --revision R --delivery ID [--limit N]
  judge iterations search --agent A --judge-group G [--group G] [--since T] [--until T] [--status S] [--limit N]
  judge run create --request-file F --selector JSON --judges a,b --summary-agent A [--judges-per-iteration N] [--judge-group G]
  judge improvement submit RUN_ID --file F
  judge run inspect RUN
  judge work claim [--run RUN]
  judge evidence search --assignment ID --artifact K [--query Q] [--cursor C]
  judge evidence get --assignment ID --artifact K --locator JSON
  judge analysis submit --assignment ID --file result.json
  judge summary claim RUN
  judge summary inputs RUN [--cursor C]
  judge summary submit RUN --file summary.json
  judge run cancel RUN
  judge work retry RUN
  help                 Show this help

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
    env = dict(os.environ)
    if json_output:
        env["TARIBOY_TOOLS_JSON"] = "1"
    os.execvpe("python3", ["python3", "-B", str(skills / relative), *forwarded], env)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
