package workflowrun

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func ip(n int) *int { return &n }

var declared = Declared{Outcomes: []string{"merged", "closed"}, Artifacts: []string{"merge_commit", "notes"}}

func TestClassifyExitTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		kind   tasks.RunKind
		exit   int
		result string
		want   tasks.Verdict
		out    string
	}{
		{"check 0", tasks.RunKindCheck, 0, "", tasks.VerdictPass, ""},
		{"check 0 ignores outcome", tasks.RunKindCheck, 0, `{"outcome":"nope"}`, tasks.VerdictPass, ""},
		{"check 111", tasks.RunKindCheck, script.QuietExit, "", tasks.VerdictFailure, ""},
		{"check 112", tasks.RunKindCheck, script.RejectExit, "", tasks.VerdictReject, ""},
		{"check other", tasks.RunKindCheck, 3, "", tasks.VerdictFailure, ""},
		{"watch 0 outcome", tasks.RunKindWatch, 0, `{"outcome":"merged"}`, tasks.VerdictOutcome, "merged"},
		{"watch 0 no file", tasks.RunKindWatch, 0, "", tasks.VerdictFailure, ""},
		{"watch 0 no outcome", tasks.RunKindWatch, 0, `{"message":"x"}`, tasks.VerdictFailure, ""},
		{"watch 0 undeclared", tasks.RunKindWatch, 0, `{"outcome":"lost"}`, tasks.VerdictFailure, ""},
		{"watch 111", tasks.RunKindWatch, script.QuietExit, "garbage", tasks.VerdictQuiet, ""},
		{"watch 112", tasks.RunKindWatch, script.RejectExit, "", tasks.VerdictFailure, ""},
		{"watch other", tasks.RunKindWatch, 1, "", tasks.VerdictFailure, ""},
		{"unknown kind", "other", 0, "", tasks.VerdictFailure, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var raw []byte
			if c.result != "" {
				raw = []byte(c.result)
			}
			v := Classify(c.kind, ip(c.exit), false, raw, nil, declared)
			if v.Kind != c.want || v.Outcome != c.out {
				t.Fatalf("got %+v, want kind %s outcome %q", v, c.want, c.out)
			}
			if v.Kind == tasks.VerdictFailure && v.Message == "" {
				t.Fatal("failure without a reason")
			}
		})
	}
}

func TestClassifyCheckPassKeepsMessageAndArtifacts(t *testing.T) {
	t.Parallel()
	v := Classify(tasks.RunKindCheck, ip(0), false, []byte(`{"message":"ok","artifacts":{"notes":"n"}}`), nil, declared)
	if v.Kind != tasks.VerdictPass || v.Message != "ok" || v.Artifacts["notes"] != "n" {
		t.Fatalf("got %+v", v)
	}
}

func TestClassifyRejectMessage(t *testing.T) {
	t.Parallel()
	v := Classify(tasks.RunKindCheck, ip(112), false, []byte(`{"message":"tests fail","artifacts":{"notes":"n"}}`), nil, declared)
	if v.Kind != tasks.VerdictReject || v.Message != "tests fail" || v.Artifacts != nil {
		t.Fatalf("got %+v", v)
	}
	v = Classify(tasks.RunKindCheck, ip(112), false, nil, nil, declared)
	if v.Kind != tasks.VerdictReject || v.Message != "the check's condition does not hold" {
		t.Fatalf("got %+v", v)
	}
	v = Classify(tasks.RunKindCheck, ip(112), false, []byte("  \n"), nil, declared)
	if v.Kind != tasks.VerdictReject || v.Message == "" {
		t.Fatalf("got %+v", v)
	}
}

func TestClassifyWatchOutcomeArtifacts(t *testing.T) {
	t.Parallel()
	v := Classify(tasks.RunKindWatch, ip(0), false, []byte(`{"outcome":"merged","message":"m","artifacts":{"merge_commit":"9f"}}`), nil, declared)
	if v.Kind != tasks.VerdictOutcome || v.Message != "m" || v.Artifacts["merge_commit"] != "9f" {
		t.Fatalf("got %+v", v)
	}
}

func TestClassifyFailureReasons(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("a", MaxArtifactBytes+1)
	cases := []struct {
		name      string
		exit      *int
		timedOut  bool
		raw       string
		resultErr error
		contains  string
	}{
		{"no exit", nil, false, "", nil, "did not exit"},
		{"timeout", ip(0), true, `{}`, nil, "timed out"},
		{"timeout killed", nil, true, "", nil, "timed out"},
		{"result error", ip(0), false, "", errors.New("boom"), "boom"},
		{"malformed", ip(0), false, `{`, nil, "result file"},
		{"not object", ip(0), false, `[1]`, nil, "result file"},
		{"null", ip(0), false, `null`, nil, "result file"},
		{"trailing", ip(0), false, `{} {}`, nil, "result file"},
		{"unknown key", ip(0), false, `{"outcomes":"x"}`, nil, "outcomes"},
		{"oversized file", ip(0), false, strings.Repeat(" ", MaxResultBytes) + `{}`, nil, "too large"},
		{"oversized message", ip(0), false, `{"message":"` + strings.Repeat("m", MaxMessageBytes+1) + `"}`, nil, "message"},
		{"undeclared artifact", ip(0), false, `{"artifacts":{"zzz":"v"}}`, nil, "zzz"},
		{"oversized artifact", ip(0), false, `{"artifacts":{"notes":"` + big + `"}}`, nil, "too large"}, // the file bound and the artifact bound are both 64 KiB
		{"empty artifact", ip(0), false, `{"artifacts":{"notes":""}}`, nil, "notes"},
		{"bad utf8 artifact", ip(0), false, "{\"artifacts\":{\"notes\":\"\xff\"}}", nil, "UTF-8"},
		{"exit code", ip(7), false, `{"message":"why"}`, nil, "exit 7"},
		{"exit code message", ip(7), false, `{"message":"why"}`, nil, "why"},
		{"exit code bad file", ip(7), false, `{`, nil, "exit 7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var raw []byte
			if c.raw != "" {
				raw = []byte(c.raw)
			}
			v := Classify(tasks.RunKindCheck, c.exit, c.timedOut, raw, c.resultErr, declared)
			if v.Kind != tasks.VerdictFailure || !strings.Contains(v.Message, c.contains) {
				t.Fatalf("got %+v, want failure containing %q", v, c.contains)
			}
		})
	}
}

func TestClassifyMatchesResultKeysExactly(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"outcome in another case":     `{"Outcome":"merged"}`,
		"message in another case":     `{"outcome":"merged","MESSAGE":"m"}`,
		"artifacts in another case":   `{"outcome":"merged","Artifacts":{"notes":"n"}}`,
		"duplicate outcome":           `{"outcome":"closed","outcome":"merged"}`,
		"duplicate message":           `{"outcome":"merged","message":"a","message":"b"}`,
		"duplicate artifacts":         `{"outcome":"merged","artifacts":{},"artifacts":{"notes":"n"}}`,
		"duplicate artifact name":     `{"outcome":"merged","artifacts":{"notes":"a","notes":"b"}}`,
		"artifact value not a string": `{"outcome":"merged","artifacts":{"notes":1}}`,
		"artifacts not an object":     `{"outcome":"merged","artifacts":["notes"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if v := Classify(tasks.RunKindWatch, ip(0), false, []byte(raw), nil, declared); v.Kind != tasks.VerdictFailure {
				t.Fatalf("got %+v, want a failure", v)
			}
		})
	}
	v := Classify(tasks.RunKindWatch, ip(0), false, []byte(`{"artifacts":null,"message":"m","outcome":"merged"}`), nil, declared)
	if v.Kind != tasks.VerdictOutcome || v.Outcome != "merged" || v.Message != "m" || v.Artifacts != nil {
		t.Fatalf("got %+v", v)
	}
}

func TestClassifyBoundsEveryMessageOnARuneBoundary(t *testing.T) {
	t.Parallel()
	long := "a" + strings.Repeat("ж", MaxMessageBytes)
	v := Classify(tasks.RunKindCheck, ip(7), false, []byte(`{"message":"`+long+`"}`), nil, declared)
	if v.Kind != tasks.VerdictFailure || len(v.Message) > MaxMessageBytes || !utf8.ValidString(v.Message) || !strings.Contains(v.Message, "exit 7") {
		t.Fatalf("exit failure message: %d bytes, valid %v", len(v.Message), utf8.ValidString(v.Message))
	}
	// A decoder error quotes the input; only 256 bytes of its text are kept.
	key := strings.Repeat("k", 1000)
	for _, raw := range []string{`{"` + key + `":1}`, `{"message":` + strings.Repeat("9", 1000) + `}`} {
		v = Classify(tasks.RunKindCheck, ip(0), false, []byte(raw), nil, declared)
		if v.Kind != tasks.VerdictFailure || len(v.Message) > len("the result file is not valid: ")+256 {
			t.Fatalf("decoder failure = %d bytes: %q", len(v.Message), v.Message)
		}
	}
	// An undeclared artifact name comes from the script and is cut too.
	v = Classify(tasks.RunKindCheck, ip(0), false, []byte(`{"artifacts":{"`+key+`":"v"}}`), nil, declared)
	if v.Kind != tasks.VerdictFailure || strings.Contains(v.Message, strings.Repeat("k", 65)) {
		t.Fatalf("undeclared artifact failure = %q", v.Message)
	}
}

func TestProtocolEnv(t *testing.T) {
	t.Parallel()
	env := ProtocolEnv(EnvValues{TaskKey: "A-1", Queue: "q", WorkflowName: "w", WorkflowVersion: "1", Status: "s",
		WorkflowDir: "/w", TaskFile: "/t.json", TaskDir: "/d", ResultFile: "/r.json"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "TARIBOY_WORKFLOW_OUTCOME") {
		t.Fatal("outcome present when empty")
	}
	for _, want := range []string{"TARIBOY_QUIET_EXIT=111", "TARIBOY_REJECT_EXIT=112", "TARIBOY_TASK_KEY=A-1", "TARIBOY_RESULT_FILE=/r.json"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %v", want, env)
		}
	}
	for i := 1; i < len(env); i++ {
		if env[i-1] > env[i] {
			t.Fatalf("not sorted: %v", env)
		}
	}
	env = ProtocolEnv(EnvValues{Outcome: "merged"})
	if !strings.Contains(strings.Join(env, "\n"), "TARIBOY_WORKFLOW_OUTCOME=merged") {
		t.Fatalf("outcome missing: %v", env)
	}
	for _, e := range env {
		if strings.HasPrefix(e, "=") {
			t.Fatalf("empty name: %q", e)
		}
	}
}
