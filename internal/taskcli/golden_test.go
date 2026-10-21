package taskcli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseGoldenPayloads pins the payload every task command sends: the body
// of its agent tools-socket action, which operator mode turns into its route.
func TestParseGoldenPayloads(t *testing.T) {
	tests := []struct {
		argv    []string
		action  string
		payload string
		noWait  bool
	}{
		{[]string{"list"}, "list", `{"limit":"50"}`, false},
		{[]string{"list", "--queue", "DEV", "--status", "plan,review", "--category", "in_progress,open", "--priority", "P0,P1", "--assignee", "worker", "--author", "user:me", "--customer", "user:you", "--group", "core", "--subtree", "DEV-1", "--waiting-for", "me", "--blocked", "false", "--text", "login bug", "--view", "all", "--sort", "updated", "--limit", "10", "--after", "DEV-9"},
			"list", `{"after":"DEV-9","assignee":"worker","author":"user:me","blocked":"false","category":"in_progress,open","customer":"user:you","group":"core","limit":"10","priority":"P0,P1","queue":"DEV","sort":"updated","status":"plan,review","status_view":"all","subtree":"DEV-1","text":"login bug","waiting_for":"me"}`, false},
		{[]string{"mine", "--queue=DEV", "--parent=DEV-1", "--blocked=true", "--limit=500"}, "mine", `{"blocked":"true","limit":"500","parent":"DEV-1","queue":"DEV"}`, false},
		{[]string{"ready"}, "ready", `{}`, false},
		{[]string{"ready", "--queue", "DEV", "--limit", "5", "--idempotency-key", "k1", "--claim"}, "ready", `{"claim":true,"idempotency_key":"k1","limit":"5","queue":"DEV"}`, false},
		{[]string{"ready", "--claim=false"}, "ready", `{}`, false},
		{[]string{"show", "DEV-1"}, "show", `{"key":"DEV-1"}`, false},
		{[]string{"create", "--queue", "DEV", "--title", "T", "--description", "D", "--pull-request", "https://example.test/pull/1", "--assignee", "worker", "--group", "core", "--priority", "P1", "--idempotency-key", "k1"},
			"create", `{"assignee":"worker","description":"D","group":"core","idempotency_key":"k1","priority":"P1","pull_request":"https://example.test/pull/1","queue":"DEV","title":"T"}`, false},
		{[]string{"create", "--parent", "DEV-1", "--title", "child"}, "create", `{"parent_key":"DEV-1","title":"child"}`, false},
		{[]string{"update", "DEV-1", "--title", "T", "--description", "D", "--status", "done", "--pull-request=", "--assignee", "worker", "--manual-block-reason", "", "--priority", "P0", "--revision", "3"},
			"update", `{"assignee":"worker","description":"D","key":"DEV-1","manual_block_reason":"","priority":"P0","pull_request":"","revision":"3","status":"done","title":"T"}`, false},
		{[]string{"update", "--manual-block-reason", "waiting", "DEV-1"}, "update", `{"key":"DEV-1","manual_block_reason":"waiting"}`, false},
		{[]string{"assign", "DEV-1", "--revision", "2", "worker"}, "assign", `{"assignee":"worker","key":"DEV-1","revision":"2"}`, false},
		{[]string{"comment", "DEV-1", "hello", "world"}, "comment", `{"body":"hello world","key":"DEV-1"}`, false},
		{[]string{"comment", "DEV-1", "--body", " text ", "--idempotency-key", "k1"}, "comment", `{"body":"text","idempotency_key":"k1","key":"DEV-1"}`, false},
		{[]string{"ask", "DEV-1", "worker", "why", "is", "it", "--idempotency-key", "k1"}, "ask", `{"body":"why is it","idempotency_key":"k1","key":"DEV-1","principal":"worker"}`, false},
		{[]string{"ask", "DEV-1", "user:me", "why"}, "ask", `{"body":"why","key":"DEV-1","principal":"user:me"}`, false},
		{[]string{"move", "DEV-1", "--parent", "DEV-2", "--revision", "4"}, "move", `{"key":"DEV-1","parent_key":"DEV-2","revision":"4"}`, false},
		{[]string{"move", "DEV-1", "--before", "DEV-3"}, "move", `{"before_key":"DEV-3","key":"DEV-1"}`, false},
		{[]string{"move", "DEV-1", "--to-root"}, "move", `{"key":"DEV-1","parent_key":""}`, false},
		{[]string{"block", "DEV-1", "--by", "DEV-2", "--revision", "1", "--idempotency-key", "k1"}, "block", `{"blocker_key":"DEV-2","idempotency_key":"k1","key":"DEV-1","revision":"1"}`, false},
		{[]string{"relate", "DEV-1", "DEV-2", "--revision", "1", "--idempotency-key", "k1"}, "relate", `{"idempotency_key":"k1","key":"DEV-1","revision":"1","target_key":"DEV-2"}`, false},
		{[]string{"done", "DEV-1", "--revision", "2", "--complete-anyway"}, "done", `{"complete_anyway":true,"key":"DEV-1","revision":"2"}`, false},
		{[]string{"done", "DEV-1", "--complete-anyway=false"}, "done", `{"key":"DEV-1"}`, false},
		{[]string{"advance", "DEV-1", "--outcome", " ready ", "--from", "develop", "--message", "PR opened"}, "advance", `{"from":"develop","key":"DEV-1","message":"PR opened","outcome":"ready"}`, false},
		{[]string{"advance", "DEV-1", "--outcome", "ready", "--no-wait"}, "advance", `{"key":"DEV-1","outcome":"ready"}`, true},
		{[]string{"artifacts", "set", "DEV-1", "plan", " step 1\n"}, "artifact_set", `{"key":"DEV-1","name":"plan","value":" step 1\n"}`, false},
		{[]string{"artifacts", "set", "DEV-1", "plan", ""}, "artifact_set", `{"key":"DEV-1","name":"plan","value":""}`, false},
		{[]string{"artifacts", "ls", "DEV-1"}, "artifact_ls", `{"key":"DEV-1"}`, false},
		{[]string{"artifacts", "show", "DEV-1", "plan"}, "artifact_show", `{"key":"DEV-1","name":"plan"}`, false},
		{[]string{"workflow", "get", "DEV-1"}, "workflow_get", `{"key":"DEV-1"}`, false},
		{[]string{"workflow", "runs", "DEV-1"}, "workflow_runs", `{"key":"DEV-1"}`, false},
		{[]string{"workflow", "log", "DEV-1", "4", "--max-bytes", "100"}, "workflow_run_log", `{"id":4,"key":"DEV-1","max_bytes":100}`, false},
		{[]string{"workflow", "move", "DEV-1", "--to", "develop", "--reason", "regression"}, "workflow_move", `{"key":"DEV-1","reason":"regression","to":"develop"}`, false},
		{[]string{"workflow", "resume", "DEV-1", "--decision", "release"}, "workflow_resume", `{"decision":"release","key":"DEV-1"}`, false},
		{[]string{"workflow", "retry", "DEV-1"}, "workflow_retry", `{"key":"DEV-1"}`, false},
		{[]string{"workflow", "retry", "DEV-1", "41"}, "workflow_retry", `{"id":41,"key":"DEV-1"}`, false},
		{[]string{"cancel", "DEV-1"}, "cancel", `{"key":"DEV-1"}`, false},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.argv, " "), func(t *testing.T) {
			got, err := parse(tt.argv)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(got.payload)
			if got.action != tt.action || string(raw) != tt.payload || got.noWait != tt.noWait {
				t.Fatalf("parse = %q %s noWait=%v; want %q %s noWait=%v", got.action, raw, got.noWait, tt.action, tt.payload, tt.noWait)
			}
		})
	}
}

// TestParseGoldenErrors pins the usage error text of every refused command.
func TestParseGoldenErrors(t *testing.T) {
	tests := []struct {
		argv []string
		want string
	}{
		{[]string{}, "tasks: a command is required"},
		{[]string{"frob"}, `tasks: unknown command "frob"`},
		{[]string{"artifacts"}, "tasks artifacts: a command is required"},
		{[]string{"workflow", "--json-ish"}, "tasks workflow: a command is required"},
		{[]string{"artifacts", "frob", "DEV-1"}, `tasks: unknown command "artifacts frob"`},
		{[]string{"mine", "--unknown"}, "unknown flag --unknown"},
		{[]string{"show", "DEV-1", "--title", "x"}, "unknown flag --title"},
		{[]string{"create", "--queue", "DEV", "--title"}, "flag --title needs a value"},
		{[]string{"create", "--title", "--queue", "DEV"}, "flag --title needs a value"},
		{[]string{"ready", "--claim=maybe"}, "flag --claim must be true or false"},
		{[]string{"list", "stray"}, "tasks list: unexpected argument: stray"},
		{[]string{"list", "--parent", "DEV-1", "--subtree", "DEV-2"}, "tasks list: --parent cannot be combined with --subtree"},
		{[]string{"mine", "--sort", "title"}, `tasks mine: --sort must be tree, updated, created, or priority, got "title"`},
		{[]string{"list", "--view", "open"}, `tasks list: --view must be active, closed, or all, got "open"`},
		{[]string{"list", "--blocked", "yes"}, `tasks list: --blocked must be true or false, got "yes"`},
		{[]string{"list", "--priority", "P0,urgent"}, `tasks list: --priority must be P0, P1, P2, or P3, got "urgent"`},
		{[]string{"list", "--category", "plan"}, `tasks list: --category must be open, in_progress, wait_customer, done, or cancelled, got "plan"`},
		{[]string{"list", "--limit", "0"}, `tasks list: --limit must be a number from 1 to 500, got "0"`},
		{[]string{"list", "--limit", "501"}, `tasks list: --limit must be a number from 1 to 500, got "501"`},
		{[]string{"list", "--limit", "ten"}, `tasks list: --limit must be a number from 1 to 500, got "ten"`},
		{[]string{"ready", "stray"}, "tasks ready: unexpected argument: stray"},
		{[]string{"show"}, "tasks show: task key is required"},
		{[]string{"show", " "}, "tasks show: task key is required"},
		{[]string{"show", "DEV-1", "extra"}, "tasks show: unexpected argument: extra"},
		{[]string{"create", "--queue", "DEV"}, "tasks create: --title is required"},
		{[]string{"create", "--title", "T"}, "tasks create: --queue or --parent is required"},
		{[]string{"create", "--queue", "DEV", "--title", "T", "extra"}, "tasks create: unexpected argument: extra"},
		{[]string{"update"}, "tasks update: task key is required"},
		{[]string{"assign", "DEV-1"}, "tasks assign: assignee is required"},
		{[]string{"assign", "DEV-1", "worker", "extra"}, "tasks assign: unexpected argument: extra"},
		{[]string{"comment"}, "tasks comment: task key is required"},
		{[]string{"comment", "DEV-1"}, "tasks comment: comment text is required"},
		{[]string{"comment", "DEV-1", "stray", "--body", "body"}, "tasks comment: unexpected argument: stray"},
		{[]string{"ask", "DEV-1"}, "tasks ask: principal is required"},
		{[]string{"ask", "DEV-1", "worker"}, "tasks ask: principal and question are required"},
		{[]string{"move", "DEV-1"}, "tasks move: pass --parent, --before, or --to-root to detach"},
		{[]string{"move", "DEV-1", "--to-root", "--parent", "DEV-2"}, "tasks move: --to-root cannot be combined with --parent or --before"},
		{[]string{"move", "DEV-1", "--to-root", "--before", "DEV-2"}, "tasks move: --to-root cannot be combined with --parent or --before"},
		{[]string{"move", "DEV-1", "extra", "--to-root"}, "tasks move: unexpected argument: extra"},
		{[]string{"block", "DEV-1"}, "tasks block: --by is required"},
		{[]string{"relate", "DEV-1"}, "tasks relate: related task key is required"},
		{[]string{"done"}, "tasks done: task key is required"},
		{[]string{"done", "DEV-1", "x"}, "tasks done: unexpected argument: x"},
		{[]string{"advance", "DEV-1"}, "tasks advance: --outcome is required"},
		{[]string{"advance", "DEV-1", "--outcome", " "}, "tasks advance: --outcome is required"},
		{[]string{"advance", "--outcome", "ready"}, "tasks advance: task key is required"},
		{[]string{"artifacts", "set", "DEV-1"}, "tasks artifacts set: artifact name is required"},
		{[]string{"artifacts", "set", "DEV-1", "plan", "v", "--file", "x"}, "tasks artifacts set: pass VALUE or --file, not both"},
		{[]string{"artifacts", "set", "DEV-1", "plan", "v", "extra"}, "tasks artifacts set: unexpected argument: extra"},
		{[]string{"artifacts", "show", "DEV-1"}, "tasks artifacts show: artifact name is required"},
		{[]string{"workflow", "log", "DEV-1"}, "tasks workflow log: run id is required"},
		{[]string{"workflow", "log", "DEV-1", "abc"}, `tasks workflow log: run id must be a positive number, got "abc"`},
		{[]string{"workflow", "log", "DEV-1", "4", "--max-bytes", "0"}, `tasks workflow log: --max-bytes must be a positive number, got "0"`},
		{[]string{"workflow", "move", "DEV-1", "--to", "develop"}, "tasks workflow move: --reason is required"},
		{[]string{"workflow", "move", "DEV-1", "--reason", "x"}, "tasks workflow move: --to is required"},
		{[]string{"workflow", "resume", "DEV-1"}, "tasks workflow resume: --decision must be continue or release"},
		{[]string{"workflow", "resume", "DEV-1", "--decision", "restart"}, "tasks workflow resume: --decision must be continue or release"},
		{[]string{"workflow", "retry", "DEV-1", "x"}, `tasks workflow retry: request id must be a positive number, got "x"`},
		{[]string{"workflow", "retry", "DEV-1", "7", "8"}, "tasks workflow retry: unexpected argument: 8"},
		{[]string{"cancel"}, "tasks cancel: task key is required"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.argv, " "), func(t *testing.T) {
			_, err := parse(tt.argv)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("parse(%q) error = %v; want %q", tt.argv, err, tt.want)
			}
		})
	}
}
