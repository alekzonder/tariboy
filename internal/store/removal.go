package store

import "database/sql"

// RemoveMode selects what agent removal deletes.
type RemoveMode string

const (
	// RemoveModeRemove deletes the agent and its settings and keeps its
	// history: the rows of tables with KeepOnRemove, iterations/, CONTEXT.md
	// and audit.jsonl. It is the default of agent rm.
	RemoveModeRemove RemoveMode = "remove"
	// RemoveModeKeep stops the agent, keeps its row and all its data, and
	// deletes only the rebuildable image/, bin/ and workdir/, so a later
	// reprovision brings it back. compose down without --volumes uses it.
	RemoveModeKeep RemoveMode = "keep"
	// RemoveModePurge deletes the agent, every registry row and the whole
	// agent directory.
	RemoveModePurge RemoveMode = "purge"
)

// CascadeParent names the tables whose agent rows ON DELETE CASCADE removes
// together with a registry table, by child table.
var CascadeParent = map[string]string{
	"script_runs":    "scripts",
	"iteration_tags": "iterations",
}

// Paths in the agent directory, as a removal report names them.
var (
	rebuildablePaths = []string{"image/", "bin/", "workdir/"}
	historyPaths     = []string{"iterations/", "CONTEXT.md", "audit.jsonl"}
)

// keptOutside is agent data no removal mode deletes: assessments reference
// the agent by value.
const keptOutside = "assessments"

// DeleteRemovableAgentRows deletes the rows a RemoveModeRemove removal
// deletes: every PhaseCore and PhaseData table without KeepOnRemove, in
// registry order. It returns the rows deleted per table.
func DeleteRemovableAgentRows(tx *sql.Tx, name string) (map[string]int64, error) {
	deleted := map[string]int64{}
	for _, at := range AgentTables {
		if at.Phase == PhaseNone || at.KeepOnRemove {
			continue
		}
		n, err := deleteAgentTableRows(tx, at, name)
		if err != nil {
			return nil, err
		}
		deleted[at.Name] = n
	}
	return deleted, nil
}

// RemovalReport lists what a removal in mode deletes and what it keeps:
// agent tables, cascade tables and paths in the agent directory. It reads
// AgentTables, so a new registry table appears in the report.
func RemovalReport(mode RemoveMode) (deleted, kept []string) {
	if mode == RemoveModeKeep {
		kept = []string{"agents (enabled=false)"}
		for _, at := range AgentTables {
			if at.Name != "agents" {
				kept = append(kept, at.Name)
			}
		}
		kept = append(kept, cascadeOnly()...)
		kept = append(kept, historyPaths...)
		return append([]string{}, rebuildablePaths...), append(kept, keptOutside)
	}
	gone := map[string]bool{}
	for _, at := range AgentTables {
		gone[at.Name] = at.Phase != PhaseNone && (mode == RemoveModePurge || !at.KeepOnRemove)
	}
	for _, at := range AgentTables {
		if parent, ok := CascadeParent[at.Name]; ok {
			gone[at.Name] = gone[parent]
		}
		deleted, kept = reportTable(deleted, kept, at, gone[at.Name])
	}
	for _, child := range cascadeOnly() {
		if gone[CascadeParent[child]] {
			deleted = append(deleted, child)
		} else {
			kept = append(kept, child)
		}
	}
	if mode == RemoveModePurge {
		return append(deleted, "agent directory"), append(kept, keptOutside)
	}
	deleted = append(deleted, rebuildablePaths...)
	kept = append(kept, historyPaths...)
	return deleted, append(kept, keptOutside)
}

func reportTable(deleted, kept []string, at AgentTable, gone bool) ([]string, []string) {
	switch {
	case !gone:
		return deleted, append(kept, at.Name)
	case at.DeleteWhere != "":
		return append(deleted, at.Name+" ("+at.DeletedPart+")"), append(kept, at.Name+" ("+at.KeptPart+")")
	default:
		return append(deleted, at.Name), kept
	}
}

// cascadeOnly lists the CascadeParent tables that are not registry tables,
// in a stable order.
func cascadeOnly() []string {
	registered := map[string]bool{}
	for _, at := range AgentTables {
		registered[at.Name] = true
	}
	var out []string
	for _, at := range AgentTables {
		for child, parent := range CascadeParent {
			if parent == at.Name && !registered[child] {
				out = append(out, child)
			}
		}
	}
	return out
}
