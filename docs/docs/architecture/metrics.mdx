---
title: Metrics
description: Task, agent, image, workflow and server metrics that the daemon computes on request from tasks, workflow visits, AI requests, iterations and assessments.
sidebar:
  label: Metrics
  icon: chart-bar
---

The metrics commands report how long tasks take, how many iterations, tokens
and US dollars they use, how often they come back for rework, and how the
assessments score them. The daemon computes every value when it receives a
request. It stores no aggregate and adds no table. The code is in
`internal/metrics`. The commands are in `internal/commands/metrics.go`.

| Command | Route | Result type |
| --- | --- | --- |
| `tariboy metrics task KEY` | `GET /api/metrics/tasks/{key}` | `TaskMetrics` |
| `tariboy metrics server` | `GET /api/metrics` | `MetricsReport` |

Access is the same as for `tariboy usage`. The daemon socket and the loopback
HTTP listener of the operator serve the commands. The tools socket of an agent
does not serve them. See [the command reference](/docs/reference/commands#metrics)
for the flags and examples.

## Units

- A time is a whole number of seconds in JSON. Its field name ends in `_s`.
  The text output of the CLI shows hours and minutes, for example `12h 05m`.
- A cost is in US dollars, rounded to 6 decimal places, as in `tariboy usage`.
- Tokens are counted by kind: `input`, `output`, `cache_write`, `cache_read`.
- A value that is not known is `null`, not zero. This applies to
  `lead_time_s` of a task that is not done, to the lead times of a slice
  without done tasks, and to `quality` without scores.

## Metrics of one task

`metrics task` accepts the current key of a task or an old key. Old keys come
from `task_key_aliases`. All metrics of the task use the current key and all
old keys.

| Field | Unit | Definition | Source |
| --- | --- | --- | --- |
| `lead_time_s` | seconds | From the first entry to `in_progress` to completion. `null` if the task is not `done` or has no start. | `tasks.started_at` and `tasks.completed_at`. If `started_at` is empty, the first `task_events` row that sets `status` to `in_progress`. |
| `status_time_s` | seconds per status | The sum of the time in each status. An open interval counts up to the time of the request. `done` and `cancelled` do not count. | A workflow task: `task_status_visits` (`left_at` − `entered_at`). A task without a workflow: the `task_events` rows that set `status`, with `task.created` or `task.imported` as `open`. |
| `usage.iterations` | count | The number of distinct iterations of the task. A request without an iteration id adds tokens and cost, but no iteration. | `COUNT(DISTINCT iteration)` of `ai_requests` with `task_id` equal to a key of the task |
| `usage.tokens` | tokens | Token sums by kind. | The same `ai_requests` rows |
| `usage.cost_usd` | US dollars | The cost sum. | The same `ai_requests` rows |
| `rework_rounds` | count | The number of visits that ended with `changes_requested`, `rework` or `conflict`. | `task_status_visits.outcome` |
| `rework_by_outcome` | count per outcome | The same count for each of the three outcomes. | `task_status_visits.outcome` |
| `rejected_advances` | count | The number of `advance` requests that a check rejected. | Sum of `task_status_visits.rejected_requests` |
| `customer_questions` | count | The number of waits for an answer of the customer. | `task_waiting_for` rows with `expected_principal` equal to `tasks.customer` |
| `quality` | score from 0 to 100 | For each axis `image_process`, `role`, `task_fit`: `avg`, `min` and `count` of the scores. An axis without scores is `null`. Without any score, `quality` is `null`. | `assessment_axes` with `verdict = scored` of the [latest assessment](#latest-assessment) of each iteration and task with the task's key |
| `agents[]` | | For each agent: `usage` and `images[]`, the image versions of that agent's iterations. | `ai_requests.agent` |
| `images[]` | | The iterations, tokens and cost on each image version: `image`, `image_ref`, `image_version`, `image_digest`. | See [Image version of an iteration](#image-version-of-an-iteration) |
| `workflows[]` | | The workflow versions of the task's visits: `name`, `version`, `digest`. Empty for a task without a workflow. | `task_status_visits.workflow_digest` → `task_workflow_images` |

A task without a workflow has no visits. Its `rework_rounds` and
`rejected_advances` are 0.

### Latest assessment

An assessment is never changed. A reassessment adds a new record. `quality`
uses only the latest record of each assessed object, so the scores of a
replaced record do not count. `tariboy assessment ls` uses the same rule, see
[assessments](/docs/plugins/built-in/assessments#latest-assessment).

- The object of an iteration assessment is its `iteration_id`.
- The object of a task assessment, with an empty `iteration_id`, is its
  `task_key`.

An assessment of an iteration and an assessment of its task are two objects.
The task's `quality` counts both. The rule that selects the latest record is
in [State model](/docs/architecture/state-model#assessments).

### Questions to the customer

`ttasks ask` and a question of the workflow engine create a `task_waiting_for`
row. The row names the principal who must answer. `customer_questions` counts
the rows that wait for the customer of the task. Only one wait for each
principal can be open on a task. A second question before the answer to the
first one makes no new row, so it counts as one question. The table
`task_workflow_questions` is not a source: no code writes it.

## Image version of an iteration

The image version comes from the iteration, not from the current
configuration of the agent.

1. The `iterations` row of `ai_requests.iteration` gives `image_ref`,
   `image_version` and `image_digest`. The image name is `image_ref` without
   its tag.
2. If retention removed the `iterations` row, `ai_requests.image_name` and
   `image_tag` give the reference and `ai_requests.image_digest` gives the
   digest. The version comes from another `iterations` row with the same
   digest.
3. If no row gives the version, the version is `unknown`. The iteration goes
   to the image slice with the version `unknown`.

## Workflow version of an iteration

The workflow version of an iteration comes from the visit of its task that
was open when the iteration started (`entered_at` ≤ start < `left_at`, or
`left_at` is empty). The start is `iterations.started_at`. Without an
`iterations` row, the start is the time of the first AI request of the
iteration. The visit's `workflow_digest` maps to `task_workflow_images.name`
and `version`.

- If no visit was open at the start, for example because the iteration
  started before the first visit, the current `tasks.workflow_digest` of the
  task gives the version.
- If `task_workflow_images` has no row for the digest, the workflow name is
  `unknown` and the version is empty.
- An iteration without a task, or with a task without a workflow, goes to the
  workflow slice `none`.

## Server report

`metrics server` reports one period. The default period is the 30 days before
the request. `--since` is an inclusive bound and `--until` is an exclusive
bound.

The report has `total`, `agents[]`, `images[]` and `workflows[]`. Each slice
has three blocks:

- `usage`: the iterations, tokens and cost of the AI requests with `ts` in the
  period. Each iteration goes to the exact agent, image version and workflow
  version of that iteration.
- `usage_without_task`: the part of `usage` whose requests have no task.
  A request without a task has a `task_id` that is `NULL` or empty. Such a
  request comes from an iteration without a selected Goal, or from an
  iteration before it ran `scripts/goal.sh set`. These requests go to the
  workflow slice `none`. The filters limit this block the same way as `usage`.
  A period without such requests gives zeros, not `null`.
- `tasks`: the aggregates of the tasks with `status = done` and
  `completed_at` in the period. The iterations, tokens and cost in this
  block are for the whole life of each task, not only for the period.

In `usage_without_task`, the tokens and `cost_usd` are an exact part of
`usage`, and `cost_usd` is never more than `usage.cost_usd`. `iterations`
counts the iterations with at least one request without a task. An
iteration that selected its Goal after its first requests counts in both
`usage.iterations` and `usage_without_task.iterations`. So you cannot
subtract `usage_without_task.iterations` from `usage.iterations` to get the
iterations with a task. The `tasks` block does not include requests without a
task. `tariboy metrics server` shows `usage_without_task.cost_usd` in the
column `NO_TASK_COST_USD`, after `COST_USD`.

| `tasks` field | Unit | Definition |
| --- | --- | --- |
| `tasks_done` | count | The done tasks of the slice |
| `mixed_tasks` | count | Image and workflow slices only: done tasks that ran on more than one version and also on this version |
| `lead_time_median_s`, `lead_time_avg_s` | seconds | Median and average of `lead_time_s`. `null` without a lead time. |
| `sum` | | Sums of `iterations`, `tokens`, `cost_usd`, `rework_rounds`, `rejected_advances`, `customer_questions` |
| `avg` | per task | The same values divided by `tasks_done`, rounded to 2 decimal places. Not in the response when `tasks_done` is 0. |
| `quality` | score | Per axis: average, minimum and count of the scores of the slice's tasks, from the [latest assessment](#latest-assessment) of each object. `null` without scores. |

### Tasks with several agents or versions

- Agent slices: a task counts in the slice of each agent that made an
  iteration for it. `total` counts each task once.
- Image slices: a task counts in a version slice only if all its iterations
  ran on that one version. A task with iterations on more than one version
  adds 1 to `mixed_tasks` of each of those versions. Its lead time, rework
  and questions go to no version slice. A task without iterations goes to no
  image slice.
- Workflow slices: the same rule, with the workflow versions of the task's
  visits. A task without a workflow goes to the slice `none`.

### Filters

`--agent`, `--image NAME[:VERSION]` and `--workflow NAME[:VERSION]` limit the
report:

- `usage` keeps only the iterations of that agent, on that image, under that
  workflow.
- `tasks` keeps only the done tasks with an iteration of that agent, with an
  iteration on that image, and with a visit under that workflow.
- The slice lists keep only the agent, the image versions and the workflow
  versions that the filter selects. A kept task adds nothing to the slices of
  other agents or versions. The version rule still uses all versions of the
  task, so a kept task with two versions adds to `mixed_tasks`.

An image name can include a registry port, as in
`--image localhost:5000/dev:1.0`. The version follows the last colon after
the last slash.

`--workflow none` selects the iterations and tasks without a workflow.
`--image NAME:unknown` selects the iterations without a known version. An
input that is not `NAME` or `NAME:VERSION` fails with `invalid_param`. A
`--since` or `--until` that is not an RFC 3339 time fails with `bad_time`.

## Retention

Retention removes `ai_requests`, `iterations` and `assessments` rows older
than the retention period. It removes `task_events`, `task_status_visits`
and `task_waiting_for` rows only with their task. As a result:

- the iterations, tokens and cost of an old task are smaller than the real
  values, or 0;
- the image version of an old iteration can be `unknown`, which can make
  its task mixed;
- `quality` can be `null` although an assessment existed.

## Performance

The metrics read the database in one read-only transaction. `ai_requests`
has no index on `task_id`, so each request scans that table a small number of
times. On a copy of a database with 67,832 AI requests, `metrics server` for
30 days answered in less than 2 seconds.
