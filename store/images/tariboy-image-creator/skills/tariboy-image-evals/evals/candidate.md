**C**

- I would not declare success from YAML validation. It establishes packaging evidence only; routing behavior remains untested.
- Next, obtain IMG-3’s approved scope and the actual image prompt/catalog. Create the missing image `evals/` before modifying instructions, with realistic requests, raw fixtures and evaluator-only criteria covering the relevant routing branches.
- Run each baseline case in a fresh context with the full composed prompt/catalog and access to referenced skills. Keep evaluator rubrics and expected answers hidden. Use disposable state or simulated tool responses; no live mutations.
- Apply the approved routing improvement, then run the same cases against the candidate. Assess actual skill requests, approval waits and applicable delivery/completion decisions. Preserve failures and repeat affected cases after fixes.
- Use `tariboy-image-evals` for whole-image routing. If a selected individual skill makes the wrong decision, invoke `writing-skills` and its separate skill suite, then repeat the image composition case. Its body is unavailable here, so I cannot claim to have followed that workflow yet.
- Retain case IDs, exact requests/fixture revisions, baseline/candidate labels, prompt/catalog/skill hashes, model/harness configuration, response/action traces, criterion verdicts supported by excerpts, failed runs and limitations; include image version/digest when built. Redact credentials and private log details.
- Report packaging and behavioral results separately through the Native Task. At present, YAML success is supplied information; no behavioral evaluation has executed. Missing prompt/catalog, fixtures and harness access remain uncertainties. Record a task question for any missing decision/access that prevents required evaluation.

This response is a simulation of proposed actions, not an executed integration or a passing evaluation.
