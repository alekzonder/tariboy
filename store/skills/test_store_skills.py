import os
from pathlib import Path
import json
import socketserver
import subprocess
import tempfile
import threading
import unittest


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
    def run_script(self, relative, args, result):
        class Handler(socketserver.StreamRequestHandler):
            def handle(handler):
                request_line = handler.rfile.readline().decode()
                headers = {}
                while line := handler.rfile.readline().decode().rstrip("\r\n"):
                    name, value = line.split(":", 1)
                    headers[name.lower()] = value.strip()
                length = int(headers.get("content-length", 0))
                server.request = (request_line.split()[:2], handler.rfile.read(length))
                body = json.dumps({"ok": True, "result": result}).encode()
                handler.wfile.write(b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" + f"Content-Length: {len(body)}\r\n\r\n".encode() + body)

        with tempfile.TemporaryDirectory() as tmp:
            path = str(Path(tmp) / "agent.sock")
            server = socketserver.UnixStreamServer(path, Handler)
            server.request = None
            thread = threading.Thread(target=server.handle_request)
            thread.start()
            process = subprocess.run(
                [ROOT / relative, *args],
                env=dict(os.environ, TARIBOY_TOOLS_SOCKET=path, TARIBOY_CLIENT_VERSION="0.46.0"),
                text=True,
                capture_output=True,
            )
            thread.join(timeout=2)
            server.server_close()
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

    def test_workdir_is_an_instruction_only_skill(self):
        skill = ROOT / "workdir" / "SKILL.md"
        self.assertTrue(skill.is_file())
        self.assertFalse((ROOT / "workdir" / "scripts").exists())


if __name__ == "__main__":
    unittest.main()
