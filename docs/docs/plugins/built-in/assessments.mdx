---
title: assessments
description: Let an assessor agent record an immutable assessment of an iteration or a task through its tools socket.
sidebar:
  label: assessments
  icon: clipboard-check
---

`assessments` lets an agent record an assessment. An assessment is an
immutable record of how well another agent did the work of one iteration or
one task: three axes with a score and the violations found. It is an optional
capability. The official `basic` image does not include it.

## Capability surface

Inside the agent, `tariboy assessment` uses `$TARIBOY_TOOLS_SOCKET`:

```bash
tariboy assessment add --file assessment.json
tariboy assessment add --file -        # read the JSON from standard input
tariboy assessment ls --iteration ITERATION-ID
tariboy assessment show as-20261102T081853Z-000001
```

`add` sends the JSON object to `POST /tools/assessment/add`. Only an agent
with the `assessments` capability can call that route. Without it, the route
returns `404 plugin_disabled`. The reads `GET /tools/assessment/ls` and
`GET /tools/assessment/show/{id}` are open to every agent. The capability
packages no skill and needs no runtime placeholder. The image's own skills
tell the agent how to assess.

The JSON object of `add` has these fields:

```json
{
  "rubric_version": "rubric-1",
  "iteration_id": "tariboy-dev-20261102082326-1",
  "task_key": "DEV-1",
  "summary": "Followed the process. Skipped one test.",
  "axes": [
    {"axis": "image_process", "verdict": "scored", "score": 80, "note": "one skipped test"},
    {"axis": "role", "verdict": "not_applicable"},
    {"axis": "task_fit", "verdict": "scored", "score": 95}
  ],
  "violations": [
    {"axis": "image_process", "description": "Skipped the failing test",
     "quote": "the test is not needed",
     "evidence": {"kind": "transcript", "iteration_id": "tariboy-dev-20261102082326-1", "locator": "event 42"}}
  ]
}
```

Give `iteration_id`, `task_key` or both. The evidence kind is `transcript`
(with `iteration_id` and `locator`), `task_event` (with `task_key` and
`sequence`) or `script_run` (with a `run_id` that starts with `srun-`).

The daemon writes the assessor fields from the socket identity and the
current iteration. It writes the image and workflow snapshots from the
iteration and task rows. Fields of that kind in the request are discarded.
The validation rules, the link rule for an iteration and a task, and the
retention of assessments are in
[State model](/docs/architecture/state-model#assessments).

## Operator access

Operators read assessments with `tariboy assessment ls` and
`tariboy assessment show ID`, or with `GET /api/assessments` and
`GET /api/assessments/{id}`. The operator API and the operator CLI cannot
write an assessment: `tariboy assessment add` without
`TARIBOY_TOOLS_SOCKET` exits with code `2`.

## Related reference

- [Agent tools](/docs/binaries/agent-tools)
- [Operator CLI](/docs/binaries/operator-cli)
