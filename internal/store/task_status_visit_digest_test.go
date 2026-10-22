package store

import (
	"path/filepath"
	"testing"
)

func TestTaskStatusVisitDigestMigrationBackfillsFromTheTask(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	s := openBeforeMigration(t, dbPath, "0071_task_status_visit_digest.sql")
	if _, err := s.DB.Exec(`
		INSERT INTO task_queues(prefix, name, created_at, updated_at) VALUES ('T', 'T', 'c', 'c');
		INSERT INTO task_workflow_images(digest, name, version, manifest, built_at) VALUES ('d1', 'w', '1', '{}', 'now');
		INSERT INTO tasks(id, task_key, queue_prefix, title, status, author, customer, created_at, updated_at, workflow_digest, workflow_status) VALUES
			(1, 'T-1', 'T', 'pinned', 'open', 'user:u', 'user:u', 'c', 'u', 'd1', 'build');
		INSERT INTO task_status_visits(task_id, sequence, status_id, entered_at, entered_by) VALUES
			(1, 1, 'build', 'c', 'user:u'), (1, 2, 'review', 'c', 'user:u')`); err != nil {
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
	var pinned int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM task_status_visits WHERE workflow_digest = 'd1'`).Scan(&pinned); err != nil {
		t.Fatal(err)
	}
	if pinned != 2 {
		t.Fatalf("visits with the task's digest = %d; want 2", pinned)
	}
}
