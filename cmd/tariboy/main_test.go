package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

func TestBinaryVersionCommandMatchesFlagWithoutHome(t *testing.T) {
	bin := t.TempDir() + "/tariboy"
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	env := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "HOME=") {
			env = append(env, item)
		}
	}
	env = append(env, "HOME=")

	outputs := make(map[string]string, 2)
	for _, arg := range []string{"--version", "version"} {
		cmd := exec.Command(bin, arg)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tariboy %s: %v\n%s", arg, err, out)
		}
		outputs[arg] = string(out)
		if want := version.Version + "\n"; outputs[arg] != want {
			t.Fatalf("tariboy %s output = %q, want %q", arg, outputs[arg], want)
		}
	}
	if outputs["version"] != outputs["--version"] {
		t.Fatalf(
			"version output %q differs from --version output %q",
			outputs["version"],
			outputs["--version"],
		)
	}
}
