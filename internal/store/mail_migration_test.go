package store

import (
	"path/filepath"
	"testing"
)

// TestMigrationRewritesScheduleChannelToRecipient pins the 0072 schedule
// rewrite: the service chat and the old inbox channel of an agent become that
// agent's principal, and any other channel becomes the owning agent.
func TestMigrationRewritesScheduleChannelToRecipient(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	s := openBeforeMigration(t, dbPath, "0072_mail.sql")
	if _, err := s.DB.Exec(`
		INSERT INTO schedules(id, agent, kind, spec, channel, next_fire_at) VALUES
			('s-service', 'alice', 'cron', '* * * * *', 'chat:service:alice', 'x'),
			('s-other-service', 'alice', 'cron', '* * * * *', 'chat:service:bob', 'x'),
			('s-inbox', 'alice', 'cron', '* * * * *', 'agent:carol:inbox', 'x'),
			('s-chat', 'alice', 'cron', '* * * * *', 'chat:tasks:alice', 'x'),
			('s-plain', 'dave', 'cron', '* * * * *', 'inbox:dave', 'x')`); err != nil {
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
	want := map[string]string{
		"s-service":       "agent:alice",
		"s-other-service": "agent:bob",
		"s-inbox":         "agent:carol",
		"s-chat":          "agent:alice",
		"s-plain":         "agent:dave",
	}
	for id, recipient := range want {
		var got string
		if err := s.DB.QueryRow(`SELECT recipient FROM schedules WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got != recipient {
			t.Errorf("%s recipient = %q; want %q", id, got, recipient)
		}
	}
	requireTable(t, s.DB, "mail")
}

// TestMigrationRestartsWorkflowIngressOnMail: the ingress cursor and the
// trigger watermarks counted the dropped message sequence; mail.seq starts
// again at 1, so both start over or every new queue letter would be skipped.
func TestMigrationRestartsWorkflowIngressOnMail(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	s := openBeforeMigration(t, dbPath, "0072_mail.sql")
	if _, err := s.DB.Exec(`UPDATE task_workflow_ingress_state SET last_message_sequence = 500 WHERE singleton = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO task_queues(prefix, name, created_at, updated_at) VALUES ('OPS', 'Ops', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO task_queue_workflow_triggers(queue_prefix, pattern, correlation_key, action, enabled,
		created_by, created_at, updated_at, created_after_sequence, activation_sequence_set)
		VALUES ('OPS', 'plugin:*', '', 'create_task', 1, 'user:customer', 'x', 'x', 400, 1)`); err != nil {
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
	var cursor, watermark int64
	if err := s.DB.QueryRow(`SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton = 1`).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT created_after_sequence FROM task_queue_workflow_triggers`).Scan(&watermark); err != nil {
		t.Fatal(err)
	}
	if cursor != 0 || watermark != 0 {
		t.Fatalf("cursor = %d, watermark = %d; want both 0", cursor, watermark)
	}
}
