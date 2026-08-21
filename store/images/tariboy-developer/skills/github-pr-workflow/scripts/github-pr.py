#!/usr/bin/env python3
"""Safe, dependency-free GitHub pull-request operations for Tariboy agents."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import selectors
import shutil
import stat
import subprocess
import sys
import tempfile
import time
from typing import Any, Callable, NoReturn
from urllib.parse import parse_qsl, urlencode, urlparse, urlunparse


API_BASE = "https://api.github.com"
MAX_RESPONSE_BYTES = 8 * 1024 * 1024
MAX_DIAGNOSTIC_BYTES = 16 * 1024
COMPONENT_RE = re.compile(r"^[A-Za-z0-9_.-]+$")
CONTROL_RE = re.compile(r"[\x00-\x1f\x7f]")
TOKEN_FOR_REDACTION = ""


class WorkflowError(Exception):
    """An actionable, already-safe utility failure."""


class SafeArgumentParser(argparse.ArgumentParser):
    """Keep invalid user input out of diagnostics."""

    def error(self, message: str) -> NoReturn:
        raise WorkflowError("invalid command arguments")


def fail(message: str) -> NoReturn:
    raise WorkflowError(message)


def redact_text(value: str) -> str:
    if TOKEN_FOR_REDACTION:
        return value.replace(TOKEN_FOR_REDACTION, "[REDACTED]")
    return value


def redact_value(value: Any) -> Any:
    if isinstance(value, str):
        return redact_text(value)
    if isinstance(value, list):
        return [redact_value(item) for item in value]
    if isinstance(value, dict):
        return {key: redact_value(item) for key, item in value.items()}
    return value


def emit_json(value: Any) -> None:
    encoded = json.dumps(
        redact_value(value), sort_keys=True, separators=(",", ":"), ensure_ascii=False
    )
    sys.stdout.write(encoded + "\n")


def validate_component(value: str, label: str) -> str:
    if not value or not COMPONENT_RE.fullmatch(value) or value in {".", ".."}:
        fail(f"invalid {label}")
    return value


def validate_repo(value: str) -> tuple[str, str]:
    if value.count("/") != 1:
        fail("repository must use OWNER/REPO")
    owner, name = value.split("/", 1)
    return validate_component(owner, "repository owner"), validate_component(
        name, "repository name"
    )


def repo_from_origin() -> str:
    try:
        result = subprocess.run(
            ["git", "config", "--get", "remote.origin.url"],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            timeout=5,
            check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise WorkflowError("cannot resolve the configured origin remote") from exc
    if result.returncode != 0 or len(result.stdout) > 4096:
        fail("repository is required and origin is unavailable")
    remote = result.stdout.strip()
    if CONTROL_RE.search(remote):
        fail("origin remote contains invalid characters")

    candidate = ""
    if remote.startswith("git@github.com:"):
        candidate = remote[len("git@github.com:") :]
    else:
        parsed = urlparse(remote)
        if parsed.scheme not in {"https", "ssh"} or parsed.hostname != "github.com":
            fail("origin remote is not a supported GitHub URL")
        candidate = parsed.path.lstrip("/")
    if candidate.endswith(".git"):
        candidate = candidate[:-4]
    validate_repo(candidate)
    return candidate


def resolve_repo(explicit: str | None) -> tuple[str, str]:
    value = explicit or os.environ.get("GITHUB_REPOSITORY") or repo_from_origin()
    return validate_repo(value)


def validate_branch(value: str, label: str) -> str:
    if not value or CONTROL_RE.search(value) or len(value.encode("utf-8")) > 1024:
        fail(f"invalid {label} branch")
    try:
        result = subprocess.run(
            ["git", "check-ref-format", "--branch", value],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=5,
            check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise WorkflowError("git is required to validate branch names") from exc
    if result.returncode != 0:
        fail(f"invalid {label} branch")
    return value


def validate_pr_number(value: str) -> int:
    if not value.isascii() or not value.isdigit():
        fail("pull request number must be a positive integer")
    number = int(value)
    if number <= 0:
        fail("pull request number must be a positive integer")
    return number


def validate_state_dir(value: str) -> Path:
    path = Path(value)
    if not path.is_absolute():
        fail("state directory must be absolute")
    try:
        info = path.lstat()
    except OSError as exc:
        raise WorkflowError("state directory is unavailable") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
        fail("state directory must be a non-symlink directory")
    if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) & 0o077:
        fail("state directory must be owner-only")
    return path


def select_token() -> str:
    global TOKEN_FOR_REDACTION
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not token:
        fail("GH_TOKEN or GITHUB_TOKEN is required")
    TOKEN_FOR_REDACTION = token
    if CONTROL_RE.search(token) or len(token.encode("utf-8")) > 8192:
        fail("GitHub token contains invalid characters")
    return token


def resolve_curl() -> str:
    override = os.environ.get("TARIBOY_GITHUB_CURL_BIN")
    if override:
        if CONTROL_RE.search(override) or not os.path.isabs(override):
            fail("TARIBOY_GITHUB_CURL_BIN must be an absolute executable path")
        path = Path(override)
        try:
            info = path.lstat()
        except OSError as exc:
            raise WorkflowError("configured curl executable is unavailable") from exc
        if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
            fail("configured curl executable must be a non-symlink regular file")
        if not os.access(path, os.X_OK):
            fail("configured curl executable is not executable")
        if TOKEN_FOR_REDACTION and TOKEN_FOR_REDACTION in str(path):
            fail("configured curl path contains credential material")
        return str(path)
    curl = shutil.which("curl")
    if not curl:
        fail("curl is required")
    return curl


def append_query(url: str, params: dict[str, Any]) -> str:
    parsed = urlparse(url)
    query = parse_qsl(parsed.query, keep_blank_values=True)
    query.extend((key, str(value)) for key, value in params.items())
    return urlunparse(parsed._replace(query=urlencode(query)))


def stop_process(process: subprocess.Popen[bytes]) -> None:
    if process.poll() is None:
        process.kill()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired as exc:
        raise WorkflowError("failed to reap curl after termination") from exc


def bounded_process_output(process: subprocess.Popen[bytes]) -> tuple[bytes, bytes]:
    if process.stdout is None or process.stderr is None:
        fail("curl output pipes are unavailable")
    streams = {
        process.stdout.fileno(): ("response", process.stdout, MAX_RESPONSE_BYTES),
        process.stderr.fileno(): ("diagnostic", process.stderr, MAX_DIAGNOSTIC_BYTES),
    }
    buffers = {"response": bytearray(), "diagnostic": bytearray()}
    selector = selectors.DefaultSelector()
    deadline = time.monotonic() + 35
    try:
        for descriptor in streams:
            os.set_blocking(descriptor, False)
            selector.register(descriptor, selectors.EVENT_READ)
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                stop_process(process)
                fail("GitHub request exceeded its time limit")
            events = selector.select(remaining)
            if not events:
                stop_process(process)
                fail("GitHub request exceeded its time limit")
            for key, _ in events:
                descriptor = key.fd
                try:
                    chunk = os.read(descriptor, 65536)
                except BlockingIOError:
                    continue
                if not chunk:
                    selector.unregister(descriptor)
                    continue
                label, _, limit = streams[descriptor]
                buffers[label].extend(chunk)
                if len(buffers[label]) > limit:
                    stop_process(process)
                    fail(f"GitHub {label} exceeded its size limit")
        remaining = max(0.0, deadline - time.monotonic())
        try:
            process.wait(timeout=remaining)
        except subprocess.TimeoutExpired:
            stop_process(process)
            fail("GitHub request exceeded its time limit")
        return bytes(buffers["response"]), bytes(buffers["diagnostic"])
    finally:
        selector.close()
        process.stdout.close()
        process.stderr.close()


class GitHubClient:
    def __init__(self, token: str, curl_bin: str):
        self.token = token
        self.curl_bin = curl_bin

    def request(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        body: dict[str, Any] | None = None,
    ) -> Any:
        if not path.startswith("/") or "//" in path or CONTROL_RE.search(path):
            fail("invalid GitHub API path")
        url = API_BASE + path
        if params:
            url = append_query(url, params)
        if self.token in url:
            fail("refusing to place the GitHub token in a URL")

        args = [
            self.curl_bin,
            "--disable",
            "--silent",
            "--show-error",
            "--fail-with-body",
            "--connect-timeout",
            "10",
            "--max-time",
            "30",
            "--max-filesize",
            str(MAX_RESPONSE_BYTES),
            "--config",
            "CONFIG_FD",
            "--header",
            "Accept: application/vnd.github+json",
            "--header",
            "X-GitHub-Api-Version: 2022-11-28",
        ]
        if method != "GET":
            args.extend(["--request", method])
        if body is not None:
            encoded_body = json.dumps(body, separators=(",", ":"), ensure_ascii=False)
            if self.token in encoded_body:
                fail("refusing to place the GitHub token in request arguments")
            args.extend(["--data-binary", encoded_body])
        args.append(url)

        read_fd, write_fd = os.pipe()
        args[args.index("CONFIG_FD")] = f"/dev/fd/{read_fd}"
        escaped = self.token.replace("\\", "\\\\").replace('"', '\\"')
        config = f'header = "Authorization: Bearer {escaped}"\n'.encode("utf-8")
        env = os.environ.copy()
        env.pop("GH_TOKEN", None)
        env.pop("GITHUB_TOKEN", None)
        process: subprocess.Popen[bytes] | None = None
        try:
            process = subprocess.Popen(
                args,
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                env=env,
                pass_fds=(read_fd,),
            )
            os.close(read_fd)
            read_fd = -1
            view = memoryview(config)
            while view:
                written = os.write(write_fd, view)
                view = view[written:]
            os.close(write_fd)
            write_fd = -1
            stdout, stderr = bounded_process_output(process)
        except OSError as exc:
            if process is not None:
                stop_process(process)
            raise WorkflowError("failed to execute curl") from exc
        finally:
            if read_fd >= 0:
                os.close(read_fd)
            if write_fd >= 0:
                os.close(write_fd)

        if process.returncode != 0:
            fail(f"GitHub request failed (curl exit {process.returncode})")
        try:
            return json.loads(stdout.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise WorkflowError("GitHub returned malformed JSON") from exc

    def paginated_array(self, path: str, *, params: dict[str, Any] | None = None) -> list[Any]:
        items: list[Any] = []
        page = 1
        while True:
            page_params = dict(params or {})
            page_params.update({"per_page": 100, "page": page})
            response = self.request("GET", path, params=page_params)
            if not isinstance(response, list):
                fail("GitHub returned an unexpected array response")
            items.extend(response)
            if len(response) < 100:
                return items
            page += 1

    def paginated_check_runs(self, path: str) -> list[Any]:
        items: list[Any] = []
        page = 1
        while True:
            response = self.request(
                "GET", path, params={"per_page": 100, "page": page}
            )
            if not isinstance(response, dict):
                fail("GitHub returned an unexpected check-runs response")
            total = response.get("total_count")
            current = response.get("check_runs")
            if not isinstance(total, int) or total < 0 or not isinstance(current, list):
                fail("GitHub returned an incomplete check-runs response")
            items.extend(current)
            if len(current) < 100:
                return items
            page += 1


def require_dict(value: Any, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        fail(f"GitHub returned an unexpected {label} response")
    return value


def integer_field(value: dict[str, Any], key: str, label: str) -> int:
    item = value.get(key)
    if isinstance(item, bool) or not isinstance(item, int) or item < 0:
        fail(f"GitHub {label} has an invalid {key}")
    return item


def text_field(
    value: dict[str, Any], key: str, label: str, *, nullable: bool = False
) -> str | None:
    item = value.get(key)
    if nullable and item is None:
        return None
    if not isinstance(item, str) or CONTROL_RE.search(item) or len(item) > MAX_RESPONSE_BYTES:
        fail(f"GitHub {label} has an invalid {key}")
    return redact_text(item)


def nonempty_text_field(value: dict[str, Any], key: str, label: str) -> str:
    item = text_field(value, key, label)
    if not item:
        fail(f"GitHub {label} has an invalid {key}")
    return item


def body_field(value: dict[str, Any], key: str, label: str) -> str:
    if key not in value:
        fail(f"GitHub {label} is missing {key}")
    item = value[key]
    if not isinstance(item, str) or "\x00" in item or len(item) > MAX_RESPONSE_BYTES:
        fail(f"GitHub {label} has an invalid {key}")
    return redact_text(item)


def validate_pr_result(value: Any) -> dict[str, Any]:
    item = require_dict(value, "pull request")
    number = integer_field(item, "number", "pull request")
    if number <= 0:
        fail("GitHub pull request has an invalid number")
    state = text_field(item, "state", "pull request")
    if state not in {"open", "closed"}:
        fail("GitHub pull request has an invalid state")
    url = text_field(item, "html_url", "pull request")
    parsed = urlparse(url or "")
    if parsed.scheme != "https" or parsed.hostname != "github.com":
        fail("GitHub pull request has an invalid URL")
    return {"number": number, "state": state, "url": url}


def canonical_pull(items: list[Any]) -> dict[str, Any] | None:
    pulls = [validate_pr_result(item) for item in items]
    open_pulls = [item for item in pulls if item["state"] == "open"]
    closed_pulls = [item for item in pulls if item["state"] == "closed"]
    if len(open_pulls) > 1:
        fail("multiple open pull requests match the requested head and base")
    if closed_pulls:
        fail("a closed pull request matches this head and base; human decision required")
    return open_pulls[0] if open_pulls else None


def pull_query(client: GitHubClient, owner: str, repo: str, head: str, base: str) -> list[Any]:
    return client.paginated_array(
        f"/repos/{owner}/{repo}/pulls",
        params={"state": "all", "head": f"{owner}:{head}", "base": base},
    )


def command_preflight(args: argparse.Namespace, client: GitHubClient) -> int:
    owner, repo = args.resolved_repo
    response = require_dict(client.request("GET", f"/repos/{owner}/{repo}"), "repository")
    full_name = text_field(response, "full_name", "repository")
    if (full_name or "").casefold() != f"{owner}/{repo}".casefold():
        fail("authenticated repository response does not match the requested repository")
    emit_json({"ok": True, "repo": full_name})
    return 0


def command_ensure(args: argparse.Namespace, client: GitHubClient) -> int:
    owner, repo = args.resolved_repo
    existing = canonical_pull(pull_query(client, owner, repo, args.head, args.base))
    if existing is not None:
        emit_json({"created": False, **existing})
        return 0

    create_body: dict[str, Any] = {
        "base": args.base,
        "head": args.head,
        "title": args.title,
    }
    if args.body is not None:
        create_body["body"] = args.body
    create_error: WorkflowError | None = None
    created = False
    try:
        validate_pr_result(
            client.request("POST", f"/repos/{owner}/{repo}/pulls", body=create_body)
        )
        created = True
    except WorkflowError as exc:
        create_error = exc

    try:
        reconciled = canonical_pull(pull_query(client, owner, repo, args.head, args.base))
    except WorkflowError:
        if create_error is not None:
            raise create_error
        raise
    if reconciled is None:
        if create_error is not None:
            raise create_error
        fail("pull request creation did not reconcile to one open pull request")
    emit_json({"created": created, **reconciled})
    return 0


def normalize_pr(value: Any, expected_number: int) -> dict[str, Any]:
    item = require_dict(value, "pull request")
    summary = validate_pr_result(item)
    if summary["number"] != expected_number:
        fail("GitHub returned the wrong pull request number")
    merged = item.get("merged")
    if not isinstance(merged, bool):
        fail("GitHub pull request has an invalid merged flag")
    merge_sha = text_field(item, "merge_commit_sha", "pull request", nullable=True)
    if merged and not merge_sha:
        fail("merged pull request is missing merge commit metadata")
    updated_at = text_field(item, "updated_at", "pull request")
    head = require_dict(item.get("head"), "pull request head")
    head_sha = text_field(head, "sha", "pull request head")
    if not head_sha:
        fail("GitHub pull request is missing its head SHA")
    return {
        "number": summary["number"],
        "state": summary["state"],
        "merged": merged,
        "merge_commit_sha": merge_sha,
        "updated_at": updated_at,
        "head_sha": head_sha,
    }


def normalize_checks(values: list[Any]) -> list[dict[str, Any]]:
    result = []
    for value in values:
        item = require_dict(value, "check run")
        result.append(
            {
                "id": integer_field(item, "id", "check run"),
                "name": text_field(item, "name", "check run"),
                "status": text_field(item, "status", "check run"),
                "conclusion": text_field(item, "conclusion", "check run", nullable=True),
                "started_at": text_field(item, "started_at", "check run", nullable=True),
                "completed_at": text_field(item, "completed_at", "check run", nullable=True),
            }
        )
    return sorted(result, key=lambda item: (item["name"] or "", item["id"]))


def normalize_statuses(values: list[Any]) -> list[dict[str, Any]]:
    result = []
    for value in values:
        item = require_dict(value, "commit status")
        result.append(
            {
                "id": integer_field(item, "id", "commit status"),
                "context": text_field(item, "context", "commit status"),
                "state": text_field(item, "state", "commit status"),
                "created_at": text_field(item, "created_at", "commit status", nullable=True),
                "updated_at": text_field(item, "updated_at", "commit status", nullable=True),
            }
        )
    return sorted(result, key=lambda item: (item["context"] or "", item["id"]))


def normalize_body_items(
    values: list[Any], kind: str, timestamp_key: str
) -> tuple[list[dict[str, Any]], dict[int, str]]:
    metadata = []
    bodies: dict[int, str] = {}
    for value in values:
        item = require_dict(value, kind)
        object_id = integer_field(item, "id", kind)
        if object_id in bodies:
            fail(f"GitHub returned duplicate {kind} IDs")
        timestamp = nonempty_text_field(item, timestamp_key, kind)
        body = body_field(item, "body", kind)
        entry: dict[str, Any] = {"id": object_id, timestamp_key: timestamp}
        if kind == "review":
            entry["state"] = nonempty_text_field(item, "state", kind)
            entry["commit_id"] = nonempty_text_field(item, "commit_id", kind)
        metadata.append(entry)
        bodies[object_id] = body
    return sorted(metadata, key=lambda item: item["id"]), bodies


def snapshot_dict(value: Any, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        fail(f"monitor snapshot has an invalid {label}")
    return value


def snapshot_integer(value: dict[str, Any], key: str, label: str) -> int:
    item = value.get(key)
    if isinstance(item, bool) or not isinstance(item, int) or item < 0:
        fail(f"monitor snapshot {label} has an invalid {key}")
    return item


def snapshot_text(
    value: dict[str, Any],
    key: str,
    label: str,
    *,
    nullable: bool = False,
    nonempty: bool = True,
) -> str | None:
    if key not in value:
        fail(f"monitor snapshot {label} is missing {key}")
    item = value[key]
    if nullable and item is None:
        return None
    if not isinstance(item, str) or CONTROL_RE.search(item):
        fail(f"monitor snapshot {label} has an invalid {key}")
    if nonempty and not item:
        fail(f"monitor snapshot {label} has an invalid {key}")
    return item


def snapshot_collection(
    snapshot: dict[str, Any],
    key: str,
    validate: Callable[[dict[str, Any]], None],
) -> None:
    values = snapshot.get(key)
    if not isinstance(values, list):
        fail(f"monitor snapshot has an invalid {key} collection")
    seen: set[int] = set()
    for index, value in enumerate(values):
        item = snapshot_dict(value, f"{key}[{index}]")
        validate(item)
        item_id = item["id"]
        if item_id in seen:
            fail(f"monitor snapshot has duplicate {key} IDs")
        seen.add(item_id)


def validate_snapshot(snapshot: dict[str, Any], repo: str, number: int) -> None:
    if (
        snapshot.get("version") != 1
        or snapshot.get("repo") != repo
        or snapshot.get("pr_number") != number
    ):
        fail("monitor snapshot does not match this repository and pull request")

    pr = snapshot_dict(snapshot.get("pr"), "pr metadata")
    if snapshot_integer(pr, "number", "pr metadata") != number:
        fail("monitor snapshot has the wrong pull request number")
    state = snapshot_text(pr, "state", "pr metadata")
    if state not in {"open", "closed"}:
        fail("monitor snapshot pr metadata has an invalid state")
    merged = pr.get("merged")
    if not isinstance(merged, bool):
        fail("monitor snapshot pr metadata has an invalid merged flag")
    merge_sha = snapshot_text(
        pr, "merge_commit_sha", "pr metadata", nullable=True
    )
    if merged and not merge_sha:
        fail("monitor snapshot merged PR is missing merge commit metadata")
    snapshot_text(pr, "updated_at", "pr metadata")
    snapshot_text(pr, "head_sha", "pr metadata")

    def validate_check(item: dict[str, Any]) -> None:
        snapshot_integer(item, "id", "check run")
        snapshot_text(item, "name", "check run")
        snapshot_text(item, "status", "check run")
        snapshot_text(item, "conclusion", "check run", nullable=True)
        snapshot_text(item, "started_at", "check run", nullable=True)
        snapshot_text(item, "completed_at", "check run", nullable=True)

    def validate_status(item: dict[str, Any]) -> None:
        snapshot_integer(item, "id", "commit status")
        snapshot_text(item, "context", "commit status")
        snapshot_text(item, "state", "commit status")
        snapshot_text(item, "created_at", "commit status", nullable=True)
        snapshot_text(item, "updated_at", "commit status", nullable=True)

    def validate_comment(timestamp_key: str) -> Callable[[dict[str, Any]], None]:
        def validate(item: dict[str, Any]) -> None:
            snapshot_integer(item, "id", "comment metadata")
            snapshot_text(item, timestamp_key, "comment metadata")

        return validate

    def validate_review(item: dict[str, Any]) -> None:
        snapshot_integer(item, "id", "review metadata")
        snapshot_text(item, "submitted_at", "review metadata")
        snapshot_text(item, "state", "review metadata")
        snapshot_text(item, "commit_id", "review metadata")

    snapshot_collection(snapshot, "check_runs", validate_check)
    snapshot_collection(snapshot, "statuses", validate_status)
    snapshot_collection(snapshot, "issue_comments", validate_comment("updated_at"))
    snapshot_collection(snapshot, "review_comments", validate_comment("updated_at"))
    snapshot_collection(snapshot, "reviews", validate_review)


def load_snapshot(path: Path, repo: str, number: int) -> dict[str, Any] | None:
    if not path.exists():
        return None
    try:
        info = path.lstat()
        if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
            fail("monitor snapshot must be a regular non-symlink file")
        if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) & 0o077:
            fail("monitor snapshot must be owner-only")
        if info.st_size > MAX_RESPONSE_BYTES:
            fail("monitor snapshot exceeds its size limit")
        value = json.loads(path.read_text(encoding="utf-8"))
    except WorkflowError:
        raise
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise WorkflowError("monitor snapshot is unreadable or malformed") from exc
    if not isinstance(value, dict):
        fail("monitor snapshot has an unexpected shape")
    validate_snapshot(value, repo, number)
    return value


def atomic_write(path: Path, value: dict[str, Any]) -> None:
    payload = (
        json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n"
    ).encode("utf-8")
    descriptor = -1
    temporary = ""
    try:
        descriptor, temporary = tempfile.mkstemp(
            prefix=f".{path.name}.", suffix=".tmp", dir=path.parent
        )
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "wb") as output:
            descriptor = -1
            output.write(payload)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
        temporary = ""
        flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0)
        directory_fd = os.open(path.parent, flags)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    except OSError as exc:
        raise WorkflowError("failed to atomically update monitor state") from exc
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        if temporary:
            try:
                os.unlink(temporary)
            except FileNotFoundError:
                pass


def item_map(snapshot: dict[str, Any] | None, key: str) -> dict[int, dict[str, Any]]:
    if snapshot is None:
        return {}
    values = snapshot.get(key)
    if not isinstance(values, list):
        fail("monitor snapshot has an unexpected shape")
    result = {}
    for value in values:
        if not isinstance(value, dict) or not isinstance(value.get("id"), int):
            fail("monitor snapshot has an unexpected shape")
        result[value["id"]] = value
    return result


def changed_untrusted_facts(
    old: dict[str, Any] | None,
    key: str,
    metadata: list[dict[str, Any]],
    bodies: dict[int, str],
    label: str,
) -> list[dict[str, Any]]:
    previous = item_map(old, key)
    facts = []
    for item in metadata:
        old_item = previous.get(item["id"])
        if old_item == item:
            continue
        facts.append(
            {
                "kind": label,
                "id": item["id"],
                "change": "new" if old_item is None else "updated",
                "untrusted": True,
                "body": bodies[item["id"]],
            }
        )
    return facts


def observation_facts(
    old: dict[str, Any] | None,
    snapshot: dict[str, Any],
    body_sets: list[tuple[str, dict[int, str], str]],
) -> list[dict[str, Any]]:
    current_pr = snapshot["pr"]
    old_pr = old.get("pr") if old else None
    facts: list[dict[str, Any]] = []

    if old is None or old_pr.get("state") != current_pr["state"] or old_pr.get("merged") != current_pr["merged"]:
        if current_pr["merged"]:
            facts.append(
                {
                    "kind": "pr_state",
                    "state": "merged",
                    "message": f"Pull request #{current_pr['number']} merged as {current_pr['merge_commit_sha']}.",
                }
            )
        elif current_pr["state"] == "closed":
            facts.append(
                {
                    "kind": "pr_state",
                    "state": "closed-unmerged",
                    "message": f"Pull request #{current_pr['number']} is closed-unmerged.",
                }
            )
        else:
            facts.append(
                {
                    "kind": "pr_state",
                    "state": "open",
                    "message": f"Pull request #{current_pr['number']} is open.",
                }
            )

    old_head = old_pr.get("head_sha") if old_pr else None
    if old_head != current_pr["head_sha"]:
        if old_head is None:
            message = f"Observed pull request head {current_pr['head_sha']}."
        else:
            message = (
                f"Pull request head changed from {old_head} to {current_pr['head_sha']}; "
                "prior check results are invalidated and replaced."
            )
        facts.append({"kind": "head", "head_sha": current_pr["head_sha"], "message": message})

    for key, label in (("check_runs", "check runs"), ("statuses", "commit statuses")):
        previous = old.get(key) if old else None
        if previous != snapshot[key]:
            facts.append(
                {
                    "kind": key,
                    "message": f"Observed changed {label} for head {current_pr['head_sha']}.",
                }
            )

    for key, bodies, label in body_sets:
        facts.extend(changed_untrusted_facts(old, key, snapshot[key], bodies, label))

    if old is not None and old != snapshot and not facts:
        facts.append({"kind": "pr_metadata", "message": "Pull request metadata changed."})
    return facts


def command_monitor(args: argparse.Namespace, client: GitHubClient) -> int:
    owner, repo = args.resolved_repo
    repository = f"{owner}/{repo}"
    snapshot_path = args.state_dir / f"pr-{args.pr}.json"
    old = load_snapshot(snapshot_path, repository, args.pr)

    pr_data = normalize_pr(
        client.request("GET", f"/repos/{owner}/{repo}/pulls/{args.pr}"), args.pr
    )
    head_path = urlencode({"sha": pr_data["head_sha"]})[4:]
    checks = normalize_checks(
        client.paginated_check_runs(f"/repos/{owner}/{repo}/commits/{head_path}/check-runs")
    )
    statuses = normalize_statuses(
        client.paginated_array(f"/repos/{owner}/{repo}/commits/{head_path}/statuses")
    )
    issue_meta, issue_bodies = normalize_body_items(
        client.paginated_array(f"/repos/{owner}/{repo}/issues/{args.pr}/comments"),
        "issue comment",
        "updated_at",
    )
    review_comment_meta, review_comment_bodies = normalize_body_items(
        client.paginated_array(f"/repos/{owner}/{repo}/pulls/{args.pr}/comments"),
        "review comment",
        "updated_at",
    )
    review_meta, review_bodies = normalize_body_items(
        client.paginated_array(f"/repos/{owner}/{repo}/pulls/{args.pr}/reviews"),
        "review",
        "submitted_at",
    )

    snapshot = {
        "version": 1,
        "repo": repository,
        "pr_number": args.pr,
        "pr": pr_data,
        "check_runs": checks,
        "statuses": statuses,
        "issue_comments": issue_meta,
        "review_comments": review_comment_meta,
        "reviews": review_meta,
    }
    if old == snapshot:
        return 2

    facts = observation_facts(
        old,
        snapshot,
        [
            ("issue_comments", issue_bodies, "untrusted_issue_comment"),
            ("review_comments", review_comment_bodies, "untrusted_review_comment"),
            ("reviews", review_bodies, "untrusted_review"),
        ],
    )
    atomic_write(snapshot_path, snapshot)
    emit_json({"changed": True, "facts": facts})
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = SafeArgumentParser(description="Safe GitHub pull-request operations")
    subparsers = parser.add_subparsers(dest="command", required=True)

    preflight = subparsers.add_parser("preflight")
    preflight.add_argument("--repo")

    ensure = subparsers.add_parser("ensure")
    ensure.add_argument("--repo")
    ensure.add_argument("--head", required=True)
    ensure.add_argument("--base", required=True)
    ensure.add_argument("--title", required=True)
    ensure.add_argument("--body")

    monitor = subparsers.add_parser("monitor")
    monitor.add_argument("--repo")
    monitor.add_argument("--pr", required=True)
    monitor.add_argument("--state-dir", required=True)
    return parser


def main() -> int:
    global TOKEN_FOR_REDACTION
    TOKEN_FOR_REDACTION = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN") or ""
    args = build_parser().parse_args()
    args.resolved_repo = resolve_repo(args.repo)
    if args.command == "ensure":
        args.head = validate_branch(args.head, "head")
        args.base = validate_branch(args.base, "base")
        if not args.title or "\x00" in args.title:
            fail("pull request title must be non-empty")
        if args.body is not None and "\x00" in args.body:
            fail("pull request body contains invalid characters")
    elif args.command == "monitor":
        args.pr = validate_pr_number(args.pr)
        args.state_dir = validate_state_dir(args.state_dir)

    token = select_token()
    client = GitHubClient(token, resolve_curl())
    if args.command == "preflight":
        if sys.version_info < (3, 9) or shutil.which("python3") is None:
            fail("Python 3 is required")
        return command_preflight(args, client)
    if args.command == "ensure":
        return command_ensure(args, client)
    return command_monitor(args, client)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except WorkflowError as exc:
        diagnostic = redact_text(str(exc)).replace("\n", " ").replace("\r", " ")
        sys.stderr.write(f"github-pr: {diagnostic[:MAX_DIAGNOSTIC_BYTES]}\n")
        raise SystemExit(1)
