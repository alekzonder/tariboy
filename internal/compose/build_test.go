package compose

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
)

// TestBuildBuildsDeclaredImages proves compose `build` actually builds the
// images declared under the file's `images:` map in-process (image.Build /
// imagefile.Parse against the shared images dir), the same mechanism Up uses
// for its build step — not a daemon route. No Caller calls are made.
func TestBuildBuildsDeclaredImages(t *testing.T) {
	workdir := t.TempDir()
	ctxDir := filepath.Join(workdir, "analyst")
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tariboyfile := "schema_version: 1\n"
	if err := os.WriteFile(filepath.Join(ctxDir, "Tariboyfile.yaml"), []byte(tariboyfile), 0o600); err != nil {
		t.Fatal(err)
	}
	imagesDir := t.TempDir()

	fc := newFake()
	r := NewRunner(fc, imagesDir, workdir, io.Discard)
	f, err := Parse([]byte(goodYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Build(f); err != nil {
		t.Fatalf("build: %v", err)
	}

	store := &image.Store{Dir: imagesDir}
	ref, err := image.ParseRef("analyst:latest")
	if err != nil {
		t.Fatal(err)
	}
	if !store.Exists(ref) {
		t.Fatal("build did not produce the declared image in the store")
	}
	if len(fc.calls) != 0 {
		t.Fatalf("build is CLI-local; it must not call the daemon, got: %v", fc.calls)
	}
}

func TestBuildDispatchesSchemaV2Images(t *testing.T) {
	workdir := t.TempDir()
	ctxDir := filepath.Join(workdir, "analyst")
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctxDir, "Tariboyfile.yaml"), []byte("schema_version: 2\nplugins: []\nprompts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	imagesDir := t.TempDir()
	r := NewRunner(newFake(), imagesDir, workdir, io.Discard)
	f, err := Parse([]byte(goodYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Build(f); err != nil {
		t.Fatal(err)
	}
	manifest, err := (&image.Store{Dir: imagesDir}).Inspect(image.Ref{Name: "analyst", Tag: "latest"})
	if err != nil || manifest.SchemaVersion != 2 {
		t.Fatalf("manifest = %#v, %v", manifest, err)
	}
}
