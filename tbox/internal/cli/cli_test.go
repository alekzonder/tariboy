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
