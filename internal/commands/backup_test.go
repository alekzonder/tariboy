package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestBackupRestoreCommands(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	// c.BaseDir is a temp dir from the helper; ensure the agents dir exists.
	os.MkdirAll(filepath.Join(c.BaseDir, "agents", "bot"), 0o700)
	os.WriteFile(filepath.Join(c.BaseDir, "agents", "bot", "config.json"), []byte(`{"Name":"bot"}`), 0o600)

	out := filepath.Join(t.TempDir(), "bot.tar.gz")
	res, err := h(t, "backup")(c, registry.Params{"target": "bot", "output": out})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["path"].(string) != out {
		t.Fatalf("backup result = %+v", res)
	}
	if fi, err := os.Stat(out); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("archive perms: %v %v", fi, err)
	}
	// Restore under a new name.
	rr, err := h(t, "restore")(c, registry.Params{"file": out, "name": "clone"})
	if err != nil {
		t.Fatal(err)
	}
	if rr.(map[string]any)["agent"] != "clone" {
		t.Fatalf("restore result = %+v", rr)
	}
	if _, err := as.Get("clone"); err != nil {
		t.Fatalf("restored agent missing: %v", err)
	}
}

func TestBackupIncludeSecretsWarns(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	os.MkdirAll(filepath.Join(c.BaseDir, "agents", "bot"), 0o700)

	out := filepath.Join(t.TempDir(), "bot.tar.gz")
	res, err := h(t, "backup")(c, registry.Params{"target": "bot", "output": out, "include-secrets": true})
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := res.(map[string]any)["warning"].(string); w == "" {
		t.Fatalf("expected a warning in backup result when --include-secrets is set, got %+v", res)
	}

	// Without --include-secrets, no warning is present.
	out2 := filepath.Join(t.TempDir(), "bot2.tar.gz")
	res2, err := h(t, "backup")(c, registry.Params{"target": "bot", "output": out2})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res2.(map[string]any)["warning"]; ok {
		t.Fatalf("did not expect a warning without --include-secrets, got %+v", res2)
	}
}

func TestBackupRejectsBadAgentName(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	out := filepath.Join(t.TempDir(), "bad.tar.gz")
	if _, err := h(t, "backup")(c, registry.Params{"target": "../etc", "output": out}); err == nil {
		t.Fatal("expected an error for an invalid agent name")
	}
}
