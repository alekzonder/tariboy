package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

func runValidate(t *testing.T, call Caller, o Options) (int, string, string) {
	t.Helper()
	o.Verb = "validate"
	var out, errOut strings.Builder
	code := Run(context.Background(), call, o, &out, &errOut)
	return code, out.String(), errOut.String()
}

func mkdirs(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateAcceptsAValidFileWithoutCallingTheDaemon(t *testing.T) {
	dir := t.TempDir()
	mkdirs(t, dir, "store", "workflows/review", "images/reviewer")
	path := writeNamedFile(t, dir, "Tariboycompose.yaml", strings.Replace(goodV2YAML, "max: 0.120.0", "max: 99.0.0", 1))
	fc := newVersionCaller("0.110.0")
	code, out, errOut := runValidate(t, fc, Options{File: path})
	if code != 0 || out != path+": valid\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if len(fc.calls) != 0 {
		t.Fatalf("validate without --daemon called the daemon: %v", fc.calls)
	}
}

// TestValidateListsEveryProblem pins acceptance criterion 5: every reference
// category, a missing path and the CLI version are reported in one run, and
// nothing calls the daemon without --daemon.
func TestValidateListsEveryProblem(t *testing.T) {
	dir := t.TempDir()
	path := writeNamedFile(t, dir, "Tariboycompose.yaml", `version: 2
tariboy: {min: 99.0.0}
stores:
  local: {path: ./no-store}
workflows:
  w: {store: ghost-store}
  local-w: {path: ./no-workflow}
images:
  i: {store: ghost-store}
  local-i: {path: ./no-image}
groups:
  g: {}
agents:
  a: {image: ghost-image:latest}
  b: {image: i:latest, group: ghost-group}
task_queues:
  DEV:
    name: Development
    workflow: ghost-workflow
    pools: {developers: [ghost-agent]}
`)
	fc := newVersionCaller("0.110.0")
	code, out, errOut := runValidate(t, fc, Options{File: path})
	if code != 1 || out != "" {
		t.Fatalf("code=%d stdout=%q; want 1 and no stdout", code, out)
	}
	for _, want := range []string{
		`- workflow "w" references undeclared store "ghost-store"`,
		`- image "i" references undeclared store "ghost-store"`,
		`- agent "a" references undeclared image "ghost-image"`,
		`- agent "b" references undeclared group "ghost-group"`,
		`- task queue "DEV" pool "developers" references undeclared agent "ghost-agent"`,
		`- task queue "DEV" references undeclared workflow "ghost-workflow"`,
		`- store "local" path "./no-store" does not exist`,
		`- workflow "local-w" path "./no-workflow" does not exist`,
		`- image "local-i" path "./no-image" does not exist`,
		`- CLI version ` + version.Version + ` does not satisfy tariboy >= 99.0.0 in ` + path,
		`compose validate: 10 problem(s) in ` + path,
	} {
		if !strings.Contains(errOut, want+"\n") {
			t.Errorf("stderr lacks %q:\n%s", want, errOut)
		}
	}
	if len(fc.calls) != 0 {
		t.Fatalf("validate without --daemon called the daemon: %v", fc.calls)
	}
}

func TestValidateListsEveryUnknownField(t *testing.T) {
	path := writeNamedFile(t, t.TempDir(), "Tariboycompose.yaml", "version: 2\nstores:\n  s: {git: x, branch: main}\nimages:\n  i: {path: ./i, digest: x}\n")
	code, _, errOut := runValidate(t, newFake(), Options{File: path})
	if code != 1 || !strings.Contains(errOut, "field branch not found") || !strings.Contains(errOut, "field digest not found") {
		t.Fatalf("code=%d stderr=%s; want both unknown fields", code, errOut)
	}
	if !strings.Contains(errOut, "compose validate: 2 problem(s)") {
		t.Fatalf("stderr = %s; want a count of 2", errOut)
	}
}

func TestValidateDaemonReadsOnlyTheDaemonStatus(t *testing.T) {
	path := writeNamedFile(t, t.TempDir(), "Tariboycompose.yaml", "version: 2\ntariboy: {min: 0.1.0, max: 0.109.0}\n")
	fc := newVersionCaller("0.110.0")
	code, _, errOut := runValidate(t, fc, Options{File: path, Daemon: true})
	// The CLI version (the checked-in Version) may sit outside the range too;
	// the daemon line is what this test pins.
	if code != 1 || !strings.Contains(errOut, "- daemon version 0.110.0 does not satisfy tariboy >= 0.1.0, <= 0.109.0 in "+path) {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if len(fc.calls) != 1 || fc.calls[0] != "GET /api/daemon/status" {
		t.Fatalf("calls = %v; want only GET /api/daemon/status", fc.calls)
	}
}

func TestValidateDaemonPassesInsideTheRange(t *testing.T) {
	path := writeNamedFile(t, t.TempDir(), "Tariboycompose.yaml", "version: 2\ntariboy: {min: 0.1.0}\n")
	code, _, errOut := runValidate(t, newVersionCaller("0.110.0"), Options{File: path, Daemon: true})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
}
