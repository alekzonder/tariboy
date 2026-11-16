package imageportable

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
)

// TestTboxPortableImports feeds the envelope tbox exports for its v3-marked
// fixture to the import path of a tariboy base directory.
func TestTboxPortableImports(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "tbox", "testdata", "compat", "v3-marked.tariboy-image.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	service := Service{BaseDir: base, StagingRoot: filepath.Join(base, "imports")}
	preview, err := service.Preview(context.Background(), bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Ref != "compat-marked:latest" {
		t.Fatalf("preview ref = %q", preview.Ref)
	}
	result, err := service.Apply(context.Background(), preview.ImportID, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Ref != "compat-marked:latest" || result.Digest != preview.Digest || result.Reused {
		t.Fatalf("result = %#v, preview = %#v", result, preview)
	}
	store := &image.Store{Dir: filepath.Join(base, "images")}
	if _, err := store.Inspect(image.Ref{Name: "compat-marked", Tag: "latest"}); err != nil {
		t.Fatal(err)
	}
}
