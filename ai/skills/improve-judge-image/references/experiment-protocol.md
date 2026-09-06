# Judge experiment protocol

Copy this record for each baseline/candidate comparison. Replace every placeholder; when a value cannot be recovered, write `unavailable — REASON`. Check `tariboy judge inspect RUN_ID` and other stored run metadata before declaring identity metadata unavailable.

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
  model/configuration (record sol and terra separately):
  other frozen settings:

Frozen dataset:
  fixture IDs and unique fixture count:
  real-data split, dataset/evidence IDs, and unique iteration count:
  real-data iteration IDs:

Independent labels (one row per target):
  target/iteration ID | label | rationale | labeler/blinding | evidence ID

Judge results (one row per accepted analysis):
  baseline/candidate | run ID | target ID | repeat | verdict/score | citations | rationale

Fixture results (baseline and candidate separately):
  confusion counts (TP/FP/TN/FN):
  accuracy (correct/labeled n/N):
  uncertain/invalid/coverage:
  citation and rationale quality:
  observed regressions:

Real-data results (baseline and candidate separately):
  confusion counts (TP/FP/TN/FN):
  accuracy (correct/labeled n/N):
  uncertain count and handling:
  invalid count and reason:
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

Repeated analyses measure variability; they do not increase the unique-target denominator. Exclude unknown, self-produced, or otherwise invalid reference labels from accuracy calculations while keeping them visible in the record.
