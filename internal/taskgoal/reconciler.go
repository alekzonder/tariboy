package taskgoal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
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

type Reconciler struct {
	agents   *agent.Store
	store    *Store
	bus      MessagePublisher
	clock    func() time.Time
	interval time.Duration
	log      *slog.Logger
	signals  chan struct{}
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
		agents: agent.NewStore(config.Store), store: NewStore(config.Store), bus: config.Bus,
		clock: config.Clock, interval: config.Interval, log: config.Log, signals: make(chan struct{}, 1),
	}
}

// Signal coalesces task and agent configuration changes into one prompt scan.
func (r *Reconciler) Signal() {
	select {
	case r.signals <- struct{}{}:
	default:
	}
}

// IterationCompleted publishes the next durable wake after a terminal
// iteration, keyed by the stable iteration ID.
func (r *Reconciler) IterationCompleted(agentName, iterationID string) {
	if err := r.Reconcile(context.Background(), agentName, iterationID); err != nil {
		r.log.Warn("task goal reconciliation failed", "agent", agentName, "iteration", iterationID, "err", err)
	}
}

// Reconcile selects current goals and publishes each non-waiting selection.
// Empty agentName scans every agent; failures are joined so later agents still
// receive their wake.
func (r *Reconciler) Reconcile(ctx context.Context, agentName, iterationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	names := []string{agentName}
	if agentName == "" {
		agents, err := r.agents.List()
		if err != nil {
			return fmt.Errorf("list agents for task goals: %w", err)
		}
		names = make([]string, len(agents))
		for i := range agents {
			names[i] = agents[i].Name
		}
	}

	var failures []error
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		goal, err := r.store.ReconcileAgent(name, r.clock().UTC())
		if err != nil {
			failures = append(failures, fmt.Errorf("reconcile task goal for %s: %w", name, err))
			continue
		}
		if goal.TaskKey == "" || goal.Waiting {
			continue
		}
		generation := iterationID
		if generation == "" {
			generation, err = r.latestTerminalIteration(name)
			if err != nil {
				failures = append(failures, fmt.Errorf("read latest iteration for task goal %s: %w", name, err))
				continue
			}
		} else {
			goal.Reason = "iteration_completed"
		}
		if _, err := r.bus.Publish(goalMessage(goal, generation)); err != nil {
			failures = append(failures, fmt.Errorf("publish task goal for %s: %w", name, err))
		}
	}
	return errors.Join(failures...)
}

func (r *Reconciler) latestTerminalIteration(agentName string) (string, error) {
	var id string
	err := r.store.db.QueryRow(`SELECT id FROM iterations
		WHERE agent=? AND ended_at<>'' ORDER BY ended_at DESC, id DESC LIMIT 1`, agentName).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func goalMessage(goal Goal, iterationID string) bus.Message {
	return bus.Message{
		IdempotencyKey: fmt.Sprintf("task-goal:%s:%s:%d:%s", goal.Agent, goal.TaskKey, goal.Revision, iterationID),
		Channel:        bus.InboxChannel(goal.Agent),
		Source:         "tasks",
		Type:           "task.goal",
		Data:           map[string]any{"task_key": goal.TaskKey, "reason": goal.Reason},
	}
}

// Run performs startup recovery, then reconciles on the bounded cadence and
// coalesced mutation signals until cancellation.
func (r *Reconciler) Run(ctx context.Context) {
	run := func() {
		if err := r.Reconcile(ctx, "", ""); err != nil && ctx.Err() == nil {
			r.log.Warn("task goal reconciliation failed", "err", err)
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
		case <-r.signals:
			run()
		}
	}
}
