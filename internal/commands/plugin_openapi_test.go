package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/registry"
)

func init() {
	typedCallSets["plugins"] = pluginsTypedCalls
	typedCallSets["plugins-running"] = runningPluginTypedCalls
}

// pluginsTypedCalls serves the plugin, harness and secret responses from the
// golden fixture. Quiet covers the null types, channels and log lines; ghost
// covers the missing log; bare covers the empty secret list.
func pluginsTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	f := newPluginsMiscFixture(t)
	return f.c, []typedCall{
		{"GET", "/api/plugins", "/api/plugins", nil},
		{"GET", "/api/plugins/{name}", "/api/plugins/echo", nil},
		{"GET", "/api/plugins/{name}", "/api/plugins/quiet", nil},
		{"GET", "/api/plugin-contributions", "/api/plugin-contributions", nil},
		{"GET", "/api/plugins/{name}/logs", "/api/plugins/echo/logs?tail=2", nil},
		{"GET", "/api/plugins/{name}/logs", "/api/plugins/quiet/logs", nil},
		{"GET", "/api/plugins/{name}/logs", "/api/plugins/ghost/logs", nil},
		{"POST", "/api/plugins/{name}/restart", "/api/plugins/echo/restart", nil},
		{"DELETE", "/api/plugins/{name}", "/api/plugins/quiet", nil},
		{"POST", "/api/plugins", "/api/plugins", map[string]any{"path": f.widgetSrc}},
		{"GET", "/api/harnesses/{type}/models", "/api/harnesses/claude/models", nil},
		{"GET", "/api/harnesses/{type}/models", "/api/harnesses/stub/models", nil},
		{"GET", "/api/harnesses/codex/approval-mode", "/api/harnesses/codex/approval-mode", nil},
		{"POST", "/api/harnesses/codex/approval-mode", "/api/harnesses/codex/approval-mode", map[string]any{"mode": harness.CodexApprovalApproveForMe}},
		{"POST", "/api/agents/{name}/secrets", "/api/agents/smoke/secrets", map[string]any{"key": "TOKEN", "value": "v"}},
		{"GET", "/api/agents/{name}/secrets", "/api/agents/smoke/secrets", nil},
		{"GET", "/api/agents/{name}/secrets", "/api/agents/bare/secrets", nil},
		{"DELETE", "/api/agents/{name}/secrets", "/api/agents/smoke/secrets?key=TOKEN", nil},
	}
}

// runningPluginTypedCalls serves plugin.routes and plugin.action, which need
// a running plugin, from a fake host. Echo answers a routes object, a status
// result and a bind response without result; silent answers an empty routes
// body (null) and a scalar result.
func runningPluginTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, _, _ := ctxWithStore(t)
	c.Plugins = &fakePlugins{
		routesFn: func(name string) (map[string]any, error) {
			if name == "silent" {
				return nil, nil
			}
			return map[string]any{"routes": map[string]any{"c1": "chat:orders"}, "has_token": true}, nil
		},
		actionFn: func(name string, body map[string]any) (map[string]any, error) {
			switch {
			case name == "silent":
				return map[string]any{"result": "done"}, nil
			case body["action"] == "status":
				return map[string]any{"result": map[string]any{"connected": true}}, nil
			}
			return map[string]any{"ok": true, "subscriptions": []any{"chat:in"}}, nil
		},
	}
	return c, []typedCall{
		{"GET", "/api/plugins/{name}/routes", "/api/plugins/echo/routes", nil},
		{"GET", "/api/plugins/{name}/routes", "/api/plugins/silent/routes", nil},
		{"POST", "/api/plugins/{name}/action", "/api/plugins/echo/action", map[string]any{"action": "status"}},
		{"POST", "/api/plugins/{name}/action", "/api/plugins/echo/action", map[string]any{"action": "bind", "data": `{"chat":"c1"}`}},
		{"POST", "/api/plugins/{name}/action", "/api/plugins/silent/action", map[string]any{"action": "ping"}},
	}
}
