package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type execCall struct {
	path string
	argv []string
	env  []string
}

// fakeClaude writes a claude stand-in that only answers --version.
func fakeClaude(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho '2.1.325 (Claude Code)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func runWithExec(t *testing.T, home, cwd string, args ...string) (int, string, *execCall) {
	t.Helper()
	claude := fakeClaude(t)
	var call execCall
	var errb bytes.Buffer
	env := Env{
		Args:    args,
		Getenv:  func(k string) string { return map[string]string{"TBOX_HOME": home}[k] },
		Environ: func() []string { return []string{"PATH=/bin", "TBOX_IMAGE=stale", "KEEP=1"} },
		Stdout:  &bytes.Buffer{},
		Stderr:  &errb,
		Cwd:     cwd,
		Now:     func() time.Time { return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC) },
		LookPath: func(n string) (string, error) {
			if n == "claude" {
				return claude, nil
			}
			return "", errors.New("not installed")
		},
		Exec: func(path string, argv, e []string) error {
			call = execCall{path, argv, e}
			return nil
		},
	}
	code := Main(env)
	if call.path == "" {
		return code, errb.String(), nil
	}
	return code, errb.String(), &call
}

func TestRunExecsHarness(t *testing.T) {
	home, src := t.TempDir(), demoSource(t)
	if code, _, e := runIn(t, home, "/", "build", "--path", src); code != 0 {
		t.Fatal(e)
	}
	_, errs, call := runWithExec(t, home, "/", "run", "demo", "claude", "--", "--model", "x")
	if call == nil {
		t.Fatalf("no exec: %s", errs)
	}
	if n := len(call.argv); n < 2 || !slices.Equal(call.argv[n-2:], []string{"--model", "x"}) {
		t.Fatalf("argv %v", call.argv)
	}
	var dir, img, keep int
	for _, kv := range call.env {
		switch {
		case strings.HasPrefix(kv, "TBOX_IMAGE_DIR="):
			dir++
		case kv == "TBOX_IMAGE=demo:latest":
			img++
		case kv == "KEEP=1":
			keep++
		case strings.HasPrefix(kv, "TBOX_IMAGE=") && kv != "TBOX_IMAGE=demo:latest":
			t.Fatalf("stale entry kept: %s", kv)
		}
	}
	if dir != 1 || img != 1 || keep != 1 {
		t.Fatalf("env %v", call.env)
	}
}

func TestRunErrors(t *testing.T) {
	home := t.TempDir()
	code, errs, _ := runWithExec(t, home, "/", "run", "demo", "vim")
	if code != 2 || !strings.Contains(errs, "claude, codex, opencode") {
		t.Fatalf("unknown harness %d %q", code, errs)
	}
	code, errs, _ = runWithExec(t, home, "/", "run", "demo")
	if code != 2 || !strings.Contains(errs, "IMAGE and HARNESS are required") {
		t.Fatalf("missing args %d %q", code, errs)
	}
	code, errs, _ = runWithExec(t, home, "/", "run", "store/demo", "claude")
	if code != 1 || !strings.Contains(errs, "store not found: store") {
		t.Fatalf("selector %d %q", code, errs)
	}
	src := demoSource(t)
	if c, _, e := runIn(t, home, "/", "build", "--path", src); c != 0 {
		t.Fatal(e)
	}
	code, errs, _ = runWithExec(t, home, "/", "run", "other", "claude")
	if code != 1 || !strings.Contains(errs, "demo:latest") {
		t.Fatalf("missing image %d %q", code, errs)
	}
}

func TestRunPathBuildsOnce(t *testing.T) {
	home, src := t.TempDir(), demoSource(t)
	_, errs, call := runWithExec(t, home, "/", "run", "--path", src, "claude")
	if call == nil || !strings.Contains(errs, "built demo:latest ") {
		t.Fatalf("first run: %q", errs)
	}
	_, errs, call = runWithExec(t, home, "/", "run", "--path", src, "claude")
	if call == nil || !strings.Contains(errs, "up to date demo ") {
		t.Fatalf("second run: %q", errs)
	}
}

func TestRunPathPrintsWarnings(t *testing.T) {
	home, src := t.TempDir(), pluginSource(t)
	_, errs, call := runWithExec(t, home, "/", "run", "--path", src, "claude")
	if call == nil || !strings.HasPrefix(errs, pluginWarning) {
		t.Fatalf("run: %q", errs)
	}
}

func TestRunFlagAfterPositionals(t *testing.T) {
	home, src := t.TempDir(), demoSource(t)
	code, errs, call := runWithExec(t, home, "/", "run", "claude", "--path", src, "--", "--model", "x")
	if code != 0 || call == nil || call.argv[len(call.argv)-1] != "x" {
		t.Fatalf("%d %q %v", code, errs, call)
	}
}

func TestDashDashEndsFlags(t *testing.T) {
	home := t.TempDir()
	if code, _, errs := runIn(t, home, "/", "build", "--", "s/img", "--update"); code != 2 || !strings.Contains(errs, "usage: tbox build") {
		t.Fatalf("%d %q", code, errs)
	}
}

func TestImageNewHintEndsWithNewline(t *testing.T) {
	r := runInteractive(t, t.TempDir(), nil, nil, "image", "new")
	if r.code != 2 || !strings.HasSuffix(r.err, "\n") {
		t.Fatalf("%d %q", r.code, r.err)
	}
}
