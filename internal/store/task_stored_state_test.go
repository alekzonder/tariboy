package store

import (
	"path/filepath"
	"testing"
)

func TestTaskStoredStateMigrationBackfillsAndTriggersKeepItCurrent(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	s := openBeforeMigration(t, dbPath, "0069_task_stored_state.sql")
	manifest := `{"definition":{"statuses":[
		{"id":"build","owner":{"kind":"pool","pool":"dev"}},
		{"id":"approve","owner":{"kind":"customer"}},
		{"id":"watch","owner":{"kind":"script"}},
		{"id":"done","owner":{"kind":""},"terminal":true}]}}`
	if _, err := s.DB.Exec(`
		INSERT INTO task_queues(prefix, name, created_at, updated_at) VALUES ('T', 'T', 'c', 'c');
		INSERT INTO task_workflow_images(digest, name, version, manifest, built_at) VALUES ('d1', 'w', '1', ?, 'now');
		INSERT INTO tasks(id, task_key, queue_prefix, title, status, author, customer, created_at, updated_at, workflow_digest, workflow_status) VALUES
			(1, 'T-1', 'T', 'pool', 'open', 'user:u', 'user:u', 'c', 'u', 'd1', 'build'),
			(2, 'T-2', 'T', 'customer', 'wait_customer', 'user:u', 'user:u', 'c', 'u', 'd1', 'approve'),
			(3, 'T-3', 'T', 'script', 'wait_customer', 'user:u', 'user:u', 'c', 'u', 'd1', 'watch'),
			(4, 'T-4', 'T', 'terminal', 'done', 'user:u', 'user:u', 'c', 'u', 'd1', 'done'),
			(5, 'T-5', 'T', 'flexible', 'open', 'user:u', 'user:u', 'c', 'u', NULL, NULL),
			(6, 'T-6', 'T', 'closed blocker', 'done', 'user:u', 'user:u', 'c', 'u', NULL, NULL);
		INSERT INTO task_relations(source_id, target_id, type, created_by, created_at) VALUES
			(1, 5, 'blocks', 'user:u', 'c'), (6, 5, 'blocks', 'user:u', 'c'), (2, 5, 'related', 'user:u', 'c')`, manifest); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state := func(id int) (string, int) {
		t.Helper()
		var kind string
		var blockers int
		if err := s.DB.QueryRow(`SELECT workflow_owner_kind, open_blockers FROM tasks WHERE id = ?`, id).Scan(&kind, &blockers); err != nil {
			t.Fatal(err)
		}
		return kind, blockers
	}
	for id, want := range map[int]string{1: "pool", 2: "customer", 3: "script", 4: "", 5: "", 6: ""} {
		if kind, _ := state(id); kind != want {
			t.Fatalf("task %d owner kind = %q; want %q", id, kind, want)
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	blockers := func(want int, step string) {
		t.Helper()
		if _, got := state(5); got != want {
			t.Fatalf("%s: open_blockers = %d; want %d", step, got, want)
		}
	}
	blockers(1, "backfill")
	exec(`UPDATE tasks SET status = 'open' WHERE id = 6`)
	blockers(2, "blocker reopened")
	exec(`UPDATE tasks SET status = 'cancelled' WHERE id = 1`)
	blockers(1, "blocker cancelled")
	exec(`UPDATE tasks SET status = 'done' WHERE id = 1`)
	blockers(1, "closed blocker moved between closed statuses")
	exec(`DELETE FROM task_relations WHERE source_id = 6 AND target_id = 5`)
	blockers(0, "relation removed")
	exec(`INSERT INTO task_relations(source_id, target_id, type, created_by, created_at) VALUES (3, 5, 'blocks', 'user:u', 'c')`)
	blockers(1, "relation added")

	exec(`UPDATE tasks SET workflow_status = 'approve' WHERE id = 1`)
	if kind, _ := state(1); kind != "customer" {
		t.Fatalf("after status change owner kind = %q", kind)
	}
	exec(`INSERT INTO tasks(id, task_key, queue_prefix, title, status, author, customer, created_at, updated_at, workflow_digest, workflow_status)
		VALUES (7, 'T-7', 'T', 'new', 'open', 'user:u', 'user:u', 'c', 'u', 'd1', 'watch')`)
	if kind, _ := state(7); kind != "script" {
		t.Fatalf("inserted task owner kind = %q", kind)
	}
	exec(`UPDATE task_workflow_images SET manifest = ? WHERE digest = 'd1'`,
		`{"definition":{"statuses":[{"id":"watch","owner":{"kind":"customer"}}]}}`)
	if kind, _ := state(7); kind != "customer" {
		t.Fatalf("after manifest change owner kind = %q", kind)
	}
}
