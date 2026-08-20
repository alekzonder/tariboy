package judge

import (
	"context"
	"github.com/alekzonder/tariboy/internal/bus"
	"time"
)

type RunnerConfig struct {
	Store       *Store
	Snapshotter interface {
		BuildRun(context.Context, string) error
	}
	Bus  *bus.Bus
	Tick time.Duration
}
type Runner struct {
	store       *Store
	snapshotter interface {
		BuildRun(context.Context, string) error
	}
	bus  *bus.Bus
	tick time.Duration
	wake chan string
}

func NewRunner(c RunnerConfig) *Runner {
	t := c.Tick
	if t <= 0 {
		t = time.Minute
	}
	return &Runner{store: c.Store, snapshotter: c.Snapshotter, bus: c.Bus, tick: t, wake: make(chan string, 32)}
}
func (r *Runner) Enqueue(id string) {
	select {
	case r.wake <- id:
	default:
	}
}
func (r *Runner) Run(ctx context.Context) {
	r.recover(ctx)
	ticker := time.NewTicker(r.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.recover(context.Background())
			return
		case id := <-r.wake:
			r.run(ctx, id)
		case <-ticker.C:
			r.recover(ctx)
		}
	}
}
func (r *Runner) recover(ctx context.Context) {
	if r.store == nil {
		return
	}
	runs, e := r.store.ListRuns(ListFilter{Statuses: []RunStatus{RunSnapshotting, RunRunning, RunSummarizing}})
	if e != nil {
		return
	}
	for _, x := range runs {
		r.run(ctx, x.ID)
	}
}
func (r *Runner) run(ctx context.Context, id string) {
	x, e := r.store.GetRun(id)
	if e != nil {
		return
	}
	if x.Status == RunSnapshotting && r.snapshotter != nil {
		if r.snapshotter.BuildRun(ctx, id) == nil {
			_ = r.store.CreateAssignments(id)
			x, _ = r.store.GetRun(id)
			r.work(x)
		}
	}
	if x.Status == RunRunning {
		_ = r.store.RecoverExpiredLeases(id)
		r.work(x)
	}
	x, _ = r.store.GetRun(id)
	if x.Status == RunSummarizing {
		r.summary(x)
	}
}
func (r *Runner) work(x Run) {
	if r.bus == nil {
		return
	}
	for _, a := range x.JudgeAgents {
		_, _ = r.bus.Publish(bus.Message{Channel: bus.InboxChannel(a), Type: "judge.work.available", Source: "system:judge", Text: "judge work available", Data: map[string]any{"run_id": x.ID}})
	}
}
func (r *Runner) summary(x Run) {
	if r.bus == nil || x.SummaryAgent == "" {
		return
	}
	_, _ = r.bus.Publish(bus.Message{Channel: bus.InboxChannel(x.SummaryAgent), Type: "judge.summary.ready", Source: "system:judge", Text: "judge summary ready", Data: map[string]any{"run_id": x.ID}})
}
