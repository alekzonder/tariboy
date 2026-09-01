package storeassets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/paths"
)

func TestEnsureInstallsVersionedSkillsWithoutChangingOldVersions(t *testing.T) {
	p := paths.New(t.TempDir())
	if err := Ensure(p, "0.33.0"); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(p.CurrentVersionStoreDir("0.33.0"), "skills", "whoami", "SKILL.md")
	first, err := os.ReadFile(oldPath)
	if err != nil || len(first) == 0 {
		t.Fatalf("read installed prompt: bytes=%d err=%v", len(first), err)
	}
	if err := Ensure(p, "0.34.0"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(oldPath)
	if err != nil || string(after) != string(first) {
		t.Fatalf("old version changed: err=%v", err)
	}
}

func TestEnsureInstallsSkillLaunchersExecutable(t *testing.T) {
	p := paths.New(t.TempDir())
	if err := Ensure(p, "0.33.0"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(p.CurrentVersionStoreDir("0.33.0"), "skills", "loop", "scripts", "loop.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("loop launcher mode = %o, want executable", info.Mode().Perm())
	}
}

func TestEnsureRejectsConflictingExistingVersion(t *testing.T) {
	p := paths.New(t.TempDir())
	dir := p.CurrentVersionStoreDir("0.33.0")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "foreign"), []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(p, "0.33.0"); err == nil {
		t.Fatal("Ensure accepted a conflicting version directory")
	}
}

func TestEnsureRejectsSymlinkedStoreAsset(t *testing.T) {
	p := paths.New(t.TempDir())
	version := "0.33.0"
	if err := Ensure(p, version); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(p.CurrentVersionStoreDir(version), "skills", "whoami", "SKILL.md")
	body, err := os.ReadFile(asset)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(outside, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(asset); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, asset); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(p, version); err == nil {
		t.Fatal("Ensure accepted a symlinked Store asset")
	}
}
