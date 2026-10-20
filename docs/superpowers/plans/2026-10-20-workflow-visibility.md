# Workflow visibility, error explanation, and return-to-agent

## Context

Queues bound to a workflow image (`docs/docs/task-workflows.mdx`) already have a
complete engine: statuses with one owner each, transitions gated by required
artifacts and check scripts, watch scripts, transition requests
(`rejected`/`failed` with message and log path), script runs with verdict and
exit code, versioned artifacts, visit counters, pauses with a decision comment,
and operator `move`/`retry`/`resume`/`cancel`.

What is missing is the presentation. In the Desktop task panel
(`ui/src/pages/tasks/WorkflowPanel.tsx`) all of this is five flat lists:
Outcomes, Artifacts, Script runs, Visits, plus a pause banner. The owner's
complaints, confirmed by exploration:

1. **No map.** Nothing shows where the task is in the workflow, which scripts
   run at each step, what they do, or which artifacts are needed to leave the
   current status. Artifacts are a flat list (`WorkflowArtifacts.tsx`) with no
   link to the outcome that requires them. The queue row (`TaskRow.tsx:214`)
   shows only a status pill and a `customer`/`script`/`paused` chip; the task
   JSON carries no summary of problems or progress.
2. **Errors are opaque.** A rejected or failed request is a red box with the raw
   `result_message` (`WorkflowOutcomes.tsx` `OutcomeNotice`); runs show
   `state verdict exit N` as plain mono text with no tone (`RunRow.tsx`), and the
   log must be opened by hand. Nothing says *what kind* of failure it is
   (check rejected vs script broken vs timeout vs agent idle vs pause) or what to
   do. Visit counters and limits ("2 of 5 rejections") are not exposed by the
   API. The manifest has no way to describe what a script does.
3. **No obvious "send back to the agent".** The engine supports it three ways
   (declared back-outcome such as `changes_requested`; the `rework` artifact of
   the store's `review` status; operator `move` with a reason that reaches the
   agent as the transition message in its Goal block,
   `internal/loop/workflow_goal.go:114-128`), but none looks like a rework action.

Decisions taken with the owner:

- **Return to agent is UI-only** on top of `advance` (customer status with a
  declared outcome to a pool status) or `move` (everything else). No new route.
- **Workflowfile gets an optional `description`** on `checks[]`, `watch`, and
  `sources[]`. Store workflows get descriptions in a follow-up commit in
  `tariboy-store`.
- **The queue row gets an attention indicator and a step counter**, backed by a
  new summary in the task JSON computed in the same plan.
- **The map is a linear stepper** of statuses in manifest order, with a compact
  variant in the row.

Intended outcome: a customer opening a workflow task sees in one glance where it
is, what must happen to move on, whether something is wrong and what to do,
and can send it back to the agent with one action; the queue list shows which
tasks need attention.

## Phases

Each phase ships on its own. Backend first, UI on top. Every phase: failing
tests first, then code, then docs in the same commit.

### Phase 0 — Spec and plan in the repo

Write `docs/superpowers/specs/2026-10-20-workflow-visibility-design.md`
(intent, goals, non-goals, decisions above, the problem table from Phase 4)
and `docs/superpowers/plans/2026-10-20-workflow-visibility.md` (this plan,
detailed). These directories need no docs build.

### Phase 1 — `description` on checks, watch, sources

**Files**
- `internal/workflowfile/workflowfile.go`: `Description string
  \`yaml:"description,omitempty" json:"description,omitempty"\`` on `Check`,
  `Watch`, `Source` (same as `Artifact.Description`).
- `internal/workflowfile/validate.go`: `description_invalid` when the value has
  more than 200 runes or contains a control character (one line, plain text).
  Report at `statuses[i].transitions[j].checks[k].description`,
  `statuses[i].watch.description`, `sources[i].description`. Artifact
  descriptions stay unvalidated (store uses multi-line text there).
- `internal/tasks/workflow_view.go`: new `CheckView{script, description,
  run_as, timeout}` (resolved defaults from `internal/workflowfile/limits.go`)
  and `OutcomeView.CheckDetails []CheckView \`json:"check_details,omitempty"\``.
  Keep `Checks []string`. Helper `checkView(check)` reused in Phase 2.
- `internal/tasks/workflow_sources.go` (`QueueSource`): `description`.
- `internal/loop/workflow_goal.go` `goalOutcomes`: render
  `checks: ./scripts/x.py (description)` from `CheckDetails` when present;
  unchanged output when absent so the golden test stays byte-identical. Pass
  descriptions through `oneLine`.
- `internal/commands/tasks_openapi.go`: `CheckView` schema, `check_details`,
  `QueueSource.description`.
- `internal/taskcli/workflow.go` `printWorkflow`: `checks: path (description)`.

**Tests (RED first)**: `workflowfile_test.go` parse fixture;
`validate_test.go` `TestValidateCodes` (201 runes, newline → `description_invalid`;
200 runes accepted); `internal/workflowimage/store_test.go` inspect passthrough;
`internal/tasks/workflow_view_test.go` `check_details` via `editManifest` +
`jsonFields`; `internal/loop/workflow_goal_test.go` case with description;
`internal/commands/tasks_workflow_test.go` OpenAPI registry.

**Docs**: `docs/docs/workflow-images.mdx` (Status fields: `watch`/checks
`description`; Sources table; Validation table `description_invalid`),
`docs/docs/task-workflows.mdx` ("The task view" `outcomes` row; Goal block
Outcomes line).

Manifests are stored as JSON (`task_workflow_images.manifest`) and unmarshalled
per call by `loadManifestTx` (`workflow_model.go:106`), so the field flows into
`GET /api/workflow-images/{name}/{tag}` and `WorkflowView` with no extra
plumbing. There is no manifest cache.

### Phase 2 — `WorkflowView` exposes the full graph, counters, limits

**Files**: `internal/tasks/workflow_view.go`, `workflow_model.go`
(`StatusVisit`), `workflow_runs.go` (`ScriptRun`), `tasks_openapi.go`,
`ui/src/lib/tasks.ts` (types).

Additive fields, all `omitempty`, so the existing JSON assertion on a terminal
status (`{"id","owner","terminal"}`) still holds:

```go
type WatchView struct { Script, Description, Every, Timeout string }          // timeout resolved
type TransitionView struct { On, To string; Requires []string; Checks []CheckView }
type LimitsView struct { IdleIterations, RejectedRequests, ScriptFailures int; UnavailableGrace string }
type StatusView struct {
    ID string; Owner string; Terminal bool                     // unchanged
    Initial bool; Cancelled bool; InstructionsPath string      // omitempty
    Watch *WatchView; Transitions []TransitionView; Limits *LimitsView // nil for terminal
}
// StatusVisit += idle_iterations, rejected_requests, script_failures (omitempty)
// ScriptRun.RequestID: json:"-" → json:"request_id,omitempty"
```

`Limits` comes from `manifest.Definition.StatusLimits(id)` (`limits.go:34`).
`statusVisitsTx` selects the three counters. The UI derives the current visit
(no `left_at`) and "2 of 5 rejections" from `visit.rejected_requests` /
`statuses[current].limits.rejected_requests`. `request_id` on runs lets the UI
pair a failed request with its run and log.

**Tests (RED first)**: `workflow_view_test.go` `TestGetWorkflowDescribesEveryStatus`
(pool status with transitions/checks/limits; script status with watch and
resolved timeout; terminal status JSON unchanged), counters after a rejection
(`requestFixture` + rejecting check), update the `wantStatuses` literal in
`TestGetWorkflowListsDeclaredArtifactsAndStatuses`; `workflow_runs_test.go`
check run has `request_id`, watch run omits it; OpenAPI registry test.

**Docs**: task-workflows.mdx "The task view" rows `statuses`, `visits`, `runs`.

### Phase 3 — `workflow_attention` and `workflow_step` on the task JSON

Computed in SQL through the single join point every task select uses:
`TaskWorkflowColumns` / `TaskWorkflowJoin` / `WorkflowScanTargets` /
`finishWorkflowFields` (`internal/tasks/workflow_model.go:63-104`; consumers:
`store.go:18`, `internal/taskgoal/store.go:33`). No N+1, no post-pass.

Join the open visit (`LEFT JOIN task_status_visits cv ON cv.task_id = t.id AND
cv.left_at = ''`; exactly one open visit per task, `enterStatusTx` closes before
it inserts) and add, for open workflow tasks only:

- the current status's manifest index and owner kind (`json_each` over
  `$.definition.statuses`; fold the existing owner-kind column into it so the
  manifest is parsed once per row) and `json_array_length` for the total;
- `cv.idle_iterations`, `cv.rejected_requests`, `cv.script_failures`;
- the latest request of the open visit (`id, state, outcome,
  substr(result_message,1,400)`) via `idx_task_transition_requests_visit`;
- the latest finished run of the open visit (`id, kind, verdict,
  substr(message,1,400)`) via `idx_task_script_runs_visit`.

```go
type WorkflowAttention struct {
    Kind      string `json:"kind"`                 // paused | failed | rejected | script_failed | idle
    Message   string `json:"message,omitempty"`    // first line, CutRunes 200 + CutMarker
    Count     int    `json:"count,omitempty"`      // the visit counter behind kind
    RequestID int64  `json:"request_id,omitempty"` // failed, rejected
    RunID     int64  `json:"run_id,omitempty"`     // script_failed, failed (when paired)
}
type WorkflowStep struct { Index int `json:"index"`; Total int `json:"total"` } // 1-based, manifest order
// Task += WorkflowAttention *WorkflowAttention `json:"workflow_attention,omitempty"`
//         WorkflowStep      *WorkflowStep      `json:"workflow_step,omitempty"`
```

Precedence in `finishWorkflowFields`: `paused` (message from
`pauseReasonText`, `workflow_pause.go:36`) > `failed` (latest request of the
open visit is `failed`; message = first line of `result_message`, which drops
the `log:` suffix) > `rejected` > `script_failed` (latest finished run is a
watch with verdict `failure`) > `idle` (`idle_iterations > 0`). Closed tasks and
flexible tasks get neither field. `Task.UnmarshalJSON` clears both pointers
with the other workflow fields. Not covered: `holder_unavailable` during the
grace period (the pause covers it) and `pending` (the panel shows it).

**Tests (RED first)**: `workflow_model_test.go` table test (paused beats
failed; failed beats rejected; rejection followed by an applied request → nil;
closed → nil; flexible → fields absent); `workflow_step` `{index:2,total:4}`;
`TestTaskJSONRoundTrip` / `model_json_test.go` for the cleared pointers; an
`EXPLAIN QUERY PLAN` test on `taskSelect` failing on `SCAN cv` / `SCAN r`
(json_each legitimately reports `SCAN s VIRTUAL TABLE`); `service_test.go`
list page carries `workflow_attention.kind == "paused"`; OpenAPI `Task` schema
gains both fields (additive, `tasks_test.go:114` contract stays).

**Docs**: task-workflows.mdx new subsection "Attention summary" under "Status
and category" + REST routes note; `docs/docs/architecture/state-model.mdx`
only if a column changes (none planned).

### Phase 4 — Task panel redesign

All under `ui/src/pages/tasks/`. Reuse `panelStyles.tsx` (`LABEL`, `MONO`,
`ROW`, `DANGER_FILL`, `PRIMARY_FILL`, `QUIET_ACTION`, `SelectShell`),
`StatusPill`/`StatusDot` (`components/ui/status.tsx`), tones
(`lib/statusTone.ts`), `useWorkflowConfirm`, radix `tooltip.tsx`, `Textarea`,
`MarkdownContent`. Tone bar precedent: `components/AuditRow.tsx`.

New pure modules (vitest, no DOM):
- `workflowGraph.ts`: `currentVisit(view)`, `visitedStatuses(view)` (set +
  back-edge count: a visit whose manifest index is lower than the previous
  visit's), `stepOf(view)`, `requiredBy(view)` → `Map<artifact,
  {on,to,missing}[]>`, `returnTargets(view)`, `counterText(view, kind)`.
- `workflowProblem.ts`: `explainProblem(view): Problem | null` with
  `{kind, tone, title, detail?, advice, request?, run?, counter?}`.

| Signal | Tone | Title | Advice | Actions |
| --- | --- | --- | --- | --- |
| pause `idle_iterations` | danger | Paused: the agent made no progress | "N of N iterations ended without a transition request. Add detail to the task or artifacts, then Continue; or Release the holder." | Continue, Release (pool), Return to agent, Cancel |
| pause `rejected_requests` | danger | Paused: checks kept rejecting | last `result_message`; "Fix what the check names or tell the agent what to change, then Continue." | same |
| pause `script_failures` | danger | Paused: a script keeps failing | last failure message; "The script could not run: image, secrets, or environment. Fix the cause, then Continue." | Continue, Release, Cancel; failing run log auto-expanded |
| pause `holder_unavailable` | danger | Paused: the agent cannot work | "Re-enable the agent and Continue, or Release the holder." | Continue, Release, Cancel |
| `last_request.state === "failed"` in current visit | danger | Check could not run: `outcome` | from the paired run (`run.request_id`): timeout → "timed out after {check.timeout}"; "exit N" → "exited with N"; `interrupted` → "the daemon restarted during the check"; "Repeating unchanged will not help. Read the log, fix the cause, then Retry." | Retry, run log auto-expanded, Return to agent |
| `last_request.state === "rejected"` in current visit | attention | Rejected: `outcome` | `result_message`; pool: "The agent sees this in its Goal and should fix it; {used} of {limit} rejections before the task pauses." customer: "Fix the artifact or send the task back." | Return to agent |
| latest run is a failed watch | danger | Watch script failed | message; "{used} of {limit} failures before the task pauses. Check the log and the queue's secrets." | run log |
| pool status, no holder, category `open` | attention | Waiting for an agent | "No agent of pool {name} is available. Add agents in queue settings." | — |
| pool status, `idle_iterations > 0` | quiet | No transition yet | "{used} of {limit} idle iterations." | — |
| `last_request.state === "pending"` | quiet | Checking… | existing polling notice | Refresh |

Components:
- `WorkflowStepper.tsx` (new): `<ol aria-label="Workflow steps">` over
  `view.statuses`; current has `aria-current="step"`, visited get a filled dot,
  terminal a square, back-edges a `↩ n` badge; `flex-wrap`, so phones never
  scroll horizontally. Hover/click opens a tooltip or inline detail with owner,
  `instructions_path`, watch `{script, description, every}`, transitions
  `on → to`, `requires`, checks with description/run_as/timeout. Export
  `WorkflowStepperCompact` for Phase 5.
- `WorkflowCurrentStep.tsx` (new card): "Step k of n · status", owner/holder;
  per outcome: target, required artifacts as ✓/missing chips, checks as
  `script — description`; hosts the existing `WorkflowOutcomes` buttons,
  message, and polling (keep that component and its tests; move
  `OutcomeNotice` out); primary actions: outcome buttons (customer),
  **Return to agent**, **Move to status…**, **Cancel task**.
- `WorkflowProblem.tsx` (new): renders `explainProblem(view)`; for `paused`
  embeds the decision buttons of `WorkflowPauseBanner` (keep it as the body,
  drop its outer fill); `RunRow` with new `autoLoad` prop for `problem.run`;
  Retry via `retryTransitionRequest`. Extract `useTransitionRequest(taskKey,
  target, {onApplied, onSettled})` from `WorkflowOutcomes.submit` into
  `workflowShared.ts` and share it with Problem and ReturnToAgent.
- `WorkflowArtifacts.tsx`: new prop `required` from `requiredBy`; groups
  "Needed now" (required by an outcome of the current status, missing first,
  with a `required by: on → to` chip) and "Other artifacts" (collapsed via
  `QUIET_ACTION`, expanded when nothing is needed). Every item shows its
  declared description plus author/time.
- `RunRow.tsx` / `WorkflowRuns.tsx`: `runTone(run)` in `workflowShared.ts`
  (`failure`/`reject` danger, `pass`/`outcome` live, `quiet` faint,
  pending/running attention, interrupted/cancelled quiet); verdict as
  `<StatusPill size="sm">`, `border-l-2` tone bar.
- `ReturnToAgent.tsx` (new): `returnTargets(view)` = pool-owned statuses from
  (a) declared outcomes of the current status whose `to` is a pool status
  (via `advance` when `view.owner === "customer"` and not paused, else `move`);
  (b) pool statuses in visit history, newest first, deduped (via `move`);
  (c) when paused in a pool status, the current status itself. Default = most
  recently visited pool status, else the first declared. Hidden when there is
  no pool status or the task is an unpaused pool status with a holder. Form:
  `SelectShell` grouped "By outcome" / "By move", required `Textarea` message
  (`maxLength 4096`), confirm via `useWorkflowConfirm` ("The agent receives
  your message in its Goal as the transition message"). Submit:
  `advanceTask(key, outcome, message, view.status)` + follow, or
  `moveTaskWorkflow(key, to, message)`; then `onChanged`.
- `WorkflowPanel.tsx`: order becomes Problem, Stepper, Current step,
  Artifacts, Runs, Visits; keep Move/Cancel and `eventSequence` refetch.

**Tests (RED first)**: extend `workflowFixtures.ts` (statuses with
transitions/limits, visit counters, `checkRun.request_id`, failed request +
run pair, paused view, no-holder view); `workflowGraph.test.ts`,
`workflowProblem.test.ts` (one case per table row + precedence);
`WorkflowStepper.test.tsx`; `WorkflowProblem.test.tsx` (Retry, auto-loaded
log, pause buttons); `ReturnToAgent.test.tsx` (default target; customer
back-outcome → `advanceTask("REL-1","rework","msg","review",target)`; script
status → `moveTaskWorkflow` with the message as reason; paused pool → current
status offered); update `WorkflowArtifacts.test.tsx`, `WorkflowRuns.test.tsx`
(`data-tone`), `WorkflowPanel.test.tsx`.

**Docs**: task-workflows.mdx "Desktop → The workflow panel" rewritten around
Problem / Stepper / Current step / Return to agent;
`docs/docs/architecture/web-ui.mdx` Tasks paragraph (components and client
calls).

### Phase 5 — Queue row

- `taskColumns.ts`: status column 110 → 148 (pill + `k/n` + 14px icon); phone
  keeps 84 with the step text `hidden sm:inline`, icon kept.
- `TaskRow.tsx` workflow branch: `[StatusPill][k/n title="Step k of n"]
  [attention]` from `task.workflow_attention`: `paused` keeps the danger pill;
  `failed`/`script_failed` → warning icon in `TONE_TEXT.danger`; `rejected` →
  icon in `TONE_TEXT.attention`; `idle` → nothing. Tooltip = `message`
  (+ count), `aria-label` "Problem: …". `waiting_on` chips stay when no icon.
- No row menu exists today (only `PriorityMenu`), so a row-level Return to
  agent is a follow-up, not part of this plan.
- **Tests**: `TaskRow.test.tsx` step text and icon; header/row widths equal;
  phone query hides the step text.
- **Docs**: `docs/docs/tasks.mdx` Desktop row paragraph; web-ui.mdx.

### Follow-up outside this repo

`tariboy-store`: add `description` to the checks, watches, and source of
`workflows/development`, `workflows/pr-review`, `workflows/research`, bump
their `workflow_version`, and run the store `make check`.

## Verification

- Per phase: `go test ./internal/workflowfile/ ./internal/workflowimage/
  ./internal/tasks/ ./internal/loop/ ./internal/commands/ ./internal/taskcli/`
  and `cd ui && npm test`.
- `make check` at the end of Phase 3 (backend) and Phase 5 (mixed).
- `scripts/workflow-e2e.sh` + `scripts/testdata/workflow-e2e/Workflowfile.yaml`:
  add `description` to `./scripts/check-env.sh` on `built` (not to
  `check-gate.sh`, whose Goal line is asserted verbatim at line 448); assert
  `outcomes[0].check_details[0].description`; after `reject_twice` assert
  `task.workflow_attention.kind == "paused"`, after one rejection `rejected`;
  assert `task.workflow_step.total`. Run `make workflow-e2e`.
- Browser suite `ui/tests/tasks-e2e.pw.ts`: new case after the existing
  workflow test with a fixture `wfui2`: `draft` (pool `writers`, check with a
  description) → `approval` (customer; `approved → probe`, `rework → draft`) →
  `probe` (script; watch exiting 1 every 1s, `limits: {script_failures: 1}`) →
  `done`. Assert: row `2/4`; stepper has 4 steps with `approval` current and
  the `draft` tooltip showing the check description; **Return to agent**
  defaults to `draft`, message confirmed → API status `draft`, visit message
  equals the text, problem block "Waiting for an agent", row `1/4`; **Move** to
  `probe` → within 10 s the row shows the red `paused` pill with the failure
  tooltip and the panel shows "Paused: a script keeps failing" with the run log
  auto-expanded; **Return to agent** now offers `draft` by move → status
  `draft`, pause cleared. (Agent-side advance from a pool status is not
  reachable from Playwright; `workflow-e2e.sh` and the vitest table cover it.)
- `make ui-dev` visual pass at 375px and 1280px (stepper wrapping, status
  column, tooltips). `make full-check` before handoff since the diff reaches
  e2e and the browser suites.
- `git diff --check`; no version bump (release-time policy).

## Risks

- **List cost**: one `json_each` over the manifest per open workflow row
  (today only `wait_customer` rows). Bounded by `json_array_length` and 400-byte
  substrings; the EXPLAIN test guards the visit/request/run probes. Fallback if
  profiling shows manifest parsing dominating: drop `workflow_step` from SQL
  and compute it in the row from a per-digest `getWorkflowImage` cache.
- **OpenAPI registry tests** (`tasks_workflow_test.go`, `tasks_test.go`) assert
  schema names and properties; each phase updates them in the same commit.
- **`WorkflowView.statuses` compatibility**: new fields `omitempty`, `Limits`
  nil for terminal statuses; `MoveForm`/`moveTargets` unchanged. The Go
  `DeepEqual` literal in the existing view test must be updated.
- **Goal block golden test** stays byte-identical because descriptions render
  only when present.
- **One-open-visit invariant** for the `cv` join: documented in the SQL
  comment and asserted in the Go test.
