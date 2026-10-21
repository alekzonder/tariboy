package store

import "database/sql"

// Phase says which agent removal step deletes a table's rows.
type Phase int

const (
	// PhaseNone rows are not deleted by agent removal: they are kept or
	// removed by ON DELETE CASCADE.
	PhaseNone Phase = iota
	// PhaseCore rows are deleted by agent.Store.Delete.
	PhaseCore
	// PhaseData rows are deleted by agent.Store.PurgeAgentData.
	PhaseData
)

// AgentTable describes one table whose rows belong to a single agent.
type AgentTable struct {
	Name string
	// Where scopes rows to the agent with exactly one ? placeholder.
	Where string
	// Scope prefixes the agent name bound to Where, e.g. "agent:" for budgets.
	Scope string
	// AgentCol is the direct agent column, "" when scoped indirectly.
	AgentCol string
	// OrderBy is the stable sort key of a backup dump.
	OrderBy string
	// MaskCols are blanked in a backup unless secrets are included.
	MaskCols []string
	Phase    Phase
	// BackupOrder is the 1-based position in an agent backup; 0 = not backed up.
	BackupOrder int
}

// AgentTables is the single registry of agent-owned tables, in deletion order.
// A new table keyed by an agent needs a row here (or an exemption with a reason
// in the schema test).
var AgentTables = []AgentTable{
	{Name: "iterations", Where: "agent = ?", AgentCol: "agent", OrderBy: "id", Phase: PhaseCore, BackupOrder: 2},
	{Name: "secrets", Where: "agent = ?", AgentCol: "agent", OrderBy: "key", MaskCols: []string{"value"}, Phase: PhaseCore, BackupOrder: 4},
	{Name: "agent_budgets", Where: "agent_name = ?", AgentCol: "agent_name", Phase: PhaseCore},
	{Name: "agents", Where: "name = ?", AgentCol: "name", OrderBy: "name", Phase: PhaseCore, BackupOrder: 1},
	// deliveries reference subscriptions by id, so clear them first.
	{Name: "deliveries", Where: "subscription_id IN (SELECT id FROM subscriptions WHERE agent = ?)", OrderBy: "subscription_id, message_id", Phase: PhaseData, BackupOrder: 11},
	{Name: "subscriptions", Where: "agent = ?", AgentCol: "agent", OrderBy: "id", Phase: PhaseData, BackupOrder: 5},
	{Name: "schedules", Where: "agent = ?", AgentCol: "agent", OrderBy: "id", Phase: PhaseData, BackupOrder: 6},
	{Name: "scripts", Where: "agent = ?", AgentCol: "agent", OrderBy: "id", Phase: PhaseData, BackupOrder: 7},
	{Name: "script_result_outbox", Where: "agent = ?", AgentCol: "agent", OrderBy: "idempotency_key", Phase: PhaseData, BackupOrder: 9},
	{Name: "ai_requests", Where: "agent = ?", AgentCol: "agent", OrderBy: "id", Phase: PhaseData, BackupOrder: 3},
	{Name: "retention_policies", Where: "agent = ?", AgentCol: "agent", Phase: PhaseData},
	{Name: "budgets", Where: "scope = ?", Scope: "agent:", Phase: PhaseData},
	// script_runs go with their scripts by ON DELETE CASCADE.
	{Name: "script_runs", Where: "agent = ?", AgentCol: "agent", OrderBy: "id", BackupOrder: 8},
	// messages are channel history shared with other agents: backed up, kept.
	{Name: "messages", Where: "produced_by_agent = ?", AgentCol: "produced_by_agent", OrderBy: "id", BackupOrder: 10},
}

// DeleteAgentRows deletes the agent's rows from every registry table of phase,
// in registry order, and returns the rows deleted per table.
func DeleteAgentRows(tx *sql.Tx, name string, phase Phase) (map[string]int64, error) {
	deleted := map[string]int64{}
	for _, at := range AgentTables {
		if at.Phase != phase || phase == PhaseNone {
			continue
		}
		res, err := tx.Exec(`DELETE FROM `+at.Name+` WHERE `+at.Where, at.Scope+name)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		deleted[at.Name] = n
	}
	return deleted, nil
}

// IterationTable is a table whose rows retention prunes with one iteration.
type IterationTable struct {
	Name string
	// Where scopes rows to the iteration id with exactly one ? placeholder.
	Where string
}

// IterationTables are pruned per iteration, in deletion order.
var IterationTables = []IterationTable{
	{Name: "ai_requests", Where: "iteration = ?"},
	{Name: "iterations", Where: "id = ?"},
}
