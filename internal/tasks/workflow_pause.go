package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// Pause reasons, stored in tasks.workflow_paused_reason, and the decisions
// that resolve a pause.
const (
	PauseIdleIterations    = "idle_iterations"
	PauseRejectedRequests  = "rejected_requests"
	PauseScriptFailures    = "script_failures"
	PauseHolderUnavailable = "holder_unavailable"

	ResumeContinue = "continue"
	ResumeRelease  = "release"
)

// maxPauseDetailBytes bounds the untrusted detail a pause comment quotes, and
// maxPauseAskerBytes the principal it names as the asker of an open question.
const (
	maxPauseDetailBytes = 1 << 10
	maxPauseAskerBytes  = 200
)

// pauseReasonText says what a pause reason means, for the customer.
func pauseReasonText(reason string) string {
	switch reason {
	case PauseIdleIterations:
		return "the holder finished too many iterations in a row in this status without requesting a transition"
	case PauseRejectedRequests:
		return "checks rejected too many transition requests in a row in this status"
	case PauseScriptFailures:
		return "scripts failed too many times in a row in this status"
	case PauseHolderUnavailable:
		return "the holder cannot work on the task (disabled, halted, out of the pool, or deleted) and the grace period ran out"
	default:
		return reason
	}
}

// pauseTx pauses a workflow task. It is a no-op when the task is already
// paused or closed. The task keeps its status and assignee; its scripts and
// pending request stop, its category becomes wait_customer, and the customer
// is asked for a decision through a wait authored by the workflow. The caller
// commits and then signals.
func (s *WorkflowEngine) pauseTx(ctx context.Context, tx *sql.Tx, task *Task, manifest workflowimage.Manifest, reason, detail string) error {
	if task.WorkflowPausedReason != "" || task.Status == StatusDone || task.Status == StatusCancelled {
		return nil
	}
	now := s.now()
	if err := stopVisitScriptsTx(ctx, tx, *task, "the task was paused: "+reason, now); err != nil {
		return err
	}
	// The customer has one open wait per task: the pause question takes over
	// a question someone else asked, so the comment names its asker.
	var asker string
	err := tx.QueryRowContext(ctx, `
		SELECT requesting_principal FROM task_waiting_for
		WHERE task_id = ? AND expected_principal = ? AND resolved_at = '' AND requesting_principal <> ?`,
		task.ID, task.Customer, workflowActor).Scan(&asker)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	status, known := currentStatus(manifest, task.WorkflowStatus)
	pool := known && !status.Terminal && status.Owner.Kind == workflowfile.OwnerPool
	previousCategory := task.Status
	task.WorkflowPausedReason = reason
	if err := applyState(task, stateIn(*task, status)); err != nil {
		return err
	}
	if _, err := saveTaskStateTx(ctx, tx, task, previousCategory, now, false); err != nil {
		return err
	}
	if _, err := appendEventTx(ctx, tx, *task, "workflow.paused", Actor{Principal: workflowActor}, map[string]any{
		"status": task.WorkflowStatus, "reason": reason,
	}, now); err != nil {
		return err
	}
	return s.askCustomerTx(ctx, tx, *task, pauseComment(*task, reason, detail, asker, pool), now)
}

// pauseComment is the workflow's question to the customer on a paused task.
// detail is untrusted: it is cleaned, bounded, and quoted as text. asker is
// who asked the customer an open question the pause wait took over, or "";
// pool says whether the status is owned by an agent pool.
func pauseComment(task Task, reason, detail, asker string, pool bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "@%s The workflow paused this task in status %q and waits for your decision.\n\n", task.Customer, task.WorkflowStatus)
	fmt.Fprintf(&b, "Reason (`%s`): %s.\n", reason, pauseReasonText(reason))
	detail, cut := CutBytes(cleanPauseDetail(detail), maxPauseDetailBytes)
	if detail = strings.TrimSpace(detail); detail != "" {
		if cut {
			detail += CutMarker
		}
		b.WriteString("\nDetails:\n\n")
		b.WriteString(quoteBlock(detail))
	}
	if asker = strings.Join(strings.Fields(cleanPauseDetail(asker)), " "); asker != "" {
		asker, _ = CutBytes(asker, maxPauseAskerBytes)
		fmt.Fprintf(&b, "\nA question from %s is still unanswered; answering it in a comment before deciding is recommended.\n", CodeSpan(asker))
	}
	resume := "Resume the task in its current status; counters are reset"
	if pool {
		resume = "Continue with the same holder; counters are reset"
	}
	fmt.Fprintf(&b, "\nDecide with one of:\n\n"+
		"- %[2]s: `ttasks workflow resume %[1]s --decision continue`\n"+
		"- Release the holder and dispatch the task again (pool statuses only): `ttasks workflow resume %[1]s --decision release`\n"+
		"- Cancel the task: `ttasks cancel %[1]s`\n\n"+
		"A plain reply does not resume the task.", task.Key, resume)
	return b.String()
}

// cleanPauseDetail makes untrusted text safe to quote line by line: CRLF and
// CR become LF, every other C0 control but tab and LF, and the Unicode line
// breaks U+0085, U+2028, and U+2029, become a space, and invalid UTF-8
// becomes U+FFFD. Every line break left is an LF, which quoteBlock prefixes.
func cleanPauseDetail(text string) string {
	text = strings.ToValidUTF8(text, "\uFFFD")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 0x20 || r == '\u0085' || r == '\u2028' || r == '\u2029':
			return ' '
		}
		return r
	}, text)
}

// quoteBlock renders text as a Markdown block quote holding a fenced block
// whose fence is longer than any backtick run in text, so headings, fences,
// and mentions inside it render as plain text.
func quoteBlock(text string) string {
	fence := FenceFor(text, 3)
	var b strings.Builder
	b.WriteString("> " + fence + "text\n")
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(strings.TrimRight("> "+line, " ") + "\n")
	}
	b.WriteString("> " + fence + "\n")
	return b.String()
}

// ResumeWorkflow resolves a pause. Customer only. continue restores the
// category of the current status with the same holder; release, only in a
// pool status, marks the holder released and dispatches the task again to
// another member. Both reset the visit's counters and answer the pause wait.
func (s *WorkflowEngine) ResumeWorkflow(ctx context.Context, actor Actor, key, decision string) (Task, error) {
	if err := requireWorkflowOperator(actor); err != nil {
		return Task{}, err
	}
	decision = strings.TrimSpace(decision)
	if decision != ResumeContinue && decision != ResumeRelease {
		return Task{}, newError(http.StatusBadRequest, "invalid_decision",
			"a pause is resolved with --decision continue or --decision release")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	task, manifest, err := artifactTaskTx(ctx, tx, actor, key)
	if err != nil {
		return Task{}, err
	}
	status, pool, err := resumeDecisionStatus(task, manifest, decision)
	if err != nil {
		return Task{}, err
	}
	now := s.now()
	if err := guardRevisionTx(ctx, tx, task); err != nil {
		return Task{}, err
	}
	if err := resolveWorkflowWaitsTx(ctx, tx, task, "", now); err != nil {
		return Task{}, err
	}
	// resumed_at keeps an iteration that started while the task was paused
	// from counting as idle in the resumed visit.
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_status_visits SET idle_iterations = 0, rejected_requests = 0, script_failures = 0, resumed_at = ?
		WHERE task_id = ? AND left_at = ''`, now, task.ID); err != nil {
		return Task{}, err
	}
	reason := task.WorkflowPausedReason
	previousAssignee, previousCategory := task.Assignee, task.Status
	task.WorkflowPausedReason = ""
	if pool && decision == ResumeRelease {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_workflow_holders SET released = 1 WHERE task_id = ? AND pool = ?`,
			task.ID, status.Owner.Pool); err != nil {
			return Task{}, err
		}
	}
	// The previous holder is sticky unless released; otherwise this is the
	// dispatch of entering the status.
	state := stateIn(task, status)
	if err := s.pickStateHolderTx(ctx, tx, task, &state); err != nil {
		return Task{}, err
	}
	if status.Owner.Kind == workflowfile.OwnerScript && status.Watch != nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_status_visits SET next_watch_at = ? WHERE task_id = ? AND left_at = ''`,
			store.FormatTime(s.clock()), task.ID); err != nil {
			return Task{}, err
		}
	}
	if err := applyState(&task, state); err != nil {
		return Task{}, err
	}
	if _, err := saveTaskStateTx(ctx, tx, &task, previousCategory, now, false); err != nil {
		return Task{}, err
	}
	if _, err := appendEventTx(ctx, tx, task, "workflow.resumed", actor, map[string]any{
		"status": task.WorkflowStatus, "decision": decision, "actor": actor.Principal, "reason": reason,
	}, now); err != nil {
		return Task{}, err
	}
	if status.Owner.Kind == workflowfile.OwnerCustomer {
		if err := s.openStatusWaitTx(ctx, tx, task, status, now); err != nil {
			return Task{}, err
		}
	}
	if err := recordAssignmentTx(ctx, tx, task, previousAssignee, now); err != nil {
		return Task{}, err
	}
	resumed, err := taskByID(ctx, tx, task.ID)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	s.signal()
	resumed.Access = "write"
	return resumed, nil
}

// resumeDecisionStatus returns the current status of paused task and whether
// a pool owns it, refusing a task that is not paused, a status the workflow
// does not declare, and a release outside a pool status.
func resumeDecisionStatus(task Task, manifest workflowimage.Manifest, decision string) (workflowfile.Status, bool, error) {
	if task.WorkflowPausedReason == "" {
		return workflowfile.Status{}, false, newError(http.StatusConflict, "workflow_not_paused", "task "+task.Key+" is not paused")
	}
	status, ok := currentStatus(manifest, task.WorkflowStatus)
	if !ok || status.Terminal {
		return workflowfile.Status{}, false, newError(http.StatusConflict, "status_unknown",
			"task "+task.Key+" is in status "+task.WorkflowStatus+", which its workflow does not declare")
	}
	pool := status.Owner.Kind == workflowfile.OwnerPool
	if decision == ResumeRelease && !pool {
		return workflowfile.Status{}, false, newError(http.StatusBadRequest, "invalid_decision",
			"release applies only to a status owned by an agent pool; status "+status.ID+" is not")
	}
	return status, pool, nil
}
