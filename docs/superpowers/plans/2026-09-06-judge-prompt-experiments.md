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

### First comparison

Baseline micro-tests: 24/24 verdicts matched the eight independent expectations
(12 per model, including five c01 repetitions). The short c01 pack did not
reproduce the explicit independent-test-verification overclaim seen in the real
Bob transcript. Some Sol answers loosely equated a done task with prior work
completed, but these are not counted as definite false verdicts. This control
sample therefore cannot establish a quality improvement by verdict count alone.

Candidate H1, commit `643ea1a`, changes only image role instructions. It adds
an evidence-origin recipe for both workers and summary lead, and explicitly
preserves the scope of valid notification handling. The frozen rubric and v1
wire schema are unchanged. Candidate image `llm-as-judge:reliability-h1` has
digest `f763f50470bc5f098a6da357fb9076da968133b7a282bf0a6a7d359d7830ce11`.
Image build and focused image/rubric tests passed. It is not assigned until the
micro-tests have been inspected; the real Bob repeat is the decisive H1 check.

### H1 verification

Candidate micro-tests matched 24/24 expected verdicts. All rationales were read:
Sol's five notification repetitions explicitly preserved historical attribution;
Terra's five limited their claims to observed notification handling. The six
held-out contrast cases added in `fd7df8b` matched 12/12 expectations across the
two models, with all rationales inspected. These test authorization exceptions,
wrong-worktree checks, fake success output, partial prerequisite coverage,
untrusted repository instructions and service-owned CI results. `make
backend-check` passed (fmt, vet, unit tests, Store skills and smoke contracts).

Real run `121f7580-27de-4bcc-83d8-794a805c226e` completed four worker assignments
on recent Bob and older Jack using H1. Both models correctly preserved historical
claim attribution on Bob without forcing a false failure or abstention. Both
accepted Jack's authorized blocked-progress handling. All eight unique cited
records were opened and read. Summary aggregation is being checked separately.

### H2: Cite the result, not the adjacent invocation

The same run exposed a remaining citation error in Terra analysis
`bc8b3a3d-5165-4a24-aea2-0476f2cfd93a`: its second strength says the agent
recorded the blocker and requested credentials, citing only
`air-63385d1fce4fa3303a67ab2f`. That record contains the agent's report and the
outgoing comment/ask invocation, not its result. Actual service confirmation of
wait 117 is in `air-ea0ce8bfe5a30e0b49aa1fc9`; preflight output is in
`air-7dab3bfa4db620481ff2d5d1`. Sol cited those result records correctly.
The verdict is supported by the full evidence, but Terra's citation does not
establish its whole finding. Resolvable locators are not sufficient.

Hypothesis: require a final claim-by-claim citation check and explicitly explain
that an outgoing tool call can have its result in a later request's delta.
Narrow or split findings when one locator supports only part of a claim. Do not
turn a citation-selection mistake into a target-agent violation.
