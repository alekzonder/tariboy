package loop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProbeScriptStateLockedIsAlive(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "run.state")
	held, err := createScriptState(path)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	alive, state, err := probeScriptState(path)
	if err != nil || !alive || state != nil {
		t.Fatalf("alive=%v state=%v err=%v, want alive", alive, state, err)
	}
	if _, err := createScriptState(path); err == nil {
		t.Fatal("a second state file was created over a live one")
	}
}

func TestProbeScriptStateFinishedReturnsResult(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "run.state")
	held, err := createScriptState(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := held.WriteString(`{"exit_code":7}`); err != nil {
		t.Fatal(err)
	}
	held.Close()
	alive, state, err := probeScriptState(path)
	if err != nil || alive || state == nil || state.ExitCode == nil || *state.ExitCode != 7 {
		t.Fatalf("alive=%v state=%+v err=%v, want finished with 7", alive, state, err)
	}
}

func TestProbeScriptStateMissingOrEmptyIsLost(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.state")
	held, err := createScriptState(empty)
	if err != nil {
		t.Fatal(err)
	}
	held.Close()
	garbage := filepath.Join(dir, "garbage.state")
	if err := os.WriteFile(garbage, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, "missing.state"), empty, garbage} {
		alive, state, err := probeScriptState(path)
		if err != nil || alive || state != nil {
			t.Fatalf("%s: alive=%v state=%+v err=%v, want lost", filepath.Base(path), alive, state, err)
		}
	}
}

func TestProbeScriptStateRejectsSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte(`{"exit_code":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "run.state")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if alive, state, err := probeScriptState(link); err == nil || alive || state != nil {
		t.Fatalf("alive=%v state=%+v err=%v, want an error for a symlink", alive, state, err)
	}
	if _, err := createScriptState(link); err == nil {
		t.Fatal("state file was created through a symlink")
	}
}
