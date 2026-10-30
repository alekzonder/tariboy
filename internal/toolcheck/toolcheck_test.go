package toolcheck

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakePath(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func missingNames(r Result) []string {
	var names []string
	for _, tool := range r.MissingTools {
		names = append(names, tool.Name)
	}
	return names
}

func TestCheckReadyWhenEveryToolIsOnPath(t *testing.T) {
	fakePath(t, "tmux", "bash", "python3", "git", "npx", "tariboy", "ttasks", "tmail", "codex")
	r := Check()
	if r.State != "ready" || len(r.MissingTools) != 0 || r.Message != "" {
		t.Fatalf("Check() = %+v, want ready", r)
	}
}

func TestCheckReportsEachMissingTool(t *testing.T) {
	fakePath(t, "bash", "python3", "git", "tariboy", "ttasks", "claude")
	r := Check()
	if r.State != "error" {
		t.Fatalf("state = %q, want error", r.State)
	}
	if got, want := missingNames(r), []string{"tmux", "npx", "tmail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	for _, tool := range r.MissingTools {
		if tool.Purpose == "" || tool.Install == "" {
			t.Fatalf("tool %+v lacks purpose or install hint", tool)
		}
	}
	if !strings.Contains(r.Message, "tmux, npx") {
		t.Fatalf("message = %q, want both names", r.Message)
	}
}

func TestCheckRequiresAtLeastOneHarness(t *testing.T) {
	fakePath(t, "tmux", "bash", "python3", "git", "npx", "tariboy", "ttasks", "tmail")
	r := Check()
	if r.State != "error" || len(r.MissingTools) != 1 || !strings.Contains(r.MissingTools[0].Name, "claude") {
		t.Fatalf("Check() = %+v, want one missing harness entry", r)
	}
}

func TestHarnessesReportsEachHarnessWithoutStub(t *testing.T) {
	fakePath(t, "claude", "agent")
	got := Harnesses()
	var types []string
	found := map[string]bool{}
	for _, h := range got {
		types = append(types, h.Type)
		found[h.Type] = h.Found
		if h.Install == "" {
			t.Fatalf("harness %+v lacks an install hint", h)
		}
	}
	if want := []string{"claude", "codex", "opencode", "cursor"}; !reflect.DeepEqual(types, want) {
		t.Fatalf("types = %v, want %v", types, want)
	}
	if !found["claude"] || !found["cursor"] || found["codex"] || found["opencode"] {
		t.Fatalf("found = %v, want claude and cursor only", found)
	}
}

func TestMissingNamesARequiredToolOnlyWhenAbsent(t *testing.T) {
	fakePath(t, "git")
	if _, missing := Missing("git"); missing {
		t.Fatal("git is on PATH but reported missing")
	}
	tool, missing := Missing("npx")
	if !missing || tool.Install != "install Node.js (npm ships npx)" {
		t.Fatalf("Missing(npx) = %+v, %v", tool, missing)
	}
	if _, missing := Missing("unknown-tool"); missing {
		t.Fatal("a tool outside the list is never reported")
	}
}
