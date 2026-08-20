-- Idle-autostop self-declaration wiring (idle-autostop epic, Task 1).
-- `productive` marks whether an iteration did useful work: an agent self-declares
-- an idle pass with `i-am-done --idle` (productive=0). A plain `i-am-done` and a
-- no_i_am_done iteration both leave it at the default 1, so only an explicit idle
-- declaration is ever non-productive. Owned by SetIterationDone alongside done_flag.
ALTER TABLE iterations ADD COLUMN productive INTEGER NOT NULL DEFAULT 1;

-- `max_idle_iterations` is the agent's auto-stop threshold, consumed by the
-- idle-stop policy (Task 2); 0 means "never auto-stop on idle". Added here so both
-- columns live in one migration. Its setter/reconcile land in later tasks.
ALTER TABLE agents ADD COLUMN max_idle_iterations INTEGER NOT NULL DEFAULT 0;
