package tasks

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Reasons CloseOrphanWaits gives for closing a wait.
const (
	orphanReasonNotFound    = "principal_not_found"
	orphanReasonInvalidName = "invalid_principal_name"
)

// orphanWait is an open wait CloseOrphanWaits closes.
type orphanWait struct {
	id        int64
	task      Task
	principal string
	requester string
	reason    string
}

// CloseOrphanWaits closes the open waits on a principal that does not exist,
// which older daemons opened for mentions such as "@agent:name." and which no
// answer can ever resolve. A wait on the customer or on a registered agent
// stays open; with no customer login every wait on a user stays open. The
// step changes no task status and logs each closure, then the count.
func CloseOrphanWaits(ctx context.Context, db *sql.DB, customer string, now func() time.Time, log *slog.Logger) (int, error) {
	customer = strings.TrimSpace(customer)
	stamp := store.FormatTime(now())
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	orphans, err := findOrphanWaitsTx(ctx, tx, customer)
	if err != nil {
		return 0, err
	}
	if err := closeOrphanWaitsTx(ctx, tx, orphans, stamp); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for _, wait := range orphans {
		log.Info("closed wait on missing principal",
			"task", wait.task.Key, "wait_id", wait.id, "principal", wait.principal, "reason", wait.reason)
	}
	log.Info("closed waits on missing principals", "count", len(orphans))
	return len(orphans), nil
}

func findOrphanWaitsTx(ctx context.Context, tx *sql.Tx, customer string) ([]orphanWait, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT w.id, w.task_id, t.task_key, w.expected_principal, w.requesting_principal
		FROM task_waiting_for w JOIN tasks t ON t.id = w.task_id
		WHERE w.resolved_at = ''
		ORDER BY w.id`)
	if err != nil {
		return nil, err
	}
	open := []orphanWait{}
	for rows.Next() {
		var wait orphanWait
		if err := rows.Scan(&wait.id, &wait.task.ID, &wait.task.Key, &wait.principal, &wait.requester); err != nil {
			rows.Close()
			return nil, err
		}
		open = append(open, wait)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	orphans := []orphanWait{}
	for _, wait := range open {
		reason, err := orphanReason(ctx, tx, customer, wait.principal)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			wait.reason = reason
			orphans = append(orphans, wait)
		}
	}
	return orphans, nil
}

// orphanReason returns why a wait on principal is closed, or "" to keep it.
// The customer is checked first and existence before the name, so the
// customer's login and a registered agent's name never close a wait even
// when a mention cannot spell them.
func orphanReason(ctx context.Context, q queryer, customer, principal string) (string, error) {
	if strings.HasPrefix(principal, "user:") && (customer == "" || principal == userPrincipal(customer)) {
		return "", nil
	}
	exists, err := principalExists(ctx, q, customer, principal)
	if err != nil || exists {
		return "", err
	}
	if !principalNameRE.MatchString(principal) {
		return orphanReasonInvalidName, nil
	}
	return orphanReasonNotFound, nil
}

func closeOrphanWaitsTx(ctx context.Context, tx *sql.Tx, orphans []orphanWait, now string) error {
	type recipient struct {
		taskID    int64
		principal string
	}
	askers := map[recipient][]string{}
	order := []orphanWait{}
	for _, wait := range orphans {
		if _, err := tx.ExecContext(ctx, `
			UPDATE task_waiting_for SET resolved_at = ?, resolving_comment_id = NULL
			WHERE id = ? AND resolved_at = ''`, now, wait.id); err != nil {
			return err
		}
		key := recipient{wait.task.ID, wait.principal}
		if _, seen := askers[key]; !seen {
			order = append(order, wait)
		}
		if !slices.Contains(askers[key], wait.requester) {
			askers[key] = append(askers[key], wait.requester)
		}
	}
	for _, wait := range order {
		key := recipient{wait.task.ID, wait.principal}
		if err := closeQuestionLettersTx(ctx, tx, wait.task, wait.principal, askers[key],
			"task-wait-cleanup:"+wait.reason, now); err != nil {
			return err
		}
	}
	return nil
}
