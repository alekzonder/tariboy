# Image creator behavioral evals

Use `tariboy-image-evals` for this image suite and existing `writing-skills`
for each independent suite under `../skills/*/evals/`. This directory contains
scenario data and evidence, not another skill-writing procedure or eval engine.

## Run

1. Choose a suite and read its `rubric.json` as evaluator. Send an independent
   fresh-context actor only the request(s) from `cases.json`, required raw
   fixtures and the instructions under test. With a harness supporting tools,
   confine it to disposable state and fake task/PR responses. Never use live
   credentials, tasks, daemons or repositories for simulated actions.
2. For the legacy control supply only `store/skills/image-creator/SKILL.md`.
   For an individual candidate supply only its `SKILL.md`; let it name required
   dependent skill invocations without claiming it executed unavailable bodies.
   For composition supply `instructions.md`, the manifest skill catalog with
   resolved paths, and access to selected skill bodies. Do not supply rubric,
   spec, plan or earlier answers. Baseline and candidate use identical requests.
3. Ask the actor to apply the instructions and report concrete next actions,
   commands, skill reads and uncertainty. Tool restrictions take precedence over
   all simulated lifecycle instructions: no writes, services or `i-am-done`.
4. Save the verbatim response and actual tool trace if available. Score each
   criterion by reading actions and their order, not by matching phrases.
   Record source hashes, harness/model, fixtures, failures and limitations.
5. Repeat failed cases after supported corrections. Run fresh per-case trials
   and pressure/wording repetitions prescribed by writing-skills before claiming
   robustness; the initial simulations below are single-sample smoke evidence.

## Initial evidence: 2026-09-09

Harness: Codex collaboration subagents, `fork_turns: none`, inherited model and
effort. Exact model identifier, sampling configuration and seed are not exposed
by this tool; no model-specific reliability claim is made. Actors could only
read allowed files and propose actions; later separate archival calls wrote
responses. No live daemon, Native Task or GitHub action was part of an eval.

`baseline.md` records one legacy-control actor for A–H. The three candidate
actors each received only their domain skill and corresponding requests.
`baseline-role.md` and `candidate-role.md` record the legacy-control and
candidate composition actors for I–L. Cases within a suite shared one actor
context; these are independent suite-level samples, not per-case repeated
trials. Full responses are preserved; parent/evaluator verdicts and hashes are
in `results.json` and each skill’s `evals/results.json`.

The skill type under test is reference/procedure guidance and rule adherence;
no comparative wording optimization was performed in the initial run. The
follow-up pressure runs below add five repetitions per delivery variant.
Statistical robustness is not claimed. The initial control
already resisted log injection and premature behavioral-success claims; these
passing controls remain regression coverage rather than new improvements.

The composition run used the source prompt and manifest catalog, selected
skill bodies and simulated task facts. It did not launch a Tariboy agent or
exercise a real harness bridge, and omitted unspecified runtime workdir data;
the candidate explicitly requested that missing fact. This proves the observed
routing decisions under those inputs, not end-to-end task execution. Packaging
is verified separately by the real Go image builder below.

## Review follow-up

The initial single-sample evidence did not satisfy the writing-skills repeated
trial requirement for delivery rules. See [pressure/README.md](pressure/README.md)
for five fresh legacy-control and five fresh candidate pressure runs, plus
fresh per-case composition controls/candidates I–L. Raw prompts, responses,
source hashes and strict criterion scores are preserved there.

The fresh K response correctly waits for plan approval but omits explicit base
synchronization from its abbreviated future plan; its strict rubric failure
is retained. K2 separately supplies recorded approval and checks the executable
branch-setup stage, including preflight and base synchronization before worktree
creation. This distinguishes an incomplete future-step description from an
observed out-of-order mutation; no live mutations were performed in either run.

## Mechanical checks (from repository root)

```bash
go test ./internal/builtinimages ./internal/imagefile ./internal/agentskills ./internal/image
cd store/images/tariboy-image-creator
npx skills experimental_install
cd ../../..
TARIBOY_TEST_IMAGE_CREATOR_FULL=1 go test ./internal/builtinimages -run TestImageCreatorBuild -count=1
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s store/images/tariboy-developer/skills/github-pr-workflow/tests -p "test*.py"
```

The default build contract is offline and excludes lock-restored upstream
skills while validating their lock entries. The required full-source gate
builds every declared skill, including existing writing-skills, with temporary
image storage. Both package actual local skills and the executable reused PR
utility. They never contact or restart a daemon. Do not label these checks
behavioral evals or a production image publication.
