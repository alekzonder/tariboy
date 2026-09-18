package agent

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// ErrInvalidTag is returned for a tag that is empty, too long, or contains a
// comma, whitespace, or a control character. The comma is reserved because the
// list filter passes tags as one comma-separated query parameter.
var ErrInvalidTag = errors.New("invalid_tag")

// ErrInvalidTime is returned for a started-at bound that is neither RFC3339 nor
// a bare YYYY-MM-DD date.
var ErrInvalidTime = errors.New("invalid_time")

const maxTagLen = 64

// TagOp selects how MutateIterationTags applies its tag list.
type TagOp string

const (
	TagOpAdd    TagOp = "add"
	TagOpRemove TagOp = "remove"
	// TagOpSet replaces each iteration's whole tag set; an empty list clears it.
	TagOpSet TagOp = "set"
)

// IterationFilter narrows a list of iterations. Tags match ANY of the listed
// tags; the started-at bounds are inclusive and accept RFC3339 or YYYY-MM-DD.
type IterationFilter struct {
	Tags          []string
	StartedAfter  string
	StartedBefore string
}

func normalizeTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	bad := func(r rune) bool { return r == ',' || unicode.IsSpace(r) || unicode.IsControl(r) }
	if tag == "" || len(tag) > maxTagLen || strings.ContainsFunc(tag, bad) {
		return "", fmt.Errorf("%w: %q", ErrInvalidTag, tag)
	}
	return tag, nil
}

func normalizeTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized, err := normalizeTag(tag)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}

// MutateIterationTags applies op to every listed iteration of one agent in a
// single transaction: either every iteration is updated or none is. An id that
// does not belong to agentName fails the whole batch with ErrNotFound. It
// returns the resulting tag set of each listed iteration so a caller does not
// need a second read.
func (s *Store) MutateIterationTags(agentName string, op TagOp, ids, tags []string) (map[string][]string, error) {
	normalized, err := normalizeTags(tags)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[string][]string{}, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, id := range ids {
		var owner string
		err := tx.QueryRow(`SELECT agent FROM iterations WHERE id=?`, id).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && owner != agentName) {
			return nil, fmt.Errorf("%w: iteration %s", ErrNotFound, id)
		}
		if err != nil {
			return nil, err
		}
		if op == TagOpSet {
			if _, err := tx.Exec(`DELETE FROM iteration_tags WHERE iteration_id=?`, id); err != nil {
				return nil, err
			}
		}
		for _, tag := range normalized {
			query := `INSERT OR IGNORE INTO iteration_tags(iteration_id, tag) VALUES (?,?)`
			if op == TagOpRemove {
				query = `DELETE FROM iteration_tags WHERE iteration_id=? AND tag=?`
			}
			if _, err := tx.Exec(query, id, tag); err != nil {
				return nil, err
			}
		}
	}
	out, err := queryIterationTags(tx, ids)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// IterationTags returns the sorted tags of each listed iteration. Every listed
// id is present in the result, with an empty slice when it has no tags.
func (s *Store) IterationTags(ids []string) (map[string][]string, error) {
	return queryIterationTags(s.db, ids)
}

type querier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func queryIterationTags(q querier, ids []string) (map[string][]string, error) {
	out := make(map[string][]string, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		out[id] = []string{}
		args = append(args, id)
	}
	if len(args) == 0 {
		return out, nil
	}
	rows, err := q.Query(`SELECT iteration_id, tag FROM iteration_tags
		WHERE iteration_id IN (`+placeholders(len(args))+`) ORDER BY iteration_id, tag`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return nil, err
		}
		out[id] = append(out[id], tag)
	}
	return out, rows.Err()
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

// startedBound normalizes one started-at filter bound into an SQL comparison.
// started_at is stored as RFC3339 written by one daemon, so a lexical string
// comparison orders it exactly like the existing ORDER BY started_at. A bare
// date covers the whole named day: the lower bound opens it, and the upper
// bound becomes an exclusive "< next day" so any time-of-day and offset on that
// day still compares inside it.
func startedBound(value string, upper bool) (op string, bound string, err error) {
	value = strings.TrimSpace(value)
	if day, dayErr := time.Parse("2006-01-02", value); dayErr == nil {
		if upper {
			return "<", day.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00", nil
		}
		return ">=", value + "T00:00:00", nil
	}
	if _, rfcErr := time.Parse(time.RFC3339, value); rfcErr != nil {
		return "", "", fmt.Errorf("%w: %q", ErrInvalidTime, value)
	}
	if upper {
		return "<=", value, nil
	}
	return ">=", value, nil
}

// FindIterations lists one agent's iterations, oldest first, narrowed by f.
func (s *Store) FindIterations(agentName string, f IterationFilter) ([]Iteration, error) {
	where := []string{"agent=?"}
	args := []any{agentName}
	for _, bound := range []struct {
		value string
		upper bool
	}{{f.StartedAfter, false}, {f.StartedBefore, true}} {
		if strings.TrimSpace(bound.value) == "" {
			continue
		}
		op, value, err := startedBound(bound.value, bound.upper)
		if err != nil {
			return nil, err
		}
		where = append(where, "started_at "+op+" ?")
		args = append(args, value)
	}
	tags, err := normalizeTags(f.Tags)
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM iteration_tags t
			WHERE t.iteration_id = iterations.id AND t.tag IN (`+placeholders(len(tags))+`))`)
		for _, tag := range tags {
			args = append(args, tag)
		}
	}
	rows, err := s.db.Query(`SELECT id, agent, trigger, status, started_at, ended_at,
		exit_code, done_flag, productive, prompt_path, cpu_ms, mem_peak_kb,
		timeout_period_s, timeout_deadline, hard_timeout_deadline,
		timeout_extensions, timeout_triggered_at, image_ref, image_version, image_digest, prompt_template_sha256, last_ai_request_at
		FROM iterations WHERE `+strings.Join(where, " AND ")+` ORDER BY started_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Iteration
	for rows.Next() {
		it, err := scanIteration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
