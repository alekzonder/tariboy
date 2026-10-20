package tasks

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// taskState is what the category, assignee, and waiting_on of a workflow task
// are derived from, as the table in docs/docs/task-workflows.mdx#status-and-category
// describes.
type taskState struct {
	Status     string // the workflow status ID, for errors
	Owner      string // owner kind of the status; "" for a terminal status
	Pool       string // the owning pool of a pool status
	Terminal   bool
	Cancelled  bool   // the terminal status declares cancelled: true
	Paused     bool   // the task has a pause reason
	Blocked    bool   // an open blocker or a manual block reason
	Holder     string // agent:NAME holding a pool status, "" when none
	HolderAsks bool   // the holder's question to the customer is open
	Assignee   string // the current assignee, kept where the table keeps it
}

// stateIn returns the state of task in status, without a holder.
func stateIn(task Task, status workflowfile.Status) taskState {
	state := taskState{
		Status: status.ID, Terminal: status.Terminal, Cancelled: status.Cancelled,
		Paused: task.WorkflowPausedReason != "", Blocked: task.Blocked, Assignee: task.Assignee,
	}
	if !status.Terminal {
		state.Owner, state.Pool = status.Owner.Kind, status.Owner.Pool
	}
	return state
}

// picksHolder reports whether entering or resuming the status dispatches a
// holder: only an unpaused, unblocked pool status does.
func (in taskState) picksHolder() bool {
	return in.Owner == workflowfile.OwnerPool && !in.Paused && !in.Blocked
}

// deriveCategory is the one place a workflow task's category, assignee, and
// waiting_on are decided.
func deriveCategory(in taskState) (category, assignee, waiting string, err error) {
	switch {
	case in.Terminal && in.Cancelled:
		return StatusCancelled, in.Assignee, "", nil
	case in.Terminal:
		return StatusDone, in.Assignee, "", nil
	case in.Paused:
		return StatusWaitCustomer, in.Assignee, WaitingOnPause, nil
	case in.Owner == workflowfile.OwnerPool && in.Holder == "":
		return StatusOpen, "", "", nil
	case in.Owner == workflowfile.OwnerPool && in.HolderAsks:
		return StatusWaitCustomer, in.Holder, WaitingOnCustomer, nil
	case in.Owner == workflowfile.OwnerPool:
		return StatusInProgress, in.Holder, "", nil
	case in.Owner == workflowfile.OwnerCustomer:
		return StatusWaitCustomer, in.Assignee, WaitingOnCustomer, nil
	case in.Owner == workflowfile.OwnerScript:
		return StatusWaitCustomer, "", WaitingOnScript, nil
	}
	return "", "", "", fmt.Errorf("workflow status %q has no owner", in.Status)
}

// waitingOn is the waiting_on of a stored workflow task: what its
// wait_customer category waits on.
func waitingOn(category, ownerKind string, paused bool) string {
	if category != StatusWaitCustomer {
		return ""
	}
	switch {
	case paused:
		return WaitingOnPause
	case ownerKind == workflowfile.OwnerCustomer, ownerKind == workflowfile.OwnerPool:
		// In a pool status the holder's question to the customer is the wait.
		return WaitingOnCustomer
	case ownerKind == workflowfile.OwnerScript:
		return WaitingOnScript
	}
	return ""
}

// applyState sets the category, assignee, and waiting_on derived from state
// on task.
func applyState(task *Task, state taskState) error {
	category, assignee, waiting, err := deriveCategory(state)
	if err != nil {
		return err
	}
	task.Status, task.Category, task.Assignee, task.WaitingOn = category, category, assignee, waiting
	return nil
}

// pickStateHolderTx dispatches the holder of a pool status into state.Holder
// when the state picks one; it stays "" when no member is eligible.
func (s *WorkflowEngine) pickStateHolderTx(ctx context.Context, tx *sql.Tx, task Task, state *taskState) error {
	if !state.picksHolder() {
		return nil
	}
	agent, err := pickHolderTx(ctx, tx, task, state.Pool, nil)
	if err != nil || agent == "" {
		return err
	}
	if err := s.recordHolderTx(ctx, tx, task, state.Pool, agent); err != nil {
		return err
	}
	state.Holder = agentPrincipal(agent)
	return nil
}
