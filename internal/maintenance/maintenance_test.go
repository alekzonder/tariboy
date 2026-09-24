package maintenance

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

var testNow = time.Date(2026, time.September, 24, 3, 0, 0, 0, time.UTC)

func open(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tariboyd.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	backups := filepath.Join(dir, "backups", "db")
	return New(st, backups, func() time.Time { return testNow }, nil), st, backups
}

func exec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func ts(daysAgo int) string {
	return testNow.AddDate(0, 0, -daysAgo).Format(time.RFC3339Nano)
}

// addTask inserts a task with one comment, an answered wait, an event with a
// read customer notification, the full footprint cleanup has to remove.
func addTask(t *testing.T, db *sql.DB, id int, parent any, status string, completedDaysAgo int) {
	t.Helper()
	completed := ""
	if status == "done" || status == "cancelled" {
		completed = ts(completedDaysAgo)
	}
	exec(t, db, `INSERT OR IGNORE INTO task_queues(prefix,name,created_at,updated_at) VALUES ('T','T',?,?)`, ts(400), ts(400))
	exec(t, db, `INSERT INTO tasks(id,task_key,queue_prefix,parent_id,title,status,author,customer,created_at,updated_at,completed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, id, "T-"+string(rune('a'+id)), "T", parent, "t", status, "user:c", "user:c", ts(400), ts(400), completed)
	res, err := db.Exec(`INSERT INTO task_comments(task_id,author,body,created_at,updated_at) VALUES (?,?,?,?,?)`, id, "user:c", "b", ts(400), ts(400))
	if err != nil {
		t.Fatal(err)
	}
	cid, _ := res.LastInsertId()
	exec(t, db, `INSERT INTO task_waiting_for(task_id,expected_principal,requesting_principal,requesting_comment_id,requested_at,resolving_comment_id,resolved_at)
		VALUES (?,?,?,?,?,?,?)`, id, "agent:a", "user:c", cid, ts(400), cid, ts(400))
	res, err = db.Exec(`INSERT INTO task_events(event_id,task_id,queue_prefix,kind,actor,created_at) VALUES (?,?,?,?,?,?)`,
		"e"+string(rune('a'+id)), id, "T", "k", "user:c", ts(400))
	if err != nil {
		t.Fatal(err)
	}
	seq, _ := res.LastInsertId()
	nid := "n" + string(rune('a'+id))
	exec(t, db, `INSERT INTO task_notification_outbox(notification_id,event_sequence,channel,message_type,next_attempt_at) VALUES (?,?,?,?,?)`, nid, seq, "c", "m", ts(400))
	exec(t, db, `INSERT INTO task_notification_state(customer_principal,notification_id,read_at) VALUES (?,?,?)`, "user:c", nid, ts(400))
}

func taskExists(t *testing.T, db *sql.DB, id int) bool {
	return count(t, db, `SELECT COUNT(*) FROM tasks WHERE id=?`, id) == 1
}

func TestDefaultsWhenUnset(t *testing.T) {
	svc, _, _ := open(t)
	got, err := svc.Settings()
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{Enabled: true, Time: "03:00", KeepBackups: 7, RetentionDays: 90, Compact: true, CompactThresholdPct: 10}
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
}

func TestSetSettingsValidates(t *testing.T) {
	svc, _, _ := open(t)
	for _, bad := range []Settings{
		{Time: "25:00", KeepBackups: 1},
		{Time: "3am", KeepBackups: 1},
		{Time: "03:00", KeepBackups: 0},
		{Time: "03:00", KeepBackups: 1, RetentionDays: -1},
		// A shorter period would delete Usage rows still counted by the
		// calendar-month budget window.
		{Time: "03:00", KeepBackups: 1, RetentionDays: 30},
		{Time: "03:00", KeepBackups: 1, RetentionDays: 90, CompactThresholdPct: 101},
	} {
		if err := svc.SetSettings(bad); err == nil {
			t.Errorf("SetSettings(%+v) accepted", bad)
		}
	}
	good := Settings{Enabled: false, Time: "23:45", KeepBackups: 2, RetentionDays: 0, Compact: false, CompactThresholdPct: 0}
	if err := svc.SetSettings(good); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Settings(); got != good {
		t.Fatalf("round trip = %+v, want %+v", got, good)
	}
}

func TestNextRun(t *testing.T) {
	loc := time.FixedZone("X", 3*3600)
	now := time.Date(2026, time.September, 24, 2, 59, 0, 0, loc)
	if got := nextRun(now, "03:00"); !got.Equal(time.Date(2026, time.September, 24, 3, 0, 0, 0, loc)) {
		t.Fatalf("before time: %v", got)
	}
	now = time.Date(2026, time.September, 24, 3, 0, 0, 0, loc)
	if got := nextRun(now, "03:00"); !got.Equal(time.Date(2026, time.September, 25, 3, 0, 0, 0, loc)) {
		t.Fatalf("at time: %v", got)
	}
}

func TestRunBacksUpRotatesAndRecordsStatus(t *testing.T) {
	svc, st, backups := open(t)
	if err := svc.SetSettings(Settings{Enabled: true, Time: "03:00", KeepBackups: 2, RetentionDays: 90}); err != nil {
		t.Fatal(err)
	}
	exec(t, st.DB, `INSERT INTO events(agent,kind) VALUES ('a','marker')`)
	if err := os.MkdirAll(backups, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"tariboyd-20260101T000000Z.db", "tariboyd-20260102T000000Z.db"} {
		if err := os.WriteFile(filepath.Join(backups, old), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	res, err := svc.Run()
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" || res.Backup == "" {
		t.Fatalf("result = %+v", res)
	}
	info, err := os.Stat(res.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode = %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(backups)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 || names[0] != "tariboyd-20260102T000000Z.db" || names[1] != filepath.Base(res.Backup) {
		t.Fatalf("rotated backups = %v", names)
	}

	bk, err := store.Open(res.Backup)
	if err != nil {
		t.Fatal(err)
	}
	defer bk.Close()
	if n := count(t, bk.DB, `SELECT COUNT(*) FROM events WHERE kind='marker'`); n != 1 {
		t.Fatalf("backup marker rows = %d", n)
	}

	last, err := svc.LastResult()
	if err != nil || last == nil || last.Backup != res.Backup {
		t.Fatalf("last result = %+v, %v", last, err)
	}
}

func TestCleanupDeletesOnlyOldFinishedTrees(t *testing.T) {
	svc, st, _ := open(t)
	db := st.DB
	// Tree 1: old done root with old cancelled child -> removed.
	addTask(t, db, 1, nil, "done", 100)
	addTask(t, db, 2, 1, "cancelled", 120)
	// Tree 3: old done root with an open child -> kept whole.
	addTask(t, db, 3, nil, "done", 100)
	addTask(t, db, 4, 3, "open", 0)
	// Tree 5: recently completed -> kept.
	addTask(t, db, 5, nil, "done", 10)
	// Tree 6: old done but related to open task 7 -> kept.
	addTask(t, db, 6, nil, "done", 100)
	addTask(t, db, 7, nil, "in_progress", 0)
	exec(t, db, `INSERT INTO task_relations(source_id,target_id,type,created_by,created_at) VALUES (6,7,'related','user:c',?)`, ts(100))
	// Trees 8 and 9: both old and related to each other -> both removed.
	addTask(t, db, 8, nil, "done", 100)
	addTask(t, db, 9, nil, "done", 100)
	exec(t, db, `INSERT INTO task_relations(source_id,target_id,type,created_by,created_at) VALUES (8,9,'blocks','user:c',?)`, ts(100))
	exec(t, db, `INSERT INTO task_idempotency(actor,action,idempotency_key,response,created_at) VALUES ('a','x','old','{}',?),('a','x','new','{}',?)`, ts(100), ts(1))

	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	for id, want := range map[int]bool{1: false, 2: false, 3: true, 4: true, 5: true, 6: true, 7: true, 8: false, 9: false} {
		if got := taskExists(t, db, id); got != want {
			t.Errorf("task %d exists = %v, want %v", id, got, want)
		}
	}
	if res.Deleted["tasks"] != 4 {
		t.Errorf("deleted tasks = %d, want 4", res.Deleted["tasks"])
	}
	for _, table := range []string{"task_comments", "task_waiting_for", "task_events", "task_notification_outbox", "task_notification_state"} {
		if n := count(t, db, `SELECT COUNT(*) FROM `+table); n != 5 {
			t.Errorf("%s rows = %d, want 5", table, n)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM task_relations`); n != 1 {
		t.Errorf("relations = %d, want 1", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM task_idempotency`); n != 1 {
		t.Errorf("idempotency rows = %d, want 1", n)
	}
}

func TestCleanupAIRequestsMessagesAndEvents(t *testing.T) {
	svc, st, _ := open(t)
	db := st.DB
	exec(t, db, `INSERT INTO ai_requests(id,ts) VALUES ('old',?),('new',?)`, ts(100), ts(1))
	exec(t, db, `INSERT INTO events(ts,kind) VALUES (?,'old'),(?,'new')`, ts(100), ts(1))
	for _, m := range []struct{ id, ts string }{{"acked", ts(100)}, {"dlq", ts(100)}, {"pending", ts(100)}, {"fresh", ts(1)}} {
		exec(t, db, `INSERT INTO messages(id,channel,ts) VALUES (?,'c',?)`, m.id, m.ts)
	}
	exec(t, db, `INSERT INTO deliveries(subscription_id,message_id,acked_at,dlq) VALUES
		('s','acked',?,0),('s','dlq',NULL,1),('s','pending',NULL,0),('s2','pending',?,0),('s','fresh',?,0)`, ts(99), ts(99), ts(1))
	exec(t, db, `INSERT INTO task_workflow_message_sequence(message_id) VALUES ('acked')`)

	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM ai_requests WHERE id<>'new'`); n != 0 {
		t.Errorf("ai_requests: only 'new' should remain")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM events WHERE kind='old'`); n != 0 {
		t.Errorf("old events remain")
	}
	var ids []string
	rows, _ := db.Query(`SELECT id FROM messages ORDER BY id`)
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if strings.Join(ids, ",") != "fresh,pending" {
		t.Errorf("messages = %v, want fresh,pending", ids)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM deliveries`); n != 3 {
		t.Errorf("deliveries = %d, want 3", n)
	}
	if res.Deleted["ai_requests"] != 1 || res.Deleted["messages"] != 2 || res.Deleted["events"] < 1 {
		t.Errorf("deleted = %v", res.Deleted)
	}
}

func TestRetentionZeroKeepsEverything(t *testing.T) {
	svc, st, _ := open(t)
	if err := svc.SetSettings(Settings{Enabled: true, Time: "03:00", KeepBackups: 7, RetentionDays: 0}); err != nil {
		t.Fatal(err)
	}
	exec(t, st.DB, `INSERT INTO ai_requests(id,ts) VALUES ('old',?)`, ts(1000))
	if _, err := svc.Run(); err != nil {
		t.Fatal(err)
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM ai_requests`); n != 1 {
		t.Fatalf("ai_requests = %d", n)
	}
}

func TestFailedBackupSkipsCleanup(t *testing.T) {
	svc, st, backups := open(t)
	exec(t, st.DB, `INSERT INTO ai_requests(id,ts) VALUES ('old',?)`, ts(100))
	// A regular file where the backup directory should be makes the backup fail.
	if err := os.MkdirAll(filepath.Dir(backups), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backups, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Run()
	if err == nil || res.Error == "" {
		t.Fatalf("expected backup failure, got %+v", res)
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM ai_requests`); n != 1 {
		t.Fatalf("cleanup ran after failed backup")
	}
	if last, _ := svc.LastResult(); last == nil || last.Error == "" {
		t.Fatalf("failure not recorded: %+v", last)
	}
}

func TestCompactShrinksDatabase(t *testing.T) {
	svc, st, _ := open(t)
	payload := strings.Repeat("x", 4000)
	for range 500 {
		exec(t, st.DB, `INSERT INTO events(ts,kind,data) VALUES (?,?,?)`, ts(100), "old", `"`+payload+`"`)
	}
	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if !res.Compacted || res.SizeAfter >= res.SizeBefore/2 {
		t.Fatalf("not compacted: %+v", res)
	}
}

func TestRunIsExclusive(t *testing.T) {
	svc, _, _ := open(t)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, err := svc.Run(); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
}
