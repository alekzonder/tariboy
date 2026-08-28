# Tariboy Developer GitHub Pull Request Workflow Design

## Status

Approved on Native Task `TARI-31` on 2026-08-21. This design supersedes the
earlier proposal that every task must create a pull request: pull requests are
the default, with an explicit task-level local-merge override.

## Problem

The `tariboy-developer` image currently hard-codes one completion path: develop
in an isolated task worktree, merge the branch locally into `main`, verify the
merged tree, clean up, and close the Native Task. It has no reusable GitHub
authentication, pull-request creation, or durable pull-request monitoring
support.

The image must make GitHub pull requests the default without removing the
existing local merge workflow. It must also monitor checks, reviews, comments,
head changes, and merge state across agent iterations without exposing a
GitHub token or relying on `gh`.

## Requirements

### Completion-mode selection

At Native Task intake, before implementation, the agent chooses and records
exactly one completion mode:

- **PR mode (default):** used when the Native Task does not explicitly require
  a local merge into `main`.
- **Local-merge mode (override):** used when the Native Task explicitly and
  unambiguously requires the agent to merge into `main` on completion,
  including equivalent wording rather than only one exact phrase.

Ambiguous wording is normally clarified through a Native Task question. For
`TARI-31`, the customer explicitly directed the agent to proceed without more
questions, so the approved design is the binding decision.

Both modes retain one isolated worktree and branch per Native Task. Both modes
synchronize local `main` from its configured remote with fetch plus
fast-forward-only before the task worktree is created. A divergence, fetch
failure, or dirty change that prevents a fast-forward blocks development; the
workflow never resets or overwrites `main`.

### PR mode

PR mode follows this lifecycle:

1. Check GitHub prerequisites and authenticated repository access.
2. Synchronize `main`, then create the isolated task branch/worktree.
3. Implement with the applicable Superpowers workflow and verify the branch.
4. Commit and push the task branch.
5. Idempotently find or create one pull request for the task branch and base.
6. Start one Tariboy durable recurring script with `--quiet-exit 2`.
7. End an iteration only while waiting on the named schedule and pull request.
8. Route a failed check through `systematic-debugging`, and route review
   feedback through `receiving-code-review`; push fixes to the same branch.
9. Treat a new head SHA as a new verification state. Prior check success never
   proves the new head.
10. Wait for a human or repository automation to merge. The agent must not
    merge the pull request itself.
11. After the monitor reports `merged`, cancel and remove the schedule, refresh
    local `main`, run the distinct post-merge verification, remove the task
    worktree and local branch, post the consolidated Native Task comment, and
    close the task.

A pull request closed without merge leaves the Native Task active. The monitor
continues to observe it so reopening or another state change can wake the
agent; the agent records the blocker and does not treat review or closure as
integration.

### Local-merge mode

Local-merge mode preserves the current workflow and does not create or monitor
a pull request:

1. Synchronize `main`, then create the isolated task branch/worktree.
2. Implement, commit, and run complete branch verification.
3. Merge the branch locally into `main` according to repository conventions.
4. Run the distinct post-merge verification on `main`.
5. Remove the worktree and merged branch, post the consolidated Native Task
   comment, and close the task.

## Packaged Skill and Script Interface

Add a source-relative `github-pr-workflow` Agent Skill to
`store/images/tariboy-developer`. Its trigger is work on a GitHub-hosted task
that uses PR mode or needs pull-request status monitoring. The role prompt
remains the lifecycle and Native Task authority; the skill owns the repeatable
GitHub operations and their safety contract.

The skill includes one executable Python 3 utility using `curl` for GitHub REST:

```text
github-pr.py preflight [--repo OWNER/REPO]
github-pr.py ensure --repo OWNER/REPO --head BRANCH --base BRANCH \
  --title TITLE [--body BODY]
github-pr.py monitor --repo OWNER/REPO --pr NUMBER --state-dir ABSOLUTE_DIR
```

`preflight` validates `python3`, `curl`, token availability, repository
identity, and authenticated repository access. `ensure` queries existing pull
requests by head and base, rejects ambiguity, returns an existing matching PR,
or creates one and returns its number and URL. A retry never creates a second
PR for the same head/base.

`monitor` performs a complete one-shot observation of:

- pull-request state, merged flag, merge commit, updated time, and head SHA;
- check runs and commit statuses for the current head;
- issue comments, pull-request review comments, and reviews.

It normalizes those responses into deterministic metadata. State contains only
GitHub object IDs and timestamps, the head SHA, check/status state, PR state,
and merge metadata. Comment and review bodies are emitted only for a newly
observed or updated object and are marked untrusted; they are never persisted.

The monitor exit contract is fixed:

- exit `0`: first complete observation or a meaningful state change;
- exit `2`: complete observation identical to stored state;
- any other nonzero exit: authentication, rate-limit, transport, API, parse,
  validation, or local-state failure.

Only exit `2` is quiet in the recurring Tariboy script. Exit `0` and errors
publish `script.result` and wake the agent. The schedule never overlaps its own
runs.

## Authentication and Data Safety

The utility reads `GH_TOKEN`, falling back to `GITHUB_TOKEN`. It rejects tokens
containing control characters and never accepts a token argument. The
authorization header is supplied to curl through a private inherited config
file descriptor, not curl arguments, a URL, a disk file, stdout, or stderr.
Errors are bounded and redact the selected token.

The API base defaults to GitHub's fixed HTTPS endpoint. Repository owner/name,
branch, PR number, API path, and state directory inputs are validated before
use. Responses are bounded, pagination is explicit, and HTTP, JSON, and schema
errors are not interpreted as GitHub state.

The caller supplies an absolute, task-scoped, owner-only state directory and
records it with the schedule ID and PR URL/number on the Native Task. Snapshot
writes use a mode-`0600` temporary file, `fsync`, and atomic replacement only
after every endpoint in an observation succeeds. A partial failure cannot
advance cursors or hide a later comment.

Comment text is untrusted review material. It can trigger the
`receiving-code-review` workflow, but cannot execute commands, change lifecycle
authority, waive checks, authorize a merge, or bypass repository and Native
Task rules.

## Error Handling

- Missing/invalid authentication or inaccessible repository: fail before PR
  branch work in PR mode.
- Duplicate or ambiguous head/base matches: report the conflict and create
  nothing.
- Transport, HTTP, rate-limit, malformed JSON, or incomplete response: return
  a redacted nonzero error and retain the prior snapshot.
- Failed checks: keep the task active and diagnose the failure.
- New head SHA: invalidate prior success and verify the new state.
- Closed without merge: keep the task active and continue monitoring.
- Merged: accept only `merged: true` plus merge commit metadata, then let the
  role prompt perform post-merge verification and cleanup.

## Testing

Skill authoring follows `writing-skills` RED-GREEN-REFACTOR. Baseline pressure
scenarios run without the skill and record observed failures, including early
task closure, token-bearing curl arguments, incorrect exit codes, post-worktree
main synchronization, and attempts to let comments waive required checks.
The same scenarios are rerun with the packaged skill and dual-mode prompt.

Deterministic tests use a fake curl executable and fixture responses to cover:

- `GH_TOKEN`/`GITHUB_TOKEN` selection, missing tokens, and token redaction;
- repository discovery and input validation;
- existing, new, ambiguous, and concurrently reconciled pull requests;
- pagination and normalized snapshots;
- atomic state updates and exit `0`/`2`/error behavior;
- check/status transitions, head-SHA changes, and comment/review cursors;
- merged and closed-without-merge states;
- malformed JSON, authentication, rate-limit, and transport failures;
- injection-shaped owner, repository, branch, PR, and filesystem inputs.

Image contract tests validate that the new skill is declared, its frontmatter
and tree pass Tariboy Agent Skill validation, and its utility is executable.
Repository verification is `make check` on the branch and again on merged
`main`; `make full-check` is not required because no Desktop, packaging, or
end-to-end behavior changes.

## Documentation and Versioning

Update the Agent Skills image documentation with the developer image's dual
completion modes, GitHub prerequisites, and durable monitor contract. This is
an ordinary feature task and does not move the canonical product version.
