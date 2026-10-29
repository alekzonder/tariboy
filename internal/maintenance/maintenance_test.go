package maintenance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

var testNow = time.Date(2026, time.September, 24, 3, 0, 0, 0, time.UTC)

func open(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tariboyd.db")
	st := storetest.OpenAt(t, dbPath)
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
	return store.FormatTime(testNow.AddDate(0, 0, -daysAgo))
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
	want := Settings{Enabled: true, Time: "03:00", KeepBackups: 7, RetentionDays: 90, ChatRetentionDays: 7, Compact: true, CompactThresholdPct: 10}
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
}

// Settings saved before chat_retention_days existed turn it on with the
// default.
func TestStoredSettingsWithoutChatRetentionGetDefault(t *testing.T) {
	svc, st, _ := open(t)
	if err := st.ConfigSet(settingsKey, `{"enabled":true,"time":"04:00","keep_backups":3,"retention_days":60,"compact":true,"compact_threshold_pct":10}`); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if got.ChatRetentionDays != 7 || got.RetentionDays != 60 {
		t.Fatalf("settings = %+v", got)
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
		{Time: "03:00", KeepBackups: 1, ChatRetentionDays: -1},
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
	for _, table := range []string{"task_comments", "task_waiting_for", "task_events", "task_notification_outbox"} {
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

// addWorkflowRows gives task id the rows a workflow task carries: a visit, a
// holder, a transition request, and an artifact.
func addWorkflowRows(t *testing.T, db *sql.DB, id int) {
	t.Helper()
	res, err := db.Exec(`INSERT INTO task_status_visits(task_id,sequence,status_id,entered_at,entered_by,left_at) VALUES (?,1,'build',?,'system:workflow',?)`,
		id, ts(400), ts(400))
	if err != nil {
		t.Fatal(err)
	}
	visit, _ := res.LastInsertId()
	exec(t, db, `INSERT INTO task_workflow_holders(task_id,pool,agent,dispatched_at) VALUES (?,'builders','a',?)`, id, ts(400))
	exec(t, db, `INSERT INTO task_transition_requests(task_id,visit_id,outcome,actor,state,created_at,finished_at) VALUES (?,?,'ok','agent:a','applied',?,?)`,
		id, visit, ts(400), ts(400))
	exec(t, db, `INSERT INTO task_artifacts(task_id,name,value,author,created_at) VALUES (?,'plan','p','agent:a',?)`, id, ts(400))
}

func TestCleanupDeletesWorkflowRowsOfClosedTaskTrees(t *testing.T) {
	svc, st, _ := open(t)
	db := st.DB
	exec(t, db, `INSERT INTO task_workflow_images(digest,name,version,manifest,built_at) VALUES ('d1','flow','1.0.0','{}',?)`, ts(400))
	// Tree 1: an old closed workflow task -> removed with its workflow rows.
	addTask(t, db, 1, nil, "done", 100)
	addWorkflowRows(t, db, 1)
	// Tree 2: an open workflow task -> kept with its workflow rows.
	addTask(t, db, 2, nil, "in_progress", 0)
	addWorkflowRows(t, db, 2)
	exec(t, db, `UPDATE tasks SET workflow_digest='d1', workflow_status='build'`)

	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if taskExists(t, db, 1) || !taskExists(t, db, 2) {
		t.Fatalf("tasks after cleanup: 1=%v 2=%v; want only 2", taskExists(t, db, 1), taskExists(t, db, 2))
	}
	for _, table := range []string{"task_status_visits", "task_workflow_holders", "task_transition_requests", "task_artifacts"} {
		if n := count(t, db, `SELECT COUNT(*) FROM `+table+` WHERE task_id=1`); n != 0 {
			t.Errorf("%s rows of the purged task = %d, want 0", table, n)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM `+table+` WHERE task_id=2`); n != 1 {
			t.Errorf("%s rows of the kept task = %d, want 1", table, n)
		}
	}
}

func TestCleanupRemovesTheFilesOfPurgedTasks(t *testing.T) {
	svc, st, _ := open(t)
	db := st.DB
	base := t.TempDir()
	svc.TasksDir = filepath.Join(base, "tasks")
	addTask(t, db, 1, nil, "done", 100)
	addTask(t, db, 2, nil, "in_progress", 0)
	addTask(t, db, 3, nil, "done", 100)
	// A key that is not a single path element is never used as a directory.
	exec(t, db, `UPDATE tasks SET task_key = '..' WHERE id = 3`)
	for _, key := range []string{"T-b", "T-c"} {
		if err := os.MkdirAll(filepath.Join(svc.TasksDir, key, "runs", "1"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(base, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if taskExists(t, db, 1) || taskExists(t, db, 3) || !taskExists(t, db, 2) {
		t.Fatalf("tasks after cleanup: 1=%v 2=%v 3=%v", taskExists(t, db, 1), taskExists(t, db, 2), taskExists(t, db, 3))
	}
	if _, err := os.Stat(filepath.Join(svc.TasksDir, "T-b")); !os.IsNotExist(err) {
		t.Fatalf("purged task directory: %v", err)
	}
	for _, kept := range []string{filepath.Join(svc.TasksDir, "T-c", "runs", "1"), keep} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s: %v", kept, err)
		}
	}
}

func TestCleanupRemovesOldOperationsAndTheirLogs(t *testing.T) {
	svc, st, _ := open(t)
	db := st.DB
	svc.OperationsDir = filepath.Join(t.TempDir(), "operations")
	if err := os.MkdirAll(svc.OperationsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	add := func(id int, status, createdAt, finishedAt string) {
		exec(t, db, `INSERT INTO operations(id, kind, title, status, created_at, finished_at) VALUES (?,?,?,?,?,?)`,
			id, "image.build", "t", status, createdAt, finishedAt)
		if err := os.WriteFile(filepath.Join(svc.OperationsDir, fmt.Sprintf("%d.log", id)), []byte("log\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	add(1, "succeeded", ts(100), ts(100))
	add(2, "failed", ts(100), ts(10))
	add(3, "running", ts(100), "")

	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if res.Deleted["operations"] != 1 {
		t.Fatalf("deleted = %v", res.Deleted)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM operations WHERE id IN (2, 3)`); n != 2 || count(t, db, `SELECT COUNT(*) FROM operations`) != 2 {
		t.Fatalf("operations left = %d", n)
	}
	if _, err := os.Stat(filepath.Join(svc.OperationsDir, "1.log")); !os.IsNotExist(err) {
		t.Fatalf("log of the purged operation: %v", err)
	}
	for _, kept := range []string{"2.log", "3.log"} {
		if _, err := os.Stat(filepath.Join(svc.OperationsDir, kept)); err != nil {
			t.Fatal(err)
		}
	}
}

// letter inserts one mail row created at createdAt. closed sets answered_at
// and dead marks it dead; to defaults to agent:a.
type letterRow struct {
	id, to, createdAt string
	closed, dead      bool
}

func insertLetters(t *testing.T, db *sql.DB, rows ...letterRow) {
	t.Helper()
	for _, r := range rows {
		if r.to == "" {
			r.to = "agent:a"
		}
		answered, dead := "", ""
		if r.closed || r.dead {
			answered = r.createdAt
		}
		if r.dead {
			dead = r.createdAt
		}
		exec(t, db, `INSERT INTO mail(id, sender, recipient, created_at, answered_at, dead_at) VALUES (?,?,?,?,?,?)`,
			r.id, "user:customer", r.to, r.createdAt, answered, dead)
	}
}

func TestCleanupAIRequestsMailAndEvents(t *testing.T) {
	svc, st, _ := open(t)
	db := st.DB
	exec(t, db, `INSERT INTO ai_requests(id,ts) VALUES ('old',?),('new',?)`, ts(100), ts(1))
	exec(t, db, `INSERT INTO events(ts,kind) VALUES (?,'old'),(?,'new')`, ts(100), ts(1))
	insertLetters(t, db,
		letterRow{id: "answered", createdAt: ts(100), closed: true},
		letterRow{id: "dead", createdAt: ts(100), dead: true},
		letterRow{id: "open", createdAt: ts(100)},
		letterRow{id: "fresh", createdAt: ts(1), closed: true},
		letterRow{id: "ingested", to: "queue:OPS", createdAt: ts(100), closed: true},
		letterRow{id: "uningested", to: "queue:OPS", createdAt: ts(100), closed: true})
	// The ingress consumed the first queue letter only.
	exec(t, db, `UPDATE task_workflow_ingress_state SET last_message_sequence=(SELECT seq FROM mail WHERE id='ingested')`)

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
	if got := letterIDs(t, db); got != "fresh,open,uningested" {
		t.Errorf("mail = %v, want fresh,open,uningested", got)
	}
	if res.Deleted["ai_requests"] != 1 || res.Deleted["mail"] != 3 || res.Deleted["events"] < 1 {
		t.Errorf("deleted = %v", res.Deleted)
	}
}

func letterIDs(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM mail ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(ids, ",")
}

func addChatMessages(t *testing.T, db *sql.DB) {
	t.Helper()
	insertLetters(t, db,
		letterRow{id: "old", createdAt: ts(8), closed: true},
		letterRow{id: "recent", createdAt: ts(6), closed: true},
		letterRow{id: "pending", createdAt: ts(8)})
	exec(t, db, `INSERT INTO ai_requests(id,ts) VALUES ('usage',?)`, ts(8))
}

// Mail history has its own shorter period that applies even when general
// retention keeps everything, and leaves the other data alone.
func TestChatRetentionPurgesOnlyOldClosedLetters(t *testing.T) {
	svc, st, _ := open(t)
	if err := svc.SetSettings(Settings{Enabled: true, Time: "03:00", KeepBackups: 7, RetentionDays: 0, ChatRetentionDays: 7}); err != nil {
		t.Fatal(err)
	}
	addChatMessages(t, st.DB)

	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if got := letterIDs(t, st.DB); got != "pending,recent" {
		t.Errorf("mail = %s, want pending,recent", got)
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM ai_requests`); n != 1 {
		t.Errorf("ai_requests = %d, want 1", n)
	}
	if res.Deleted["mail"] != 1 {
		t.Errorf("deleted = %v", res.Deleted)
	}
}

// With both periods set, closed letters expire at the shorter one.
func TestChatRetentionIsShorterThanGeneralRetention(t *testing.T) {
	svc, st, _ := open(t)
	addChatMessages(t, st.DB) // default settings: 90 days, chat 7 days
	if _, err := svc.Run(); err != nil {
		t.Fatal(err)
	}
	if got := letterIDs(t, st.DB); got != "pending,recent" {
		t.Errorf("mail = %s, want pending,recent", got)
	}
}

func TestChatRetentionZeroFollowsGeneralRetention(t *testing.T) {
	svc, st, _ := open(t)
	if err := svc.SetSettings(Settings{Enabled: true, Time: "03:00", KeepBackups: 7, RetentionDays: 0, ChatRetentionDays: 0}); err != nil {
		t.Fatal(err)
	}
	addChatMessages(t, st.DB)
	if _, err := svc.Run(); err != nil {
		t.Fatal(err)
	}
	if got := letterIDs(t, st.DB); got != "old,pending,recent" {
		t.Errorf("mail = %s, want old,pending,recent", got)
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

// The backup runs on its own connection as a WAL read, so an open write does
// not hold it back and it does not hold back the writer: the UI keeps working.
func TestBackupDoesNotWaitForTheWriter(t *testing.T) {
	svc, st, _ := open(t)
	exec(t, st.DB, `INSERT INTO ai_requests(id,ts) VALUES ('committed',?)`, ts(1))
	tx, err := st.DB.Begin() // holds the only writer connection
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO ai_requests(id,ts) VALUES ('open',?)`, ts(1)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	var path string
	go func() {
		var err error
		path, err = svc.backup(7)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("backup waited for the writer connection")
	}
	copy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	if n := count(t, copy, `SELECT COUNT(*) FROM ai_requests`); n != 1 {
		t.Fatalf("backup has %d rows, want only the committed one", n)
	}
}

func TestFailedBackupSkipsCleanup(t *testing.T) {
	svc, st, backups := open(t)
	exec(t, st.DB, `INSERT INTO ai_requests(id,ts) VALUES ('old',?)`, ts(100))
	insertLetters(t, st.DB, letterRow{id: "old", createdAt: ts(100), closed: true})
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
	if n := count(t, st.DB, `SELECT COUNT(*) FROM mail`); n != 1 {
		t.Fatalf("chat history purged after failed backup")
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

func TestCleanupKeepsUsageForLongestRollingBudget(t *testing.T) {
	svc, st, _ := open(t)
	exec(t, st.DB, `INSERT INTO budgets(scope,limit_usd,period_s,mode) VALUES ('global',10,?,'block')`, 200*24*3600)
	exec(t, st.DB, `INSERT INTO ai_requests(id,ts) VALUES ('in-window',?),('too-old',?)`, ts(150), ts(250))
	if res, err := svc.Run(); err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM ai_requests WHERE id='in-window'`); n != 1 {
		t.Fatal("deleted a Usage row inside the rolling budget period")
	}
	if n := count(t, st.DB, `SELECT COUNT(*) FROM ai_requests WHERE id='too-old'`); n != 0 {
		t.Fatal("kept a Usage row older than every window")
	}
}

func TestRotationIgnoresForeignFiles(t *testing.T) {
	svc, _, backups := open(t)
	if err := svc.SetSettings(Settings{Enabled: true, Time: "03:00", KeepBackups: 1, RetentionDays: 90}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backups, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"tariboyd-before-upgrade.db", "tariboyd-20260101T000000Z.db.partial"} {
		if err := os.WriteFile(filepath.Join(backups, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := svc.Run()
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(backups)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != filepath.Base(res.Backup)+",tariboyd-before-upgrade.db" {
		t.Fatalf("backups dir = %v", names)
	}
}

// clock is a goroutine-safe fake wall clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

func TestLoopFiresOnWallClockAndSkipsLateWakes(t *testing.T) {
	dir := t.TempDir()
	st := storetest.OpenAt(t, filepath.Join(dir, "tariboyd.db"))
	day := func(d, h, m int) time.Time { return time.Date(2026, time.September, d, h, m, 0, 0, time.UTC) }
	clk := &clock{t: day(24, 2, 0)}
	svc := New(st, filepath.Join(dir, "backups"), clk.now, nil)

	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.Loop(ctx, func(time.Duration) <-chan time.Time { return tick })
		close(done)
	}()
	// A second tick is received only after the first one was fully handled.
	step := func(at time.Time) {
		clk.set(at)
		tick <- at
		tick <- at
	}
	started := func() string {
		r, err := svc.LastResult()
		if err != nil {
			t.Fatal(err)
		}
		if r == nil {
			return ""
		}
		return r.StartedAt
	}

	step(day(24, 2, 30))
	if got := started(); got != "" {
		t.Fatalf("ran before 03:00: %s", got)
	}
	step(day(24, 3, 1))
	if got := started(); got != "2026-09-24T03:01:00.000000000Z" {
		t.Fatalf("scheduled run = %q", got)
	}
	// Woken hours after the slot (a sleeping laptop): skipped, not caught up.
	step(day(25, 9, 0))
	if got := started(); got != "2026-09-24T03:01:00.000000000Z" {
		t.Fatalf("late wake ran: %q", got)
	}
	step(day(26, 3, 0))
	if got := started(); got != "2026-09-26T03:00:00.000000000Z" {
		t.Fatalf("next night = %q", got)
	}
	// Disabled: the slot passes without a run.
	if err := svc.SetSettings(Settings{Enabled: false, Time: "03:00", KeepBackups: 7, RetentionDays: 90}); err != nil {
		t.Fatal(err)
	}
	// The loop may have read the settings before SetSettings wrote them; a
	// tick before the slot makes it read them again.
	step(day(27, 2, 0))
	step(day(27, 3, 0))
	if got := started(); got != "2026-09-26T03:00:00.000000000Z" {
		t.Fatalf("disabled run = %q", got)
	}
	cancel()
	<-done
}

func TestRunPrunesIterationsAfterBackup(t *testing.T) {
	svc, _, backups := open(t)
	sawBackup := false
	svc.PruneIterations = func() (int64, error) {
		matches, _ := filepath.Glob(filepath.Join(backups, "tariboyd-*.db"))
		sawBackup = len(matches) == 1
		return 3, nil
	}
	res, err := svc.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !sawBackup || res.Deleted["iterations"] != 3 {
		t.Fatalf("sawBackup=%v deleted=%v", sawBackup, res.Deleted)
	}
}

func TestFailedBackupSkipsIterationPrune(t *testing.T) {
	svc, _, backups := open(t)
	if err := os.MkdirAll(filepath.Dir(backups), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backups, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	svc.PruneIterations = func() (int64, error) { called = true; return 0, nil }
	if _, err := svc.Run(); err == nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
