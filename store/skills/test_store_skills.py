import os
from pathlib import Path
import json
import socketserver
import subprocess
import tempfile
import threading
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parent
COMMANDS = {
    "whoami": "whoami",
    "loop": "loop",
    "context": "context",
    "status": "status",
    "current-task": "current_task",
    "messages": "messages",
    "schedule": "schedule",
    "scripts": "scripts",
    "image-creator": "image_creator",
    "llm-as-judge": "judge",
    "tasks": "tasks",
}


class StoreSkillsTest(unittest.TestCase):
    def run_script(self, relative, args, result, version="0.46.0", envelope=None):
        class Handler(socketserver.StreamRequestHandler):
            def handle(handler):
                request_line = handler.rfile.readline().decode()
                headers = {}
                while line := handler.rfile.readline().decode().rstrip("\r\n"):
                    name, value = line.split(":", 1)
                    headers[name.lower()] = value.strip()
                length = int(headers.get("content-length", 0))
                server.request = (request_line.split()[:2], handler.rfile.read(length))
                body = json.dumps(envelope or {"ok": True, "result": result}).encode()
                handler.wfile.write(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" + f"Content-Length: {len(body)}\r\nX-Tariboy-Version: {version}\r\n\r\n".encode() + body)

        with tempfile.TemporaryDirectory() as tmp:
            path = str(Path(tmp) / "agent.sock")
            server = socketserver.UnixStreamServer(path, Handler)
            server.request = None
            errors = []

            def serve():
                try:
                    server.serve_forever(poll_interval=0.01)
                except BaseException as error:
                    errors.append(error)

            thread = threading.Thread(target=serve)
            thread.start()
            try:
                process = subprocess.run(
                    [ROOT / relative, *args],
                    env=dict(os.environ, TARIBOY_TOOLS_SOCKET=path, TARIBOY_CLIENT_VERSION="0.46.0"),
                    text=True,
                    capture_output=True,
                )
            finally:
                server.shutdown()
                thread.join(timeout=2)
                server.server_close()
                self.assertFalse(thread.is_alive(), "test socket server did not stop")
                if errors:
                    raise errors[0]
        return process, server.request

    def test_each_command_skill_has_a_direct_self_contained_entrypoint(self):
        for skill, command in COMMANDS.items():
            with self.subTest(skill=skill):
                script = ROOT / skill / "scripts" / f"{command}.sh"
                self.assertTrue(script.is_file(), script)
                implementation = script.with_suffix(".py")
                self.assertTrue(implementation.is_file(), implementation)
                source = implementation.read_text()
                self.assertNotIn("agent-tools", source)
                self.assertNotIn("from client import", source)
                self.assertNotIn("sys.path.insert", source)

    def test_skill_instructions_use_owning_direct_entrypoints(self):
        expected = {
            "messages": "scripts/messages.sh",
            "schedule": "scripts/schedule.sh",
            "scripts": "scripts/scripts.sh",
            "image-creator": "scripts/image_creator.sh",
            "tasks": "scripts/tasks.sh",
            "workdir": "scripts/scripts.sh",
        }
        for skill, entrypoint in expected.items():
            with self.subTest(skill=skill):
                text = (ROOT / skill / "SKILL.md").read_text()
                self.assertIn(entrypoint, text)
                self.assertNotIn("tools ", text)

    def test_direct_entrypoint_reports_missing_socket(self):
        env = dict(os.environ)
        env.pop("TARIBOY_TOOLS_SOCKET", None)
        result = subprocess.run(
            [ROOT / "whoami/scripts/whoami.sh"], env=env, text=True, capture_output=True
        )
        self.assertEqual(result.returncode, 2)
        self.assertEqual(
            result.stderr,
            "tools: TARIBOY_TOOLS_SOCKET is not set (are you running inside an agent?)\n",
        )

    def test_direct_entrypoints_preserve_representative_routes_and_payloads(self):
        cases = [
            ("whoami/scripts/whoami.sh", [], "GET", "/tools/whoami", None),
            ("loop/scripts/loop.sh", ["done", "--idle=false"], "POST", "/tools/loop/done", {"idle": False}),
            ("context/scripts/context.sh", ["set", "next", "step"], "POST", "/tools/context/set", {"text": "next step"}),
            ("status/scripts/status.sh", ["set", "reviewing"], "POST", "/tools/status/set", {"message": "reviewing"}),
            ("current-task/scripts/current_task.sh", ["TARI-41"], "POST", "/tools/task/current", {"id": "TARI-41"}),
            ("messages/scripts/messages.sh", ["message", "processed", "m-1", "done"], "POST", "/tools/message/processed", {"id": "m-1", "result": "done"}),
            ("schedule/scripts/schedule.sh", ["cancel", "s-1"], "POST", "/tools/schedule/cancel", {"id": "s-1"}),
            ("scripts/scripts/scripts.sh", ["rerun", "scr-1"], "POST", "/tools/script/rerun", {"id": "scr-1"}),
            ("image-creator/scripts/image_creator.sh", ["build", "--name", "reviewer", "--path", "./image"], "POST", "/tools/image/build", {"name": "reviewer", "tag": "latest", "path": "./image"}),
            ("llm-as-judge/scripts/judge.sh", ["work", "claim"], "POST", "/tools/judge/action/work.claim", {}),
            ("tasks/scripts/tasks.sh", ["show", "TARI-1"], "POST", "/tools/tasks/show", {"key": "TARI-1"}),
        ]
        for relative, args, method, route, body in cases:
            with self.subTest(relative=relative):
                process, request = self.run_script(relative, args, {})
                self.assertEqual(process.returncode, 0, process.stderr)
                self.assertEqual(request[0], [method, route])
                self.assertEqual(request[1], b"" if body is None else json.dumps(body).encode())

    def test_direct_entrypoint_preserves_json_output(self):
        process, request = self.run_script(
            "whoami/scripts/whoami.sh", ["--json"], {"agent": "alice"}
        )
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(json.loads(process.stdout), {"agent": "alice", "client_version": "0.46.0"})
        self.assertEqual(request[0], ["GET", "/tools/whoami"])

    def test_direct_command_accepts_json_before_its_subcommand(self):
        process, request = self.run_script(
            "context/scripts/context.sh", ["--json", "get"], {"text": "handoff"}
        )
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(process.stdout, "handoff\n")
        self.assertEqual(request[0], ["GET", "/tools/context/get"])

    def test_direct_entrypoint_preserves_daemon_errors_and_plain_output(self):
        process, request = self.run_script(
            "status/scripts/status.sh", [], {},
            envelope={"ok": False, "error": {"code": "plugin_disabled", "message": "status disabled"}},
        )
        self.assertEqual(process.returncode, 1)
        self.assertEqual(process.stderr, "status disabled\n")
        self.assertEqual(request[0], ["GET", "/tools/status"])

        process, _ = self.run_script(
            "status/scripts/status.sh", [], {"enabled": True, "items": ["a", "b"], "missing": None}
        )
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(process.stdout, "enabled: true\nitems: [a b]\nmissing: <nil>\n")

    def test_direct_entrypoint_warns_on_version_mismatch(self):
        process, _ = self.run_script(
            "whoami/scripts/whoami.sh", [], {"agent": "alice"}, version="0.47.0"
        )
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertIn("client version 0.46.0 does not match daemon version 0.47.0", process.stderr)
        self.assertIn("client_version: 0.46.0", process.stdout)

    def test_direct_entrypoint_rejects_unknown_flags_before_request(self):
        process, request = self.run_script("loop/scripts/loop.sh", ["done", "--bogus"], {})
        self.assertEqual(process.returncode, 2)
        self.assertIn("unknown flag --bogus", process.stderr)
        self.assertIsNone(request)

    def test_socket_server_stops_when_launch_raises(self):
        with patch("subprocess.run", side_effect=RuntimeError("launch failed")):
            with self.assertRaisesRegex(RuntimeError, "launch failed"):
                self.run_script("whoami/scripts/whoami.sh", [], {})

    def test_workdir_is_an_instruction_only_skill(self):
        skill = ROOT / "workdir" / "SKILL.md"
        self.assertTrue(skill.is_file())
        self.assertFalse((ROOT / "workdir" / "scripts").exists())


if __name__ == "__main__":
    unittest.main()
