package taskreminder

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	basestore "github.com/alekzonder/tariboy/internal/store"
)

func TestStoreEligibleAppliesThresholdToZeroAndPositiveIntervals(t *testing.T) {
	base := openReminderStore(t)
	reminders := NewStore(base)
	now := mustReminderTime(t, "2026-08-21T10:05:00Z")

	insertReminderAgent(t, base, "zero", true, true, 0, "2026-08-21T09:00:00Z")
	insertReminderAgent(t, base, "positive", true, true, 60, "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-2", "zero", "open", "2026-08-21T10:00:00Z")
	insertReminderTask(t, base, "REM-1", "zero", "open", "2026-08-21T10:00:00Z")
	insertReminderTask(t, base, "REM-3", "positive", "in_progress", "2026-08-21T10:00:00Z")

	beforeThreshold, err := reminders.Eligible(Policy{Enabled: true, IdleThresholdS: 300}, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("Eligible before threshold: %v", err)
	}
	if len(beforeThreshold) != 0 {
		t.Fatalf("Eligible before threshold = %#v, want none", beforeThreshold)
	}

	got, err := reminders.Eligible(Policy{Enabled: true, IdleThresholdS: 300}, now)
	if err != nil {
		t.Fatalf("Eligible at threshold: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Eligible count = %d, want 2: %#v", len(got), got)
	}
	if got[0].Agent != "positive" || got[1].Agent != "zero" {
		t.Fatalf("Eligible agents = %#v, want positive then zero", []string{got[0].Agent, got[1].Agent})
	}
	if !reflect.DeepEqual(got[1].TaskKeys, []string{"REM-1", "REM-2"}) {
		t.Fatalf("zero task keys = %#v, want sorted keys", got[1].TaskKeys)
	}
	for _, candidate := range got {
		if candidate.ActivityAt != mustReminderTime(t, "2026-08-21T10:00:00Z") {
			t.Fatalf("%s activity = %s, want task fallback", candidate.Agent, candidate.ActivityAt)
		}
		if candidate.Fingerprint == "" {
			t.Fatalf("%s fingerprint is empty", candidate.Agent)
		}
	}
}

func TestStoreEligibleExcludesDisabledLoopDisabledAndTerminalTasks(t *testing.T) {
	base := openReminderStore(t)
	reminders := NewStore(base)
	now := mustReminderTime(t, "2026-08-21T10:05:00Z")

	insertReminderAgent(t, base, "master-off", false, true, 0, "2026-08-21T09:00:00Z")
	insertReminderAgent(t, base, "loop-off", true, false, 0, "2026-08-21T09:00:00Z")
	insertReminderAgent(t, base, "done", true, true, 0, "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-1", "master-off", "open", "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-2", "loop-off", "open", "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-3", "done", "done", "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-4", "done", "cancelled", "2026-08-21T09:00:00Z")

	got, err := reminders.Eligible(Policy{Enabled: true, IdleThresholdS: 300}, now)
	if err != nil {
		t.Fatalf("Eligible: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Eligible = %#v, want none", got)
	}
}

func TestStoreMarkSentSuppressesUnchangedGeneration(t *testing.T) {
	base := openReminderStore(t)
	reminders := NewStore(base)
	now := mustReminderTime(t, "2026-08-21T10:00:00Z")

	insertReminderAgent(t, base, "worker", true, true, 0, "2026-08-21T09:00:00Z")
	insertReminderTask(t, base, "REM-1", "worker", "open", "2026-08-21T09:00:00Z")
	insertReminderIteration(t, base, "worker-1", "worker", "done", "2026-08-21T09:45:00Z")

	got, err := reminders.Eligible(Policy{Enabled: true, IdleThresholdS: 900}, now)
	if err != nil {
		t.Fatalf("Eligible: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Eligible count = %d, want 1", len(got))
	}
	if got[0].ActivityAt != mustReminderTime(t, "2026-08-21T09:45:00Z") {
		t.Fatalf("activity = %s, want terminal iteration boundary", got[0].ActivityAt)
	}
	if err := reminders.MarkSent(got[0], now); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	again, err := reminders.Eligible(Policy{Enabled: true, IdleThresholdS: 900}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Eligible after MarkSent: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("Eligible after MarkSent = %#v, want none", again)
	}
}

func openReminderStore(t *testing.T) *basestore.Store {
	t.Helper()
	base, err := basestore.Open(filepath.Join(t.TempDir(), "reminders.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })
	return base
}

func insertReminderAgent(t *testing.T, base *basestore.Store, name string, enabled, loopEnabled bool, interval int, createdAt string) {
	t.Helper()
	if _, err := base.DB.Exec(`INSERT INTO agents(name,image_ref,created_at,enabled,loop_enabled,interval_s) VALUES (?,?,?,?,?,?)`, name, "basic:latest", createdAt, enabled, loopEnabled, interval); err != nil {
		t.Fatalf("insert agent %s: %v", name, err)
	}
}

func insertReminderTask(t *testing.T, base *basestore.Store, key, agent, status, at string) {
	t.Helper()
	if _, err := base.DB.Exec(`INSERT OR IGNORE INTO task_queues(prefix,name,created_at,updated_at) VALUES ('REM','Reminders',?,?)`, at, at); err != nil {
		t.Fatalf("insert queue: %v", err)
	}
	if _, err := base.DB.Exec(`INSERT INTO tasks(task_key,queue_prefix,title,status,author,customer,assignee,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, key, "REM", key, status, "user:customer", "user:customer", "agent:"+agent, at, at); err != nil {
		t.Fatalf("insert task %s: %v", key, err)
	}
}

func insertReminderIteration(t *testing.T, base *basestore.Store, id, agent, status, endedAt string) {
	t.Helper()
	if _, err := base.DB.Exec(`INSERT INTO iterations(id,agent,trigger,status,started_at,ended_at) VALUES (?,?,?,?,?,?)`, id, agent, "manual", status, "2026-08-21T09:30:00Z", endedAt); err != nil {
		t.Fatalf("insert iteration %s: %v", id, err)
	}
}

func mustReminderTime(t *testing.T, raw string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
