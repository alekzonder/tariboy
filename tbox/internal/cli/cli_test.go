package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/version"
)

func run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Main(Env{Args: args, Stdout: &out, Stderr: &errb})
	return code, out.String(), errb.String()
}

func TestVersionCommand(t *testing.T) {
	for _, a := range []string{"version", "--version"} {
		code, out, _ := run(a)
		if code != 0 || out != version.Version+"\n" {
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
	if code := Main(env); code != 2 || !strings.Contains(errb.String(), "not available inside the session of") {
		t.Fatalf("build: %d %q", code, errb.String())
	}
	env.Args = []string{"skills"}
	if code := Main(env); code != 1 {
		t.Fatalf("skills with a broken directory: %d", code)
	}
	env.Args = []string{"version"}
	if code := Main(env); code != 0 || out.String() != version.Version+"\n" {
		t.Fatalf("version: %d %q", code, out.String())
	}
	env.Args = []string{"help"}
	if code := Main(env); code != 0 {
		t.Fatalf("help: %d", code)
	}
}

func TestUsageInsideSession(t *testing.T) {
	var out, errb bytes.Buffer
	env := Env{
		Args:   []string{"help"},
		Getenv: func(k string) string { return map[string]string{"TBOX_IMAGE_DIR": "/x"}[k] },
		Stdout: &out,
		Stderr: &errb,
	}
	if code := Main(env); code != 0 {
		t.Fatalf("help: %d", code)
	}
	got := out.String()
	for _, want := range []string{"skills", "skill NAME", "skill exec NAME SCRIPT", "version", "help"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage lacks %q: %s", want, got)
		}
	}
	if strings.Contains(got, "build") {
		t.Errorf("usage lists build: %s", got)
	}
}

func TestHelpGoesToStdout(t *testing.T) {
	for _, a := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		code, out, errs := run(a...)
		if code != 0 || errs != "" || !strings.HasPrefix(out, "usage: tbox <command>") {
			t.Fatalf("%v: %d %q %q", a, code, out, errs)
		}
		if strings.Contains(out, "[") && strings.Contains(out, "build clean") {
			t.Fatalf("%v: prints a Go slice: %q", a, out)
		}
		for name := range commands {
			if !strings.Contains(out, "\n  "+name) {
				t.Errorf("help lacks command %q", name)
			}
		}
	}
	code, out, errs := run("bogus")
	if code != 2 || out != "" || !strings.Contains(errs, "usage: tbox <command>") {
		t.Fatalf("unknown: %d %q %q", code, out, errs)
	}
}

func TestSubcommandHelp(t *testing.T) {
	cases := map[string][]string{
		"usage: tbox build":          {"build", "--help"},
		"usage: tbox run":            {"run", "-h"},
		"usage: tbox clean [IMAGE]":  {"clean", "--help"},
		"usage: tbox image new NAME": {"image", "new", "--help"},
		"usage: tbox image export":   {"image", "export", "-h"},
		"usage: tbox store":          {"store", "add", "--help"},
		"usage: tbox skill STORE":    {"skill", "-h"},
		"usage: tbox images":         {"images", "--help"},
	}
	for want, args := range cases {
		code, out, errs := run(args...)
		if code != 0 || errs != "" || !strings.HasPrefix(out, want) {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	// After "--" a help flag belongs to the harness.
	if code, out, _ := runIn(t, t.TempDir(), "/", "run", "x", "claude", "--", "--help"); code == 0 && strings.HasPrefix(out, "usage") {
		t.Fatal("help after -- was intercepted")
	}
}

func TestCleanRejectsFlags(t *testing.T) {
	for _, a := range []string{"-x", "--force"} {
		code, _, errs := run("clean", a)
		if code != 2 || !strings.Contains(errs, "usage: tbox clean [IMAGE]") {
			t.Errorf("%s: %d %q", a, code, errs)
		}
	}
}
