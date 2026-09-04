import os
from pathlib import Path
import json
import shutil
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
    def run_script(
        self,
        relative,
        args,
        result,
        version="0.46.0",
        envelope=None,
        client_version="0.46.0",
    ):
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
                env = dict(os.environ, TARIBOY_TOOLS_SOCKET=path)
                if client_version is None:
                    env.pop("TARIBOY_CLIENT_VERSION", None)
                else:
                    env["TARIBOY_CLIENT_VERSION"] = client_version
                process = subprocess.run(
                    [ROOT / relative, *args],
                    env=env,
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

    def test_each_executable_skill_supports_top_level_help(self):
        env = dict(os.environ)
        env.pop("TARIBOY_TOOLS_SOCKET", None)
        for skill, command in COMMANDS.items():
            with self.subTest(skill=skill):
                result = subprocess.run(
                    [ROOT / skill / "scripts" / f"{command}.sh", "--help"],
                    env=env,
                    text=True,
                    capture_output=True,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(result.stdout)
                self.assertIn("usage:", result.stdout)
                self.assertEqual(result.stderr, "")

    def test_launcher_uses_resolved_python_interpreter(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake_python = Path(tmp) / "python3"
            fake_python.write_text("#!/bin/sh\nexit 91\n")
            fake_python.chmod(0o700)
            env = dict(
                os.environ,
                PATH=tmp + os.pathsep + os.environ["PATH"],
                TARIBOY_PYTHON3=os.path.realpath(os.sys.executable),
            )
            result = subprocess.run(
                [ROOT / "whoami/scripts/whoami.sh", "--help"],
                env=env,
                text=True,
                capture_output=True,
            )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("usage:", result.stdout)

    def test_context_help_describes_get_and_set(self):
        env = dict(os.environ)
        env.pop("TARIBOY_TOOLS_SOCKET", None)
        result = subprocess.run(
            [ROOT / "context/scripts/context.sh", "--help"],
            env=env,
            text=True,
            capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "usage: context.sh get | set <text...> [--json]\n")
        self.assertEqual(result.stderr, "")

    def test_store_skills_have_no_duplicate_prompt_files(self):
        self.assertEqual(list(ROOT.glob("*/prompt.md")), [])

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

    def test_message_reply_accepts_positional_text(self):
        process, request = self.run_script(
            "messages/scripts/messages.sh",
            ["message", "reply", "m-1", "delivered", "status"],
            {},
        )
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(request[0], ["POST", "/tools/message/reply"])
        self.assertEqual(
            json.loads(request[1]),
            {"id": "m-1", "text": "delivered status", "type": ""},
        )

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

    def test_bridge_copy_uses_producing_store_version_without_env_override(self):
        with tempfile.TemporaryDirectory() as tmp:
            bridge = (
                Path(tmp)
                / "agents/alice/image-bridges"
                / ("a" * 64)
                / "2/codex"
            )
            skill = bridge / "skills/whoami"
            shutil.copytree(ROOT / "whoami", skill)
            (bridge / "bridge-manifest.json").write_text(
                json.dumps(
                    {"skills": [{"name": "whoami", "client_version": "0.45.2"}]}
                )
            )
            process, request = self.run_script(
                skill / "scripts/whoami.sh",
                [],
                {"agent": "alice"},
                version="0.47.0",
                client_version=None,
            )
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(request[0], ["GET", "/tools/whoami"])
        self.assertIn(
            "client version 0.45.2 does not match daemon version 0.47.0",
            process.stderr,
        )
        self.assertIn("client_version: 0.45.2", process.stdout)

    def test_direct_entrypoint_rejects_unknown_flags_before_request(self):
        process, request = self.run_script("loop/scripts/loop.sh", ["done", "--bogus"], {})
        self.assertEqual(process.returncode, 2)
        self.assertIn("unknown flag --bogus", process.stderr)
        self.assertIsNone(request)

    def test_scripts_run_rejects_schedule_only_flags_before_request(self):
        for flag in ("--every=60", "--quiet-exit=0"):
            with self.subTest(flag=flag):
                process, request = self.run_script(
                    "scripts/scripts/scripts.sh",
                    ["run", "once", flag, "--", "echo", "done"],
                    {},
                )
                self.assertEqual(process.returncode, 2)
                self.assertIn("unknown flag", process.stderr)
                self.assertIsNone(request)

    def test_flexible_task_ask_rejects_workflow_flags_before_request(self):
        workflow_flags = (
            "--question=ignored",
            "--context=context",
            "--blocking-scope=task",
            "--anchor=checkpoint",
            "--suggested-answer=yes",
            "--options=yes,no",
            "--artifacts=1",
            "--task-revision=2",
            "--assignment-revision=3",
        )
        for flag in workflow_flags:
            with self.subTest(flag=flag):
                process, request = self.run_script(
                    "tasks/scripts/tasks.sh",
                    ["ask", "TARI-41", "user:alice", "Choose", flag],
                    {},
                )
                self.assertEqual(process.returncode, 2)
                self.assertIn("workflow", process.stderr)
                self.assertIsNone(request)

    def test_tasks_update_pull_request(self):
        for value in ("https://github.com/o/r/pull/7", ""):
            with self.subTest(value=value):
                process, request = self.run_script(
                    "tasks/scripts/tasks.sh",
                    ["update", "TARI-43", "--revision", "2", f"--pull-request={value}"],
                    {},
                )
                self.assertEqual(process.returncode, 0, process.stderr)
                self.assertEqual(request[0], ["POST", "/tools/tasks/update"])
                self.assertEqual(
                    json.loads(request[1]),
                    {"key": "TARI-43", "revision": "2", "pull_request": value},
                )

    def test_tasks_create_pull_request(self):
        process, request = self.run_script(
            "tasks/scripts/tasks.sh",
            [
                "create",
                "--queue=TARI",
                "--title=Ship it",
                "--pull-request=https://github.com/o/r/pull/7",
            ],
            {},
        )
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertEqual(request[0], ["POST", "/tools/tasks/create"])
        self.assertEqual(
            json.loads(request[1]),
            {
                "queue": "TARI",
                "title": "Ship it",
                "pull_request": "https://github.com/o/r/pull/7",
            },
        )

    def test_current_task_rejects_ambiguous_arguments_before_request(self):
        for args in (
            ["TARI-41", "TARI-42"],
            ["TARI-41", "--clear"],
            ["--clear=true"],
        ):
            with self.subTest(args=args):
                process, request = self.run_script(
                    "current-task/scripts/current_task.sh", args, {}
                )
                self.assertEqual(process.returncode, 2)
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
