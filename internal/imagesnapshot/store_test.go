package imagesnapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	storedb "github.com/alekzonder/tariboy/internal/store"
)

func TestCaptureStoresImmutableContentAndReusesDigest(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte("schema_version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "PROMPT.md"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".tariboy-source.json"), []byte(`{"internal":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s := Store{DB: db.DB, Root: filepath.Join(base, "snapshots")}
	first, err := s.Capture(context.Background(), "demo:v1", "sha256:image-one", "demo", source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Capture(context.Background(), "demo:v2", "sha256:image-two", "demo", source)
	if err != nil {
		t.Fatal(err)
	}
	if first.SourceDigest == "" || second.SourceDigest != first.SourceDigest {
		t.Fatalf("source digests = %q and %q, want one non-empty digest", first.SourceDigest, second.SourceDigest)
	}
	if first.RelativeDir != second.RelativeDir {
		t.Fatalf("relative dirs = %q and %q, want content-addressed reuse", first.RelativeDir, second.RelativeDir)
	}

	root, err := s.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(filepath.Join(root, "PROMPT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != "first\n" {
		t.Fatalf("snapshot prompt = %q", prompt)
	}
	if _, err := os.Stat(filepath.Join(root, ".tariboy-source.json")); !os.IsNotExist(err) {
		t.Fatalf("internal source metadata entered snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "PROMPT.md"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := s.Capture(context.Background(), "demo:v3", "sha256:image-three", "demo", source)
	if err != nil {
		t.Fatal(err)
	}
	if third.SourceDigest == first.SourceDigest {
		t.Fatal("changed source reused the old digest")
	}
}

func TestCaptureRejectsUnsafeEntriesWithoutPublishing(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside"), filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}

	s := Store{DB: db.DB, Root: filepath.Join(base, "snapshots")}
	if _, err := s.Capture(context.Background(), "demo:v1", "sha256:image", "demo", source); err == nil {
		t.Fatal("Capture accepted a symlink")
	}
	if _, ok, err := s.Lookup(context.Background(), "demo:v1"); err != nil || ok {
		t.Fatalf("Lookup after rejected capture = ok %v, err %v", ok, err)
	}
}
