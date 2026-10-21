package tasks

import (
	"strings"
	"testing"
)

// TestEveryTaskSpecBuilds reads the fields of every request, so a field type
// the wire forms do not support panics here rather than in a command.
func TestEveryTaskSpecBuilds(t *testing.T) {
	actions, routes := map[string]bool{}, map[string]bool{}
	for _, spec := range Specs() {
		if actions[spec.Action] {
			t.Errorf("action %s is listed twice", spec.Action)
		}
		actions[spec.Action] = true
		names := map[string]bool{}
		for _, f := range Fields(spec.New()) {
			names[f.Name] = true
		}
		if spec.Path != "" {
			route := spec.Method + " " + spec.Path
			if routes[route] {
				t.Errorf("%s: route %s has two owners", spec.Action, route)
			}
			routes[route] = true
			for _, segment := range strings.Split(spec.Path, "/") {
				if strings.HasPrefix(segment, "{") && !names[strings.Trim(segment, "{}")] {
					t.Errorf("%s: no field for path parameter %s", spec.Action, segment)
				}
			}
		}
		if via, ok := SpecFor(spec.Via); spec.Via != "" && (!ok || via.Path == "") {
			t.Errorf("%s: via %q has no route", spec.Action, spec.Via)
		}
		if (spec.agent == nil) != spec.OperatorOnly {
			t.Errorf("%s: an agent action exactly when not operator-only", spec.Action)
		}
	}
}

func TestFieldsRefuseAnUnsupportedType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Fields accepted a float64 field")
		}
	}()
	Fields(&struct {
		Ratio float64 `json:"ratio"`
	}{})
}

// TestAgentSocketKeys pins the body keys each agent tools-socket action reads.
// The tariboy-store agent skills depend on them.
func TestAgentSocketKeys(t *testing.T) {
	want := map[string]string{
		"list":             "queue status category priority assignee author customer group parent subtree waiting_for blocked text status_view sort limit after scope_agent by_author by_customer",
		"ready":            "queue limit idempotency_key claim",
		"show":             "key",
		"create":           "queue parent_key title description pull_request assignee group priority idempotency_key",
		"update":           "key title description status pull_request assignee manual_block_reason priority revision",
		"assign":           "key assignee revision",
		"comment":          "key body idempotency_key",
		"ask":              "key principal body idempotency_key",
		"move":             "key parent_key before_key revision",
		"relate":           "key target_key type revision idempotency_key",
		"block":            "key blocker_key revision idempotency_key",
		"done":             "key revision complete_anyway",
		"advance":          "key outcome message from",
		"artifact_set":     "key name value",
		"artifact_ls":      "key",
		"artifact_show":    "key name",
		"workflow_get":     "key",
		"workflow_runs":    "key",
		"workflow_run_log": "key id max_bytes",
		"request_get":      "key id",
	}
	for _, spec := range Specs() {
		if spec.OperatorOnly {
			continue
		}
		var keys []string
		for _, f := range Fields(spec.Request) {
			if f.Name != "" {
				keys = append(keys, f.Name)
			}
		}
		action := spec.Action
		if action == "mine" {
			action = "list"
		}
		if got := strings.Join(keys, " "); got != want[action] {
			t.Errorf("%s keys = %q; want %q", spec.Action, got, want[action])
		}
	}
}
