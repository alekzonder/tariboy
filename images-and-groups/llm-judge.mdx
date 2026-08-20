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
