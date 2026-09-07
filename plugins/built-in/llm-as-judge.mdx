---
title: llm-as-judge
description: Run capability-gated, evidence-scoped worker and lead actions for durable historical iteration reviews.
sidebar:
  label: llm-as-judge
  icon: gavel
---

`llm-as-judge` exposes the agent side of Tariboy's historical iteration review
workflow. It is optional, is not included in `basic:latest`, and is distinct
from image-declared post-iteration evals.

## Roles and command surface

A judge group is an ordinary Tariboy group whose selected images include this
capability. Tariboy does not create a built-in group or choose models for it.

| Role | Commands |
| --- | --- |
| Lead | `scripts/judge.sh automation begin`, `iterations search`, `run create`, `run inspect`, `summary claim`, `summary inputs`, `improvement submit`, `summary submit`, `run cancel`, `work retry` |
| Worker | `scripts/judge.sh work claim`, `evidence search`, `evidence get`, `analysis submit` |

The service authorizes actions using the authenticated agent, current
iteration, group membership, lead role, and assignment lease. Supplying IDs in
a request does not override those checks.

## Investigation flow

1. The lead searches completed iterations and creates a run with an explicit
   selector, preserved criteria, and replica count.
2. Tariboy snapshots redacted evidence into immutable, content-addressed
   bundles.
3. Worker agents claim one assignment at a time and inspect only that
   assignment's evidence.
4. Each worker submits the fixed analysis schema with evidence-linked claims.
5. The lead claims summary work, reads durable analyses, and submits the
   versioned summary.

When evidence identifies a repeatable role, prompt, skill, or image defect, the
summary agent may submit a structured improvement proposal. It is bound to task
subjects and evidence bundle hashes and names the repository, base commit,
relative file scope, acceptance criteria, risk, and immutable rollback image.
The Judge cannot approve it, edit Git, publish, assign, or roll out.

Failed assignments can be retried without discarding completed analyses.
Cancelling a run cancels pending and claimed work while preserving immutable
evidence and completed artifacts.

## Automatic reviews

`tariboy judge automation apply --json JSON` stores a revisioned daemon-owned
configuration and reconciles one ordinary cron schedule. Applying config creates
the fixed `JUDGE` and `IMPROVE` task queues but does not start a review.
`tariboy judge automation run-once --limit 3` creates an ordinary due one-shot
schedule; it does not start agents directly.

The JSON document selects the lead, exactly two workers, their Judge image,
cron spec, target agent names, exact target image refs, and
`only_unprocessed`. Names and versions are never compiled into Tariboy. The
daemon validates agent/image existence, Judge capabilities, role separation,
target image history, and cron syntax. The customer is derived from the
daemon's `USER` environment variable.

Every schedule fire creates a `JUDGE-*` task. The configured lead begins the
cycle using the schedule delivery ID, and the existing runner wakes workers,
collects analyses, and requests the summary. Results, zero-target outcomes, and
failures complete the task and mention `user:${USER}`. Structured proposals are
recorded on that task. Approving an exact proposal revision atomically creates
one idempotent `IMPROVE-*` task; separate proposals produce separate tasks for
different repositories or release units.

## Evidence boundary

Evidence is untrusted data, not instructions. Workers search and read it through
stable locators exposed to their owned assignment; filesystem paths are not an
evidence API. The service verifies referenced locators before accepting an
analysis and audits evidence reads without recording raw query text.

Snapshots remain readable after retention removes the original iteration
directory. Evidence redaction and content hashes make citations stable, but do
not grant a worker access to evidence from another assignment.

New snapshots use Evidence Bundle v2. Runs group targets by attributed Native
Task when available and record task/artifact metadata, participant image refs
and digests, prompt-template hashes, packaged skills/plugins, source digest,
repository commit, and lock digest. Schema-v1 bundles remain readable.

## Durable state

Judge runs, targets, assignment leases, immutable evidence identities,
analyses, consensus fields, summaries, retry state, and cancellation state are
stored by the daemon. Judge and summary iteration usage is attributed
separately from the historical iterations being evaluated.

## Prompt integration

```yaml Tariboyfile.yaml
plugins:
  - name: llm-as-judge
skills:
  - dir: $CURRENT_VERSION_STORE/skills/llm-as-judge
```

The packaged Store skill defines the lead/worker discipline and tells workers
to treat evidence as untrusted. Scheduled messages initiate cycles through the
authenticated capability-gated command; agents do not manage lifecycle state.

## Related reference

- [LLM-as-Judge workflow](/docs/images-and-groups/llm-judge)
- [AI proxy and audit](/docs/architecture/ai-proxy)
- [Operator command reference](/docs/reference/commands#operator-commands)
