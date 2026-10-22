package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

// Catches agent.run creating a missing cwd without create_cwd (a typo must
// still fail) or failing to create it when the operator asked for it.
func TestAgentRunCreateCwd(t *testing.T) {
	t.Parallel()
	c, _, fc := ctxWithStore(t)
	base := t.TempDir()

	missing := filepath.Join(base, "no", "such")
	if _, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "name": "w1", "cwd": missing}); !isCode(err, "bad_cwd") {
		t.Fatalf("missing cwd without create_cwd err = %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("cwd created without create_cwd: %v", err)
	}

	if _, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "name": "w2", "cwd": missing, "create_cwd": true}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(missing); err != nil || !fi.IsDir() {
		t.Fatalf("cwd not created: %v", err)
	}
	if fc.ran.Cwd != missing {
		t.Fatalf("RunSpec cwd = %q, want %q", fc.ran.Cwd, missing)
	}

	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h(t, "agent.run")(c, registry.Params{"image": "basic:latest", "name": "w3", "cwd": file, "create_cwd": true}); !isCode(err, "bad_cwd") {
		t.Fatalf("file cwd with create_cwd err = %v", err)
	}
}

// Catches agent.cwd creating a directory without create, creating one on a
// clear, or ignoring create for a new missing path.
func TestAgentCwdCreate(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	base := t.TempDir()
	call := func(p registry.Params) error {
		p["name"] = "a1"
		_, err := registry.Call(c, context.Background(), agentCwd(), p)
		return err
	}

	missing := filepath.Join(base, "no", "such")
	if err := call(registry.Params{"value": missing}); !isCode(err, "bad_cwd") {
		t.Fatalf("missing cwd without create err = %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("cwd created without create: %v", err)
	}

	if err := call(registry.Params{"value": missing, "create": true}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(missing); err != nil || !fi.IsDir() {
		t.Fatalf("cwd not created: %v", err)
	}
	if a, _ := getAgent(c, "a1"); a.Cwd != missing {
		t.Fatalf("cwd = %q, want %q", a.Cwd, missing)
	}

	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := call(registry.Params{"value": file, "create": true}); !isCode(err, "bad_cwd") {
		t.Fatalf("file cwd with create err = %v", err)
	}

	if err := call(registry.Params{"value": "", "create": true}); err != nil {
		t.Fatal(err)
	}
	if a, _ := getAgent(c, "a1"); a.Cwd != "" {
		t.Fatalf("cwd not cleared: %q", a.Cwd)
	}
}
