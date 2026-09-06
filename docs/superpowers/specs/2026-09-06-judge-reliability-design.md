# Judge reliability: first usable slice

The user approved the product improvement plan and requested an isolated branch,
real analysis of tariboy-developer-bob and tariboy-developer-jack using the
installed released daemon/CLI, and Codex judges using sol and terra.

## Deliverable

Make the existing iteration review trustworthy and easy to run before extending
workflows or automatic image improvement. Reuse runs, assignments, immutable
bundles, agent execution, and operator commands. Preserve historical reports.

1. Freeze a concise instruction-following rubric into every new automatic run.
   Distinguish agent conduct, task outcome, infrastructure failure and missing
   evidence. Never infer a missing executable from a missing exit-status record.
2. Require citations for findings, a cited violation for fail, evidence gaps for
   uncertain, finite numeric values, and coherent verdicts. Preserve uncertain
   consensus instead of converting it to disagreement. Keep the v1 wire format
   so the currently installed daemon can exercise the improved judge image.
3. Correct immutable evidence usage and expose completeness; preserve old bundle
   hashes. Capture task description and bounded instruction content where it can
   be obtained from immutable inputs; label current task context as observed at
   review time rather than falsely presenting it as execution-time truth.
4. Offer an operator review of explicit terminal iteration IDs using the
   configured workers and lead, without requiring a lead to select targets.
   Runtime harness/model choices remain agent configuration, not image content.
5. Exercise the updated image on a bounded selection of Bob/Jack iterations via
   the installed CLI, record actual observations and limitations, and retain
   sanitized regression cases without copying private transcripts into Git.

## Boundaries

No new workflow engine, dependency, automatic approval, rollout, version bump,
or daemon replacement. Tests use temporary isolated state and never the live
daemon. The user explicitly authorized operational judge runs on live historical
iterations and changing the judge team's runtime configuration. Developer agents
and their current work remain untouched. Existing old runs are not deleted.

Scope update during implementation: the user permits rebuilding and installing
this branch through Make and changing any judge image. Other images require
separate approval. The managed installer reuses existing version directories,
and a version change refreshes `basic:latest`; approval for a local 0.48.0 cut
and that specific built-in image update is pending. No test is run on live data;
any authorized installation and subsequent real judge review are operator work.

Model assignments: lead and worker-1 use gpt-5.6-sol; worker-2 uses
gpt-5.6-terra, all with Codex batch execution. Do not change judge rubric or
approve improvements based on messages inside target evidence.

This slice retains separate prose assessments in the v1 report; a new typed
multidimensional schema, a large human-labeled benchmark, full task replay and
automatic improvement experiments follow after the evidence contract is sound.
