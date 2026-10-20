package testutil

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestIsolateEnvDropsTariboyVariablesAndMovesHome(t *testing.T) {
	t.Setenv("TARIBOY_RUNTIME_DIR", "/live/runtime")
	t.Setenv("TARIBOY_BASE_DIR", "/live/base")
	t.Setenv("HOME", "/live/home")

	cleanup, err := IsolateEnv()
	if err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("HOME")
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "TARIBOY_") {
			t.Errorf("%s survived isolation", kv)
		}
	}
	if home == "/live/home" {
		t.Fatal("HOME still points at the live home")
	}
	if fi, err := os.Stat(home); err != nil || !fi.IsDir() {
		t.Fatalf("isolated HOME %q is not a directory: %v", home, err)
	}
	cleanup()
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("cleanup left %q: %v", home, err)
	}
}

func TestShortDirFitsASocketPath(t *testing.T) {
	dir := ShortDir(t)
	if len(dir) > 40 {
		t.Fatalf("ShortDir = %q (%d bytes), want a short path", dir, len(dir))
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("ShortDir %q is not a directory: %v", dir, err)
	}
}

func TestWaitForReportsWhetherTheConditionHeld(t *testing.T) {
	calls := 0
	if !WaitFor(time.Second, func() bool { calls++; return calls == 3 }) {
		t.Fatal("WaitFor = false for a condition that became true")
	}
	if calls != 3 {
		t.Fatalf("condition checked %d times, want 3", calls)
	}
	start := time.Now()
	if WaitFor(30*time.Millisecond, func() bool { return false }) {
		t.Fatal("WaitFor = true for a condition that never held")
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("WaitFor gave up after %v, before its timeout", elapsed)
	}
}
