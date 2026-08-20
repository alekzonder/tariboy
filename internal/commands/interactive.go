package commands

import (
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/shim"
)

func agentScreen() registry.Command {
	return registry.Command{
		Path:    "agent.screen",
		Summary: "Capture the interactive screen (tmux capture-pane)",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/screen"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			out, err := c.Control.Screen(str(p, "name"))
			if err != nil {
				return nil, api.UserError{Code: "screen_failed", Msg: err.Error()}
			}
			return map[string]any{"screen": out}, nil
		},
	}
}

func agentSendKeys() registry.Command {
	return registry.Command{
		Path:    "agent.send-keys",
		Summary: "Send keys into the interactive session (raw items, or a line+Enter)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "keys", Type: registry.String, Help: "line to send (a trailing Enter is added)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/send-keys"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			name := str(p, "name")
			// The web UI sends raw per-keystroke items ({text}|{key}), transmitted
			// with NO trailing Enter. The CLI still sends a single `keys` line,
			// which keeps the legacy line+Enter behavior.
			if items, ok := parseKeyItems(p["items"]); ok {
				if err := c.Control.SendKeysItems(name, items); err != nil {
					return nil, api.UserError{Code: "send_keys_failed", Msg: err.Error()}
				}
				return map[string]any{"name": name, "sent": len(items)}, nil
			}
			if err := c.Control.SendKeys(name, str(p, "keys")); err != nil {
				return nil, api.UserError{Code: "send_keys_failed", Msg: err.Error()}
			}
			return map[string]any{"name": name, "sent": true}, nil
		},
	}
}

// parseKeyItems decodes a JSON `items` array (already decoded into
// registry.Params as []any of {text|key} objects) into []shim.KeyItem. Returns
// ok=false when the field is absent, not an array, or yields no valid items.
func parseKeyItems(v any) ([]shim.KeyItem, bool) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil, false
	}
	out := make([]shim.KeyItem, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		it := shim.KeyItem{}
		if t, ok := m["text"].(string); ok {
			it.Text = t
		}
		if k, ok := m["key"].(string); ok {
			it.Key = k
		}
		if it.Text != "" || it.Key != "" {
			out = append(out, it)
		}
	}
	return out, len(out) > 0
}
