package commands

import (
	"strconv"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

// agentStatusHistory returns an agent's status-message timeline, read from the
// per-agent audit.jsonl (every status set is mirrored there as type="status"
// by the loop manager). Newest-first. Optional ?limit caps the count.
func agentStatusHistory() registry.Command {
	return registry.Command{
		Path:    "agent.status.history",
		Summary: "List an agent's status-message history (from the audit log)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "limit", Flag: "limit", Type: registry.String, Help: "max events (default all)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/status/history"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			name := str(p, "name")
			if _, err := getAgent(c, name); err != nil {
				return nil, err
			}
			path := agentdir.New(paths.New(c.BaseDir).AgentsDir(), name).AuditLog()
			evs, err := audit.ReadEvents(path, 0, 0)
			if err != nil {
				return nil, err
			}
			out := []map[string]any{}
			for i := len(evs) - 1; i >= 0; i-- { // newest-first
				ev := evs[i]
				if ev.Type != "status" {
					continue
				}
				msg, _ := ev.Data["message"].(string)
				out = append(out, map[string]any{"ts": ev.TS, "message": msg, "iteration_id": ev.IterationID})
			}
			if s := str(p, "limit"); s != "" {
				if n, err := strconv.Atoi(s); err == nil && n > 0 && len(out) > n {
					out = out[:n]
				}
			}
			return map[string]any{"events": out, "count": len(out)}, nil
		},
	}
}
