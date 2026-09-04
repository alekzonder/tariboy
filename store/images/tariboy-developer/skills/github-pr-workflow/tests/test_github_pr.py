#!/usr/bin/env python3
"""Black-box contracts for the github-pr-workflow utility.

The utility is intentionally not present while these tests are introduced.  The
fake curl process is the GitHub boundary: it proves the future utility uses an
inherited curl config descriptor for authentication and gives each test a
deterministic sequence of API responses.
"""

import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import textwrap
import unittest


REPO = "alekzonder/tariboy"
HEAD = "tari-31-pr-workflow"
BASE = "main"
SECRET = "super-secret-token"
SKILL_DIR = Path(__file__).resolve().parents[1]
UTILITY = SKILL_DIR / "scripts" / "github-pr.py"


FAKE_CURL = r'''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

args = sys.argv[1:]
secret = os.environ["FAKE_CURL_EXPECTED_TOKEN"]
if secret in "\0".join(args):
    raise SystemExit("token appeared in curl argv")
if os.environ.get("FAKE_CURL_REQUIRE_DISABLE") == "1":
    if not (Path(os.environ["HOME"]) / ".curlrc").is_file():
        raise SystemExit("hostile curlrc fixture is missing")
    if not args or args[0] != "--disable":
        raise SystemExit("--disable was not the first curl option")
try:
    config = args[args.index("--config") + 1]
except (ValueError, IndexError) as exc:
    raise SystemExit("curl config descriptor was not supplied") from exc
if not config.startswith("/dev/fd/"):
    raise SystemExit("curl config is not an inherited descriptor")
config_text = Path(config).read_text(encoding="utf-8")
if "Authorization: Bearer " + secret not in config_text:
    raise SystemExit("authorization header was not supplied through config")
for candidate in Path(os.environ["FAKE_CURL_PROCESS_TEMP"]).rglob("*"):
    if candidate.is_file() and secret.encode("utf-8") in candidate.read_bytes():
        raise SystemExit("token materialized in the process temporary directory")

method = "GET"
for flag in ("-X", "--request"):
    if flag in args:
        method = args[args.index(flag) + 1]
body = None
for flag in ("--data", "--data-raw", "--data-binary"):
    if flag in args:
        body = json.loads(args[args.index(flag) + 1])
url = next((arg for arg in reversed(args) if arg.startswith("https://")), None)
if url is None:
    raise SystemExit("curl invocation has no HTTPS URL")
record_path = Path(os.environ["FAKE_CURL_RECORDS"])
with record_path.open("a", encoding="utf-8") as out:
    out.write(json.dumps({"method": method, "url": url, "json": body}, sort_keys=True) + "\n")

queue_path = Path(os.environ["FAKE_CURL_QUEUE"])
queue = json.loads(queue_path.read_text(encoding="utf-8"))
if not queue:
    raise SystemExit("unexpected curl request")
response = queue.pop(0)
queue_path.write_text(json.dumps(queue), encoding="utf-8")
for stream_name, output in (("stdout_bytes", sys.stdout.buffer), ("stderr_bytes", sys.stderr.buffer)):
    stream_bytes = response.get(stream_name)
    if stream_bytes is None:
        continue
    remaining = stream_bytes
    chunk = b"x" * 65536
    while remaining:
        output.write(chunk[:min(remaining, len(chunk))])
        output.flush()
        remaining -= min(remaining, len(chunk))
    Path(os.environ["FAKE_CURL_STREAM_COMPLETE"]).write_text(stream_name, encoding="utf-8")
    raise SystemExit(response.get("exit", 0))
payload = response.get("body", {})
if isinstance(payload, str):
    sys.stdout.write(payload)
else:
    sys.stdout.write(json.dumps(payload))
raise SystemExit(response.get("exit", 0))
'''


class FakeCurl:
    def __init__(self, responses, require_disable=False):
        self.responses = responses
        self.require_disable = require_disable
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name)
        self.binary = self.path / "curl"
        self.queue = self.path / "queue.json"
        self.records = self.path / "records.jsonl"
        self.stream_complete = self.path / "stream-complete"

    def __enter__(self):
        self.binary.write_text(textwrap.dedent(FAKE_CURL), encoding="utf-8")
        self.binary.chmod(0o700)
        self.set_responses(self.responses)
        return self

    def __exit__(self, *_):
        self.temp.cleanup()

    def set_responses(self, responses):
        self.queue.write_text(json.dumps(responses), encoding="utf-8")
        self.stream_complete.unlink(missing_ok=True)

    def records_json(self):
        if not self.records.exists():
            return []
        return [json.loads(line) for line in self.records.read_text(encoding="utf-8").splitlines()]

    def env(self, expected_token, process_temp):
        return {
            "TARIBOY_GITHUB_CURL_BIN": str(self.binary),
            "FAKE_CURL_QUEUE": str(self.queue),
            "FAKE_CURL_RECORDS": str(self.records),
            "FAKE_CURL_EXPECTED_TOKEN": expected_token,
            "FAKE_CURL_PROCESS_TEMP": str(process_temp),
            "FAKE_CURL_REQUIRE_DISABLE": "1" if self.require_disable else "0",
            "FAKE_CURL_STREAM_COMPLETE": str(self.stream_complete),
        }


def pr(head="abc123", state="open", merged=False, merge_commit_sha=None):
    return {
        "number": 31,
        "state": state,
        "merged": merged,
        "merge_commit_sha": merge_commit_sha,
        "updated_at": "2026-08-21T12:00:00Z",
        "html_url": "https://github.com/alekzonder/tariboy/pull/31",
        "head": {"sha": head},
    }


def monitor_responses(pr_body=None, checks=None, statuses=None, comments=None, review_comments=None, reviews=None):
    return [
        {"body": pr_body or pr()},
        {"body": {"total_count": len(checks or []), "check_runs": checks or []}},
        {"body": statuses or []},
        {"body": comments or []},
        {"body": review_comments or []},
        {"body": reviews or []},
    ]


class GitHubPRWorkflowTests(unittest.TestCase):
    maxDiff = None

    def setUp(self):
        if not UTILITY.is_file():
            self.fail(f"github PR utility is missing: {UTILITY}")

    def run_utility(
        self,
        *args,
        curl,
        gh_token=SECRET,
        github_token=None,
        github_repository=None,
        origin_url=None,
    ):
        with tempfile.TemporaryDirectory() as process_sandbox:
            sandbox = Path(process_sandbox)
            home = sandbox / "home"
            xdg_state = sandbox / "xdg-state"
            xdg_cache = sandbox / "xdg-cache"
            xdg_config = sandbox / "xdg-config"
            xdg_data = sandbox / "xdg-data"
            xdg_runtime = sandbox / "xdg-runtime"
            process_temp = sandbox / "tmp"
            working_dir = sandbox / "cwd"
            for directory in (home, xdg_state, xdg_cache, xdg_config, xdg_data, xdg_runtime, process_temp, working_dir):
                directory.mkdir(mode=0o700)
            if origin_url is not None:
                subprocess.run(
                    ["git", "init", "--quiet"],
                    cwd=working_dir,
                    check=True,
                    capture_output=True,
                    text=True,
                )
                subprocess.run(
                    ["git", "config", "remote.origin.url", origin_url],
                    cwd=working_dir,
                    check=True,
                    capture_output=True,
                    text=True,
                )
            (home / ".curlrc").write_text('header = "X-Hostile-Curlrc: loaded"\n', encoding="utf-8")
            env = os.environ.copy()
            env.pop("GH_TOKEN", None)
            env.pop("GITHUB_TOKEN", None)
            env.pop("GITHUB_REPOSITORY", None)
            env.update({
                "HOME": str(home),
                "PWD": str(working_dir),
                "XDG_STATE_HOME": str(xdg_state),
                "XDG_CACHE_HOME": str(xdg_cache),
                "XDG_CONFIG_HOME": str(xdg_config),
                "XDG_DATA_HOME": str(xdg_data),
                "XDG_RUNTIME_DIR": str(xdg_runtime),
                "TMPDIR": str(process_temp),
                "TMP": str(process_temp),
                "TEMP": str(process_temp),
            })
            expected_token = gh_token if gh_token is not None else (github_token if github_token is not None else SECRET)
            env.update(curl.env(expected_token, sandbox))
            if gh_token is not None:
                env["GH_TOKEN"] = gh_token
            if github_token is not None:
                env["GITHUB_TOKEN"] = github_token
            if github_repository is not None:
                env["GITHUB_REPOSITORY"] = github_repository
            result = subprocess.run(
                [sys.executable, str(UTILITY), *args],
                cwd=working_dir,
                env=env,
                text=True,
                capture_output=True,
                check=False,
            )
            self.assert_no_secret_on_disk(sandbox)
            for state_dir in self.absolute_state_dirs(args, sandbox):
                self.assert_no_secret_on_disk(state_dir)
            return result

    def assert_secret_free(self, result):
        self.assertNotIn(SECRET, result.stdout)
        self.assertNotIn(SECRET, result.stderr)

    def assert_no_secret_on_disk(self, root):
        for candidate in root.rglob("*"):
            if candidate.is_file():
                self.assertNotIn(SECRET.encode("utf-8"), candidate.read_bytes(), candidate)

    def absolute_state_dirs(self, args, sandbox):
        state_dirs = []
        for index, arg in enumerate(args[:-1]):
            if arg != "--state-dir":
                continue
            candidate = Path(args[index + 1])
            if candidate.is_absolute() and not candidate.is_relative_to(sandbox):
                state_dirs.append(candidate)
        return state_dirs

    def assert_no_requests(self, curl):
        self.assertEqual(curl.records_json(), [])

    def test_preflight_rejects_missing_token_before_network(self):
        with FakeCurl([]) as curl:
            result = self.run_utility("preflight", "--repo", REPO, curl=curl, gh_token=None)
        self.assertNotEqual(result.returncode, 0)
        self.assert_no_requests(curl)

    def test_preflight_falls_back_to_github_token_without_leaking_it(self):
        with FakeCurl([{"body": {"full_name": REPO}}]) as curl:
            result = self.run_utility("preflight", "--repo", REPO, curl=curl, gh_token=None, github_token=SECRET)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_secret_free(result)
        self.assertEqual(len(records), 1)
        self.assertNotIn(SECRET, json.dumps(records))

    def test_preflight_gives_gh_token_precedence_and_redacts_invalid_value(self):
        with FakeCurl([{"body": {"full_name": REPO}}]) as curl:
            result = self.run_utility("preflight", "--repo", REPO, curl=curl, gh_token=SECRET, github_token="fallback-token")
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(records), 1)
        with FakeCurl([]) as curl:
            invalid = self.run_utility("preflight", "--repo", REPO, curl=curl, gh_token=SECRET + "\ninvalid", github_token="fallback-token")
        self.assertNotEqual(invalid.returncode, 0)
        self.assert_secret_free(invalid)
        self.assert_no_requests(curl)

    def test_repository_discovery_uses_github_repository_and_configured_origin(self):
        cases = [
            ("environment", {"github_repository": REPO}),
            ("origin", {"origin_url": "git@github.com:alekzonder/tariboy.git"}),
        ]
        for name, discovery in cases:
            with self.subTest(name=name), FakeCurl([{"body": {"full_name": REPO}}]) as curl:
                result = self.run_utility("preflight", curl=curl, **discovery)
                records = curl.records_json()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout), {"ok": True, "repo": REPO})
            self.assertEqual(len(records), 1)
            self.assertIn(f"/repos/{REPO}", records[0]["url"])

    def test_curl_disables_default_config_before_every_other_option(self):
        with FakeCurl([{"body": {"full_name": REPO}}], require_disable=True) as curl:
            result = self.run_utility("preflight", "--repo", REPO, curl=curl)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_secret_free(result)

    def test_curl_output_pipes_are_stopped_at_bounded_sizes(self):
        cases = [
            ("stdout", {"stdout_bytes": 16 * 1024 * 1024}),
            ("stderr", {"stderr_bytes": 1024 * 1024}),
        ]
        for name, response in cases:
            with self.subTest(name=name), FakeCurl([response]) as curl:
                result = self.run_utility("preflight", "--repo", REPO, curl=curl)
                completed = curl.stream_complete.exists()
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(completed, f"curl completed writing oversized {name}")
            self.assertNotIn("Traceback", result.stderr)
            self.assert_secret_free(result)

    def test_ensure_returns_one_existing_open_pull_request(self):
        with FakeCurl([{"body": [pr()]}]) as curl:
            result = self.run_utility("ensure", "--repo", REPO, "--head", HEAD, "--base", BASE, "--title", "TARI-31", curl=curl)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"created": False, "number": 31, "state": "open", "url": pr()["html_url"]})
        self.assertEqual([record["method"] for record in records], ["GET"])

    def test_ensure_returns_one_existing_closed_pull_request_for_monitoring(self):
        closed = pr(state="closed")
        with FakeCurl([{"body": [closed]}]) as curl:
            result = self.run_utility("ensure", "--repo", REPO, "--head", HEAD, "--base", BASE, "--title", "TARI-31", curl=curl)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            json.loads(result.stdout),
            {
                "created": False,
                "number": 31,
                "requires_decision": True,
                "state": "closed",
                "url": closed["html_url"],
            },
        )
        self.assertEqual([record["method"] for record in records], ["GET"])

    def test_ensure_creates_then_reconciles_one_pull_request(self):
        with FakeCurl([{"body": []}, {"body": pr()}, {"body": [pr()]}]) as curl:
            result = self.run_utility("ensure", "--repo", REPO, "--head", HEAD, "--base", BASE, "--title", "TARI-31", curl=curl)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"created": True, "number": 31, "state": "open", "url": pr()["html_url"]})
        self.assertEqual([record["method"] for record in records], ["GET", "POST", "GET"])
        self.assertEqual(records[1]["json"], {"base": BASE, "head": HEAD, "title": "TARI-31"})

    def test_ensure_reconciles_uncertain_failed_create_to_one_existing_pull_request(self):
        responses = [
            {"body": []},
            {"body": {"message": "upstream response was lost"}, "exit": 22},
            {"body": [pr()]},
        ]
        with FakeCurl(responses) as curl:
            result = self.run_utility(
                "ensure",
                "--repo",
                REPO,
                "--head",
                HEAD,
                "--base",
                BASE,
                "--title",
                "TARI-31",
                curl=curl,
            )
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            json.loads(result.stdout),
            {"created": False, "number": 31, "state": "open", "url": pr()["html_url"]},
        )
        self.assertEqual([record["method"] for record in records], ["GET", "POST", "GET"])

    def test_ensure_rejects_ambiguous_matches_without_creating(self):
        other = pr(head="def456")
        other["number"] = 32
        with FakeCurl([{"body": [pr(), other]}]) as curl:
            result = self.run_utility("ensure", "--repo", REPO, "--head", HEAD, "--base", BASE, "--title", "TARI-31", curl=curl)
            records = curl.records_json()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("POST", [record["method"] for record in records])

    def test_invalid_repository_branch_pr_and_state_are_rejected_before_network(self):
        cases = [
            ("preflight", "--repo", "owner/repo;rm"),
            ("ensure", "--repo", REPO, "--head", "bad branch", "--base", BASE, "--title", "TARI-31"),
            ("monitor", "--repo", REPO, "--pr", "0", "--state-dir", "relative"),
        ]
        with FakeCurl([]) as curl:
            for args in cases:
                with self.subTest(args=args):
                    result = self.run_utility(*args, curl=curl)
                    self.assertNotEqual(result.returncode, 0)
            with tempfile.TemporaryDirectory() as state_parent:
                actual = Path(state_parent) / "actual"
                actual.mkdir()
                linked = Path(state_parent) / "linked"
                linked.symlink_to(actual, target_is_directory=True)
                result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", str(linked), curl=curl)
                self.assertNotEqual(result.returncode, 0)
            self.assert_no_requests(curl)

    def test_monitor_first_unchanged_and_check_change_have_exact_exit_contract(self):
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
            first = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            snapshot = Path(state_dir) / "pr-31.json"
            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertTrue(snapshot.exists())
            self.assertEqual(stat.S_IMODE(snapshot.stat().st_mode), 0o600)
            self.assertNotIn(SECRET, snapshot.read_text(encoding="utf-8"))
            curl.set_responses(monitor_responses())
            unchanged = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            curl.set_responses(monitor_responses(checks=[{"id": 7, "name": "check", "status": "completed", "conclusion": "success"}]))
            changed = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
        self.assertEqual(unchanged.returncode, 2, unchanged.stderr)
        self.assertEqual(changed.returncode, 0, changed.stderr)

    def test_monitor_failed_check_and_status_transitions_emit_actionable_metadata(self):
        successful_check = {
            "id": 7,
            "name": "unit-tests",
            "status": "completed",
            "conclusion": "success",
        }
        pending_status = {"id": 8, "context": "legacy-ci", "state": "pending"}
        failed_check = {**successful_check, "conclusion": "failure"}
        failed_status = {**pending_status, "state": "failure"}
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(
            monitor_responses(checks=[successful_check], statuses=[pending_status])
        ) as curl:
            initial = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
            curl.set_responses(
                monitor_responses(checks=[failed_check], statuses=[failed_status])
            )
            changed = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
        self.assertEqual(initial.returncode, 0, initial.stderr)
        self.assertEqual(changed.returncode, 0, changed.stderr)
        facts = json.loads(changed.stdout)["facts"]
        check_fact = next(fact for fact in facts if fact["kind"] == "check_runs")
        status_fact = next(fact for fact in facts if fact["kind"] == "statuses")
        self.assertEqual(check_fact["head_sha"], "abc123")
        self.assertEqual(
            check_fact["check_runs"],
            [
                {
                    "id": 7,
                    "name": "unit-tests",
                    "status": "completed",
                    "conclusion": "failure",
                    "started_at": None,
                    "completed_at": None,
                }
            ],
        )
        self.assertEqual(status_fact["head_sha"], "abc123")
        self.assertEqual(
            status_fact["statuses"],
            [
                {
                    "id": 8,
                    "context": "legacy-ci",
                    "state": "failure",
                    "created_at": None,
                    "updated_at": None,
                }
            ],
        )

    def test_monitor_new_head_invalidates_old_successful_checks(self):
        old_check = {"id": 7, "name": "old-success-check", "status": "completed", "conclusion": "success"}
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
            curl.set_responses(monitor_responses(pr_body=pr(head="old-head"), checks=[old_check]))
            first = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            old_state = (Path(state_dir) / "pr-31.json").read_text(encoding="utf-8")
            curl.set_responses(monitor_responses(pr_body=pr(head="new-head"), checks=[]))
            result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            state = (Path(state_dir) / "pr-31.json").read_text(encoding="utf-8")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertIn("old-head", old_state)
        self.assertIn("old-success-check", old_state)
        self.assertIn("success", old_state.lower())
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("new-head", state)
        self.assertNotIn("old-head", state)
        self.assertNotIn("old-success-check", state)
        self.assertNotIn("success", state.lower())
        self.assertIn("new-head", result.stdout)
        self.assertIn("head", result.stdout.lower())
        self.assertIn("check", result.stdout.lower())
        self.assertTrue(any(marker in result.stdout.lower() for marker in ("invalidate", "replace", "reset")), result.stdout)

    def test_monitor_emits_each_untrusted_comment_and_review_only_when_new_or_updated(self):
        comments = [{"id": 1, "updated_at": "2026-08-21T12:00:00Z", "body": "initial issue body"}]
        review_comments = [{"id": 2, "updated_at": "2026-08-21T12:00:00Z", "body": "review comment body"}]
        reviews = [{"id": 3, "submitted_at": "2026-08-21T12:00:00Z", "body": "review body", "state": "COMMENTED", "commit_id": "abc123"}]
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses(comments=comments, review_comments=review_comments, reviews=reviews)) as curl:
            first = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            curl.set_responses(monitor_responses(comments=comments, review_comments=review_comments, reviews=reviews))
            unchanged = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            updated_comments = [{"id": 1, "updated_at": "2026-08-22T12:00:00Z", "body": "updated comment body"}]
            curl.set_responses(monitor_responses(comments=updated_comments, review_comments=review_comments, reviews=reviews))
            updated = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            state = (Path(state_dir) / "pr-31.json").read_text(encoding="utf-8")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertGreaterEqual(first.stdout.lower().count("untrusted"), 3)
        for body in ("initial issue body", "review comment body", "review body"):
            self.assertIn(body, first.stdout)
            self.assertNotIn(body, state)
        self.assertEqual(unchanged.returncode, 2, unchanged.stderr)
        for body in ("initial issue body", "review comment body", "review body"):
            self.assertNotIn(body, unchanged.stdout)
        self.assertEqual(updated.returncode, 0, updated.stderr)
        self.assertIn("untrusted", updated.stdout.lower())
        self.assertIn("updated comment body", updated.stdout)
        for body in ("initial issue body", "review comment body", "review body"):
            self.assertNotIn(body, updated.stdout)
        for body in ("updated comment body", "review comment body", "review body"):
            self.assertNotIn(body, state)

    def test_monitor_emits_review_state_and_commit_id_with_untrusted_review_fact(self):
        review = {
            "id": 3,
            "submitted_at": "2026-08-21T12:00:00Z",
            "body": "changes requested",
            "state": "CHANGES_REQUESTED",
            "commit_id": "reviewed-commit",
        }
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(
            monitor_responses(reviews=[review])
        ) as curl:
            result = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
        self.assertEqual(result.returncode, 0, result.stderr)
        facts = json.loads(result.stdout)["facts"]
        review_fact = next(fact for fact in facts if fact["kind"] == "untrusted_review")
        self.assertEqual(review_fact["state"], "CHANGES_REQUESTED")
        self.assertEqual(review_fact["commit_id"], "reviewed-commit")
        self.assertEqual(review_fact["body"], "changes requested")

    def test_monitor_ignores_pending_review_without_submitted_at(self):
        review = {
            "id": 3,
            "submitted_at": None,
            "body": "unfinished draft review",
            "state": "PENDING",
            "commit_id": "reviewed-commit",
        }
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(
            monitor_responses(reviews=[review])
        ) as curl:
            result = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            snapshot = json.loads(
                (Path(state_dir) / "pr-31.json").read_text(encoding="utf-8")
            )
        self.assertFalse(
            any(fact["kind"] == "untrusted_review" for fact in json.loads(result.stdout)["facts"])
        )
        self.assertNotIn(review["body"], result.stdout)
        self.assertEqual(snapshot["reviews"], [])

    def test_monitor_rejects_incomplete_comment_and_review_objects_without_advancing_state(self):
        cases = [
            ("issue body missing", 3, [{"id": 1, "updated_at": "2026-08-21T12:00:00Z"}]),
            ("issue body null", 3, [{"id": 1, "updated_at": "2026-08-21T12:00:00Z", "body": None}]),
            ("issue timestamp null", 3, [{"id": 1, "updated_at": None, "body": "body"}]),
            ("review comment timestamp missing", 4, [{"id": 2, "body": "body"}]),
            ("review body missing", 5, [{"id": 3, "submitted_at": "2026-08-21T12:00:00Z", "state": "COMMENTED", "commit_id": "abc123"}]),
            ("review state missing", 5, [{"id": 3, "submitted_at": "2026-08-21T12:00:00Z", "body": "body", "commit_id": "abc123"}]),
            ("review commit null", 5, [{"id": 3, "submitted_at": "2026-08-21T12:00:00Z", "body": "body", "state": "COMMENTED", "commit_id": None}]),
            ("pending review id invalid", 5, [{"id": None, "submitted_at": None, "body": "body", "state": "PENDING", "commit_id": "abc123"}]),
            ("pending review body missing", 5, [{"id": 3, "submitted_at": None, "state": "PENDING", "commit_id": "abc123"}]),
            ("pending review commit null", 5, [{"id": 3, "submitted_at": None, "body": "body", "state": "PENDING", "commit_id": None}]),
            ("pending review duplicate id", 5, [
                {"id": 3, "submitted_at": None, "body": "first", "state": "PENDING", "commit_id": "abc123"},
                {"id": 3, "submitted_at": None, "body": "second", "state": "PENDING", "commit_id": "abc123"},
            ]),
        ]
        for name, endpoint_index, objects in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
                initial = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
                snapshot = Path(state_dir) / "pr-31.json"
                before = snapshot.read_bytes()
                responses = monitor_responses()
                responses[endpoint_index] = {"body": objects}
                curl.set_responses(responses)
                result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
                self.assertEqual(initial.returncode, 0, initial.stderr)
                self.assertEqual(result.returncode, 1)
                self.assertNotIn("Traceback", result.stderr)
                self.assertEqual(snapshot.read_bytes(), before)
                self.assertEqual(list(Path(state_dir).glob("*.tmp")), [])

    def test_monitor_rejects_empty_normalized_fields_without_advancing_state(self):
        malformed_observations = [
            ("pull request updated time", lambda responses: responses[0]["body"].__setitem__("updated_at", "")),
            ("check name", lambda responses: responses.__setitem__(1, {"body": {"total_count": 1, "check_runs": [{"id": 7, "name": "", "status": "completed", "conclusion": "success"}]}})),
            ("check status", lambda responses: responses.__setitem__(1, {"body": {"total_count": 1, "check_runs": [{"id": 7, "name": "ci", "status": "", "conclusion": "success"}]}})),
            ("check conclusion", lambda responses: responses.__setitem__(1, {"body": {"total_count": 1, "check_runs": [{"id": 7, "name": "ci", "status": "completed", "conclusion": ""}]}})),
            ("check start time", lambda responses: responses.__setitem__(1, {"body": {"total_count": 1, "check_runs": [{"id": 7, "name": "ci", "status": "completed", "conclusion": "success", "started_at": ""}]}})),
            ("status context", lambda responses: responses.__setitem__(2, {"body": [{"id": 8, "context": "", "state": "success"}]})),
            ("status state", lambda responses: responses.__setitem__(2, {"body": [{"id": 8, "context": "legacy-ci", "state": ""}]})),
            ("status created time", lambda responses: responses.__setitem__(2, {"body": [{"id": 8, "context": "legacy-ci", "state": "success", "created_at": ""}]})),
        ]
        for name, mutate in malformed_observations:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as state_dir, FakeCurl(
                monitor_responses()
            ) as curl:
                initial = self.run_utility(
                    "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
                )
                snapshot = Path(state_dir) / "pr-31.json"
                before = snapshot.read_bytes()
                responses = monitor_responses()
                mutate(responses)
                curl.set_responses(responses)
                result = self.run_utility(
                    "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
                )
                self.assertEqual(initial.returncode, 0, initial.stderr)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("Traceback", result.stderr)
                self.assertEqual(snapshot.read_bytes(), before)
                self.assertEqual(list(Path(state_dir).glob("*.tmp")), [])

    def test_monitor_rejects_malformed_nested_snapshot_before_network(self):
        mutations = [
            ("pr null", lambda snapshot: snapshot.__setitem__("pr", None)),
            ("collection object", lambda snapshot: snapshot.__setitem__("check_runs", {})),
            ("collection null member", lambda snapshot: snapshot.__setitem__("statuses", [None])),
            ("comment bad id", lambda snapshot: snapshot.__setitem__("issue_comments", [{"id": "bad", "updated_at": "2026-08-21T12:00:00Z"}])),
            ("review member null", lambda snapshot: snapshot.__setitem__("reviews", [None])),
        ]
        for name, mutate in mutations:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
                initial = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
                snapshot_path = Path(state_dir) / "pr-31.json"
                snapshot = json.loads(snapshot_path.read_text(encoding="utf-8"))
                mutate(snapshot)
                snapshot_path.write_text(json.dumps(snapshot), encoding="utf-8")
                before = snapshot_path.read_bytes()
                curl.records.write_text("", encoding="utf-8")
                curl.set_responses([])
                result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
                self.assertEqual(initial.returncode, 0, initial.stderr)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("Traceback", result.stderr)
                self.assertEqual(snapshot_path.read_bytes(), before)
                self.assert_no_requests(curl)

    def test_monitor_reports_merged_and_closed_without_merge_as_distinct_states(self):
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses(pr_body=pr(merged=True, merge_commit_sha="merge123"))) as curl:
            merged = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
        self.assertEqual(merged.returncode, 0, merged.stderr)
        self.assertIn("merged", merged.stdout.lower())
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses(pr_body=pr(state="closed"))) as curl:
            closed = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
        self.assertEqual(closed.returncode, 0, closed.stderr)
        self.assertIn("closed", closed.stdout.lower())
        self.assertIn("unmerged", closed.stdout.lower())

    def test_monitor_rejects_malformed_http_rate_and_transport_failures_without_advancing_state(self):
        failures = [
            ("malformed", {"body": "{"}),
            ("401", {"body": {"message": "Bad credentials"}, "exit": 22}),
            ("403", {"body": {"message": "Forbidden"}, "exit": 22}),
            ("rate limited", {"body": {"message": "API rate limit exceeded"}, "exit": 22}),
            ("transport", {"body": "network down", "exit": 7}),
        ]
        for name, failure in failures:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
                initial = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
                snapshot = Path(state_dir) / "pr-31.json"
                before = snapshot.read_bytes()
                curl.set_responses([failure])
                result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
                self.assertEqual(initial.returncode, 0, initial.stderr)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(snapshot.read_bytes(), before)
                self.assert_secret_free(result)

    def test_monitor_does_not_publish_partial_observation_or_temp_snapshot(self):
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
            initial = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            snapshot = Path(state_dir) / "pr-31.json"
            before = snapshot.read_bytes()
            partial = monitor_responses()
            partial[3] = {"body": {"message": "Forbidden"}, "exit": 22}
            curl.set_responses(partial)
            result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            leftovers = list(Path(state_dir).glob("*.tmp"))
            self.assertEqual(initial.returncode, 0, initial.stderr)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(snapshot.read_bytes(), before)
            self.assertEqual(leftovers, [])

    def test_monitor_paginates_check_runs_statuses_and_each_comment_collection(self):
        check_runs = [{"id": i, "name": f"check-{i}", "status": "completed", "conclusion": "success"} for i in range(101)]
        statuses = [{"id": i, "context": f"status-{i}", "state": "success"} for i in range(101)]
        issue_comments = [{"id": i, "updated_at": "2026-08-21T12:00:00Z", "body": f"issue-{i}"} for i in range(101)]
        review_comments = [{"id": i, "updated_at": "2026-08-21T12:00:00Z", "body": f"review-comment-{i}"} for i in range(101)]
        reviews = [{"id": i, "submitted_at": "2026-08-21T12:00:00Z", "body": f"review-{i}", "state": "COMMENTED", "commit_id": "abc123"} for i in range(101)]
        responses = [
            {"body": pr()},
            {"body": {"total_count": 101, "check_runs": check_runs[:100]}},
            {"body": {"total_count": 101, "check_runs": check_runs[100:]}},
            {"body": statuses[:100]},
            {"body": statuses[100:]},
            {"body": issue_comments[:100]},
            {"body": issue_comments[100:]},
            {"body": review_comments[:100]},
            {"body": review_comments[100:]},
            {"body": reviews[:100]},
            {"body": reviews[100:]},
        ]
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(responses) as curl:
            result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        urls = [record["url"] for record in records]
        for endpoint in ("/check-runs", "/statuses", "/issues/31/comments", "/pulls/31/comments", "/pulls/31/reviews"):
            with self.subTest(endpoint=endpoint):
                self.assertTrue(any(endpoint in url and "page=2" in url for url in urls), urls)

    def test_monitor_rejects_oversized_multi_page_collection_without_advancing_state(self):
        large_name = "x" * (50 * 1024)
        check_runs = [
            {"id": item_id, "name": large_name, "status": "completed", "conclusion": "success"}
            for item_id in range(180)
        ]
        oversized = [
            {"body": pr()},
            {"body": {"total_count": 180, "check_runs": check_runs[:100]}},
            {"body": {"total_count": 180, "check_runs": check_runs[100:]}},
            {"body": []},
            {"body": []},
            {"body": []},
            {"body": []},
        ]
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
            initial = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
            snapshot = Path(state_dir) / "pr-31.json"
            before = snapshot.read_bytes()
            curl.records.write_text("", encoding="utf-8")
            curl.set_responses(oversized)
            result = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
            records = curl.records_json()
            after = snapshot.read_bytes()
            leftovers = list(Path(state_dir).glob("*.tmp"))
        self.assertEqual(initial.returncode, 0, initial.stderr)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("Traceback", result.stderr)
        self.assertTrue(any("/check-runs" in record["url"] and "page=2" in record["url"] for record in records))
        self.assertFalse(any("/statuses" in record["url"] for record in records))
        self.assertEqual(after, before)
        self.assertEqual(leftovers, [])

    def test_monitor_rejects_oversized_encoded_snapshot_without_advancing_state(self):
        large_name = "x" * (50 * 1024)
        checks = [
            {"id": item_id, "name": large_name, "status": "completed", "conclusion": "success"}
            for item_id in range(85)
        ]
        statuses = [
            {"id": item_id, "context": large_name, "state": "success"}
            for item_id in range(85)
        ]
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
            initial = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
            snapshot = Path(state_dir) / "pr-31.json"
            before = snapshot.read_bytes()
            curl.set_responses(monitor_responses(checks=checks, statuses=statuses))
            result = self.run_utility(
                "monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl
            )
            after = snapshot.read_bytes()
            leftovers = list(Path(state_dir).glob("*.tmp"))
        self.assertEqual(initial.returncode, 0, initial.stderr)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("Traceback", result.stderr)
        self.assertEqual(after, before)
        self.assertEqual(leftovers, [])


if __name__ == "__main__":
    unittest.main(verbosity=2)
