package judge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type selectedIteration struct{ ID, Agent, Status string }

func terminal(status string) bool {
	switch status {
	case "done", "no_i_am_done", "harness_error", "timeout", "killed":
		return true
	}
	return false
}

func (s *Store) selectIterations(ctx context.Context, tx *sql.Tx, selector Selector) ([]selectedIteration, error) {
	seen := make(map[string]struct{})
	out := make([]selectedIteration, 0)
	add := func(row selectedIteration) error {
		if _, ok := seen[row.ID]; ok {
			return nil
		}
		if !terminal(row.Status) {
			return fmt.Errorf("%w: %s (%s)", ErrNonTerminalIteration, row.ID, row.Status)
		}
		seen[row.ID] = struct{}{}
		out = append(out, row)
		return nil
	}
	for _, id := range selector.ExplicitIDs {
		var row selectedIteration
		err := tx.QueryRowContext(ctx, "SELECT id, agent, status FROM iterations WHERE id=?", id).Scan(&row.ID, &row.Agent, &row.Status)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: iteration %s", ErrNotFound, id)
		}
		if err != nil {
			return nil, err
		}
		if err := add(row); err != nil {
			return nil, err
		}
	}
	if len(selector.Agents) == 0 && selector.Group == "" && selector.Since == "" && selector.Until == "" && len(selector.Statuses) == 0 {
		return out, nil
	}
	where, args := []string{"1=1"}, []any{}
	if len(selector.Agents) > 0 {
		where = append(where, "i.agent IN ("+placeholders(len(selector.Agents))+")")
		for _, v := range selector.Agents {
			args = append(args, v)
		}
	}
	if selector.Group != "" {
		where = append(where, "a.\"group\"=?")
		args = append(args, selector.Group)
	}
	if selector.Since != "" {
		where = append(where, "i.started_at>=?")
		args = append(args, selector.Since)
	}
	if selector.Until != "" {
		where = append(where, "i.started_at<=?")
		args = append(args, selector.Until)
	}
	if len(selector.Statuses) > 0 {
		where = append(where, "i.status IN ("+placeholders(len(selector.Statuses))+")")
		for _, v := range selector.Statuses {
			args = append(args, v)
		}
	}
	order := "ASC"
	if selector.Order == "newest" {
		order = "DESC"
	}
	if selector.Order != "" && selector.Order != "oldest" && selector.Order != "newest" {
		return nil, fmt.Errorf("judge: invalid selector order %q", selector.Order)
	}
	q := "SELECT i.id,i.agent,i.status FROM iterations i JOIN agents a ON a.name=i.agent WHERE " + strings.Join(where, " AND ") + " ORDER BY i.started_at " + order + ", i.id " + order
	if selector.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, selector.Limit)
	}
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row selectedIteration
		if err := rows.Scan(&row.ID, &row.Agent, &row.Status); err != nil {
			return nil, err
		}
		if err := add(row); err != nil {
			return nil, err
		}
	}
	return out, rows.Err()
}
func placeholders(n int) string { return strings.TrimRight(strings.Repeat("?,", n), ",") }
