# Judge experiment protocol

Copy this record for each baseline/candidate comparison. Replace every placeholder. Use `unavailable — REASON` when a value or execution status is unverified, `not run — REASON` only when non-execution is established, and `mismatch — EXPECTED/OBSERVED` only after comparing identities. Keep analyses `reported` until stored metadata validates acceptance. Check `tariboy judge inspect RUN_ID` and other stored run metadata before classifying identity metadata.

```text
Experiment ID/date:
Operator:
Authorized scope:
Hypothesis:
Decision rule:
Stop conditions:

Budget:
  approved limit:
  measured spent cost (tokens/currency/elapsed/invocations, as available):
  remaining budget:

Baseline:
  image ref:
  resolved image digest:
  prompt-template SHA-256:
  rubric/skill identity:
Candidate:
  image ref:
  resolved image digest:
  prompt-template SHA-256:
  rubric/skill identity:
Single candidate change:

Controls:
  harness:
  model/configuration (record each model run separately; name models outside scope):
  other frozen settings:

Frozen dataset:
  fixture IDs and unique fixture count:
  real-data split, dataset/evidence IDs, and unique iteration count:
  real-data iteration IDs:

Independent labels (one row per target):
  target/iteration ID | label | rationale | labeler/blinding | evidence ID

Judge results (one row per reported analysis, with acceptance verified separately):
  baseline/candidate | run ID | target ID | repeat | reported/validated accepted | verdict/score | citations | rationale

Fixture results (baseline and candidate separately):
  execution status (default unavailable unless non-execution is established):
  verdict counts (expected fail→fail, fail→pass, pass→pass, pass→fail, expected/observed uncertain):
  accuracy (correct/labeled n/N; 0/0 when no independently labeled cases):
  uncertain Judge verdicts / invalid reference labels / coverage:
  citation and rationale quality:
  observed regressions:

Real-data results (baseline and candidate separately):
  verdict counts (expected fail→fail, fail→pass, pass→pass, pass→fail, expected/observed uncertain):
  accuracy (correct/labeled n/N; 0/0 when no independently labeled cases):
  uncertain Judge-verdict count and handling:
  unknown/invalid reference-label count and reason:
  independently labeled coverage (numerator/denominator):
  citation quality and failures:
  rationale quality and failures:
  observed regressions:

Repetition:
  unique target count:
  analyses per target:
  repeat variability/per-target spread:
  total accepted analyses:

Decision: stop / revise / promote
Decision rationale:
Small-sample conclusion (when applicable): regression not detected on this sample
Unresolved or unavailable fields:
```

Repeated analyses measure variability; they do not increase the unique-target denominator. Exclude unknown, self-produced, or otherwise invalid reference labels from accuracy calculations while keeping them visible in the record. A model not selected for a scoped experiment limits generalization; it does not invalidate results from the selected model.
