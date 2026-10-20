# Workflow visibility design

Design agreed with the customer on 2026-10-20. The implementation plan is
`docs/superpowers/plans/2026-10-20-workflow-visibility.md`.

## Intent

A queue bound to a workflow image already has a complete engine: statuses with
one owner each, transitions gated by required artifacts and check scripts,
watch scripts, transition requests that end `rejected` or `failed` with a
message and a log, script runs with a verdict and an exit code, versioned
artifacts, visit counters, pauses, and the operator's `move`, `retry`,
`resume`, and `cancel`.

What was missing was the presentation. The Desktop task panel showed five flat
lists, so the customer could not tell where the task was in its workflow,
which scripts guard each step and what they verify, which artifacts the next
step needs, why a script or an agent failed and what to do about it, or how to
send the task back to the agent with new requirements. The queue list showed a
status pill and nothing of a task's trouble.

## Goals

- A customer opening a workflow task sees, in order: what is wrong and what to
  do about it; where the task is in its workflow and what each step needs;
  what may happen next; what the task produced; what ran.
- A customer can send a task back to an agent with one action and a message,
  from any status, and the agent reads the message in its goal.
- The queue list shows which workflow tasks need a person and where each is.
- A workflow author can describe each check, watch, and source in one line,
  and that line reaches the customer and the agent.

## Non-goals

- A new daemon operation for rework: the declared outcomes and the operator's
  move already express it; the panel chooses between them.
- A graph drawing of the status machine: a linear stepper in manifest order,
  with a return count where the task came back, is enough for the workflows
  the Store ships.
- A row menu in the task list: the row gains a step counter and a problem
  mark; actions stay in the task panel.
- Changing the script protocol, the limits, or the pause decisions.

## Decisions

- **Return to agent is UI-only.** Where the current status declares an
  outcome to a pool status and the customer may declare it (a customer
  status), the panel sends `advance` with the message. Everywhere else, and
  for a paused task, it sends the operator's `move` to the chosen pool status
  with the message as the reason. The targets are the pool statuses a declared
  outcome leads to, the pool statuses the task visited (newest first), and,
  when paused in a pool status, the status itself as a restart. The action is
  hidden on a closed task and on an unpaused pool status an agent holds.
- **The Workflowfile gains `description`** on `checks[]`, `watch`, and
  `sources[]`: one line of plain text, at most 200 characters, validated as
  `description_invalid`. The task view exposes it as `check_details` next to
  the script paths, with the resolved `run_as` and timeout; the Goal block and
  `ttasks workflow get` name a described check as `path (description)`. The
  Store's workflows receive descriptions in a separate change.
- **The task view describes every status**, not only the current one:
  `initial`, `instructions_path`, `transitions` with `requires` and resolved
  `checks`, `watch`, and the effective `limits`; terminal statuses carry
  `cancelled`. Visits carry their counters; a check run carries its
  `request_id`. All additive.
- **Every task JSON summarizes an open workflow task** for a list row:
  `workflow_step` (`index`, `total`) and `workflow_attention` (`paused`,
  `failed`, `rejected`, `script_failed`, or `idle`, with the first line of the
  message, the visit counter behind it, and the request and run ids), derived
  in the task select from the manifest and the open visit, so a page costs no
  extra query. The engine keeps them in step after a pause or a close it makes
  on a task it already read.
- **The panel's problem block picks one explanation**: a failed request
  (log open, Retry), a rejected request (who fixes it), a failed watch (log
  open), a pool with no agent free, or an idling holder, each with the
  script's own words, the counter against the limit, and an advice line. A
  pause keeps its banner; a pending request says nothing here.
- **Artifacts are grouped by need**: the ones an outcome of the current status
  requires come first, missing before set, each naming the outcome; the rest
  fold away. Runs carry the tone of their verdict.

## Constraints

- No new routes; every new field is additive and omitted when empty, so
  existing clients and the OpenAPI contract tests keep passing.
- The list cost stays bounded: one manifest `json_each` per open workflow row,
  two indexed probes by visit id, messages cut in SQL.
- Messages and logs stay text in Desktop; artifact values stay Markdown
  without raw HTML.
- The panel wraps on a phone: no horizontal scroll.

## Left to the builder

The exact wording of the advice lines, the icons, and the widths; the
Playwright scenario that drives a failing watch and a return to the agent
against an isolated daemon.
