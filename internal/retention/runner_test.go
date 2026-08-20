package retention

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/store"
)

func TestRunnerTicksThenStops(t *testing.T) {
	dir := t.TempDir()
	s, _ := store.Open(filepath.Join(dir, "x.db"))
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	agentsDir := filepath.Join(dir, "agents")
	mkIter(t, as, agentsDir, "bot", "bot-1-1", "2026-07-01T10:00:00Z")
	mkIter(t, as, agentsDir, "bot", "bot-1-2", "2026-07-02T10:00:00Z")
	ps := NewStore(s)
	ps.Set("bot", Policy{KeepIterations: 1, Archive: false})
	pr := NewPruner(s, as, ps, agentsDir, func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) }, discardLog())

	// Injected after() fires once immediately, then blocks -> one prune, then idle.
	var ticks atomic.Int32
	after := func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		if ticks.Add(1) == 1 {
			ch <- time.Now()
		}
		return ch
	}
	r := NewRunner(pr, time.Hour, after, discardLog())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if its, _ := as.ListIterations("bot"); len(its) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("prune never ran")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
}
