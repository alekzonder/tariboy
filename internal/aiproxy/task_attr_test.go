package aiproxy

import (
	"database/sql"
	"sync"
	"testing"
	"time"
)

// TestMigration0018Applies checks the task_id/epic_id columns and their index
// exist on a fresh DB (store.Open auto-applies embedded migrations).
func TestMigration0018Applies(t *testing.T) {
	s := newStore(t)
	// Column presence: a SELECT naming both columns must not error.
	if _, err := s.db.Exec(`SELECT task_id, epic_id FROM ai_requests LIMIT 0`); err != nil {
		t.Fatalf("task_id/epic_id columns missing: %v", err)
	}
	// Index presence.
	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_ai_requests_agent_task'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("idx_ai_requests_agent_task missing: %v", err)
	}
}

// TestInsertTaskAttribution: a request carrying task/epic lands with the columns
// set; an untagged request lands with SQL NULL (not "").
func TestInsertTaskAttribution(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC)

	tagged := sampleReq("r-tagged", "alice", "basic", 0.10, base)
	tagged.TaskID, tagged.EpicID = "dev-t-3e1.1", "dev-t-3e1"
	untagged := sampleReq("r-untagged", "alice", "basic", 0.10, base.Add(time.Minute))

	if err := s.InsertBatch([]AIRequest{tagged, untagged}); err != nil {
		t.Fatal(err)
	}

	read := func(id string) (task, epic sql.NullString) {
		if err := s.db.QueryRow(
			`SELECT task_id, epic_id FROM ai_requests WHERE id=?`, id,
		).Scan(&task, &epic); err != nil {
			t.Fatal(err)
		}
		return
	}

	task, epic := read("r-tagged")
	if !task.Valid || task.String != "dev-t-3e1.1" || !epic.Valid || epic.String != "dev-t-3e1" {
		t.Fatalf("tagged row = task=%v epic=%v", task, epic)
	}
	task, epic = read("r-untagged")
	if task.Valid || epic.Valid {
		t.Fatalf("untagged row should be NULL/NULL, got task=%v epic=%v", task, epic)
	}
}

// TestUpdateTaskResolves: UpdateTask by iteration id (and by token) mutates the
// live token so Resolve reflects it; clearing with empty strings works too.
func TestUpdateTaskResolves(t *testing.T) {
	reg := NewTokenRegistry(nil)
	tok, err := reg.Mint(Attribution{Agent: "alice", Iteration: "alice-1"})
	if err != nil {
		t.Fatal(err)
	}

	// By iteration id.
	if n := reg.UpdateTask("alice-1", "dev-t-3e1.1", "dev-t-3e1"); n != 1 {
		t.Fatalf("UpdateTask by iteration updated %d tokens, want 1", n)
	}
	got, _ := reg.Resolve(tok)
	if got.TaskID != "dev-t-3e1.1" || got.EpicID != "dev-t-3e1" {
		t.Fatalf("after update by iteration: %+v", got)
	}

	// By token string.
	if n := reg.UpdateTask(tok, "dev-t-3e1.2", "dev-t-3e1"); n != 1 {
		t.Fatalf("UpdateTask by token updated %d tokens, want 1", n)
	}
	got, _ = reg.Resolve(tok)
	if got.TaskID != "dev-t-3e1.2" {
		t.Fatalf("after update by token: %+v", got)
	}

	// Clearing.
	reg.UpdateTask("alice-1", "", "")
	got, _ = reg.Resolve(tok)
	if got.TaskID != "" || got.EpicID != "" {
		t.Fatalf("after clear: %+v", got)
	}

	// Unknown key is a no-op.
	if n := reg.UpdateTask("nope", "x", "y"); n != 0 {
		t.Fatalf("unknown key updated %d tokens, want 0", n)
	}
}

// TestUpdateTaskRace hammers UpdateTask against Resolve/Mint concurrently; run
// with -race it proves the mutation shares the registry lock.
func TestUpdateTaskRace(t *testing.T) {
	reg := NewTokenRegistry(nil)
	tok, err := reg.Mint(Attribution{Agent: "a", Iteration: "a-1"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				reg.UpdateTask("a-1", "t", "e")
				reg.Resolve(tok)
				reg.UpdateTask(tok, "", "")
				_, _ = reg.Mint(Attribution{Agent: "a", Iteration: "a-2"})
			}
		}(i)
	}
	wg.Wait()
}
