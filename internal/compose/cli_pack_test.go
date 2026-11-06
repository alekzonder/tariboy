package compose

import (
	"encoding/json"
	"strings"
	"testing"
)

type pluginsCaller struct{ names []string }

func (c pluginsCaller) Call(method, route string, _ any) (json.RawMessage, error) {
	rows := []map[string]string{}
	for _, n := range c.names {
		rows = append(rows, map[string]string{"name": n})
	}
	return json.Marshal(map[string]any{"plugins": rows, "count": len(rows)})
}

// validate --archive --daemon names each external plugin an archived image
// needs that the daemon lacks; built-in plugins need no install.
func TestPluginProblemsNameMissingExternalPlugins(t *testing.T) {
	p := &packed{plugins: map[string][]string{"dev:latest": {"tasks", "ghost", "present"}}}
	problems := pluginProblems(pluginsCaller{names: []string{"present"}}, p)
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "image dev:latest needs plugin ghost") {
		t.Fatalf("problems = %v", problems)
	}
}
