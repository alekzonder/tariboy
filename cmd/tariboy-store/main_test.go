package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBinaryBuildsAndVersion builds the binary and checks --version + the
// required --data-dir guard. Run from the module root by `go test ./...`.
func TestBinaryBuildsAndVersion(t *testing.T) {
	bin := t.TempDir() + "/tariboy-store"
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("--version: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatal("--version printed nothing")
	}
	// Missing --data-dir must exit non-zero.
	if err := exec.Command(bin).Run(); err == nil {
		t.Fatal("running without --data-dir must fail")
	}
}
