package workflowrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/tasks"
	"github.com/alekzonder/tariboy/internal/testutil"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// requireFailed checks a run that ended before its script produced anything,
// and returns the failure message.
func requireFailed(t *testing.T, r Result) string {
	t.Helper()
	if r.Verdict.Kind != tasks.VerdictFailure || r.ExitCode != nil || r.TimedOut {
		t.Fatalf("result = %+v", r)
	}
	if r.Started.IsZero() || r.Finished.Before(r.Started) {
		t.Fatalf("times = %v, %v", r.Started, r.Finished)
	}
	return r.Verdict.Message
}

// requireLogEndsWith checks that the log holds the header and then only the
// failure reason, so nothing of the run came between.
func requireLogEndsWith(t *testing.T, r Result, spec Spec, message string) {
	t.Helper()
	want := fmt.Sprintf("script: %s\ncwd: %s\nerror: %s\n", spec.ScriptPath, spec.Cwd, message)
	if got := readLog(t, r); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestExecuteFailsWithoutALogBeforeTheLogExists(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "exit 0\n")
	blocker := filepath.Join(f.dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Spec){
		"run dir":  func(s *Spec) { s.RunDir = filepath.Join(blocker, "run") },
		"task dir": func(s *Spec) { s.TaskDir = filepath.Join(blocker, "task") },
	} {
		t.Run(name, func(t *testing.T) {
			spec := f.spec(tasks.RunKindCheck)
			mutate(&spec)
			r := Execute(context.Background(), spec)
			message := requireFailed(t, r)
			if !strings.HasPrefix(message, "cannot create directory "+blocker) {
				t.Fatalf("message = %q", message)
			}
			if r.LogPath != "" || !r.Started.Equal(r.Finished) {
				t.Fatalf("result = %+v", r)
			}
		})
	}
}

func TestExecutePreparationFailuresAreLogged(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		prepare func(t *testing.T, f fixture, s *Spec)
		want    func(f fixture) string
	}{
		"snapshot": {
			prepare: func(t *testing.T, f fixture, _ *Spec) { mustMkdir(t, filepath.Join(f.runDir, "task.json")) },
			want: func(f fixture) string {
				taskFile := filepath.Join(f.runDir, "task.json")
				return "cannot write " + taskFile + ": open " + taskFile + ": is a directory"
			},
		},
		"stale result": {
			prepare: func(t *testing.T, f fixture, _ *Spec) { mustMkdir(t, filepath.Join(f.resultFile, "child")) },
			want: func(f fixture) string {
				return "cannot remove the stale result file " + f.resultFile + ": remove " + f.resultFile + ": directory not empty"
			},
		},
		"cwd is a file": {
			prepare: func(_ *testing.T, f fixture, s *Spec) { s.Cwd = f.script },
			want:    func(f fixture) string { return "the working directory " + f.script + " is not a directory" },
		},
		"relative script": {
			prepare: func(_ *testing.T, _ fixture, s *Spec) { s.ScriptPath = "check.sh" },
			want:    func(fixture) string { return "the script path check.sh is not absolute" },
		},
		"script is a directory": {
			prepare: func(_ *testing.T, f fixture, s *Spec) { s.ScriptPath = f.dir },
			want:    func(f fixture) string { return "the script " + f.dir + " is not a regular file" },
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, "exit 0\n")
			spec := f.spec(tasks.RunKindCheck)
			tc.prepare(t, f, &spec)
			called := false
			spec.OnStart = func(int) { called = true }
			r := Execute(context.Background(), spec)
			message := requireFailed(t, r)
			if want := tc.want(f); message != want {
				t.Fatalf("message = %q, want %q", message, want)
			}
			if called || !r.Started.Equal(r.Finished) {
				t.Fatalf("called = %v, result = %+v", called, r)
			}
			requireLogEndsWith(t, r, spec, message)
		})
	}
}

// The files of the run are prepared before the working directory is checked,
// and every check comes before the look at the context.
func TestExecutePreparationOrder(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "exit 0\n")
	mustMkdir(t, f.runDir)
	if err := os.WriteFile(f.resultFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	spec := f.spec(tasks.RunKindCheck)
	spec.Cwd = filepath.Join(f.dir, "nowhere")
	r := Execute(ctx, spec)
	message := requireFailed(t, r)
	if !strings.HasPrefix(message, "the working directory ") {
		t.Fatalf("message = %q", message)
	}
	if _, err := os.Stat(f.resultFile); !os.IsNotExist(err) {
		t.Fatalf("stale result not removed: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(f.runDir, "task.json")); err != nil || string(b) != string(spec.Snapshot) {
		t.Fatalf("snapshot = %q, %v", b, err)
	}
	requireLogEndsWith(t, r, spec, message)
}

// A script that cannot start is the one failure after Started was set for the
// run itself, and it still has an open log to report to.
func TestExecuteStartFailureIsLogged(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "exit 0\n")
	if err := testutil.WriteExecutable(f.script, []byte("#!/nonexistent/interpreter\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := f.spec(tasks.RunKindCheck)
	called := false
	spec.OnStart = func(int) { called = true }
	r := Execute(context.Background(), spec)
	message := requireFailed(t, r)
	if !strings.HasPrefix(message, "cannot start the script "+f.script+": ") || called {
		t.Fatalf("message = %q, called = %v", message, called)
	}
	requireLogEndsWith(t, r, spec, message)
}

func TestExecuteZeroTimeoutUsesTheDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "sleep 0.2\nexit 0\n")
	spec := f.spec(tasks.RunKindCheck)
	spec.Timeout = 0
	r := Execute(context.Background(), spec)
	if r.Verdict.Kind != tasks.VerdictPass || r.TimedOut {
		t.Fatalf("result = %+v", r)
	}
}

func TestExecuteNilEnvIsEmptyNotInherited(t *testing.T) {
	// Sequential: t.Setenv.
	t.Setenv("WORKFLOWRUN_LEAK_SENTINEL", "1")
	out := filepath.Join(t.TempDir(), "env.txt")
	f := newFixture(t, "/usr/bin/env > '"+out+"'\n")
	spec := f.spec(tasks.RunKindCheck)
	spec.Env = nil
	r := Execute(context.Background(), spec)
	if r.Verdict.Kind != tasks.VerdictPass {
		t.Fatalf("verdict = %+v", r.Verdict)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "WORKFLOWRUN_LEAK_SENTINEL") {
		t.Fatalf("env inherited: %q", b)
	}
}
