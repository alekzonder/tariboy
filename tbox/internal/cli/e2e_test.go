package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/cli"
)

// TestMain lets the test binary act as the tbox executable: the fake harness
// script calls it back with TBOX_E2E_CHILD=1.
func TestMain(m *testing.M) {
	if os.Getenv("TBOX_E2E_CHILD") == "1" {
		if os.Getenv("TBOX_E2E_LOCAL") == "1" {
			cli.UseLocalGit()
		}
		cwd, _ := os.Getwd()
		os.Exit(cli.Main(cli.Env{
			Args: os.Args[1:], Getenv: os.Getenv, Environ: os.Environ,
			Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Cwd: cwd,
			Exec: syscall.Exec, LookPath: exec.LookPath, Now: time.Now,
		}))
	}
	os.Exit(m.Run())
}

// childEnviron is the environment of this test process without the image
// session variables, so a test run from inside a tbox session does not put
// the child tbox into agent mode.
func childEnviron() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "TBOX_IMAGE_DIR", "TBOX_IMAGE", "TBOX_SKILL_DIR":
			continue
		}
		out = append(out, kv)
	}
	return out
}

func TestEndToEnd(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(bin, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("tbox", "#!/bin/sh\nTBOX_E2E_CHILD=1 exec \""+self+"\" \"$@\"\n")
	write("claude", "#!/bin/sh\n"+
		"if [ \"$1\" = --version ]; then echo '2.1.325 (Claude Code)'; exit 0; fi\n"+
		"tbox skills\ntbox skill exec hello-skill scripts/run.sh ok\n")
	home := t.TempDir()
	fixture, err := filepath.Abs("../../testdata/compat/v2-basic")
	if err != nil {
		t.Fatal(err)
	}
	env := append(childEnviron(), "TBOX_HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"), "TBOX_E2E_CHILD=1")
	run := func(args ...string) string {
		cmd := exec.Command(self, args...)
		cmd.Env = env
		cmd.Dir = t.TempDir()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tbox %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("build", "--path", fixture, "--name", "compat-basic")
	out := run("run", "compat-basic", "claude")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "hello-skill\t") {
		t.Fatalf("skills output %q", out)
	}
	if strings.Count(out, "\t") != 1 {
		t.Fatalf("expected exactly one skill listed: %q", out)
	}
	if !strings.Contains(lines[len(lines)-1], "hello") {
		t.Fatalf("script output missing: %q", out)
	}
}
