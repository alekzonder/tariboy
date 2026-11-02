package assessmentcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/client"
)

type call struct {
	method, route string
	body          any
}

type recorder struct {
	calls  []call
	result json.RawMessage
	err    error
}

func (r *recorder) Call(method, route string, body any) (json.RawMessage, error) {
	r.calls = append(r.calls, call{method, route, body})
	return r.result, r.err
}

// useRecorder replaces newCaller for one test and records the sockets used.
func useRecorder(t *testing.T, result string) (*recorder, *[]string) {
	t.Helper()
	old := newCaller
	t.Cleanup(func() { newCaller = old })
	rec := &recorder{result: json.RawMessage(result)}
	var sockets []string
	newCaller = func(socket string) Caller {
		sockets = append(sockets, socket)
		return rec
	}
	return rec, &sockets
}

const showResult = `{"assessment":{"id":"as-20261102T080001Z-000001","created_at":"2026-11-02T08:00:01.000000000Z",
"assessor_agent":"judge","assessor_image_ref":"judge:latest","assessor_image_version":"1.0.0","assessor_image_digest":"sha-j",
"rubric_version":"rubric-1","iteration_id":"it-dev","task_key":"DEV-1","subject_agent":"dev","image_ref":"developer:latest",
"image_version":"0.23.1","image_digest":"sha-d","workflow_digest":"","workflow_version":"","summary":"Followed the process.",
"axes":[{"axis":"image_process","verdict":"scored","score":80,"note":"ok"},{"axis":"role","verdict":"not_applicable","note":""},
{"axis":"task_fit","verdict":"scored","score":0,"note":""}],
"violations":[{"axis":"task_fit","description":"refused","quote":"I will not","evidence":{"kind":"task_event","iteration_id":"","locator":"","task_key":"DEV-1","sequence":4,"run_id":""}}]}}`

const lsResult = `{"assessments":[{"id":"as-20261102T080001Z-000001","created_at":"2026-11-02T08:00:01.000000000Z",
"assessor_agent":"judge","assessor_image_ref":"","assessor_image_version":"","assessor_image_digest":"","rubric_version":"r",
"iteration_id":"it-dev","task_key":"","subject_agent":"dev","image_ref":"developer:latest","image_version":"0.23.1",
"image_digest":"","workflow_digest":"","workflow_version":"","summary":"s",
"axes":[{"axis":"image_process","verdict":"scored","score":80,"note":""},{"axis":"role","verdict":"not_applicable","note":""},
{"axis":"task_fit","verdict":"scored","score":0,"note":""}],"violation_count":2}]}`

func run(t *testing.T, tools string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, tools, "/run/daemon.sock", strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestAddWithoutTheToolsSocketRefuses(t *testing.T) {
	rec, _ := useRecorder(t, `{}`)
	code, _, errOut := run(t, "", "{}", "add", "--file", "-")
	if code != 2 || !strings.Contains(errOut, "assessment add: needs the agent tools socket (TARIBOY_TOOLS_SOCKET); operators cannot record assessments") {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("a refused add called the daemon: %v", rec.calls)
	}
}

func TestAddSendsTheFileToTheToolsSocket(t *testing.T) {
	rec, sockets := useRecorder(t, showResult)
	code, out, errOut := run(t, "/run/tools.sock", `{"summary":"s","axes":[]}`, "add", "--file", "-")
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
	want := []call{{"POST", "/tools/assessment/add", map[string]any{"summary": "s", "axes": []any{}}}}
	if !reflect.DeepEqual(rec.calls, want) || !reflect.DeepEqual(*sockets, []string{"/run/tools.sock"}) {
		t.Fatalf("calls %+v sockets %v", rec.calls, *sockets)
	}
	if !strings.Contains(out, "as-20261102T080001Z-000001") {
		t.Fatalf("stdout %q", out)
	}

	path := filepath.Join(t.TempDir(), "a.json")
	if err := os.WriteFile(path, []byte(`{"summary":"from file"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run(t, "/run/tools.sock", "", "add", "--file", path); code != 0 {
		t.Fatalf("file add: code %d stderr %q", code, errOut)
	}
	if body := rec.calls[1].body.(map[string]any); body["summary"] != "from file" {
		t.Fatalf("file body %v", body)
	}
	for _, args := range [][]string{{"add"}, {"add", "--file"}} {
		if code, _, _ := run(t, "/run/tools.sock", "", args...); code != 2 {
			t.Fatalf("%v: code %d; want 2", args, code)
		}
	}
	if code, _, errOut := run(t, "/run/tools.sock", "[1]", "add", "--file", "-"); code != 2 || !strings.Contains(errOut, "JSON object") {
		t.Fatalf("non-object: code %d stderr %q", code, errOut)
	}
}

func TestLsBuildsTheQueryInBothModes(t *testing.T) {
	args := []string{"ls", "--iteration", "it-dev", "--task", "DEV-1", "--agent", "dev", "--assessor", "judge",
		"--image", "developer:latest", "--image-version", "0.23.1", "--limit", "5"}
	query := map[string]string{"iteration": "it-dev", "task": "DEV-1", "agent": "dev", "assessor": "judge",
		"image": "developer:latest", "image_version": "0.23.1", "limit": "5"}
	for _, tc := range []struct{ tools, route, socket string }{
		{"/run/tools.sock", "/tools/assessment/ls", "/run/tools.sock"},
		{"", "/api/assessments", "/run/daemon.sock"},
	} {
		rec, sockets := useRecorder(t, lsResult)
		code, out, errOut := run(t, tc.tools, "", args...)
		if code != 0 {
			t.Fatalf("code %d stderr %q", code, errOut)
		}
		if want := []call{{"GET", tc.route, query}}; !reflect.DeepEqual(rec.calls, want) || (*sockets)[0] != tc.socket {
			t.Fatalf("calls %+v sockets %v", rec.calls, *sockets)
		}
		for _, cell := range []string{"ID", "VIOLATIONS", "as-20261102T080001Z-000001", "developer:latest", "0.23.1", "80", "-"} {
			if !strings.Contains(out, cell) {
				t.Fatalf("table lacks %q:\n%s", cell, out)
			}
		}
	}
}

func TestShowPrintsAxesAndViolations(t *testing.T) {
	for _, tc := range []struct{ tools, route string }{
		{"/run/tools.sock", "/tools/assessment/show/as-1"},
		{"", "/api/assessments/as-1"},
	} {
		rec, _ := useRecorder(t, showResult)
		code, out, errOut := run(t, tc.tools, "", "show", "as-1")
		if code != 0 {
			t.Fatalf("code %d stderr %q", code, errOut)
		}
		if rec.calls[0].method != "GET" || rec.calls[0].route != tc.route {
			t.Fatalf("calls %+v", rec.calls)
		}
		for _, text := range []string{"Followed the process.", "image_process", "80", "not_applicable", "refused",
			"I will not", "task_event DEV-1#4", "judge:latest"} {
			if !strings.Contains(out, text) {
				t.Fatalf("show lacks %q:\n%s", text, out)
			}
		}
	}
	if code, _, _ := run(t, "", "", "show"); code != 2 {
		t.Fatal("show without an id did not fail with 2")
	}
}

func TestJSONPrintsTheResult(t *testing.T) {
	useRecorder(t, lsResult)
	code, out, _ := run(t, "", "", "ls", "--json")
	if code != 0 || strings.TrimSpace(out) != lsResult {
		t.Fatalf("code %d stdout %q", code, out)
	}
}

func TestErrorsAndUsage(t *testing.T) {
	rec, _ := useRecorder(t, "")
	rec.err = &client.APIError{Code: "invalid_assessment", Msg: "summary is required"}
	code, _, errOut := run(t, "/run/tools.sock", "{}", "add", "--file", "-")
	if code != 1 || !strings.Contains(errOut, "error (invalid_assessment): summary is required") {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
	for _, args := range [][]string{{}, {"frobnicate"}, {"ls", "--bogus"}, {"ls", "--limit", "x"}} {
		if code, _, _ := run(t, "", "", args...); code != 2 {
			t.Fatalf("%v: code %d; want 2", args, code)
		}
	}
	code, out, _ := run(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "tariboy assessment add --file PATH") {
		t.Fatalf("help: code %d %q", code, out)
	}
}
