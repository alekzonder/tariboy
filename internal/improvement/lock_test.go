package improvement

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func digestText(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

func TestLoadAndValidateLock(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor", "messages.md"), []byte("locked prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := "schema_version: 1\ntariboy_version: 0.18.0\nprompt_dependencies:\n  messages:\n    repository: tariboy-core\n    upstream_commit: 82fd301\n    upstream_path: store/skills/messages/prompt.md\n    upstream_sha256: " + digestText("locked prompt\n") + "\n    local_path: vendor/messages.md\n    mode: upstream\n"
	if err := os.WriteFile(filepath.Join(dir, "tariboy.lock.yaml"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLock(dir, parsed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor", "messages.md"), []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLock(dir, parsed); err == nil {
		t.Fatal("upstream drift accepted")
	}
}

func TestLockRejectsUnsafeForksAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "prompt.md")); err != nil {
		t.Fatal(err)
	}
	for name, dep := range map[string]PromptDependency{
		"symlink":            {Repository: "core", UpstreamCommit: "abc", UpstreamPath: "prompt.md", UpstreamSHA256: digestText("secret"), LocalPath: "prompt.md", Mode: "upstream"},
		"fork-no-local-hash": {Repository: "core", UpstreamCommit: "abc", UpstreamPath: "prompt.md", UpstreamSHA256: digestText("base"), LocalPath: "fork.md", Mode: "fork"},
		"traversal":          {Repository: "core", UpstreamCommit: "abc", UpstreamPath: "prompt.md", UpstreamSHA256: digestText("base"), LocalPath: "../outside", Mode: "upstream"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateLock(dir, Lock{SchemaVersion: 1, PromptDependencies: map[string]PromptDependency{name: dep}}); err == nil {
				t.Fatal("unsafe dependency accepted")
			}
		})
	}
}
