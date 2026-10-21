package bus

import (
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// The queue-depth gauge reads only pending deliveries, not the whole history.
func TestPendingTotalUsesPendingIndex(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	if plan := queryPlan(t, b, pendingTotalSQL, nil); !strings.Contains(plan, "INDEX idx_deliveries_pending") {
		t.Fatalf("PendingTotal plan does not use idx_deliveries_pending:\n%s", plan)
	}
}

// A message id costs one primary-key lookup on the channel row, whatever the
// channel history holds.
func TestNextMessageIDReadsChannelRowByKey(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	plan := queryPlan(t, b, channelSeqSQL, []any{"chat:x"})
	if strings.Contains(plan, "messages") || !strings.Contains(plan, "SEARCH channels USING INDEX sqlite_autoindex_channels_1 (name=?)") {
		t.Fatalf("nextMessageID plan is not a channel key lookup:\n%s", plan)
	}
}

func newBusAt(t *testing.T, now *time.Time) *Bus {
	t.Helper()
	return New(storetest.Open(t), func() time.Time { return *now })
}

func publishID(t *testing.T, b *Bus, ch string) string {
	t.Helper()
	m, err := b.Publish(Message{Channel: ch, Type: "note"})
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

func TestMessageIDSequencePerSecond(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	b := newBusAt(t, &now)
	ch := ChatChannel("seq")
	for i, want := range []string{"000000001", "000000002", "000000003"} {
		if id := publishID(t, b, ch); !strings.HasSuffix(id, "-20260706100000-"+want) {
			t.Fatalf("publish %d id=%s, want seq %s", i, id, want)
		}
	}
	now = now.Add(time.Second)
	if id := publishID(t, b, ch); !strings.HasSuffix(id, "-20260706100001-000000001") {
		t.Fatalf("new second id=%s, want seq 1", id)
	}
}

// When the clock steps back into a second that already has ids, the next id
// continues after them instead of colliding.
func TestMessageIDAfterClockStepsBack(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	b := newBusAt(t, &now)
	ch := ChatChannel("back")
	publishID(t, b, ch)
	now = now.Add(2 * time.Second)
	publishID(t, b, ch)
	now = now.Add(-2 * time.Second)
	if id := publishID(t, b, ch); !strings.HasSuffix(id, "-20260706100000-000000002") {
		t.Fatalf("id after clock step back=%s, want seq 2", id)
	}
	now = now.Add(2 * time.Second)
	if id := publishID(t, b, ch); !strings.HasSuffix(id, "-20260706100002-000000002") {
		t.Fatalf("id after clock recovery=%s, want seq 2", id)
	}
}

// A channel row without a counter (written before the counter existed) takes
// its first sequence from the stored ids of that second.
func TestMessageIDSeedsCounterFromHistory(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	b := newBusAt(t, &now)
	ch := ChatChannel("seed")
	publishID(t, b, ch)
	publishID(t, b, ch)
	if _, err := b.db.Exec(`UPDATE channels SET id_second=NULL, id_seq=0 WHERE name=?`, ch); err != nil {
		t.Fatal(err)
	}
	if id := publishID(t, b, ch); !strings.HasSuffix(id, "-20260706100000-000000003") {
		t.Fatalf("id after counter reset=%s, want seq 3", id)
	}
}
