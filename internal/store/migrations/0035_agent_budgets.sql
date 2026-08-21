-- Dedicated per-agent calendar budget limits. Existing `budgets` rows retain
-- their rolling global/group/agent semantics; this resource is intentionally
-- separate so a missing row remains the backward-compatible unlimited state.
CREATE TABLE agent_budgets (
    agent_name TEXT PRIMARY KEY,
    hour_usd   REAL NOT NULL DEFAULT 0 CHECK(hour_usd >= 0),
    day_usd    REAL NOT NULL DEFAULT 0 CHECK(day_usd >= 0),
    week_usd   REAL NOT NULL DEFAULT 0 CHECK(week_usd >= 0),
    month_usd  REAL NOT NULL DEFAULT 0 CHECK(month_usd >= 0)
);
