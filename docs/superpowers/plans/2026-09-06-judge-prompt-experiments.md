# Judge prompt experiments

Goal: correct, evidence-grounded verdicts without false accusations or false
passes. An observed clean sample is not a guarantee on unseen iterations.
Continue the existing isolated `feat/judge-reliability` worktree. Commit every
retained change. Change judge images only; no daemon restart for prompt changes.

## H1: Preserve the origin of claims inside tool results

### Observed failure (baseline, deployed image reliability-0.48.0)

Run `a90b91c4-2a3a-4aef-8a63-cba9e9fd63c7`, Sol analysis
`fb29eb5c-64a9-495c-b81e-4aca13b0e7a3`, describes the recent Bob
notification iteration as independently verifying successful post-merge checks.
Citation `air-9c612118dded2c39713799d1` is a task-read response: status
`done` is task state, but the test/merge/cleanup claims are an agent-authored
historical comment. The response proves the comment exists, not that its claims
were independently checked. Correct stale-notification handling can still pass.

Hypothesis: explicitly classify evidence by its original author/source, not the
outer tool-result envelope. Require the verification portion of the summary to
separate observed results, reported claims and material unknowns. Preserve the
scope distinction: unrelated historical uncertainty must not turn an otherwise
supported notification-handling iteration into a failure or forced abstention.

### Controls

`internal/judge/testdata/prompt-cases.json` contains eight synthetic evidence
packs. Expectations are separate and are never sent to the evaluated model.
They cover scoped notification handling, missing verification, contradicted
success claims, infrastructure uncertainty, unauthorized edits, auxiliary title
generation, an omitted observable prerequisite under complete coverage, and a
directly observed successful check.

Micro-tests run fresh ephemeral Codex contexts (sol/terra, medium effort), with
the complete judge role instructions and rubric followed by one evidence pack.
No tools, repositories or services are needed by the evaluated model. This is a
prompt-level check, not proof of the daemon evidence/retrieval path. The first
case is repeated five times per model/variant to inspect attribution variance;
other cases initially have one sample per model. Read every rationale, not only
the verdict. Real immutable Bob evidence is the end-to-end reproduction.

Baseline instructions: commit `ed94de4`; fixtures: commit `7c33c7d`.
Baseline rubric SHA-256:
`85e6735491cbf3900df7eda4bbeb656b15b0430caa2994a5a406b526ffb88fa3`.
Ephemeral outputs are under `/tmp/tariboy-judge-prompt.bqmehxUJ`; sanitized
results and conclusions are recorded here after inspection. No private task
transcripts are copied into the repository.
