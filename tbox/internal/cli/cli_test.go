package cli

import (
	"bytes"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Main(Env{Args: args, Stdout: &out, Stderr: &errb})
	return code, out.String(), errb.String()
}

func TestVersionCommand(t *testing.T) {
	for _, a := range []string{"version", "--version"} {
		code, out, _ := run(a)
		if code != 0 || out != "0.1.0\n" {
			t.Errorf("%s: code %d out %q", a, code, out)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errs := run("bogus")
	if code != 2 || !strings.Contains(errs, "bogus") {
		t.Fatalf("code %d stderr %q", code, errs)
	}
}

func TestNoArgsPrintsUsage(t *testing.T) {
	code, _, errs := run()
	if code != 2 || !strings.Contains(errs, "usage") {
		t.Fatalf("code %d stderr %q", code, errs)
	}
}

func TestAgentModeGate(t *testing.T) {
	var out, errb bytes.Buffer
	env := Env{
		Args:    []string{"build"},
		Getenv:  func(k string) string { return map[string]string{"TBOX_IMAGE_DIR": t.TempDir() + "/gone"}[k] },
		Environ: func() []string { return nil },
		Stdout:  &out,
		Stderr:  &errb,
	}
	if code := Main(env); code != 2 || !strings.Contains(errb.String(), "not available inside an image session") {
		t.Fatalf("build: %d %q", code, errb.String())
	}
	env.Args = []string{"skills"}
	if code := Main(env); code != 1 {
		t.Fatalf("skills with a broken directory: %d", code)
	}
	env.Args = []string{"version"}
	if code := Main(env); code != 0 || out.String() != "0.1.0\n" {
		t.Fatalf("version: %d %q", code, out.String())
	}
	env.Args = []string{"help"}
	if code := Main(env); code != 0 {
		t.Fatalf("help: %d", code)
	}
}
