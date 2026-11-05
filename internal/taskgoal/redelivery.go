package taskgoal

import (
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	basestore "github.com/alekzonder/tariboy/internal/store"
)

// redeliveryPauseMax bounds the growing pause before a Goal is sent again
// after failed iterations. It never shortens the configured cooldown.
const redeliveryPauseMax = 10 * time.Minute

// failureStreakWindow is how many recent iterations FailureStreak reads: the
// pause reaches its bound long before.
const failureStreakWindow = 64

// EventGoalRedelivered is the audit event written for each Goal sent again
// after a failed iteration.
const EventGoalRedelivered = "goal_redelivered"

// Recorder is the slice of the agent's audit log the reconciler writes to.
type Recorder interface {
	Record(typ, source, iterationID string, data map[string]any) int64
}

// Stopper stops an armed timer.
type Stopper interface {
	Stop() bool
}

// FailureStreak is an agent's run of failed iterations, newest first.
type FailureStreak struct {
	// Count is the iterations in a row that ended without loop.sh done
	// (harness_error, timeout, no_i_am_done) since the last done one; killed
	// and failed iterations are skipped.
	Count      int
	LastID     string
	LastStatus agent.IterationStatus
	LastEnded  time.Time
	Running    bool // the newest iteration still runs
	Halted     bool // a stop policy halted the loop (agents.error_reason)
	Cooldown   time.Duration
}

// Failed reports whether the newest iteration is a finished failure.
func (f FailureStreak) Failed() bool { return isFailure(f.LastStatus) }

// Due is when the Goal is sent again after the newest failed iteration.
func (f FailureStreak) Due() time.Time {
	return f.LastEnded.Add(redeliveryPause(f.Cooldown, f.Count))
}

func isFailure(status agent.IterationStatus) bool {
	switch status {
	case agent.IterationHarnessError, agent.IterationTimeout, agent.IterationNoIAmDone:
		return true
	}
	return false
}

// FailureStreak reads the agent's failed iterations in a row from the
// iterations table, so the pause survives a daemon restart.
func (s *Store) FailureStreak(agentName string) (FailureStreak, error) {
	var streak FailureStreak
	var cooldownS int
	var errorReason string
	if err := s.db.QueryRow(`SELECT goal_delivery_cooldown_s, error_reason FROM agents WHERE name=?`, agentName).
		Scan(&cooldownS, &errorReason); err != nil {
		return FailureStreak{}, err
	}
	streak.Cooldown = time.Duration(cooldownS) * time.Second
	streak.Halted = errorReason != ""
	// rowid DESC is insertion order, newest first, as in agent.IdleStreak.
	rows, err := s.db.Query(`SELECT id, status, ended_at FROM iterations WHERE agent=?
		ORDER BY rowid DESC LIMIT ?`, agentName, failureStreakWindow)
	if err != nil {
		return FailureStreak{}, err
	}
	defer rows.Close()
	first := true
	for rows.Next() {
		var id, status, endedAt string
		if err := rows.Scan(&id, &status, &endedAt); err != nil {
			return FailureStreak{}, err
		}
		st := agent.IterationStatus(status)
		if first {
			first = false
			streak.LastID, streak.LastStatus = id, st
			streak.Running = st == agent.IterationRunning
			// Only a failed newest iteration needs its end time: the pause
			// runs from it.
			if isFailure(st) && endedAt != "" {
				if streak.LastEnded, err = basestore.ParseTime(endedAt); err != nil {
					return FailureStreak{}, fmt.Errorf("iteration %s ended_at: %w", id, err)
				}
			}
		}
		if st == agent.IterationDone {
			break
		}
		if isFailure(st) {
			streak.Count++
		}
	}
	return streak, rows.Err()
}

// redeliveryPause is the wait after the n-th failed iteration in a row:
// max(cooldown, min(cooldown × 2^(n−1), redeliveryPauseMax)).
func redeliveryPause(cooldown time.Duration, n int) time.Duration {
	pause := cooldown
	for i := 1; i < n && pause < redeliveryPauseMax; i++ {
		pause *= 2
	}
	return max(cooldown, min(pause, redeliveryPauseMax))
}

// redeliver decides the Goal wake after a failed newest iteration and
// reports whether it handled the agent. It sends the Goal again with reason
// retry once the growing pause has passed, at most once per failed
// iteration, and writes a goal_redelivered event; before that it arms the
// timer. A halted loop gets nothing until Start clears the halt.
func (r *Reconciler) redeliver(name string, goal Goal) (bool, error) {
	streak, err := r.store.FailureStreak(name)
	if err != nil {
		return true, fmt.Errorf("read failed iterations of %s: %w", name, err)
	}
	if !streak.Failed() {
		return false, nil
	}
	if streak.Halted {
		return true, nil
	}
	now := r.clock().UTC()
	if due := streak.Due(); now.Before(due) {
		r.armRedelivery(due)
		return true, nil
	}
	goal.Reason = "retry"
	letter := goalLetter(goal, streak.LastID)
	sent, err := r.store.letterSent(letter.To, letter.IdempotencyKey)
	if err != nil || sent {
		return true, err
	}
	allowed, err := r.store.GoalDeliveryAllowed(name, now)
	if err != nil {
		return true, fmt.Errorf("check task goal delivery for %s: %w", name, err)
	}
	if !allowed {
		return true, nil
	}
	if _, err := r.mail.Send(letter); err != nil {
		return true, fmt.Errorf("send task goal for %s: %w", name, err)
	}
	if err := r.store.MarkGoalDelivered(name, now); err != nil {
		return true, fmt.Errorf("record task goal delivery for %s: %w", name, err)
	}
	if r.audit != nil {
		if rec := r.audit(name); rec != nil {
			rec.Record(EventGoalRedelivered, "system", streak.LastID, map[string]any{
				"task_key": goal.TaskKey, "failures": streak.Count,
				"pause_s": int(redeliveryPause(streak.Cooldown, streak.Count) / time.Second), "last_status": string(streak.LastStatus),
			})
		}
	}
	return true, nil
}

// letterSent reports whether recipient already holds a letter with key.
func (s *Store) letterSent(recipient, key string) (bool, error) {
	var sent bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM mail WHERE recipient=? AND idempotency_key=?)`, recipient, key).Scan(&sent)
	return sent, err
}

// armRedelivery arms the one redelivery timer for due unless it is already
// armed for an earlier or equal time. The timer wakes a full pass.
func (r *Reconciler) armRedelivery(due time.Time) {
	r.timerMu.Lock()
	defer r.timerMu.Unlock()
	if r.timer != nil && !due.Before(r.timerDue) {
		return
	}
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timerGen++
	gen := r.timerGen
	r.timerDue = due
	r.timer = r.afterFunc(due.Sub(r.clock()), func() {
		r.timerMu.Lock()
		if r.timerGen == gen {
			r.timer = nil
		}
		r.timerMu.Unlock()
		r.Signal()
	})
}

func (r *Reconciler) stopRedeliveryTimer() {
	r.timerMu.Lock()
	defer r.timerMu.Unlock()
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.timerGen++
}
