package schedule

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
)

// wireDeadline reproduces the daemon's request-deadline seam (daemon.Run): a
// Request with a --deadline arms a one-shot timeout schedule that publishes a
// type=timeout event into the requester's inbox; a reply cancels it by
// correlation id. Kept in one place so the timeout-fires and reply-cancels paths
// exercise the exact production wiring.
func wireDeadline(b *bus.Bus, sched *Store, now func() time.Time) {
	b.SetDeadlineHooks(
		func(tx *sql.Tx, agent, correlationID, deadline string) error {
			dur, err := time.ParseDuration(deadline)
			if err != nil {
				return fmt.Errorf("request deadline %q: %w", deadline, err)
			}
			tmpl, err := json.Marshal(map[string]any{
				"type": "timeout",
				"text": "request timed out with no reply",
				"data": map[string]any{"correlation_id": correlationID},
			})
			if err != nil {
				return err
			}
			_, err = sched.AddTx(tx, Schedule{
				Agent: agent, Kind: "oneshot",
				Spec:            now().UTC().Add(dur).Format(time.RFC3339),
				Channel:         bus.InboxChannel(agent),
				MessageTemplate: string(tmpl),
				CorrelationID:   correlationID,
			})
			return err
		},
		func(tx *sql.Tx, correlationID string) error { return sched.CancelByCorrelationTx(tx, correlationID) },
	)
}

// TestRequestDeadlineTimeoutFires is the end-to-end deadline path against the
// real bus + schedule store + scheduler (spec §4.2): a group-style request with
// a deadline that gets NO reply must produce a type=timeout event carrying the
// correlation id in the requester's inbox once the deadline passes.
func TestRequestDeadlineTimeoutFires(t *testing.T) {
	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	now := base
	clock := func() time.Time { return now }

	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	b := bus.New(s, clock)
	sched := NewStore(s, clock)
	wireDeadline(b, sched, clock)

	// The requester receives the timeout on its own inbox; the target receives
	// the request on its group direct channel.
	if _, err := b.Subscribe("alice", bus.InboxChannel("alice"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Subscribe("carol", bus.GroupDirect("dev", "carol"), nil, nil); err != nil {
		t.Fatal(err)
	}

	req, err := b.Request("alice", bus.GroupDirect("dev", "carol"), "help", "5m")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if req.CorrelationID == "" {
		t.Fatal("request has no correlation id")
	}
	// The deadline armed exactly one timeout schedule for the requester.
	if list, _ := sched.List("alice"); len(list) != 1 {
		t.Fatalf("armed schedules = %+v", list)
	}

	// Before the deadline: nothing fires.
	scheduler := NewScheduler(sched, b, slog.New(slog.NewTextHandler(io.Discard, nil)), clock, nil)
	if n, _ := scheduler.fireDue(base.Add(time.Minute)); n != 0 {
		t.Fatalf("fired %d before deadline", n)
	}

	// After the deadline with no reply: the timeout fires once into alice's inbox.
	now = base.Add(6 * time.Minute)
	n, err := scheduler.fireDue(now)
	if err != nil || n != 1 {
		t.Fatalf("fireDue after deadline: n=%d err=%v", n, err)
	}
	msgs, err := b.Pending("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("alice inbox = %+v", msgs)
	}
	to := msgs[0]
	if to.Type != "timeout" || to.Kind != "event" {
		t.Fatalf("timeout message type/kind = %q/%q", to.Type, to.Kind)
	}
	if to.Data["correlation_id"] != req.CorrelationID {
		t.Fatalf("timeout correlation = %v, want %s", to.Data["correlation_id"], req.CorrelationID)
	}
	// A one-shot disables itself after firing — it does not fire again.
	if n, _ := scheduler.fireDue(base.Add(time.Hour)); n != 0 {
		t.Fatalf("one-shot re-fired: %d", n)
	}
}

// TestRequestDeadlineReplyCancelsTimeout is the other e2e leg: a reply landing
// before the deadline cancels the armed timeout so it never fires.
func TestRequestDeadlineReplyCancelsTimeout(t *testing.T) {
	base := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	now := base
	clock := func() time.Time { return now }

	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	b := bus.New(s, clock)
	sched := NewStore(s, clock)
	wireDeadline(b, sched, clock)

	if _, err := b.Subscribe("alice", bus.InboxChannel("alice"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Subscribe("carol", bus.GroupDirect("dev", "carol"), nil, nil); err != nil {
		t.Fatal(err)
	}

	req, err := b.Request("alice", bus.GroupDirect("dev", "carol"), "help", "5m")
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := sched.List("alice"); len(list) != 1 {
		t.Fatalf("armed schedules = %+v", list)
	}

	// Reply before the deadline cancels the timeout and retires the pending.
	if _, err := b.Reply("carol", req.ID, "on it", nil, ""); err != nil {
		t.Fatal(err)
	}
	if list, _ := sched.List("alice"); len(list) != 0 {
		t.Fatalf("reply did not cancel timeout: %+v", list)
	}
	if pend, _ := b.PendingRequests("alice"); len(pend) != 0 {
		t.Fatalf("reply did not retire pending: %+v", pend)
	}

	// Even past the deadline, nothing fires — the schedule is gone.
	scheduler := NewScheduler(sched, b, slog.New(slog.NewTextHandler(io.Discard, nil)), clock, nil)
	now = base.Add(time.Hour)
	if n, _ := scheduler.fireDue(now); n != 0 {
		t.Fatalf("cancelled timeout still fired: %d", n)
	}
}
