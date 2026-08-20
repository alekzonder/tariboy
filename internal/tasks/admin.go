package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

func (s *Service) requireCustomer(actor Actor) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if !actor.IsCustomer || actor.Principal != userPrincipal(s.customer) {
		return domainError(http.StatusForbidden, "forbidden", "customer access required")
	}
	return nil
}

func (s *Service) ListQueues(ctx context.Context, actor Actor) ([]Queue, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	query := `
		SELECT prefix, name, description, responsible_agent, next_number,
		       revision, created_at, updated_at
		FROM task_queues`
	args := []any{}
	if !actor.IsCustomer {
		agent := strings.TrimPrefix(actor.Principal, "agent:")
		query += ` WHERE responsible_agent = ? OR EXISTS (
			SELECT 1 FROM task_queue_owners qo
			WHERE qo.queue_prefix = task_queues.prefix AND qo.agent = ?
		)`
		args = append(args, agent, agent)
	}
	query += ` ORDER BY prefix`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Queue{}
	for rows.Next() {
		var queue Queue
		if err := rows.Scan(&queue.Prefix, &queue.Name, &queue.Description,
			&queue.ResponsibleAgent, &queue.NextNumber, &queue.Revision,
			&queue.CreatedAt, &queue.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, queue)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Owners, err = queueOwners(ctx, s.db, out[i].Prefix)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) GetQueue(ctx context.Context, actor Actor, prefix string) (Queue, error) {
	if err := validateActor(actor); err != nil {
		return Queue{}, err
	}
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	var queue Queue
	err := s.db.QueryRowContext(ctx, `
		SELECT prefix, name, description, responsible_agent, next_number,
		       revision, created_at, updated_at
		FROM task_queues WHERE prefix = ?`, prefix).Scan(
		&queue.Prefix, &queue.Name, &queue.Description, &queue.ResponsibleAgent,
		&queue.NextNumber, &queue.Revision, &queue.CreatedAt, &queue.UpdatedAt)
	if err == sql.ErrNoRows {
		return Queue{}, domainError(http.StatusNotFound, "queue_not_found", "queue not found")
	}
	if err != nil {
		return Queue{}, err
	}
	if !actor.IsCustomer {
		agent := strings.TrimPrefix(actor.Principal, "agent:")
		allowed := queue.ResponsibleAgent == agent
		if !allowed {
			var count int
			if err := s.db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM task_queue_owners
				WHERE queue_prefix = ? AND agent = ?`, prefix, agent).Scan(&count); err != nil {
				return Queue{}, err
			}
			allowed = count > 0
		}
		if !allowed {
			return Queue{}, domainError(http.StatusNotFound, "queue_not_found", "queue not found")
		}
	}
	queue.Owners, err = queueOwners(ctx, s.db, prefix)
	return queue, err
}

func queueOwners(ctx context.Context, q queryer, prefix string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT agent FROM task_queue_owners
		WHERE queue_prefix = ? ORDER BY agent`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			return nil, err
		}
		out = append(out, owner)
	}
	return out, rows.Err()
}

func (s *Service) UpdateQueue(ctx context.Context, actor Actor, prefix string, in UpdateQueueInput) (Queue, error) {
	if err := s.requireCustomer(actor); err != nil {
		return Queue{}, err
	}
	current, err := s.GetQueue(ctx, actor, prefix)
	if err != nil {
		return Queue{}, err
	}
	if in.Revision <= 0 || in.Revision != current.Revision {
		return Queue{}, &Error{Status: http.StatusConflict, Code: "revision_conflict",
			Msg: "queue was changed by another actor", Data: map[string]any{
				"current_revision": current.Revision, "current": current,
			}}
	}
	if in.Name != nil {
		current.Name = strings.TrimSpace(*in.Name)
		if current.Name == "" {
			return Queue{}, domainError(http.StatusBadRequest, "missing_name", "queue name is required")
		}
	}
	if in.Description != nil {
		current.Description = strings.TrimSpace(*in.Description)
	}
	if in.ResponsibleAgent != nil {
		current.ResponsibleAgent = strings.TrimPrefix(normalizeAssignee(*in.ResponsibleAgent), "agent:")
	}
	if in.Owners != nil {
		current.Owners = normalizeOwners(*in.Owners)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Queue{}, err
	}
	defer tx.Rollback()
	now := s.now()
	result, err := tx.ExecContext(ctx, `
		UPDATE task_queues
		SET name = ?, description = ?, responsible_agent = ?,
		    revision = revision + 1, updated_at = ?
		WHERE prefix = ? AND revision = ?`,
		current.Name, current.Description, current.ResponsibleAgent,
		now, current.Prefix, current.Revision)
	if err != nil {
		return Queue{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Queue{}, err
	}
	if affected != 1 {
		var fresh Queue
		if err := tx.QueryRowContext(ctx, `
			SELECT prefix, name, description, responsible_agent, next_number,
			       revision, created_at, updated_at
			FROM task_queues WHERE prefix = ?`, current.Prefix).Scan(
			&fresh.Prefix, &fresh.Name, &fresh.Description, &fresh.ResponsibleAgent,
			&fresh.NextNumber, &fresh.Revision, &fresh.CreatedAt, &fresh.UpdatedAt,
		); err != nil {
			return Queue{}, err
		}
		fresh.Owners, err = queueOwners(ctx, tx, fresh.Prefix)
		if err != nil {
			return Queue{}, err
		}
		return Queue{}, &Error{Status: http.StatusConflict, Code: "revision_conflict",
			Msg: "queue was changed by another actor", Data: map[string]any{
				"current_revision": fresh.Revision, "current": fresh,
			}}
	}
	if in.Owners != nil {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM task_queue_owners WHERE queue_prefix = ?`, current.Prefix); err != nil {
			return Queue{}, err
		}
		for _, owner := range current.Owners {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO task_queue_owners(queue_prefix, agent) VALUES (?, ?)`,
				current.Prefix, owner); err != nil {
				return Queue{}, err
			}
		}
	}
	current.Revision++
	current.UpdatedAt = now
	if _, err := appendQueueEventTx(ctx, tx, current, "task.queue_updated", actor,
		map[string]any{"name": current.Name}, now); err != nil {
		return Queue{}, err
	}
	if err := tx.Commit(); err != nil {
		return Queue{}, err
	}
	s.signal()
	return current, nil
}

func normalizeOwners(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, owner := range values {
		owner = strings.TrimPrefix(normalizeAssignee(owner), "agent:")
		if owner == "" || seen[owner] {
			continue
		}
		seen[owner] = true
		out = append(out, owner)
	}
	sort.Strings(out)
	return out
}

func (s *Service) Principals(ctx context.Context, actor Actor) (PrincipalInfo, error) {
	if err := validateActor(actor); err != nil {
		return PrincipalInfo{}, err
	}
	info := PrincipalInfo{Customer: userPrincipal(s.customer), Agents: []string{}, Groups: []string{}}
	rows, err := s.db.QueryContext(ctx, `SELECT name, "group" FROM agents ORDER BY name`)
	if err != nil {
		return PrincipalInfo{}, err
	}
	groups := map[string]bool{}
	for rows.Next() {
		var name, group string
		if err := rows.Scan(&name, &group); err != nil {
			rows.Close()
			return PrincipalInfo{}, err
		}
		info.Agents = append(info.Agents, name)
		if group != "" {
			groups[group] = true
		}
	}
	if err := rows.Close(); err != nil {
		return PrincipalInfo{}, err
	}
	groupRows, err := s.db.QueryContext(ctx, `SELECT name FROM groups ORDER BY name`)
	if err != nil {
		return PrincipalInfo{}, err
	}
	for groupRows.Next() {
		var group string
		if err := groupRows.Scan(&group); err != nil {
			groupRows.Close()
			return PrincipalInfo{}, err
		}
		groups[group] = true
	}
	if err := groupRows.Close(); err != nil {
		return PrincipalInfo{}, err
	}
	for group := range groups {
		info.Groups = append(info.Groups, group)
	}
	sort.Strings(info.Groups)
	return info, nil
}

func (s *Service) ListEvents(ctx context.Context, actor Actor, key string, after int64, limit int) ([]Event, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	var task Task
	var err error
	if key != "" {
		detail, getErr := s.GetTask(ctx, actor, key)
		if getErr != nil {
			return nil, getErr
		}
		task = detail.Task
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := `
		SELECT e.sequence, e.event_id, COALESCE(t.task_key, ''), e.queue_prefix,
		       e.kind, e.actor, e.task_revision, e.payload, e.created_at
		FROM task_events e
		LEFT JOIN tasks t ON t.id = e.task_id
		WHERE e.sequence > ?`
	args := []any{after}
	if key != "" {
		query += ` AND e.task_id = ?`
		args = append(args, task.ID)
	} else if !actor.IsCustomer {
		visible, visibleErr := visibleTaskIDs(ctx, s.db, actor)
		if visibleErr != nil {
			return nil, visibleErr
		}
		agent := strings.TrimPrefix(actor.Principal, "agent:")
		query += ` AND (`
		if len(visible) > 0 {
			query += `e.task_id IN (` + placeholders(len(visible)) + `) OR `
			args = append(args, idsArgs(visible)...)
		}
		query += `(
			e.task_id IS NULL AND EXISTS (
				SELECT 1
				FROM task_queues q
				LEFT JOIN task_queue_owners qo
				  ON qo.queue_prefix = q.prefix AND qo.agent = ?
				WHERE q.prefix = e.queue_prefix
				  AND (q.responsible_agent = ? OR qo.agent IS NOT NULL)
			)
		))`
		args = append(args, agent, agent)
	}
	query += ` ORDER BY e.sequence LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var event Event
		var payload string
		if err := rows.Scan(&event.Sequence, &event.EventID, &event.TaskKey, &event.Queue,
			&event.Kind, &event.Actor, &event.TaskRevision, &payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.Payload = map[string]any{}
		_ = json.Unmarshal([]byte(payload), &event.Payload)
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Service) ListNotifications(ctx context.Context, actor Actor, includeDismissed bool) ([]Notification, error) {
	if err := s.requireCustomer(actor); err != nil {
		return nil, err
	}
	query := `
		SELECT o.notification_id, o.channel, o.message_type, o.text, e.actor,
		       COALESCE(t.task_key, ''), o.event_sequence, e.created_at,
		       o.published_at, state.read_at, state.dismissed_at
		FROM task_notification_outbox o
		JOIN task_events e ON e.sequence = o.event_sequence
		LEFT JOIN tasks t ON t.id = e.task_id
		JOIN task_notification_state state
		  ON state.notification_id = o.notification_id
		 AND state.customer_principal = ?
		WHERE o.channel = ?`
	args := []any{actor.Principal, actor.Principal}
	if !includeDismissed {
		query += ` AND state.dismissed_at = ''`
	}
	query += ` ORDER BY o.event_sequence DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var notification Notification
		if err := rows.Scan(&notification.ID, &notification.Channel, &notification.Type,
			&notification.Text, &notification.RequestingPrincipal, &notification.TaskKey, &notification.EventSeq,
			&notification.CreatedAt, &notification.PublishedAt,
			&notification.ReadAt, &notification.DismissedAt); err != nil {
			return nil, err
		}
		out = append(out, notification)
	}
	return out, rows.Err()
}

func (s *Service) MarkNotification(ctx context.Context, actor Actor, id, action string) (Notification, error) {
	if err := s.requireCustomer(actor); err != nil {
		return Notification{}, err
	}
	column := ""
	switch action {
	case "read":
		column = "read_at"
	case "dismiss":
		column = "dismissed_at"
	default:
		return Notification{}, domainError(http.StatusBadRequest, "invalid_action", "notification action is invalid")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE task_notification_state SET `+column+` = ?
		WHERE customer_principal = ? AND notification_id = ?`,
		s.now(), actor.Principal, id)
	if err != nil {
		return Notification{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return Notification{}, domainError(http.StatusNotFound, "notification_not_found", "notification not found")
	}
	list, err := s.ListNotifications(ctx, actor, true)
	if err != nil {
		return Notification{}, err
	}
	for _, notification := range list {
		if notification.ID == id {
			return notification, nil
		}
	}
	return Notification{}, domainError(http.StatusNotFound, "notification_not_found", "notification not found")
}
