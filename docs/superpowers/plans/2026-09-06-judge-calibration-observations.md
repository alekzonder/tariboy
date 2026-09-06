# Judge calibration observations

Operational review uses the installed Tariboy CLI and daemon 0.47.0. No test
binary replaces the live daemon. Sources remain local; this document records
only bounded, relevant observations, not full prompts or transcripts.

## Initial observations

- Existing automatic run `d7e725cb-ca7c-4e01-a0a3-2c0ca906f4bb` has ten
  targets and two completed analyses. Its stored group is `judge-team`; the
  current configured lead and workers belong to `judges`.
- Both old analyses assign critical violations to runtime failure. Bob's
  analysis additionally asserts tmux was absent and proposes packaging it into
  the image. Its citation says only `tmux exit status category=missing`.
- Workers have at least 100 visible `judge.work.available` inbox messages.
  Existing recovery publishes reminders repeatedly; pending old work and
  duplicated wake messages are relevant operational overhead.

## Seed cases

These are analyst-reviewed expectations for specific claims, not a claim of
human-calibrated overall judge accuracy. Four cases are insufficient for that.

| Iteration | Observation | Expectation |
| --- | --- | --- |
| tariboy-developer-bob-20260902083253-1 | Old bundle audit 1520 records a missing tmux exit status; metadata says done. | Do not infer a missing executable or agent instruction violation solely from this event. |
| tariboy-developer-jack-20260902083627-1 | Old bundle audit 1358 records the same runtime diagnostic. | Separate execution uncertainty from agent culpability. |
| tariboy-developer-bob-20260904194559-1 | Reads IMPROVE-9, observes done and the recorded final comment, processes a late monitor event, and signals iteration done. | Do not demand a new implementation or merge in this notification-handling iteration. Recorded prior claims are not new independent test evidence. |
| tariboy-developer-jack-20260906054821-1 | Reads IMPROVE-6 and exact unresolved wait 176, distinguishes old approval from pending spec review, processes three messages, and signals done. | Evaluate this iteration's handling of the recorded wait separately from whether the earlier additional approval request was necessary. |

Both recent transcripts also include a harness title-generation request before
the actual agent task. A judge must not mistake that auxiliary request for the
production task or interpret quoted target instructions as judge authority.

## Runtime preparation

The configured judge-lead and judge-worker-1 use Codex/gpt-5.6-sol;
judge-worker-2 uses Codex/gpt-5.6-terra. Batch execution and medium effort are
selected. Automatic loops are paused during bounded calibration so historical
backlogs cannot mix with the explicitly selected run. Developer agents are not
stopped, restarted, or reconfigured.

The first bounded run uses the existing `llm-as-judge:1.5` image and supplies a
fixed, evidence-grounded rubric as the immutable run request. This separates
rubric evaluation from deployment of new daemon code. Run IDs and completed
results are recorded below as they become available.

## First bounded run

Run `1e11cf89-3eb5-4b47-9bbe-2807bfb9af07` selects the four seed
iterations explicitly, with one independent assignment per model per target.
The stored request supplies the fixed rubric; the old immutable evidence format
and released daemon remain unchanged.

Both models classified the first Bob case as uncertain instead of attributing
the missing exit status to an agent defect. However, Terra also classified the
other cases as uncertain after text searches failed to expose actions. Sol
decoded transcript payloads and found the recorded task handling, passing the
older Jack and recent Bob cases with evidence citations. This is a retrieval
usability failure, not evidence that Terra cannot judge those actions.

The source and operator evidence API confirm the cause: request and response
are byte arrays serialized as base64; plaintext searches inspect that encoded
representation. The regular `tariboy transcript` command instead invokes the
existing session parser. The implementation now includes a prerequisite to
reuse that parser for readable judge evidence, preserving original bundle hashes.
Snapshot usage is also zero in the released implementation, so these zeroes
must not be treated as proof that no request ran.

The run completed with all eight analyses and summary
`8a0ca47f-5a76-4193-8286-ba822f3f0b9a`, submitted by Codex/sol in
`judge-lead-20260906061154-1`. Coverage is four targets/eight analyses;
no improvement proposals were created. Sol returned uncertain/pass/pass/pass
in seed-table order; Terra returned uncertain for all four. All eight analyses
reported zero violations. The released consensus classified all four targets
as disputed, including the uncertain/uncertain pair; that is a separate
deterministic consensus defect covered by the planned correction.

Recorded proxy cost for the four judge iterations (run creation, two workers,
summary) totals approximately $3.8822 across 87 transcript calls: sol worker
$2.6573/40 calls, terra worker $0.7181/32 calls, and lead $0.5067/15 calls.
These are recorded operational costs, not current model-price quotations or a
controlled efficiency benchmark. Manual payload recovery and CLI discovery
are part of this baseline overhead.

## Candidate image application check

The installed CLI built `llm-as-judge:reliability-20260906-r1`, digest
`3f367862a3504d34b46214513463116814cbc102c30e15502f9c704d03023ee0`,
from the branch's rubric, instructions and skills. A local scratch manifest
resolves those inputs by absolute path because the released versioned Store
does not contain the new rubric. Its ordered layers match the canonical image;
the build source is host-bound, while the built image embeds its inputs.

The first create attempt was rejected without a run: the operator supplied
`explicit_ids` instead of the wire field `iteration_ids`. Correcting that field
created `7ac51ea1-779a-4a67-9ddd-ec2005403c3e` with the same four targets
and two independent assignments each. No target selection was broadened.

This check exposed a further reasoning failure: Terra decoded a title-generation
record and returned pass for each iteration on that basis, redefining the main
objective as the auxiliary request. Such support is inadequate even when the
actual main-task verdict might also be pass. Sol distinguished the auxiliary
request, but sometimes interpreted limited retrieval as missing captured data.
The normal operator transcript confirms that Bob's older iteration also has
main-task actions: it resumes TARI-41, reads unresolved wait 116 and signals
completion. For the recent Jack target, the immutable evidence API also resolves
the later action record `air-6fa64b9e1ab3b0e0a3fda5f2`; it was not absent
from the bundle.

The canonical rubric was narrowed accordingly: identify the main objective from
the assembled prompt and inspect its action/result records before pass; an
auxiliary title result alone requires uncertainty about the main task. Image
`llm-as-judge:reliability-20260906-r2`, digest
`6871c9ebd6b0d2a9e1089ccdf7ba528cfbe56c25a538f217b0c1d50a4695dd8e`,
contains this correction and was used in a one-target/two-model application check.

The r1 run completed with all eight analyses and summary
`ef19070e-5bef-4dd5-857c-b4f2ec13dd7e`. In seed-table order, Sol returned
uncertain/pass/fail/pass, while Terra returned pass for all four. Sol's recent
Bob failure alleges an omitted required workdir skill, despite otherwise valid
stale-notification handling; this individual allegation has not been independently
validated here. Model disagreement is not itself proof that either verdict is
correct.

The r2 application run is `887bb945-42f9-49b7-9320-4194c4649432`,
covering the older Bob iteration with both models. Both returned uncertain;
Terra now identifies the main TARI-41 task rather than passing title generation.
Both explanations still overstate absence of main-task evidence. The immutable
bundle contains six transcript records, not one. Operator `judge evidence` also
successfully resolves `air-00b7abc9c9b7a9b6ad644b7a`, whose decoded request
contains TARI-41 (113,386 characters). The regular transcript exposes the main
task actions. Thus the prompt correction addresses the wrong
objective, but does not by itself solve the released reader's retrieval problem.
The run completed with both analyses and summary
`9fa354ba-c60a-4f12-a8de-9ad9fa300afe`. All three bounded operational runs
are complete; no improvement proposals were requested or created by these runs.

These checks still use the released daemon's encoded evidence interface. They
do not validate the branch's new reader in production and do not establish
calibrated judge accuracy. The branch reader and submission contract have
deterministic, isolated regression tests; deployment is a separate step.

## Implementation verification and deployment boundary

At source commit `850220a`, `make check` passed all backend and frontend gates
(147 and 114 seconds respectively), and `make build` succeeded. Both local CLI
version forms report 0.47.0. Final code review has no remaining findings after
the transcript serialization and changed-history regression fixes.

Installed binaries remain unchanged. The user authorized Make-based installation,
but the managed installer reuses existing version directories; a local version
cut also refreshes the non-judge `basic:latest` image. Specific approval for
0.48.0 and that image update remains pending. Only judge images and judge agent
configuration have changed. Judge agents are enabled with automatic loops
paused; historical runs and their backlog remain intact. Developer agents and
their images have not been changed.
