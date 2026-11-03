package store

import (
	"slices"
	"strings"
	"testing"
)

// TestRemovalReportNamesEveryAgentTable proves each removal mode names every
// registry table and every table that cascades from one exactly once, either
// as deleted or as kept.
func TestRemovalReportNamesEveryAgentTable(t *testing.T) {
	t.Parallel()
	for _, mode := range []RemoveMode{RemoveModeRemove, RemoveModeKeep, RemoveModePurge} {
		deleted, kept := RemovalReport(mode)
		all := append(slices.Clone(deleted), kept...)
		for _, at := range AgentTables {
			n := 0
			for _, item := range all {
				if item == at.Name || strings.HasPrefix(item, at.Name+" ") {
					n++
				}
			}
			want := 1
			if at.DeleteWhere != "" && mode != RemoveModeKeep {
				want = 2 // a partly deleted table is named in both lists
			}
			if n != want {
				t.Errorf("%s: table %s named %d times in %v / %v, want %d", mode, at.Name, n, deleted, kept, want)
			}
		}
		for child := range CascadeParent {
			if slices.Contains(deleted, child) == slices.Contains(kept, child) {
				t.Errorf("%s: cascade table %s must be in exactly one list: %v / %v", mode, child, deleted, kept)
			}
		}
	}
}

func TestRemovalReportRemoveMode(t *testing.T) {
	t.Parallel()
	deleted, kept := RemovalReport(RemoveModeRemove)
	for _, want := range []string{"agents", "secrets", "agent_budgets", "budgets", "schedules", "scripts", "script_runs",
		"script_result_outbox", "retention_policies", "mail (inbox)", "image/", "bin/", "workdir/"} {
		if !slices.Contains(deleted, want) {
			t.Errorf("remove deleted = %v, missing %s", deleted, want)
		}
	}
	for _, want := range []string{"iterations", "iteration_tags", "ai_requests", "mail (sent)", "iterations/",
		"CONTEXT.md", "audit.jsonl", "assessments"} {
		if !slices.Contains(kept, want) {
			t.Errorf("remove kept = %v, missing %s", kept, want)
		}
	}
}

func TestRemovalReportKeepAndPurgeModes(t *testing.T) {
	t.Parallel()
	deleted, kept := RemovalReport(RemoveModeKeep)
	if !slices.Equal(deleted, []string{"image/", "bin/", "workdir/"}) {
		t.Errorf("keep deleted = %v", deleted)
	}
	if !slices.Contains(kept, "agents (enabled=false)") || !slices.Contains(kept, "mail") {
		t.Errorf("keep kept = %v", kept)
	}
	deleted, kept = RemovalReport(RemoveModePurge)
	if !slices.Contains(deleted, "agent directory") || !slices.Contains(deleted, "iterations") {
		t.Errorf("purge deleted = %v", deleted)
	}
	if !slices.Equal(kept, []string{"mail (sent)", "assessments"}) {
		t.Errorf("purge kept = %v", kept)
	}
}

// TestCascadeParentMatchesSchema keeps CascadeParent in step with the
// ON DELETE CASCADE foreign keys to registry tables.
func TestCascadeParentMatchesSchema(t *testing.T) {
	t.Parallel()
	s := open(t)
	registered := map[string]bool{}
	for _, at := range AgentTables {
		registered[at.Name] = true
	}
	rows, err := s.DB.Query(`SELECT m.name, f."table" FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table' AND f.on_delete = 'CASCADE'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			t.Fatal(err)
		}
		if registered[parent] {
			got[child] = parent
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for child, parent := range got {
		if CascadeParent[child] != parent {
			t.Errorf("table %s cascades from %s; CascadeParent says %q", child, parent, CascadeParent[child])
		}
	}
	for child := range CascadeParent {
		if got[child] == "" {
			t.Errorf("CascadeParent lists %s, which cascades from no registry table", child)
		}
	}
}
