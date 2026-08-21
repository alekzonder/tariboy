# Agent USD Budgets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add four independent, calendar-aligned USD limits to every agent and expose the shared budget projection in the proxy and Desktop UI.

**Architecture:** A new `agent_budgets` SQLite resource stores one set of hour/day/week/month limits per agent, without changing legacy global/group budgets. `aiproxy.Store` derives current calendar-window spend and exhaustion; the command layer attaches that projection to all agent reads and exposes an atomic configuration route. The proxy checks it immediately before forwarding, while the UI renders the server-provided projection in the agent header, sidebar status, and one Configuration section.

**Tech Stack:** Go 1.26, SQLite migrations, existing command registry/API, React/TypeScript, Vitest.

**Spec:** `outside/superpowers/specs/2026-08-21-agent-usd-budget-design.md`

## Global Constraints

- Existing global and group budgets retain their rolling-window API and behavior.
- `0` is the default and means unlimited; all four non-zero agent limits apply together.
- Agent accounting uses local calendar hour/day/ISO week/month boundaries and durable immutable `ai_requests.cost_usd` history.
- Proxy denial happens before upstream access and every test daemon has isolated base/runtime state and no live listener.
- No version bump or generated Desktop artifacts are committed.

---

### Task 1: Persist and derive agent budget status

**Files:**
- Create: `internal/store/migrations/0034_agent_budgets.sql`
- Modify: `internal/aiproxy/budget.go`, `internal/aiproxy/store.go`
- Test: `internal/aiproxy/budget_test.go`, `internal/aiproxy/store_test.go`, `internal/store/store_test.go`

**Interfaces:**
- Produces `AgentBudget`, `AgentBudgetStatus`, `Store.SetAgentBudget`, `Store.AgentBudgetStatus`, and calendar boundary helpers for commands and proxy code.

- [ ] **Step 1: Write failing store tests**

  Add tests that begin with no row and expect four zero limits, reject a negative/non-finite limit, and insert timestamped costs around the local hour/day/Monday/month boundaries. Assert current spend, ordered exhausted periods, and migration-owned table/index names.

- [ ] **Step 2: Run the focused tests and observe failure**

  Run: `go test ./internal/aiproxy ./internal/store -run 'Test(AgentBudget|StoreMigration)' -count=1`

  Expected: compile/test failure because the agent-budget API and migration do not exist.

- [ ] **Step 3: Implement the minimal durable model**

  Add an additive `agent_budgets(agent_name PRIMARY KEY, hour_usd, day_usd, week_usd, month_usd)` migration and agent-ownership cleanup. In `aiproxy.Store`, validate finite non-negative values, atomically upsert all four limits, calculate local calendar starts (Monday for week), sum `ai_requests.cost_usd` from each start for the named agent, and derive the exhausted non-zero windows.

- [ ] **Step 4: Run the focused tests and observe success**

  Run: `go test ./internal/aiproxy ./internal/store -run 'Test(AgentBudget|StoreMigration)' -count=1`

- [ ] **Step 5: Commit**

  `git add internal/store/migrations/0034_agent_budgets.sql internal/aiproxy/budget.go internal/aiproxy/store.go internal/aiproxy/budget_test.go internal/aiproxy/store_test.go internal/store/store_test.go && git commit -m "feat: persist agent USD budgets"`

### Task 2: Expose and enforce the common projection

**Files:**
- Modify: `internal/aiproxy/budget.go`, `internal/aiproxy/proxy.go`, `internal/commands/agents.go`, `internal/commands/daemon.go`
- Test: `internal/aiproxy/budget_test.go`, `internal/aiproxy/forward_test.go`, `internal/commands/agents_test.go`

**Interfaces:**
- Consumes `Store.AgentBudgetStatus(agent, now)`.
- Produces `GET/POST /api/agents/{name}/budget`, an additive `budget` property on every agent projection, and agent-specific proxy budget denial.

- [ ] **Step 1: Write failing API and proxy tests**

  Cover atomic four-limit update/read, malformed input rejection without changing persisted limits, absence-as-zero projection, list/read status propagation, and a request blocked when any active calendar period is exhausted without calling the upstream transport.

- [ ] **Step 2: Run focused tests and observe failure**

  Run: `go test ./internal/commands ./internal/aiproxy -run 'Test(AgentBudget|Proxy.*AgentBudget)' -count=1`

- [ ] **Step 3: Implement the command and proxy integration**

  Register agent budget read/save handlers, validate all four submitted fields before `SetAgentBudget`, append the shared projection in `agentView`/list/status readers, and make `BudgetCache.Check` include named exhausted agent periods alongside legacy decisions. Preserve existing legacy cache behavior and denial response/security boundaries.

- [ ] **Step 4: Run focused tests and observe success**

  Run: `go test ./internal/commands ./internal/aiproxy -run 'Test(AgentBudget|Proxy.*AgentBudget)' -count=1`

- [ ] **Step 5: Commit**

  `git add internal/aiproxy/budget.go internal/aiproxy/proxy.go internal/aiproxy/budget_test.go internal/aiproxy/forward_test.go internal/commands/agents.go internal/commands/agents_test.go internal/commands/daemon.go && git commit -m "feat: enforce per-agent USD budgets"`

### Task 3: Render and edit agent budgets in Desktop UI

**Files:**
- Modify: `ui/src/lib/types.ts`, `ui/src/pages/agents/AgentConfigurationTab.tsx`, `ui/src/pages/agents/AgentWorkspace.tsx`, relevant shared agent-list component
- Test: `ui/src/pages/agents/AgentConfigurationTab.test.tsx`, `ui/src/pages/agents/AgentWorkspace.test.tsx`, relevant agent-list test

**Interfaces:**
- Consumes the additive `AgentView.budget` API projection.
- Produces a host-pinned `Agent budgets (USD)` configuration section, header budget rows, and an explicit `Out of budget` list state.

- [ ] **Step 1: Write failing Vitest cases**

  Add a configuration test that reads four values/spends, submits one atomic `POST` to the selected route host, labels zero as Unlimited, and renders multiple exhausted periods. Add header/sidebar tests that show configured spend and derived out-of-budget status without recomputing it client-side.

- [ ] **Step 2: Run the focused UI tests and observe failure**

  Run: `cd ui && npm test -- AgentConfigurationTab AgentWorkspace`

- [ ] **Step 3: Implement minimal UI rendering**

  Extend the agent type with optional budget projection for old-daemon compatibility. Add a section-local four-input draft and save action to Configuration, preserving route-host targeting and existing dirty-section behavior. Add the same display rows and explicit exhaustion copy to the agent header and list using only daemon-provided data.

- [ ] **Step 4: Run the focused UI tests and observe success**

  Run: `cd ui && npm test -- AgentConfigurationTab AgentWorkspace`

- [ ] **Step 5: Commit**

  `git add ui/src/lib/types.ts ui/src/pages/agents/AgentConfigurationTab.tsx ui/src/pages/agents/AgentConfigurationTab.test.tsx ui/src/pages/agents/AgentWorkspace.tsx ui/src/pages/agents/AgentWorkspace.test.tsx && git commit -m "feat: show agent USD budgets"`

### Task 4: Document and verify the completed behavior

**Files:**
- Modify: `docs/docs/architecture/ai-proxy.mdx`, `docs/docs/architecture/web-ui.mdx`, `docs/docs/security-controls.mdx`

- [ ] **Step 1: Document the dedicated calendar-budget resource**

  State the zero/unlimited default, four calendar windows, multi-window proxy block semantics, shared agent projection, and no-upstream-on-denial boundary.

- [ ] **Step 2: Run documentation validation**

  Run: `cd docs && npm run doctor && npm run build`

- [ ] **Step 3: Run final branch verification**

  Run: `make check && git diff --check`

- [ ] **Step 4: Inspect and commit**

  `git add docs/docs/architecture/ai-proxy.mdx docs/docs/architecture/web-ui.mdx docs/docs/security-controls.mdx && git commit -m "docs: describe agent USD budgets"`
