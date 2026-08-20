package judge

import (
	"encoding/json"
	"strings"
	"testing"
)

// The operator HTTP API returns Run/Target/Analysis/Summary verbatim, and the
// web UI (ui/src/lib/judge.ts) reads them as snake_case. Without explicit json
// tags Go emits the exported field names (PascalCase), leaving every field
// undefined in the UI and white-screening the judge pages. Lock the contract so
// that regression cannot recur.
func TestOperatorJSONIsSnakeCase(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want []string
	}{
		{"run", Run{}, []string{"id", "created_at", "creator_iteration", "original_request",
			"judge_group", "lead_agent", "summary_agent", "judge_agents", "judges_per_iteration",
			"max_attempts", "status", "targets_total", "targets_ready", "assignments_total",
			"assignments_completed", "manifest_hash", "current_summary_version", "last_error"}},
		{"target", Target{}, []string{"id", "run_id", "iteration", "agent", "sequence",
			"bundle_hash", "snapshot_status", "target_state", "consensus_verdict",
			"assignments_completed", "assignments_failed", "assignments_pending"}},
		{"analysis", Analysis{}, []string{"id", "run_id", "target_id", "assignment_id",
			"judge_agent", "judge_iteration", "created_at", "schema_version", "result"}},
		{"summary", Summary{}, []string{"id", "run_id", "summary_agent", "version", "result"}},
	}
	for _, c := range cases {
		b, err := json.Marshal(c.v)
		if err != nil {
			t.Fatalf("%s: marshal: %v", c.name, err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s: unmarshal: %v", c.name, err)
		}
		for _, k := range c.want {
			if _, ok := m[k]; !ok {
				t.Errorf("%s: missing snake_case key %q in %s", c.name, k, b)
			}
		}
		for k := range m {
			if k != strings.ToLower(k) || strings.ContainsAny(k, " ") {
				t.Errorf("%s: non-snake_case key %q leaked (PascalCase?)", c.name, k)
			}
		}
	}
}
