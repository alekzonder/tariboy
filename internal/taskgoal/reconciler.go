package taskgoal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/mail"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

const DefaultReconcileInterval = time.Minute

// SenderGoal is the sender of every Goal letter.
const SenderGoal = "system:goal"

// Sender is the slice of the mail service the reconciler needs.
type Sender interface {
	Send(mail.Letter) ([]mail.Letter, error)
}

type ReconcilerConfig struct {
	Store    *basestore.Store
	Mail     Sender
	Clock    func() time.Time
	Interval time.Duration
	Log      *slog.Logger
}

type Reconciler struct {
	agents   *agent.Store
	store    *Store
	mail     Sender
	clock    func() time.Time
	interval time.Duration
	log      *slog.Logger
	signals  chan struct{}
	mu       sync.Mutex
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
		agents: agent.NewStore(config.Store), store: NewStore(config.Store), mail: config.Mail,
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

// IterationCompleted sends the next durable wake after a terminal
// iteration, keyed by the stable iteration ID.
func (r *Reconciler) IterationCompleted(agentName, iterationID string) {
	if err := r.Reconcile(context.Background(), agentName, iterationID); err != nil {
		r.log.Warn("task goal reconciliation failed", "agent", agentName, "iteration", iterationID, "err", err)
	}
}

// Reconcile selects current goals and sends a Goal letter for each
// non-waiting selection.
// Empty agentName scans every agent; failures are joined so later agents still
// receive their wake.
func (r *Reconciler) Reconcile(ctx context.Context, agentName, iterationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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
		allowed, err := r.store.GoalDeliveryAllowed(name, r.clock().UTC())
		if err != nil {
			failures = append(failures, fmt.Errorf("check task goal delivery for %s: %w", name, err))
			continue
		}
		if !allowed {
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
		if _, err := r.mail.Send(goalLetter(goal, generation)); err != nil {
			failures = append(failures, fmt.Errorf("send task goal for %s: %w", name, err))
		} else if err := r.store.MarkGoalDelivered(name, r.clock().UTC()); err != nil {
			failures = append(failures, fmt.Errorf("record task goal delivery for %s: %w", name, err))
		}
	}
	return errors.Join(failures...)
}

func (r *Reconciler) latestTerminalIteration(agentName string) (string, error) {
	var id string
	err := r.store.db.QueryRow(`SELECT id FROM iterations
		WHERE agent=? AND ended_at<>'' ORDER BY ended_at DESC, id DESC LIMIT 1`, agentName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// goalLetter is the request that wakes the agent for its Goal; its reply is
// the agent's report on it.
func goalLetter(goal Goal, iterationID string) mail.Letter {
	return mail.Letter{
		IdempotencyKey: fmt.Sprintf("task-goal:%s:%s:%d:%s", goal.Agent, goal.TaskKey, goal.Revision, iterationID),
		From:           SenderGoal,
		To:             "agent:" + goal.Agent,
		Kind:           "task.goal",
		Subject:        "Goal " + goal.TaskKey,
		Body:           fmt.Sprintf("Work your Goal %s (%s): read the Goal block of this prompt.", goal.TaskKey, goal.Reason),
		Data:           map[string]any{"task_key": goal.TaskKey, "reason": goal.Reason},
		NeedsReply:     true,
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
