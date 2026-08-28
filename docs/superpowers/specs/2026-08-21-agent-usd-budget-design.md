# Agent USD Budget Design

## Summary

Tariboy will let an operator configure four independent USD limits for each
agent: calendar hour, day, week, and month. Each limit defaults to `0`, which
means unlimited. Before forwarding an AI-proxy request, the daemon will total
the calling agent's immutable request costs in every enabled calendar window;
if any total has reached its configured limit, it will deny the request.

The existing global and group budget behavior remains unchanged. Agent budgets
are a dedicated persisted resource rather than an encoding inside the existing
single-row-per-scope budget model.

## Goals

- Configure hour, day, week, and month USD limits per agent in Configuration.
- Enforce all non-zero limits simultaneously in the AI proxy.
- Use calendar-aligned windows: the current hour, local calendar day, ISO week
  (Monday start), and calendar month. Each resets at its boundary.
- Treat an agent as **out of budget** when one or more enabled limits are
  exhausted.
- Show the exhausted window and consumed USD clearly in the agent header.
- Show current spend beside every configured input as `<spent> / <budget>` in
  both the header and Configuration.

## Non-goals

- Altering global or group budget semantics, API, or storage.
- Carrying unused allowance between calendar periods.
- Estimating or reserving the cost of a request before its immutable request
  cost is available; enforcement follows the existing request-cost accounting
  model.
- Changing provider pricing, Usage history, agent lifecycle, or task
  attribution.

## Persistence and API

Add a dedicated `agent_budgets` row keyed by agent name. It stores four
non-negative USD limits: `hour_usd`, `day_usd`, `week_usd`, and `month_usd`.
Absent rows read as all-zero limits for backward compatibility; saving any
limit creates or updates the row. Deleting an agent removes its row through the
existing agent ownership cleanup.

The agent read projection gains a single `budget` object containing the four
configured limits, each window's current-period spend, and a derived
`exhausted` collection. All reader surfaces (agent list, agent inspect/status,
and the Desktop's header/configuration reads) consume this common projection.
Configuration updates accept all four limits as one validated, atomic request;
negative, non-finite, and malformed USD values are rejected before persistence.

## Calendar accounting and enforcement

For one request, derive the window starts from the daemon's current local time:

| Window | Start |
| --- | --- |
| Hour | beginning of the current local hour |
| Day | local midnight |
| Week | local Monday at midnight (ISO week) |
| Month | first local day of the month at midnight |

The budget query sums persisted `ai_requests.cost_usd` for the authenticated
agent at or after each start. A zero limit is omitted from enforcement. If a
non-zero window has spent an amount greater than or equal to its limit, the
proxy returns the established budget-denial response before upstream provider
access and identifies the exhausted window(s). This denial is deterministic
from durable request history and current configuration; no stale UI cache is an
authority for proxy enforcement.

After a successful proxied call records its immutable cost, future requests
observe the updated totals. A single call may pass immediately before its cost
is recorded and cause the next request to be denied; this is consistent with
the current cost-accounting boundary and avoids speculative pricing.

## User experience

Configuration adds an **Agent budgets (USD)** section with four labelled
inputs: Hour, Day, Week, and Month. Each row renders the live current-period
value as `<spent> / <budget input>`; a zero input is labelled Unlimited. The
section saves all limits together and preserves the existing explicit-host
target for reads and writes.

An agent with any configured limit shows a compact budget block in its header,
using the same four rows and spent values. When a limit is exhausted, the
header has an explicit **Out of budget** status naming every exhausted period
and its `spent / limit` amount. The Agents sidebar/list consumes the same
derived status rather than independently recomputing it.

For older daemons that do not supply the additive projection, the Desktop
keeps its current compatible rendering and does not invent a budget state.

## Error handling and safety

- Proxy denial never contacts the upstream provider and preserves existing
  loopback token, audit, and redaction boundaries.
- Missing historical cost rows behave as zero spend; existing immutable costs
  are never recalculated when a limit changes.
- Store migrations are additive and preserve existing budget rows and Usage.
- Every test daemon uses isolated base/runtime directories and disables its
  HTTP listener; no verification touches a live daemon.

## Testing

Focused Go tests cover migration/defaults, validation, each calendar boundary,
multi-window denial, zero/unlimited behavior, and the shared agent projection.
API tests verify atomic update/read contracts. React tests verify configuration
and header/sidebar states, including multiple exhausted periods. Documentation
describes the new per-agent calendar-budget behavior and zero default.
