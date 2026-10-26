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
