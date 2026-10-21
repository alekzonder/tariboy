package main

import (
	"strings"
	"testing"
)

func TestUpdateHelp(t *testing.T) {
	isolate(t)
	out, _, code := runCLI(t, "update", "--help")
	if code != 0 || !strings.Contains(out, "tariboy update") || !strings.Contains(out, "--cleanup") {
		t.Fatalf("code=%d help=%q does not describe the command", code, out)
	}
}

func TestUpdateRejectsBadArguments(t *testing.T) {
	isolate(t)
	for _, argv := range [][]string{
		{"update", "--keep", "3"},
		{"update", "--cleanup", "--keep", "0"},
		{"update", "--cleanup", "--keep", "abc"},
		{"update", "--cleanup", "--keep"},
		{"update", "--yolo"},
		{"update", "1.0.0", "2.0.0"},
	} {
		_, errOut, code := runCLI(t, argv...)
		if code != 2 || !strings.HasPrefix(errOut, "update") {
			t.Fatalf("%v: code=%d stderr=%q, want a usage error", argv, code, errOut)
		}
	}
}

func TestUpdateReportsFailure(t *testing.T) {
	isolate(t)
	t.Setenv("HOME", t.TempDir())
	_, errOut, code := runCLI(t, "update", "0.64.0")
	if code != 1 || !strings.HasPrefix(errOut, "update:") {
		t.Fatalf("code=%d stderr=%q, want the update failure", code, errOut)
	}
}
