package scriptnotify

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// A row becomes due exactly at next_attempt_at, compared by time within the
// same second: 10ns is before an 11ns attempt.
func TestPublisherDueComparesSameSecondByTime(t *testing.T) {
	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	db, runID := resultFixture(t, base)
	if _, err := db.DB.Exec(`UPDATE script_result_outbox SET next_attempt_at=? WHERE run_id=?`, store.FormatTime(base.Add(11)), runID); err != nil {
		t.Fatal(err)
	}
	now := base.Add(10)
	fake := &fakeBus{}
	publisher := New(db.DB, fake, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 0 {
		t.Fatalf("published %d messages before next_attempt_at", len(fake.messages))
	}
	now = base.Add(11)
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("messages at next_attempt_at = %d, want 1", len(fake.messages))
	}
	var publishedAt string
	if err := db.DB.QueryRow(`SELECT published_at FROM script_result_outbox WHERE run_id=?`, runID).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if publishedAt != store.FormatTime(now) {
		t.Fatalf("published_at = %q, want %q", publishedAt, store.FormatTime(now))
	}
}

// A failed publish schedules the retry in the stored format.
func TestPublisherBackoffIsCanonical(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	db, runID := resultFixture(t, now)
	publisher := New(db.DB, &fakeBus{fail: true}, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	var next string
	if err := db.DB.QueryRow(`SELECT next_attempt_at FROM script_result_outbox WHERE run_id=?`, runID).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if want := store.FormatTime(now.Add(2 * time.Second)); next != want {
		t.Fatalf("next_attempt_at = %q, want %q", next, want)
	}
}
