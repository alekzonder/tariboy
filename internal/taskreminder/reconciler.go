package taskreminder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

const DefaultReconcileInterval = time.Minute

type MessagePublisher interface {
	Publish(bus.Message) (bus.Message, error)
}

type ReconcilerConfig struct {
	Store    *basestore.Store
	Bus      MessagePublisher
	Clock    func() time.Time
	Interval time.Duration
	Log      *slog.Logger
}

// Reconciler publishes one ordinary inbox reminder for each eligible task
// generation and records the generation only after Publish succeeds.
type Reconciler struct {
	configStore *basestore.Store
	store       *Store
	bus         MessagePublisher
	clock       func() time.Time
	interval    time.Duration
	log         *slog.Logger
}

func NewReconciler(config ReconcilerConfig) *Reconciler {
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.Interval <= 0 {
		config.Interval = DefaultReconcileInterval
	}
	if config.Log == nil {
		config.Log = slog.Default()
	}
	return &Reconciler{
		configStore: config.Store,
		store:       NewStore(config.Store),
		bus:         config.Bus,
		clock:       config.Clock,
		interval:    config.Interval,
		log:         config.Log,
	}
}

// Reconcile reads the current policy on every scan. Candidate failures are
// accumulated while later agents continue, so one unavailable inbox cannot
// prevent unrelated reminders from being delivered.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, _, err := r.configStore.ConfigGet("task_reminder")
	if err != nil {
		return fmt.Errorf("read task reminder policy: %w", err)
	}
	policy, err := ParsePolicy(raw)
	if err != nil {
		return fmt.Errorf("read task reminder policy: %w", err)
	}
	candidates, err := r.store.Eligible(policy, r.clock().UTC())
	if err != nil {
		return fmt.Errorf("select task reminder candidates: %w", err)
	}

	var failures []error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		_, err := r.bus.Publish(bus.Message{
			IdempotencyKey: "task-reminder:" + candidate.Fingerprint,
			Channel:        bus.InboxChannel(candidate.Agent),
			Source:         "tasks",
			Type:           "task.reminder",
			Data: map[string]any{
				"reason":           "assigned-work-idle",
				"idle_threshold_s": policy.IdleThresholdS,
				"task_keys":        append([]string(nil), candidate.TaskKeys...),
			},
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("publish task reminder for %s: %w", candidate.Agent, err))
			continue
		}
		if err := r.store.MarkSent(candidate, r.clock().UTC()); err != nil {
			failures = append(failures, fmt.Errorf("mark task reminder sent for %s: %w", candidate.Agent, err))
		}
	}
	return errors.Join(failures...)
}

// Run performs an initial scan, then scans on a bounded cadence until the
// daemon context is cancelled. Scan errors are logged and retried later.
func (r *Reconciler) Run(ctx context.Context) {
	run := func() {
		if err := r.Reconcile(ctx); err != nil && ctx.Err() == nil {
			r.log.Warn("task reminder reconciliation failed", "err", err)
		}
	}
	run()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
