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
try:
    config = args[args.index("--config") + 1]
except (ValueError, IndexError) as exc:
    raise SystemExit("curl config descriptor was not supplied") from exc
if not config.startswith("/dev/fd/"):
    raise SystemExit("curl config is not an inherited descriptor")
config_text = Path(config).read_text(encoding="utf-8")
if "Authorization: Bearer " + secret not in config_text:
    raise SystemExit("authorization header was not supplied through config")

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
payload = response.get("body", {})
if isinstance(payload, str):
    sys.stdout.write(payload)
else:
    sys.stdout.write(json.dumps(payload))
raise SystemExit(response.get("exit", 0))
'''


class FakeCurl:
    def __init__(self, responses):
        self.responses = responses
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name)
        self.binary = self.path / "curl"
        self.queue = self.path / "queue.json"
        self.records = self.path / "records.jsonl"

    def __enter__(self):
        self.binary.write_text(textwrap.dedent(FAKE_CURL), encoding="utf-8")
        self.binary.chmod(0o700)
        self.set_responses(self.responses)
        return self

    def __exit__(self, *_):
        self.temp.cleanup()

    def set_responses(self, responses):
        self.queue.write_text(json.dumps(responses), encoding="utf-8")

    def records_json(self):
        if not self.records.exists():
            return []
        return [json.loads(line) for line in self.records.read_text(encoding="utf-8").splitlines()]

    def env(self, expected_token=SECRET):
        return {
            "TARIBOY_GITHUB_CURL_BIN": str(self.binary),
            "FAKE_CURL_QUEUE": str(self.queue),
            "FAKE_CURL_RECORDS": str(self.records),
            "FAKE_CURL_EXPECTED_TOKEN": expected_token,
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

    def run_utility(self, *args, curl, gh_token=SECRET, github_token=None):
        env = os.environ.copy()
        env.pop("GH_TOKEN", None)
        env.pop("GITHUB_TOKEN", None)
        expected_token = gh_token if gh_token is not None else (github_token if github_token is not None else SECRET)
        env.update(curl.env(expected_token))
        if gh_token is not None:
            env["GH_TOKEN"] = gh_token
        if github_token is not None:
            env["GITHUB_TOKEN"] = github_token
        return subprocess.run(
            [sys.executable, str(UTILITY), *args],
            cwd=SKILL_DIR,
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )

    def assert_secret_free(self, result):
        self.assertNotIn(SECRET, result.stdout)
        self.assertNotIn(SECRET, result.stderr)

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

    def test_ensure_returns_one_existing_open_pull_request(self):
        with FakeCurl([{"body": [pr()]}]) as curl:
            result = self.run_utility("ensure", "--repo", REPO, "--head", HEAD, "--base", BASE, "--title", "TARI-31", curl=curl)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"created": False, "number": 31, "state": "open", "url": pr()["html_url"]})
        self.assertEqual([record["method"] for record in records], ["GET"])

    def test_ensure_creates_then_reconciles_one_pull_request(self):
        with FakeCurl([{"body": []}, {"body": pr()}, {"body": [pr()]}]) as curl:
            result = self.run_utility("ensure", "--repo", REPO, "--head", HEAD, "--base", BASE, "--title", "TARI-31", curl=curl)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"created": True, "number": 31, "state": "open", "url": pr()["html_url"]})
        self.assertEqual([record["method"] for record in records], ["GET", "POST", "GET"])
        self.assertEqual(records[1]["json"], {"base": BASE, "head": HEAD, "title": "TARI-31"})

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

    def test_monitor_new_head_invalidates_prior_snapshot(self):
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses()) as curl:
            self.assertEqual(self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl).returncode, 0)
            curl.set_responses(monitor_responses(pr_body=pr(head="new-head")))
            result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            state = (Path(state_dir) / "pr-31.json").read_text(encoding="utf-8")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("new-head", state)

    def test_monitor_emits_untrusted_new_comment_and_review_bodies_without_persisting_them(self):
        comments = [{"id": 1, "updated_at": "2026-08-21T12:00:00Z", "body": "comment body"}]
        review_comments = [{"id": 2, "updated_at": "2026-08-21T12:00:00Z", "body": "review comment body"}]
        reviews = [{"id": 3, "submitted_at": "2026-08-21T12:00:00Z", "body": "review body", "state": "COMMENTED"}]
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(monitor_responses(comments=comments, review_comments=review_comments, reviews=reviews)) as curl:
            result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            state = (Path(state_dir) / "pr-31.json").read_text(encoding="utf-8")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("untrusted", result.stdout.lower())
        self.assertIn("comment body", result.stdout)
        for body in ("comment body", "review comment body", "review body"):
            self.assertNotIn(body, state)

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

    def test_monitor_paginates_all_collections(self):
        first_page = [{"id": i, "updated_at": "2026-08-21T12:00:00Z", "body": "body"} for i in range(100)]
        second_page = [{"id": 100, "updated_at": "2026-08-22T12:00:00Z", "body": "tail"}]
        responses = [
            {"body": pr()},
            {"body": {"total_count": 0, "check_runs": []}},
            {"body": []},
            {"body": first_page},
            {"body": second_page},
            {"body": []},
            {"body": []},
        ]
        with tempfile.TemporaryDirectory() as state_dir, FakeCurl(responses) as curl:
            result = self.run_utility("monitor", "--repo", REPO, "--pr", "31", "--state-dir", state_dir, curl=curl)
            records = curl.records_json()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(any("page=2" in record["url"] for record in records), records)


if __name__ == "__main__":
    unittest.main(verbosity=2)
