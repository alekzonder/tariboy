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

// TestUpAndBuildRefuseSectionsTheyDoNotApply pins that up and build reject a
// file with stores, workflows or a Store image before any call, instead of
// applying part of it.
func TestUpAndBuildRefuseSectionsTheyDoNotApply(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"stores", "version: 2\nstores:\n  team: {path: ./s}\n", "section stores is not applied by compose up yet"},
		{"workflows", "version: 2\nworkflows:\n  w: {path: ./w}\n", "section workflows is not applied by compose up yet"},
		{"store image", "version: 2\nstores:\n  team: {path: ./s}\nimages:\n  i: {store: team}\n", "section stores is not applied"},
	} {
		for _, verb := range []string{"up", "build"} {
			t.Run(tc.name+"/"+verb, func(t *testing.T) {
				path := writeNamedFile(t, t.TempDir(), "Tariboycompose.yaml", tc.doc)
				fc := newVersionCaller("0.110.0")
				var errOut strings.Builder
				if code := Run(context.Background(), fc, Options{Verb: verb, File: path, NoStart: true}, io.Discard, &errOut); code != 1 || !strings.Contains(errOut.String(), tc.want) {
					t.Fatalf("code=%d stderr=%s; want 1 and %q", code, errOut.String(), tc.want)
				}
				if len(fc.calls) != 0 {
					t.Fatalf("calls = %v; want none", fc.calls)
				}
			})
		}
	}
	if err := (File{Version: 2, Images: map[string]ImageSpec{"i": {SourceSpec: SourceSpec{Store: "team"}}}}).UnappliedSections(); err == nil || !strings.Contains(err.Error(), `image "i" from a Store is not applied`) {
		t.Fatalf("store image error = %v", err)
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
