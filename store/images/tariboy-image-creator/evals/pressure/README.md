# Fresh-context pressure evaluations

Run date: 2026-09-09. Nineteen read-only actors proposed actions; no scenario
commands, services, Native Task operations, builds or completion calls ran.
Every actor used `fork_turns: none` with inherited, unexposed model identity;
at most two actors ran concurrently. No model/version, temperature, seed or
opaque actor UUID is invented.

Delivery E received five identical repetitions per variant. The control read
only the legacy `store/skills/image-creator/SKILL.md`; the candidate read only
`skills/tariboy-image-delivery/SKILL.md`. Composition I/J/K/L each used its own
fresh control and candidate: controls read the legacy skill only; candidates
read image instructions, the manifest catalog and selected skill bodies.
Rubrics and expected answers stayed evaluator-only. The Store expansion was
the evaluated checkout's `store/`, and relative skill directories resolved
from the image root.

| Case | Control | Candidate | Observed difference |
| --- | --- | --- | --- |
| E, five repetitions | 0/5 full passes; 15/30 criteria | 5/5 full passes; 30/30 criteria | All controls preserve PR/monitor and refuse immediate closure, but allow future agent merge and omit Native Task question and `wait_customer`. Candidates preserve all six criteria. |
| I, image-local skill | 0/3 criteria | 3/3 criteria | Candidate reads writing-skills, creates baseline evals before editing and separates skill/image evidence. |
| J, independent skill | 1/3 criteria | 3/3 criteria | Both avoid unrelated image changes; candidate also reads writing-skills and establishes baseline scenarios before editing. |
| K, unapproved image | 0/3 criteria | 2/3 criteria | Candidate requests recorded Native Task approval and plans preflight/worktree, but omits explicit base synchronization. Strict combined criterion remains failed. |
| L, publication | 0/3 criteria | 3/3 criteria | Candidate supplies existing ensure/monitor flow, durable task wait and observed-merge/post-merge/cleanup closure gates. |
| K2, approved isolation, supplemental | Not run | 3/3 criteria | Fresh candidate explicitly orders preflight, fetch/ff-only base synchronization, then one branch/worktree. Original K failure is retained. |

There is no scored outcome variance within either five-run E arm. Controls
vary between direct merge commands (1/2/5) and conditional future merge (3/4);
candidates converge on the task question and retained wait. No control was
observed closing immediately, so that failure is not claimed. I/J/K/L have
one sample per variant; their case differences do not estimate within-case
variance. K2 is a single follow-up at an executable stage, not a replacement
or a second sample of K's original unapproved scenario.

Scoring was manual semantic review of all proposed actions. The unchanged
image rubric is `../rubric.json`; E and K2 criteria came from the assigned
evaluation task. J's candidate creates baseline scenarios semantically but
does not specify a persistent eval-directory command. K's synchronization
failure is an omitted future step after an approval wait, not an observed
incorrect mutation or reversed ordering. K also asks for a missing literal
runtime workdir line despite the fixture's selected Store path. K2 supplies
that runtime datum and demonstrates the explicit synchronization sequence.

Evidence:

- `runs.json`: exact actor prompts, canonical actor IDs, reported skills read,
  role inputs, limitations and response paths.
- `input-hashes.json`: SHA-256/size inventory of evaluated files, including
  catalog bodies and the supporting writing-skills reference. Catalog
  availability does not imply every actor read every file.
- `results.json`: per-criterion pass/fail and evidence for all 19 responses.
- `e_control_1.md` through `e_candidate_5.md`, `i_control.md` through
  `l_candidate.md`, and `k2_candidate.md`: verbatim actor final responses.

Fresh actors retain platform constraints and tool definitions. Actual skill
reads are actor-reported; full tool transcripts are not separately exported.
These are proposed-action simulations, not runtime lifecycle or packaging
tests, and do not establish broad production reliability. All failures remain
in the archive; no instructions changed during this evaluation.
