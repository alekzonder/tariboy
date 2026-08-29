# Automatic Judge Review Design

## Goal

Run LLM-as-Judge without manual agent starts. An operator edits one validated
JSON document, Tariboy applies it to the existing schedule subsystem, and every
scheduled or one-shot cycle evaluates configured unprocessed iterations,
records its work in Native Tasks, and asks the current customer for attention
when a result, question, or failure exists.

## Non-goals

- No new scheduler, cron parser, workflow engine, or polling loop.
- No hard-coded agent names, image refs, repositories, or customer login.
- No automatic source modification, plan approval, publication, or rollout.
- No form-based configuration UI or new editor dependency.

## Configuration

`tariboyd` owns a singleton revisioned Judge automation document:

```json
{
  "schema_version": 1,
  "enabled": true,
  "judge": {
    "lead": "tariboy-judge-lead",
    "workers": ["tariboy-judge-worker", "tariboy-judge-worker-2"],
    "image_ref": "llm-as-judge:1.3"
  },
  "schedule": {"spec": "0 */3 * * *"},
  "targets": {
    "agents": ["tariboy-developer-jack", "tariboy-developer-bob"],
    "image_refs": ["tariboy-developer:0.5", "tariboy-developer:0.6"],
    "only_unprocessed": true
  }
}
```

The values above are examples, not defaults. Agent names and every image ref
come exclusively from the stored document. The only fixed operational values
are the explicitly required queue prefixes `JUDGE` and `IMPROVE`. The customer
principal is `user:${USER}`, where `USER` is read by `tariboyd` from its
environment; an empty value makes validation fail.

`tariboyd` parses raw JSON with unknown fields rejected and validates:

- schema version, non-empty unique lists, and exactly two distinct workers;
- the existing five-field cron parser;
- existence and role separation of lead, workers, and target agents;
- existence and required capabilities of the configured Judge image;
- existence of every target image ref and its use by a configured target;
- a non-empty `USER` environment value.

Validation errors contain a JSON Pointer path and bounded message. Apply repeats
all validation and never trusts an earlier validation response.

## Daemon API and CLI

Operator routes expose get, validate, apply, and run-once operations. Validate
and apply accept raw JSON. Apply stores a canonical document, revision, and hash
and reconciles one ordinary recurring schedule. The schedule message contains
only its type and configuration revision; the daemon loads the exact revision
when it fires.

The CLI is a thin HTTP wrapper:

```text
tariboy judge automation get
tariboy judge automation validate --file judge.json
tariboy judge automation apply --file judge.json
tariboy judge automation run-once --limit 3
```

`apply` provisions and schedules but does not start an evaluation. `run-once`
validates the active revision and creates an ordinary immediately due one-shot
schedule with a bounded selection limit. It does not invoke an agent lifecycle
command.

## Existing Schedule and Wake Path

There is no Judge-specific timer. The existing durable schedule service fires
the recurring or one-shot row and publishes `judge.review.requested` to the
configured lead inbox. The existing bus wake hook starts the enabled lead.
`judge.work.available` wakes workers, `judge.summary.ready` wakes the lead, and
Native Task replies wake the task assignee.

Judge agents have no blind periodic interval. Apply ensures the configured
lead and workers remain enabled and compatible with the configured Judge image;
idle completion leaves them stopped but wakeable.

## Queue Provisioning

Apply idempotently creates both Native Task queues immediately:

- `JUDGE` for review cycles, findings, questions, and results;
- `IMPROVE` for approved improvement plans.

Queue provisioning and configuration/schedule replacement are one daemon-side
transaction. Agents do not receive customer-only queue administration
authority. Changing the configured lead updates the `JUDGE` queue ownership.

## Review Cycle

Each schedule fire creates exactly one root `JUDGE-N` task assigned to the
configured lead. The task and cycle use idempotency derived from the schedule
fire and configuration revision.

The lead creates a Judge run using selectors from that exact revision. The
selector supports exact target agents, exact image refs, terminal statuses,
ordering, an optional one-shot limit, and `only_unprocessed`. Selection and
target insertion are one transaction. `only_unprocessed` excludes iterations
already targeted by an active, partial, or completed run and permits selection
again only when every prior containing run was cancelled.

Evidence snapshotting, work publication, worker leases, analysis submission,
summary readiness, and summary submission reuse the current Judge runner. Two
configured workers claim assignments until none remain. Every analysis records
the target image ref and digest.

The lead posts the final summary and proposal IDs to `JUDGE-N`, mentions
`@user:${USER}`, and completes the task when no decision is needed. With no
eligible targets it still records a zero-target result and completes the task.
Questions set `waiting_for=user:${USER}`. A daemon-side terminal-failure fallback
records and notifies failures even when the lead cannot finish its own task.

Overlapping fires do not start duplicate review work for the same revision. A
fire finding an active cycle records a skipped result in its own required task.

## Improvement Proposals and Tasks

The summary lead may submit structured, evidence-linked improvement proposals
through the existing Judge capability. One proposal targets exactly one
repository, base commit, and logical release unit. Different repositories or
independent release units require separate proposals.

Plan approval remains an operator-only action bound to the exact proposal
revision hash. In the same transaction that records approval, `tariboyd`
creates one `IMPROVE-N` task per approved proposal with idempotency key
`improvement:<proposal-id>:<revision-hash>`.

Each improvement task contains the source `JUDGE-N`, repository, base commit,
approved file allowlist, intended changes, acceptance criteria, checks,
evidence citations, risk, rollback target, and proposal identity. Upstream and
downstream repository changes become separate related tasks; downstream work
is blocked by its upstream task. Rejection creates no improvement task. A
changed proposal revision requires a new approval and cannot silently mutate
the task created for the old approved hash.

## UI

The existing Judge runs page gains an Automation section with a plain labelled
`textarea` and `Validate`, `Apply`, and `Reset` actions. It sends raw text to
the selected explicit daemon. It performs no business validation and displays
daemon JSON Pointer diagnostics. Apply revalidates server-side and replaces the
editor with the canonical stored document on success. Unsaved text remains in
React memory and is not persisted in Web Storage.

The existing proposal UI continues to own plan approval. Successful approval
shows the created `IMPROVE-*` task keys and links back to the source Judge task.

## Failure and Safety

- Invalid apply is side-effect free.
- Schedule, queue, and configuration mutations are daemon-authoritative and
  revision checked.
- Agent and image changes activate only at existing safe iteration boundaries.
- A live cycle is never repaired by manual agent starts or prompts.
- Failures are persisted in `JUDGE-N` and notify `user:${USER}`.
- Existing loopback, authentication, redaction, evidence, and approval
  boundaries remain unchanged.

## Release and Verification

This adds user-facing API, CLI, and UI capability, so the requested release is
`0.44.0`, set only with `scripts/set-version.sh`. The Judge image is rebuilt as
a new immutable version containing schedule, Tasks, and Judge instructions.

Implementation uses focused failing tests, isolated daemon directories, UI
unit tests, production Desktop Playwright and `tauri-driver`, `make check`, and
`make full-check`. The final automatic smoke uses `run-once --limit 3`, then
only observes the resulting schedule, agents, run, tasks, notifications, and
summary without intervening in agent work.
