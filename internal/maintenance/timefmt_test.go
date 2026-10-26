package maintenance

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// Rows on both sides of the cutoff within one second are split by time: a
// row 1ns older than the cutoff is deleted, a row at the cutoff is kept, even
// where RFC3339Nano would give the two values different widths.
func TestCleanupSplitsSameSecondRowsAtTheCutoff(t *testing.T) {
	dir := t.TempDir()
	st := storetest.OpenAt(t, filepath.Join(dir, "tariboyd.db"))
	now := testNow.Add(11)
	svc := New(st, filepath.Join(dir, "backups", "db"), func() time.Time { return now }, nil)
	db := st.DB
	cutoff := now.AddDate(0, 0, -90)
	older, at := store.FormatTime(cutoff.Add(-1)), store.FormatTime(cutoff)
	exec(t, db, `INSERT INTO ai_requests(id,ts) VALUES ('old',?),('at',?)`, older, at)
	exec(t, db, `INSERT INTO events(ts,kind) VALUES (?,'old'),(?,'at')`, older, at)
	exec(t, db, `INSERT INTO task_idempotency(actor,action,idempotency_key,response,created_at) VALUES ('a','x','old','',?),('a','x','at','',?)`, older, at)
	chatCutoff := now.AddDate(0, 0, -7)
	insertLetters(t, db, letterRow{id: "old", createdAt: store.FormatTime(chatCutoff.Add(-1)), closed: true},
		letterRow{id: "at", createdAt: store.FormatTime(chatCutoff), closed: true})

	res, err := svc.Run()
	if err != nil || res.Error != "" {
		t.Fatalf("run: %+v %v", res, err)
	}
	for _, q := range []string{
		`SELECT group_concat(id) FROM ai_requests`,
		`SELECT group_concat(kind) FROM events`,
		`SELECT group_concat(idempotency_key) FROM task_idempotency`,
		`SELECT group_concat(id) FROM mail`,
	} {
		var got string
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if got != "at" {
			t.Errorf("%s = %q, want only the row at the cutoff", q, got)
		}
	}
	if _, err := store.ParseTime(res.StartedAt); err != nil {
		t.Errorf("started_at: %v", err)
	}
	if _, err := store.ParseTime(res.FinishedAt); err != nil {
		t.Errorf("finished_at: %v", err)
	}
}
