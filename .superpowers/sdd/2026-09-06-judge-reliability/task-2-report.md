# Task 2 report — grounded rubric and submission contract

## Scope

- Added one canonical bundled rubric at `store/prompts/judge-rubric.md`.
- `judge.ReviewCriteria()` reads that asset and returns its text plus SHA-256.
  Automatic cycles persist both in `OriginalRequest`; the helper is reusable by
  operator review.
- Enforced v1 finite score/confidence, cited strengths and violations, fail /
  pass / uncertain verdict coherence, and preserved all-uncertain consensus.
- Added the rubric as an ordered Judge image prompt layer, aligned evidence and
  proposal instructions, repaired `evidence get` string locators, and added
  command-specific help.

## RED evidence

```text
go test ./internal/judge -run 'TestValidateAnalysisEnforcesVerdictEvidenceContract|TestConsensusKeepsAllUncertainEvenWhenScoresSpread' -count=1
FAIL: all-uncertain consensus was disputed; missing citations, verdict evidence,
and finite-number checks were accepted.

python3 -m unittest store/skills/test_store_skills.py -k test_judge_evidence_get_sends_stable_string_locator
FAIL: --locator was incorrectly parsed as a JSON object.

go test ./internal/judge -run TestBuiltinJudgeImageDeclaresAutomaticCycleContract -count=1
FAIL: Judge image manifest lacked the canonical rubric layer.
```

## GREEN evidence

```text
go test ./internal/judge -count=1
ok github.com/alekzonder/tariboy/internal/judge

python3 -m unittest store/skills/test_store_skills.py
Ran 24 tests ... OK

python3 /home/agent/.codex/skills/.system/skill-creator/scripts/quick_validate.py store/skills/llm-as-judge
Skill is valid!

git diff --check
```

No live daemon, target repository, or production data was accessed by this task.
