package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/registry"
)

func init() { typedCallSets["plugins"] = pluginsTypedCalls }

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
