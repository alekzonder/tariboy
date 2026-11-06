package daemon

import (
	"context"
	"log/slog"
	"sync"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// taskBudgetHooks connect the AI proxy to task budgets: check runs on the
// request path, exhausted pauses or blocks the task off it, once at a time
// for each task key.
type taskBudgetHooks struct {
	svc      *tasks.Service
	log      *slog.Logger
	inflight sync.Map // task key -> struct{}
	wg       sync.WaitGroup
}

func newTaskBudgetHooks(svc *tasks.Service, log *slog.Logger) *taskBudgetHooks {
	return &taskBudgetHooks{svc: svc, log: log}
}

func (h *taskBudgetHooks) check(key string) (string, bool, error) {
	block, over, err := h.svc.ExhaustedBudget(context.Background(), key)
	return block.Key, over, err
}

// exhausted returns at once. A failure is logged; the next refusal retries.
func (h *taskBudgetHooks) exhausted(key string) {
	if _, busy := h.inflight.LoadOrStore(key, struct{}{}); busy {
		return
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer h.inflight.Delete(key)
		if err := h.svc.HandleBudgetExhausted(context.Background(), key); err != nil {
			h.log.Warn("task budget exhausted: pause or block failed", "task", key, "err", err)
		}
	}()
}
