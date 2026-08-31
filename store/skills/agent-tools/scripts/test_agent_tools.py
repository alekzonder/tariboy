import json
import os
from pathlib import Path
import socketserver
import subprocess
import tempfile
import threading
import unittest


ROOT = Path(__file__).resolve().parents[4]


class UnixHTTPHandler(socketserver.StreamRequestHandler):
    def handle(self):
        request_line = self.rfile.readline().decode()
        headers = {}
        while line := self.rfile.readline().decode().rstrip("\r\n"):
            name, value = line.split(":", 1)
            headers[name.lower()] = value.strip()
        length = int(headers.get("content-length", 0))
        self.server.requests.append((request_line.split()[:2], self.rfile.read(length)))
        body = json.dumps(self.server.envelope).encode()
        self.wfile.write(
            b"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n"
            + f"Content-Length: {len(body)}\r\nX-Tariboy-Version: {self.server.version}\r\n\r\n".encode()
            + body
        )


class UnixHTTPServer(socketserver.UnixStreamServer):
    def __init__(self, path, result, version="0.46.0", envelope=None):
        self.envelope = envelope or {"ok": True, "result": result}
        self.version = version
        self.requests = []
        super().__init__(path, UnixHTTPHandler)
        self.timeout = 0.25


class AgentToolsTest(unittest.TestCase):
    def run_script(self, relative, args, response, version="0.46.0", envelope=None):
        with tempfile.TemporaryDirectory() as tmp:
            sock = str(Path(tmp) / "agent.sock")
            server = UnixHTTPServer(sock, response, version, envelope)
            thread = threading.Thread(target=server.handle_request)
            thread.start()
            env = dict(os.environ, TARIBOY_TOOLS_SOCKET=sock, TARIBOY_CLIENT_VERSION="0.46.0")
            result = subprocess.run(
                ["python3", ROOT / "store/skills" / relative, *args],
                env=env,
                text=True,
                capture_output=True,
            )
            thread.join(timeout=2)
            server.server_close()
        return result, server.requests

    def test_whoami_uses_agent_socket_and_prints_result(self):
        with tempfile.TemporaryDirectory() as tmp:
            sock = str(Path(tmp) / "agent.sock")
            server = UnixHTTPServer(sock, {"agent": "alice", "iteration": "iter-1"})
            thread = threading.Thread(target=server.handle_request)
            thread.start()
            env = dict(os.environ, TARIBOY_TOOLS_SOCKET=sock, TARIBOY_CLIENT_VERSION="0.46.0")
            result = subprocess.run(
                ["python3", ROOT / "store/skills/whoami/scripts/whoami.py"],
                env=env,
                text=True,
                capture_output=True,
            )
            thread.join(timeout=2)
            server.server_close()

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("agent: alice", result.stdout)
        self.assertEqual(server.requests, [(["GET", "/tools/whoami"], b"")])

    def test_dispatcher_preserves_json_version_and_errors(self):
        result, requests = self.run_script(
            "agent-tools/scripts/tools.py", ["--json", "whoami"],
            {"agent": "alice", "daemon_version": "0.46.0"},
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {
            "agent": "alice", "client_version": "0.46.0", "daemon_version": "0.46.0",
        })
        self.assertEqual(requests, [(["GET", "/tools/whoami"], b"")])

        result, _ = self.run_script(
            "status/scripts/status.py", [], {},
            envelope={"ok": False, "error": {"code": "plugin_disabled", "message": "status disabled"}},
        )
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stderr, "status disabled\n")

        env = dict(os.environ, TARIBOY_CLIENT_VERSION="0.46.0")
        result = subprocess.run(
            ["python3", ROOT / "store/skills/agent-tools/scripts/tools.py", "--version"],
            env=env, text=True, capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "0.46.0\n")

    def test_dispatcher_help_preserves_command_reference(self):
        result = subprocess.run(
            ["python3", ROOT / "store/skills/agent-tools/scripts/tools.py", "help"],
            text=True,
            capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        for command in [
            "tasks work show|complete|release ASSIGNMENT",
            "message processed ID <result...>",
            "script schedule NAME --every SECONDS",
            "judge summary submit RUN --file summary.json",
        ]:
            self.assertIn(command, result.stdout)

    def test_plain_output_matches_previous_cli_format(self):
        result, _ = self.run_script(
            "status/scripts/status.py",
            [],
            {"enabled": True, "items": ["a", "b"], "missing": None},
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            result.stdout,
            "enabled: true\nitems: [a b]\nmissing: <nil>\n",
        )

        result, _ = self.run_script(
            "status/scripts/status.py",
            [],
            ["a", True, None],
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, '["a",true,null]\n')

    def test_version_mismatch_warns_without_changing_success(self):
        result, _ = self.run_script(
            "whoami/scripts/whoami.py", [], {"agent": "alice", "daemon_version": "0.47.0"}, version="0.47.0",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("client version 0.46.0 does not match daemon version 0.47.0", result.stderr)
        self.assertIn("client_version: 0.46.0", result.stdout)

    def test_unknown_flags_are_rejected_before_request(self):
        cases = [
            ("whoami/scripts/whoami.py", ["--bogus"]),
            ("status/scripts/status.py", ["set", "working", "--bogus"]),
            ("context/scripts/context.py", ["set", "next", "--bogus"]),
            ("loop/scripts/loop.py", ["done", "--bogus"]),
            ("current-task/scripts/current_task.py", ["TARI-41", "--bogus"]),
            ("schedule/scripts/schedule.py", ["cancel", "sched-1", "--bogus"]),
            ("scripts/scripts/scripts.py", ["rerun", "scr-1", "--bogus"]),
            ("messages/scripts/messages.py", ["group", "info", "--bogus"]),
        ]
        for relative, args in cases:
            with self.subTest(relative=relative):
                result, requests = self.run_script(relative, args, {})
                self.assertEqual(result.returncode, 2)
                self.assertIn("unknown flag --bogus", result.stderr)
                self.assertEqual(requests, [])

    def test_core_skill_scripts_map_commands(self):
        cases = [
            ("status/scripts/status.py", [], {"state": "running"}, "GET", "/tools/status", None),
            ("status/scripts/status.py", ["set", "reviewing", "diff"], {}, "POST", "/tools/status/set", {"message": "reviewing diff"}),
            ("context/scripts/context.py", ["get"], {"text": "handoff"}, "GET", "/tools/context/get", None),
            ("context/scripts/context.py", ["set", "next", "step"], {}, "POST", "/tools/context/set", {"text": "next step"}),
            ("loop/scripts/loop.py", ["done", "--idle=false"], {}, "POST", "/tools/loop/done", {"idle": False}),
            ("loop/scripts/loop.py", ["done", "--idle", "false"], {}, "POST", "/tools/loop/done", {"idle": False}),
            ("loop/scripts/loop.py", ["start"], {}, "POST", "/tools/loop/control", {"action": "start"}),
            ("current-task/scripts/current_task.py", ["TARI-41"], {}, "POST", "/tools/task/current", {"id": "TARI-41"}),
            ("current-task/scripts/current_task.py", ["--clear"], {}, "POST", "/tools/task/current", {"clear": True}),
        ]
        for relative, args, response, method, route, body in cases:
            with self.subTest(relative=relative, args=args):
                result, requests = self.run_script(relative, args, response)
                self.assertEqual(result.returncode, 0, result.stderr)
                raw = b"" if body is None else json.dumps(body).encode()
                self.assertEqual(requests, [([method, route], raw)])
        result, _ = self.run_script("context/scripts/context.py", ["get"], {"text": "handoff"})
        self.assertEqual(result.stdout, "handoff\n")

    def test_messages_skill_maps_commands(self):
        cases = [
            (["message", "send", "--channel", "chat:ops", "--type", "note", "--subject", "env=prod", "--text", "hello", "--data", '{"n":1}'], "/tools/message/send", {"channel": "chat:ops", "type": "note", "text": "hello", "subject": {"env": "prod"}, "data": {"n": 1}}),
            (["message", "processed", "m-1", "read", "and", "acted"], "/tools/message/processed", {"id": "m-1", "result": "read and acted"}),
            (["message", "reply", "m-2", "--text", "answer", "--data", '{"ok":true}'], "/tools/message/reply", {"id": "m-2", "text": "answer", "type": "", "data": {"ok": True}}),
            (["message", "dlq", "requeue", "m-3"], "/tools/message/dlq/requeue", {"id": "m-3"}),
            (["request", "--channel", "svc:q", "--text", "do it", "--deadline", "5m"], "/tools/request", {"channel": "svc:q", "text": "do it", "deadline": "5m"}),
            (["channel", "subscribe", "issues:query", "--type", "ticket.*", "--matcher", '{"subject.env":"prod"}', "--params", '{"query":"Open"}'], "/tools/channel/subscribe", {"channel": "issues:query", "type": "ticket.*", "matcher": {"subject.env": "prod"}, "params": {"query": "Open"}}),
            (["group", "request", "worker", "--text", "status?", "--deadline", "5m"], "/tools/group/request", {"member": "worker", "text": "status?", "deadline": "5m"}),
            (["group", "loop", "start", "worker"], "/tools/group/loop", {"member": "worker", "action": "start"}),
        ]
        for args, route, body in cases:
            with self.subTest(args=args):
                result, requests = self.run_script("messages/scripts/messages.py", args, {})
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(requests, [(["POST", route], json.dumps(body).encode())])

        response = {"channels": [{"name": "issues:query", "kind": "chat", "provider": "issues", "params": ["query"], "help": "search issues"}]}
        result, requests = self.run_script("messages/scripts/messages.py", ["sources"], response)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(requests, [(["GET", "/tools/sources"], b"")])
        self.assertEqual(result.stdout, "issues:query  chat  provider=issues  params: {query}  search issues\n")

    def test_schedule_scripts_and_image_skill_commands(self):
        cases = [
            ("schedule/scripts/schedule.py", ["add", "--kind", "cron", "--spec", "*/5 * * * *", "--channel", "agent:a:inbox", "--message", '{"text":"wake"}'], "/tools/schedule/add", {"kind": "cron", "spec": "*/5 * * * *", "channel": "agent:a:inbox", "message": {"text": "wake"}}),
            ("schedule/scripts/schedule.py", ["cancel", "sched-1"], "/tools/schedule/cancel", {"id": "sched-1"}),
            ("scripts/scripts/scripts.py", ["run", "check", "--description", "one", "--", "make", "check", "--fast"], "/tools/script/run", {"name": "check", "description": "one", "command": "make check --fast"}),
            ("scripts/scripts/scripts.py", ["schedule", "watch", "--every", "60", "--quiet-exit", "2", "--", "check-ci"], "/tools/script/schedule", {"name": "watch", "description": "watch", "command": "check-ci", "interval_seconds": 60, "quiet_exit": 2}),
            ("scripts/scripts/scripts.py", ["rerun", "scr-1"], "/tools/script/rerun", {"id": "scr-1"}),
            ("image-creator/scripts/image_creator.py", ["build", "--name", "reviewer", "--path", "./image"], "/tools/image/build", {"name": "reviewer", "tag": "latest", "path": "./image"}),
        ]
        for relative, args, route, body in cases:
            with self.subTest(relative=relative, args=args):
                result, requests = self.run_script(relative, args, {})
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(requests, [(["POST", route], json.dumps(body).encode())])

    def test_tasks_skill_maps_flexible_and_workflow_commands(self):
        cases = [
            (["mine", "--queue", "TARI", "--waiting-for", "me"], "mine", {"queue": "TARI", "waiting_for": "me"}),
            (["ready", "--claim", "--limit", "2"], "ready", {"claim": True, "limit": "2"}),
            (["show", "TARI-1"], "show", {"key": "TARI-1"}),
            (["create", "--queue", "TARI", "--title", "new", "--priority", "P0"], "create", {"queue": "TARI", "title": "new", "priority": "P0"}),
            (["update", "TARI-1", "--status", "in_progress", "--manual-block-reason="], "update", {"key": "TARI-1", "status": "in_progress", "manual_block_reason": ""}),
            (["assign", "TARI-1", "worker", "--revision", "4"], "assign", {"key": "TARI-1", "assignee": "worker", "revision": "4"}),
            (["comment", "TARI-1", "progress", "update"], "comment", {"key": "TARI-1", "body": "progress update"}),
            (["ask", "TARI-1", "user:agent", "Which", "option?"], "ask", {"key": "TARI-1", "principal": "user:agent", "body": "Which option?"}),
            (["move", "TARI-2", "--to-root"], "move", {"key": "TARI-2", "parent_key": ""}),
            (["block", "TARI-2", "--by", "TARI-3"], "block", {"key": "TARI-2", "blocker_key": "TARI-3"}),
            (["relate", "TARI-1", "TARI-4"], "relate", {"key": "TARI-1", "target_key": "TARI-4"}),
            (["done", "TARI-1", "--complete-anyway"], "done", {"key": "TARI-1", "complete_anyway": True}),
            (["work", "next", "--queue", "TARI", "--idempotency-key", "next-1"], "work_next", {"queue": "TARI", "idempotency_key": "next-1"}),
            (["work", "complete", "42", "--task-revision", "7", "--assignment-revision", "3", "--outcome", "approved", "--idempotency-key", "finish"], "work_complete", {"assignment_id": "42", "task_revision": "7", "assignment_revision": "3", "outcome": "approved", "idempotency_key": "finish"}),
            (["artifacts", "add", "42", "--name", "result", "--type", "json", "--content="], "artifact_add", {"assignment_id": "42", "name": "result", "type": "json", "content": ""}),
            (["artifacts", "show", "42", "9", "--task", "TARI-1"], "artifact_show", {"assignment_id": "42", "artifact_id": "9", "task_key": "TARI-1"}),
            (["ask", "42", "--question", "Proceed?", "--context", "risk", "--blocking-scope", "assignment", "--options", "yes,no", "--artifacts", "8,9"], "workflow_ask", {"assignment_id": "42", "question": "Proceed?", "context": "risk", "blocking_scope": "assignment", "options": ["yes", "no"], "artifact_attachments": [8, 9]}),
            (["questions", "42"], "questions", {"assignment_id": "42"}),
            (["answer", "9", "--assignment", "42", "--answer", "yes"], "workflow_answer", {"question_id": "9", "assignment_id": "42", "answer": "yes"}),
            (["observe", "subscribe", "42", "ci:*", "--reaction", "wake_current"], "observe_subscribe", {"assignment_id": "42", "pattern": "ci:*", "reaction": "wake_current"}),
            (["observe", "cancel", "42", "sub-1"], "observe_cancel", {"assignment_id": "42", "subscription_id": "sub-1"}),
        ]
        for args, action, body in cases:
            with self.subTest(args=args):
                result, requests = self.run_script("tasks/scripts/tasks.py", args, {})
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(requests[0][0], ["POST", "/tools/tasks/" + action])
                self.assertEqual(json.loads(requests[0][1]), body)

        result, requests = self.run_script(
            "tasks/scripts/tasks.py", ["--json", "show", "TARI-1"], {"key": "TARI-1"},
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, '{"key":"TARI-1"}\n')
        self.assertEqual(requests[0][0], ["POST", "/tools/tasks/show"])

    def test_judge_skill_maps_authenticated_actions_and_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            request_file = Path(tmp) / "request.txt"
            result_file = Path(tmp) / "result.json"
            request_file.write_text("judge this exactly")
            result_file.write_text('{"verdict":"pass"}')
            cases = [
                (["automation", "begin", "--revision", "7", "--delivery", "d-1", "--limit", "3"], "automation.begin", {"config_revision": 7, "delivery_id": "d-1", "limit": 3}),
                (["iterations", "search", "--agent", "target", "--judge-group", "judges", "--group", "targets", "--status", "done,error"], "iterations.search", {"judge_group": "judges", "selector": {"agents": ["target"], "group": "targets", "statuses": ["done", "error"]}}),
                (["run", "create", "--request-file", str(request_file), "--selector", '{"iteration_ids":["i-1"]}', "--judges", "j1,j2", "--summary-agent", "lead", "--judges-per-iteration", "2"], "run.create", {"original_request": "judge this exactly", "selector": {"iteration_ids": ["i-1"]}, "judge_agents": ["j1", "j2"], "summary_agent": "lead", "judges_per_iteration": 2}),
                (["evidence", "get", "--assignment", "a-1", "--artifact", "audit", "--locator", '{"line":2}'], "evidence.get", {"assignment_id": "a-1", "artifact": "audit", "locator": {"line": 2}}),
                (["analysis", "submit", "--assignment", "a-1", "--file", str(result_file)], "analysis.submit", {"assignment_id": "a-1", "result": {"verdict": "pass"}, "raw_submission": '{"verdict":"pass"}'}),
                (["summary", "inputs", "run-1", "--cursor", "next"], "summary.inputs", {"run_id": "run-1", "cursor": "next"}),
                (["improvement", "submit", "run-1", "--file", str(result_file)], "improvement.submit", {"run_id": "run-1", "result": {"verdict": "pass"}, "raw_submission": '{"verdict":"pass"}'}),
            ]
            for args, action, body in cases:
                with self.subTest(args=args):
                    result, requests = self.run_script("llm-as-judge/scripts/judge.py", args, {})
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(requests[0][0], ["POST", "/tools/judge/action/" + action])
                    self.assertEqual(json.loads(requests[0][1]), body)


if __name__ == "__main__":
    unittest.main()
