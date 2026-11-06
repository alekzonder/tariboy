package compose

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// versionCaller is the fake daemon with a GET /api/daemon/status answer.
type versionCaller struct {
	*fakeCaller
	version string
}

func (c *versionCaller) Call(method, route string, body any) (json.RawMessage, error) {
	if method == "GET" && route == "/api/daemon/status" {
		c.calls = append(c.calls, method+" "+route)
		c.bodies = append(c.bodies, body)
		return mustJSON(map[string]any{"state": "running", "version": c.version}), nil
	}
	return c.fakeCaller.Call(method, route, body)
}

func newVersionCaller(v string) *versionCaller {
	return &versionCaller{fakeCaller: newFake(), version: v}
}

const constrainedYAML = `version: 2
tariboy: {min: 99.0.0}
images:
  analyst: {path: ./analyst}
agents:
  scout: {image: analyst:latest}
`

// TestDaemonVerbsRefuseAnUnsatisfiedVersionBeforeAnyChange pins acceptance
// criterion 4: with a daemon outside the file's tariboy range, a verb fails
// after reading the daemon status and sends no other call.
func TestDaemonVerbsRefuseAnUnsatisfiedVersionBeforeAnyChange(t *testing.T) {
	for _, verb := range []string{"up", "down", "build", "ps", "status", "start", "stop", "restart", "kill", "rm", "exec", "logs"} {
		t.Run(verb, func(t *testing.T) {
			path := writeNamedFile(t, t.TempDir(), "Tariboycompose.yaml", constrainedYAML)
			fc := newVersionCaller("0.110.0")
			var errOut strings.Builder
			o := Options{Verb: verb, File: path, NoStart: true}
			if verb == "exec" {
				o.Args = []string{"scout"}
			}
			if code := Run(context.Background(), fc, o, io.Discard, &errOut); code != 1 {
				t.Fatalf("code=%d stderr=%s; want 1", code, errOut.String())
			}
			want := "daemon version 0.110.0 does not satisfy tariboy >= 99.0.0 in " + path
			if !strings.Contains(errOut.String(), want) {
				t.Fatalf("stderr = %q; want it to contain %q", errOut.String(), want)
			}
			if len(fc.calls) != 1 || fc.calls[0] != "GET /api/daemon/status" {
				t.Fatalf("calls = %v; want only GET /api/daemon/status", fc.calls)
			}
		})
	}
}

func TestSatisfiedVersionLetsTheVerbRun(t *testing.T) {
	path := writeNamedFile(t, t.TempDir(), "Tariboycompose.yaml", strings.Replace(constrainedYAML, "99.0.0", "0.110.0", 1))
	fc := newVersionCaller("0.110.0")
	var errOut strings.Builder
	if code := Run(context.Background(), fc, Options{Verb: "ps", File: path}, io.Discard, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if countCalls(fc.fakeCaller, "GET /api/daemon/status") != 1 || countCalls(fc.fakeCaller, "GET /api/agents") == 0 {
		t.Fatalf("calls = %v; want the status read, then the verb", fc.calls)
	}
}

func TestFileWithoutTariboyKeySkipsTheVersionRead(t *testing.T) {
	path := writeNamedFile(t, t.TempDir(), "Tariboycompose.yaml", goodYAML)
	fc := newVersionCaller("0.110.0")
	if code := Run(context.Background(), fc, Options{Verb: "ps", File: path}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if countCalls(fc.fakeCaller, "GET /api/daemon/status") != 0 {
		t.Fatalf("calls = %v; want no status read", fc.calls)
	}
}

// TestTeamImportRefusesSourceSections pins that a team import still rejects
// stores, workflows and a Store image, and tells the user to apply the file
// with compose up.
func TestTeamImportRefusesSourceSections(t *testing.T) {
	for _, tc := range []struct {
		f    File
		want string
	}{
		{File{Version: 2, Stores: map[string]StoreSpec{"team": {Path: "./s"}}}, "section stores is not applied by team import; apply the file with compose up"},
		{File{Version: 2, Workflows: map[string]WorkflowSpec{"w": {SourceSpec{Path: "./w"}}}}, "section workflows is not applied by team import; apply the file with compose up"},
		{File{Version: 2, Images: map[string]ImageSpec{"i": {SourceSpec: SourceSpec{Store: "team"}}}}, `image "i" from a Store is not applied by team import; apply the file with compose up`},
	} {
		if err := tc.f.UnappliedSections(); err == nil || err.Error() != tc.want {
			t.Errorf("UnappliedSections = %v; want %q", err, tc.want)
		}
	}
}

// TestUpAndBuildApplySourceSections pins that up and build no longer refuse
// a file with stores and workflows.
func TestUpAndBuildApplySourceSections(t *testing.T) {
	for _, verb := range []string{"up", "build"} {
		dir := t.TempDir()
		path := writeNamedFile(t, dir, "Tariboycompose.yaml", "version: 2\nstores:\n  team: {path: ./s}\n")
		fc := newSourcesCaller()
		fc.remote[filepath.Join(dir, "s")] = &sourceState{}
		var errOut strings.Builder
		if code := Run(context.Background(), fc, Options{Verb: verb, File: path, NoStart: true}, io.Discard, &errOut); code != 0 {
			t.Fatalf("%s: code=%d stderr=%s", verb, code, errOut.String())
		}
		if fc.registered["team"] == nil {
			t.Fatalf("%s did not create the Store: %v", verb, fc.calls)
		}
	}
}

func TestCheckDaemonVersionNamesTheAbsoluteFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeNamedFile(t, dir, "Tariboycompose.yaml", constrainedYAML)
	var errOut strings.Builder
	_ = Run(context.Background(), newVersionCaller("0.110.0"), Options{Verb: "ps"}, io.Discard, &errOut)
	if want := filepath.Join(dir, "Tariboycompose.yaml"); !strings.Contains(errOut.String(), want) {
		t.Fatalf("stderr = %q; want the absolute path %s", errOut.String(), want)
	}
}
