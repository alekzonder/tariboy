---
name: improve-judge-image
description: Use when improving or calibrating the llm-as-judge agent image, comparing rubric changes, or investigating unreliable iteration reviews.
---

# Improve the Judge Image

Improve Judge behavior with paired, reproducible evidence. Treat an image tag as mutable; compare resolved image digests and prompt-template hashes.

## Safety and authorization

- Confirm the authorized image scope and experiment budget before any live experiment. Changing another image, enabling workers or Autopilot, acquiring data, or spending beyond that budget needs separate approval.
- Installing this skill does not authorize or start an experiment. Keep live daemon state and the operator's base/runtime directories untouched unless the user explicitly approves the live work.
- Freeze the evaluation set before editing. Keep tuning examples separate from held-out targets, and do not tune on held-out misses.
- Stop when the approved scope or budget is exhausted, provenance is mismatched, or independent labels are insufficient for the proposed comparison. Report the limitation; do not invent a result.

## Workflow

1. Record scope, approved budget, hypothesis, decision rule, and stop conditions before running work.
2. Resolve and freeze the baseline image digest, template hash, rubric, target IDs, and immutable evidence. Inspect stored run metadata before marking a missing hash unavailable; legacy gaps describe that run, not current agent state.
3. Collect independent human labels and rationales without exposing Judge outputs. Preserve unknown or invalid labels outside the accuracy denominator.
4. Make one Judge-image change. Build a new candidate identity; never rewrite baseline history.
5. Run fixed short positive, negative, and insufficient-evidence controls first. Measure Codex sol and terra separately. One accepted analysis per target is sufficient by default; eligible workers form a pool, while `--judges-per-iteration` controls repeated analyses. Neither setting guarantees a specific number of paid model invocations.
6. After the controls pass, and only with explicit scope and budget approval, review the same frozen Bob/Jack snapshots for baseline and candidate. Example inputs:

   ```bash
   tariboy judge review ITERATION_ID --judges-per-iteration 1
   tariboy judge inspect RUN_ID
   ```

7. Compare paired outcomes, citation and rationale quality, coverage, invalid/uncertain cases, repeat variability, and measured cost. A regression or exhausted budget means stop, save results, and do not promote. Roll back only your own authorized Judge-image change, never user state. Otherwise apply the user's decision rule or choose the next single change; do not invent a universal sample-size threshold.

## Required record

For every experiment, copy and complete [the experiment protocol](references/experiment-protocol.md). Every field is required: use `unavailable` or `not run` with a reason instead of omission. Keep source-experiment measurements separate from simulated tests of this skill.
