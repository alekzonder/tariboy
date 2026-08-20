---
title: status
description: Publish an operator-visible progress message intended to stay on one line without completing the current iteration.
sidebar:
  label: status
  icon: activity
---

`status` gives an agent a small, operator-visible progress surface. Messages
are intended to stay on one line, although the API stores the supplied string
without enforcing that presentation convention. It is an optional capability
included in `basic:latest`.

## Capability surface

```bash
tools status
tools status set "running the focused regression test"
```

`tools status` returns the agent's live state, loop-enabled flag, current
iteration, message, and update timestamp. `status set` replaces only the
one-line message and timestamp. It does not finish the iteration, enable or
disable the loop, or change an error reason.

## State and history

The current message and RFC 3339 update time live on the durable agent record.
Unrelated configuration updates preserve them. Each agent-authored change is
also appended as a `status` event in that agent's audit timeline with the
current iteration ID when one exists.

The same current value is visible through the Desktop agent status and the
operator `tariboy agent status show` command. Historical values are available
through `tariboy agent status history`.

## Prompt integration

The Store prompt teaches agents when to update the line:

```yaml Tariboyfile.yaml
plugins:
  - name: status
prompts:
  - file: $CURRENT_VERSION_STORE/skills/status/prompt.md
```

There is no status runtime placeholder. The current status is read on demand
through `tools status`; enabling the capability does not inject it into the
prompt.

## Failure behavior

The capability gate prevents agents without `status` from reading or changing
the status API. An empty message is accepted and clears the visible line while
recording the update. A stale idle-limit message may be cleared when an agent is
started, but a genuine agent-authored status is preserved.

## Related reference

- [Agent tools](/docs/binaries/agent-tools)
- [Autopilot](/docs/autopilot)
- [Security and controls](/docs/security-controls)
