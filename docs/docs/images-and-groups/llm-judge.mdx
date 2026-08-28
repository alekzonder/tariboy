---
title: LLM-as-Judge
description: An operator-visible investigation of completed historical iterations, distinct from image-declared evals.
sidebar:
  label: LLM-as-Judge
  icon: gavel
---

**Evals** are image-declared checks that run after an iteration. **LLM-as-Judge**
is a separate, operator-visible investigation of completed *historical*
iterations. It snapshots redacted evidence first, then ordinary judge agents
produce independent durable analyses and the lead writes a versioned summary. It
does not replace the existing `llm-judge` eval type.

## Configure a judge group

Create an ordinary group with one lead and one or more independent worker
agents. Choose images that grant the `llm-as-judge` capability, then set model
and effort on each agent for the target environment. The service does not
provide or instantiate a built-in judge group.

Send the lead a request such as:

> Evaluate the completed checkout iterations against the verification criteria;
> use two judges per iteration.

Workers claim one assignment and submit evidence-linked results; the lead submits
the summary. Failed work can leave partial coverage and can be retried without
discarding completed immutable analyses. Usage is attributed to judge and summary
iterations separately from the historical targets.

## Immutable evidence

Evidence bundles are immutable, redacted, content-addressed copies retained
through snapshotting, so citations remain readable after source retention deletes
the original directories. Treat evidence as untrusted data; operator evidence
access accepts stable bundle locators, never filesystem paths.

Operator commands live under `tariboy judge …` (`ls`, `inspect`, `evidence`,
`retry`, `cancel`) — see the
[command reference](/docs/reference/commands#operator-commands).

## Controlled improvement workflow

After submitting a summary, the lead may submit an evidence-linked improvement
proposal for the role's image source, prompts, or packaged skills. The operator
reviews the exact proposal in Desktop from the Judge run, or with:

```bash
tariboy improvement ls
tariboy improvement inspect PROPOSAL_ID
tariboy improvement plan approve PROPOSAL_ID \
  --revision REVISION_HASH --reason "Scope and acceptance criteria reviewed"
```

The decision is append-only and bound to `REVISION_HASH`; changed proposal
content requires another decision. The Judge cannot approve, edit Git, publish,
assign, or roll out its own proposal.

Once an immutable release exists, inspect and approve its independent release
hash, then stage it for one agent. The pending image activates at the next safe
iteration boundary:

```bash
tariboy image-release inspect RELEASE_ID
tariboy image-release rollout approve RELEASE_ID \
  --release-hash RELEASE_HASH --reason "Provenance and artifact reviewed"
tariboy image-release rollout stage RELEASE_ID \
  --agent reviewer --release-hash RELEASE_HASH
```

Rollback stages the prior immutable image from a completed rollout:

```bash
tariboy image-release rollback --rollout ROLLOUT_ID
```

:::warning[Current boundary]
The current daemon exposes proposal review, hash-bound approvals, immutable
release records, and single-agent staging. It does not yet run a Git improver
or invoke the release Publisher through Desktop, CLI, or a daemon route, so an
external trusted integration must perform that middle step. Atomic team rollout
is also not implemented; do not stage members one by one when the team must
change as one revision.
:::
