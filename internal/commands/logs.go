package commands

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

// logsCommand's non-follow path is a REMOTE command: the handler runs in the
// daemon (which owns the Store) and returns recent agent-scoped events. With -f
// cli.Run instead runs followAgentLogs, a CLI-local SSE stream over the socket.
// Registering the DB read as remote is what keeps `logs <agent>` (no -f) from
// dereferencing a nil Store when it dispatches in the CLI process.
func logsCommand() registry.Command {
	return registry.Command{
		Path:    "logs",
		Summary: "Stream or print an agent's events (-f to follow)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "follow", Flag: "follow", Short: "f", Type: registry.Bool, Help: "follow the live SSE stream"},
			{Name: "source", Flag: "source", Type: registry.String, Help: "comma types: message,stream,audit,iteration"},
			{Name: "since", Flag: "since", Type: registry.String, Help: "return events with seq greater than this (follow cursor)"},
			{Name: "before", Flag: "before", Type: registry.String, Help: "return events with seq less than this (scroll-back cursor)"},
			{Name: "limit", Flag: "limit", Type: registry.String, Help: "max events for a before= page (default 50, cap 200)"},
			{Name: "iteration", Flag: "iteration", Type: registry.String, Help: "return all events for this iteration id"},
			{Name: "type", Flag: "type", Type: registry.String, Help: "comma types: keep only events whose type is one of these"},
			{Name: "q", Flag: "q", Type: registry.String, Help: "full-text: keep only events whose record contains this substring (case-insensitive)"},
			{Name: "distinct", Flag: "distinct", Type: registry.String, Help: "distinct=types returns the sorted set of event types present"},
		},
		HTTP:       &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/logs"},
		FollowFlag: "follow",
		Follow:     followAgentLogs,
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			// Runs in the daemon: agent-scoped events from the per-agent audit.jsonl.
			// With ?since=<seq> returns events after that seq in chronological order
			// (the follow cursor); without it, the recent 50 newest-first.
			name := str(p, "name")
			// distinct=types: the "from real data" preset list for the audit-log
			// type filter. Checked first as it ignores the cursor params.
			if str(p, "distinct") == "types" {
				types, err := distinctTypes(c, name)
				if err != nil {
					return nil, err
				}
				return map[string]any{"types": types}, nil
			}
			// A type and/or full-text filter scans the whole log (like before=)
			// and returns matches newest-first, capped so a broad query can't
			// return an unbounded payload. capped=true flags a truncated result.
			if t := str(p, "type"); t != "" || str(p, "q") != "" {
				rows, capped, err := filteredEvents(c, name, t, str(p, "q"), 500)
				if err != nil {
					return nil, err
				}
				return map[string]any{"events": rows, "count": len(rows), "capped": capped}, nil
			}
			if id := str(p, "iteration"); id != "" {
				rows, err := eventsForIteration(c, name, id)
				if err != nil {
					return nil, err
				}
				return map[string]any{"events": rows, "count": len(rows)}, nil
			}
			if b := str(p, "before"); b != "" {
				before, _ := strconv.ParseInt(b, 10, 64)
				limit := 50
				if ls := str(p, "limit"); ls != "" {
					if n, err := strconv.Atoi(ls); err == nil && n > 0 {
						limit = n
					}
				}
				if limit > 200 {
					limit = 200
				}
				rows, err := eventsBefore(c, name, before, limit)
				if err != nil {
					return nil, err
				}
				return map[string]any{"events": rows, "count": len(rows)}, nil
			}
			if s := str(p, "since"); s != "" {
				since, _ := strconv.ParseInt(s, 10, 64)
				rows, err := eventsSince(c, name, since)
				if err != nil {
					return nil, err
				}
				return map[string]any{"events": rows, "count": len(rows)}, nil
			}
			rows, err := recentEvents(c, name, 50)
			if err != nil {
				return nil, err
			}
			return map[string]any{"events": rows, "count": len(rows)}, nil
		},
	}
}

// recentEvents returns the newest limit audit events for an agent, read from the
// per-agent audit.jsonl (the durable audit log). Shape is unchanged from the
// former DB-backed reader ({kind, data, at}) so the web UI needs no change.
func recentEvents(c *registry.Ctx, agent string, limit int) ([]map[string]any, error) {
	path := agentdir.New(paths.New(c.BaseDir).AgentsDir(), agent).AuditLog()
	evs, err := audit.ReadEvents(path, limit, 0)
	if err != nil {
		return nil, err
	}
	// Newest-first to match the previous `ORDER BY id DESC`. Non-nil so an agent
	// with zero events serializes as [] not null (a nil slice → JSON null crashes
	// the web UI audit-log page).
	out := []map[string]any{}
	for i := len(evs) - 1; i >= 0; i-- {
		out = append(out, eventRow(evs[i]))
	}
	return out, nil
}

// eventsSince returns audit events with seq > since in chronological (oldest
// first) order — the incremental cursor read that powers `logs -f`.
func eventsSince(c *registry.Ctx, agent string, since int64) ([]map[string]any, error) {
	path := agentdir.New(paths.New(c.BaseDir).AgentsDir(), agent).AuditLog()
	evs, err := audit.ReadEvents(path, 0, since)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, ev := range evs {
		out = append(out, eventRow(ev))
	}
	return out, nil
}

func eventRow(ev audit.Event) map[string]any {
	data := "{}"
	if ev.Data != nil {
		if b, err := json.Marshal(ev.Data); err == nil {
			data = string(b)
		}
	}
	return map[string]any{"seq": ev.Seq, "kind": ev.Type, "source": ev.Source,
		"iteration_id": ev.IterationID, "data": data, "at": ev.TS}
}

// eventsForIteration returns all events for one iteration, chronological.
func eventsForIteration(c *registry.Ctx, agent, iterationID string) ([]map[string]any, error) {
	path := agentdir.New(paths.New(c.BaseDir).AgentsDir(), agent).AuditLog()
	evs, err := audit.ReadByIteration(path, iterationID)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, ev := range evs {
		out = append(out, eventRow(ev))
	}
	return out, nil
}

// distinctTypes returns the sorted distinct event types present in the agent's
// audit log — the preset list backing the audit-log type filter.
func distinctTypes(c *registry.Ctx, agent string) ([]string, error) {
	path := agentdir.New(paths.New(c.BaseDir).AgentsDir(), agent).AuditLog()
	return audit.DistinctTypes(path)
}

// filteredEvents returns events matching a comma-separated type list AND a
// full-text query, newest-first (matching recentEvents), capped to limit. The
// bool reports whether the match set was truncated to the cap.
func filteredEvents(c *registry.Ctx, agent, types, query string, limit int) ([]map[string]any, bool, error) {
	path := agentdir.New(paths.New(c.BaseDir).AgentsDir(), agent).AuditLog()
	// Read all matches first so we can report truncation, then keep the newest.
	evs, err := audit.ReadFiltered(path, splitCSV(types), query, 0)
	if err != nil {
		return nil, false, err
	}
	capped := false
	if limit > 0 && len(evs) > limit {
		evs = evs[len(evs)-limit:]
		capped = true
	}
	// Newest-first to match recentEvents; non-nil so zero matches serialize as [].
	out := []map[string]any{}
	for i := len(evs) - 1; i >= 0; i-- {
		out = append(out, eventRow(evs[i]))
	}
	return out, capped, nil
}

// splitCSV splits a comma-separated list, dropping empty/blank fields.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// eventsBefore returns up to limit events with seq < before, chronological.
func eventsBefore(c *registry.Ctx, agent string, before int64, limit int) ([]map[string]any, error) {
	path := agentdir.New(paths.New(c.BaseDir).AgentsDir(), agent).AuditLog()
	evs, err := audit.ReadBefore(path, before, limit)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, ev := range evs {
		out = append(out, eventRow(ev))
	}
	return out, nil
}
