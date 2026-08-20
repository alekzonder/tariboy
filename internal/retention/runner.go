package retention

import (
	"context"
	"log/slog"
	"time"
)

// RetentionAPI bundles the policy store and the pruner for the command layer
// (set on registry.Ctx).
type RetentionAPI struct {
	Policies *Store
	Pruner   *Pruner
}

// Runner periodically prunes every agent on an injected interval/after seam,
// drained before the store closes (spec §12/§13).
type Runner struct {
	pruner   *Pruner
	interval time.Duration
	after    func(time.Duration) <-chan time.Time
	log      *slog.Logger
}

func NewRunner(p *Pruner, interval time.Duration, after func(time.Duration) <-chan time.Time, log *slog.Logger) *Runner {
	if interval <= 0 {
		interval = time.Hour
	}
	if after == nil {
		after = time.After
	}
	if log == nil {
		log = slog.Default()
	}
	return &Runner{pruner: p, interval: interval, after: after, log: log}
}

func (r *Runner) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.after(r.interval):
			if _, err := r.pruner.PruneAll(false); err != nil {
				r.log.Warn("retention prune-all", "err", err)
			}
		}
	}
}
