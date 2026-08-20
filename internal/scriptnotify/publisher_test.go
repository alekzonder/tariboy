package scriptnotify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/store"
)

type fakeBus struct {
	messages []bus.Message
	fail     bool
}

func (f *fakeBus) Publish(message bus.Message) (bus.Message, error) {
	f.messages = append(f.messages, message)
	if f.fail {
		return bus.Message{}, errors.New("bus unavailable")
	}
	message.ID = "message-" + message.IdempotencyKey
	return message, nil
}

func resultFixture(t *testing.T, now time.Time) (*store.Store, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	scripts := script.NewStore(db, func() time.Time { return now })
	_, run, err := scripts.CreateOnce("alice", script.CreateOnce{Name: "check", Description: "check repo", Command: "make check"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := scripts.ClaimRun("alice", run.ID, now.Format(time.RFC3339), "/tmp/check.log"); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	exit := 2
	if _, err := scripts.CompleteRun("alice", run.ID, script.Completion{Status: script.RunFailed, ExitCode: &exit, FinishedAt: now.Format(time.RFC3339), LogPath: "/tmp/check.log"}); err != nil {
		t.Fatal(err)
	}
	return db, run.ID
}

func TestPublisherFlushPublishesScriptResultWithRunIdempotency(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	db, runID := resultFixture(t, now)
	fake := &fakeBus{}
	publisher := New(db.DB, fake, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("messages=%d want 1", len(fake.messages))
	}
	message := fake.messages[0]
	if message.IdempotencyKey != "script-result:"+runID || message.Channel != bus.InboxChannel("alice") || message.Source != "script" || message.Type != "script.result" || message.ProducedByAgent != "alice" {
		t.Fatalf("message=%#v", message)
	}
	if message.Data["run_id"] != runID || message.Data["exit_code"] != float64(2) || message.Data["log_path"] != "/tmp/check.log" {
		t.Fatalf("message data=%#v", message.Data)
	}
	if message.Text == "" || message.Text == "make check" {
		t.Fatalf("unsafe/empty message text=%q", message.Text)
	}
	var publishedAt, messageID string
	if err := db.DB.QueryRow(`SELECT published_at,message_id FROM script_result_outbox WHERE run_id=?`, runID).Scan(&publishedAt, &messageID); err != nil || publishedAt == "" || messageID == "" {
		t.Fatalf("published=%q message=%q err=%v", publishedAt, messageID, err)
	}
}

func TestPublisherLeavesFailedRowPendingUntilBackoff(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	db, runID := resultFixture(t, now)
	fake := &fakeBus{fail: true}
	publisher := New(db.DB, fake, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var publishedAt, nextAttempt, lastError string
	if err := db.DB.QueryRow(`SELECT attempts,published_at,next_attempt_at,last_error FROM script_result_outbox WHERE run_id=?`, runID).Scan(&attempts, &publishedAt, &nextAttempt, &lastError); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || publishedAt != "" || nextAttempt <= now.Format(time.RFC3339) || lastError == "" {
		t.Fatalf("attempts=%d published=%q next=%q error=%q", attempts, publishedAt, nextAttempt, lastError)
	}
	fake.fail = false
	if err := publisher.Flush(context.Background()); err != nil || len(fake.messages) != 1 {
		t.Fatalf("retry before backoff messages=%d err=%v", len(fake.messages), err)
	}
	now = now.Add(3 * time.Second)
	if err := publisher.Flush(context.Background()); err != nil || len(fake.messages) != 2 {
		t.Fatalf("retry after backoff messages=%d err=%v", len(fake.messages), err)
	}
}

func TestPublisherRetryUsesStableIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	db, _ := resultFixture(t, now)
	fake := &fakeBus{fail: true}
	publisher := New(db.DB, fake, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.fail = false
	now = now.Add(3 * time.Second)
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 2 || fake.messages[0].IdempotencyKey != fake.messages[1].IdempotencyKey {
		t.Fatalf("idempotency keys=%#v", fake.messages)
	}
}

func TestPublisherPublishesAfterDefinitionRemoval(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	db, runID := resultFixture(t, now)
	var scriptID string
	if err := db.DB.QueryRow(`SELECT script_id FROM script_result_outbox WHERE run_id=?`, runID).Scan(&scriptID); err != nil {
		t.Fatal(err)
	}
	if err := script.NewStore(db, func() time.Time { return now }).RemoveDefinition("alice", scriptID); err != nil {
		t.Fatal(err)
	}
	fake := &fakeBus{}
	publisher := New(db.DB, fake, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 || fake.messages[0].Data["run_id"] != runID {
		t.Fatalf("messages after definition removal=%#v", fake.messages)
	}
}
