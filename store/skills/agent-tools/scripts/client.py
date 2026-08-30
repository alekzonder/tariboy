import http.client
import json
import os
from pathlib import Path
import socket
import sys


class UsageError(Exception):
    pass


class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost")
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(self.path)


def call(method, route, body=None):
    connection = UnixHTTPConnection(os.environ["TARIBOY_TOOLS_SOCKET"])
    payload = None if method == "GET" else json.dumps(body or {}).encode()
    connection.request(method, route, payload, {"Content-Type": "application/json"})
    response = connection.getresponse()
    daemon_version = response.getheader("X-Tariboy-Version", "")
    envelope = json.load(response)
    connection.close()
    version = client_version()
    if daemon_version and daemon_version != version:
        print(
            f"warning: client version {version} does not match daemon version {daemon_version}; "
            f"this client ({sys.argv[0]}) may not know the daemon's newer flags",
            file=sys.stderr,
        )
    if not envelope.get("ok"):
        raise RuntimeError(envelope.get("error", {}).get("message", "daemon returned failure without error detail"))
    return envelope["result"]


def client_version():
    return os.environ.get("TARIBOY_CLIENT_VERSION") or Path(__file__).resolve().parents[3].name


def reject_unknown_flags(args, allowed=()):
    allowed = set(allowed)
    for arg in args:
        if arg.startswith("--") and arg[2:].split("=", 1)[0] not in allowed:
            raise UsageError(f"unknown flag --{arg[2:].split('=', 1)[0]}")


def print_result(result):
    if os.environ.get("TARIBOY_TOOLS_JSON") == "1":
        print(json.dumps(result, separators=(",", ":")))
        return
    if isinstance(result, dict):
        for key in sorted(result):
            print(f"{key}: {result[key]}")
    else:
        print(result)


def parse_flags(args, start=0, allowed=()):
    values = {}
    positionals = []
    allowed = set(allowed)
    index = start
    while index < len(args):
        arg = args[index]
        if arg.startswith("--"):
            name, separator, value = arg[2:].partition("=")
            if name not in allowed:
                raise UsageError(f"unknown flag --{name}")
            if not separator:
                if index + 1 < len(args) and not args[index + 1].startswith("--"):
                    index += 1
                    value = args[index]
                else:
                    value = "true"
            values[name] = value
        else:
            positionals.append(arg)
        index += 1
    return values, positionals


def execute(action):
    try:
        action()
        return 0
    except KeyError:
        print("tools: TARIBOY_TOOLS_SOCKET is not set (are you running inside an agent?)", file=sys.stderr)
        return 2
    except UsageError as error:
        print(error, file=sys.stderr)
        return 2
    except OSError:
        print("tools: agent socket is not reachable", file=sys.stderr)
        return 2
    except (RuntimeError, ValueError) as error:
        print(error, file=sys.stderr)
        return 1


def main(method, route, body=None, text_key=None, cli_args=(), allowed_flags=()):
    def action():
        reject_unknown_flags(cli_args, allowed_flags)
        result = call(method, route, body)
        if route == "/tools/whoami" and isinstance(result, dict):
            result.setdefault("client_version", client_version())
        print(result[text_key]) if text_key else print_result(result)

    return execute(action)
