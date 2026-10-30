package store

import (
	"sort"
	"testing"
)

// agentKeyedExempt lists agent-keyed tables that agent removal deliberately
// leaves alone. Every entry needs a reason.
var agentKeyedExempt = map[string]string{
	"events":                  "audit log: kept after the agent is removed",
	"eval_results":            "eval history keyed by image and iteration: kept for image quality reports",
	"task_agent_pool_members": "tasks own pool membership (FK RESTRICT to agents; a purge Remove rejects a pool member first)",
	"task_queue_owners":       "tasks own queue ownership",
	"task_reminders":          "tasks own reminder lifecycle",
	"task_workflow_holders":   "tasks own workflow holders (cascade from tasks)",
}

var agentKeyColumns = map[string]bool{"agent": true, "agent_name": true, "produced_by_agent": true}

// TestAgentKeyedTablesAreRegistered fails when a migration adds a table keyed
// by an agent that is neither in AgentTables, removed by ON DELETE CASCADE from
// a registry table, nor exempt with a reason.
func TestAgentKeyedTablesAreRegistered(t *testing.T) {
	t.Parallel()
	s := open(t)
	registry := map[string]bool{}
	for _, at := range AgentTables {
		if registry[at.Name] {
			t.Fatalf("AgentTables lists %s twice", at.Name)
		}
		registry[at.Name] = true
	}
	rows, err := s.DB.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	exists := map[string]bool{}
	for _, table := range tables {
		exists[table] = true
	}
	for name := range registry {
		if !exists[name] {
			t.Errorf("AgentTables lists missing table %s", name)
		}
	}
	for name, reason := range agentKeyedExempt {
		if !exists[name] {
			t.Errorf("agentKeyedExempt lists missing table %s", name)
		}
		if registry[name] {
			t.Errorf("table %s is both registered and exempt", name)
		}
		if reason == "" {
			t.Errorf("exempt table %s has no reason", name)
		}
	}

	var missing []string
	for _, table := range tables {
		if registry[table] || agentKeyedExempt[table] != "" {
			continue
		}
		keyed := false
		cols, err := s.DB.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		for cols.Next() {
			var col string
			if err := cols.Scan(&col); err != nil {
				t.Fatal(err)
			}
			keyed = keyed || agentKeyColumns[col]
		}
		if err := cols.Close(); err != nil {
			t.Fatal(err)
		}
		cascaded := false
		fks, err := s.DB.Query(`SELECT "table", on_delete FROM pragma_foreign_key_list(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		for fks.Next() {
			var parent, onDelete string
			if err := fks.Scan(&parent, &onDelete); err != nil {
				t.Fatal(err)
			}
			if !registry[parent] {
				continue
			}
			if onDelete == "CASCADE" {
				cascaded = true
			} else {
				keyed = true
			}
		}
		if err := fks.Close(); err != nil {
			t.Fatal(err)
		}
		if keyed && !cascaded {
			missing = append(missing, table)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("agent-keyed tables missing from store.AgentTables (add a row or an agentKeyedExempt reason): %v", missing)
	}
}
