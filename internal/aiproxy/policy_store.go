package aiproxy

import (
	"database/sql"
	"encoding/json"
	"time"
)

// rowScanner unifies *sql.Row and *sql.Rows for the shared scan helper (aiproxy
// scans inline elsewhere; this is the package's first scan interface).
type rowScanner interface{ Scan(dest ...any) error }

// PolicyRule is one stored rule of the general policy engine (spec §9). Allow/Deny
// are model globs; the rate-limit fields and the model-policy fields are used
// according to Kind.
type PolicyRule struct {
	ID          string
	Priority    int
	Scope       string // global | agent:<name> | group:<g>
	ModelGlob   string // '' = any
	Kind        string // rate-limit | model-policy
	MaxRequests int
	MaxTokens   int
	WindowS     int
	Allow       []string
	Deny        []string
	Route       string
	Enabled     bool
	CreatedAt   string
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetRule upserts a rule by id (generating one when empty).
func (s *Store) SetRule(r PolicyRule) error {
	if r.ID == "" {
		r.ID = NewRequestID(nil)
	}
	allow := mustJSONList(r.Allow)
	deny := mustJSONList(r.Deny)
	_, err := s.db.Exec(`INSERT INTO proxy_rules
		(id, priority, scope, model_glob, kind, max_requests, max_tokens, window_s, allow, deny, route, enabled)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			priority=excluded.priority, scope=excluded.scope, model_glob=excluded.model_glob,
			kind=excluded.kind, max_requests=excluded.max_requests, max_tokens=excluded.max_tokens,
			window_s=excluded.window_s, allow=excluded.allow, deny=excluded.deny,
			route=excluded.route, enabled=excluded.enabled`,
		r.ID, r.Priority, r.Scope, r.ModelGlob, r.Kind, r.MaxRequests, r.MaxTokens, r.WindowS,
		allow, deny, r.Route, boolToInt(r.Enabled))
	return err
}

func (s *Store) ListRules() ([]PolicyRule, error) {
	rows, err := s.db.Query(`SELECT id, priority, scope, model_glob, kind, max_requests, max_tokens,
		window_s, allow, deny, route, enabled FROM proxy_rules ORDER BY priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicyRule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRule(id string) (PolicyRule, bool, error) {
	row := s.db.QueryRow(`SELECT id, priority, scope, model_glob, kind, max_requests, max_tokens,
		window_s, allow, deny, route, enabled FROM proxy_rules WHERE id=?`, id)
	r, err := scanRule(row)
	if err == sql.ErrNoRows {
		return PolicyRule{}, false, nil
	}
	if err != nil {
		return PolicyRule{}, false, err
	}
	return r, true, nil
}

func (s *Store) DeleteRule(id string) error {
	_, err := s.db.Exec(`DELETE FROM proxy_rules WHERE id=?`, id)
	return err
}

// RequestCountSince counts ai_requests since a cutoff. members==nil ⇒ all agents
// (global scope); non-empty ⇒ agent IN (...). An empty (non-nil) slice ⇒ 0.
func (s *Store) RequestCountSince(members []string, since time.Time) (int, error) {
	q, args, ok := countQuery("COUNT(*)", members, since)
	if !ok {
		return 0, nil
	}
	var c int
	err := s.db.QueryRow(q, args...).Scan(&c)
	return c, err
}

// TokenSumSince sums input+output tokens since a cutoff, same member semantics.
func (s *Store) TokenSumSince(members []string, since time.Time) (int, error) {
	q, args, ok := countQuery("COALESCE(SUM(input_tokens+output_tokens),0)", members, since)
	if !ok {
		return 0, nil
	}
	var c int
	err := s.db.QueryRow(q, args...).Scan(&c)
	return c, err
}

// countQuery builds an aggregate over ai_requests. Returns ok=false when members
// is a non-nil empty slice (a scope with no members can have no rows).
func countQuery(agg string, members []string, since time.Time) (string, []any, bool) {
	if members != nil && len(members) == 0 {
		return "", nil, false
	}
	q := `SELECT ` + agg + ` FROM ai_requests WHERE ts>=?`
	args := []any{since.UTC().Format(time.RFC3339Nano)}
	if members != nil {
		q += ` AND agent IN (`
		for i, m := range members {
			if i > 0 {
				q += ","
			}
			q += "?"
			args = append(args, m)
		}
		q += ")"
	}
	return q, args, true
}

func mustJSONList(v []string) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func scanRule(row rowScanner) (PolicyRule, error) {
	var r PolicyRule
	var allow, deny string
	var enabled int
	if err := row.Scan(&r.ID, &r.Priority, &r.Scope, &r.ModelGlob, &r.Kind, &r.MaxRequests,
		&r.MaxTokens, &r.WindowS, &allow, &deny, &r.Route, &enabled); err != nil {
		return PolicyRule{}, err
	}
	r.Enabled = enabled != 0
	_ = json.Unmarshal([]byte(allow), &r.Allow)
	_ = json.Unmarshal([]byte(deny), &r.Deny)
	return r, nil
}
