package imageportable

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/paths"
)

// tboxCompatDir holds the fixtures that pin the formats tbox shares with
// tariboy.
var tboxCompatDir = filepath.Join("..", "..", "tbox", "testdata", "compat")

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

var updateTboxCompat = flag.Bool("update-tbox-compat", false, "regenerate tbox/testdata/compat/v2-tariboy.tariboy-image.tar.gz")

// buildTariboyFixture builds the tbox v2-tariboy fixture (a built-in plugin
// with its packaged skill and runtime prompt entries) into the image store
// of base, with tariboy's own build.
func buildTariboyFixture(t *testing.T, base string) image.Manifest {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join(tboxCompatDir, "v2-tariboy"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := imagefile.ParseV2(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &image.Store{Dir: paths.Paths{Base: base}.ImagesDir()}
	clock := func() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }
	manifest, err := image.BuildV2(src, imagefile.ResolveRoots{}, image.Ref{Name: "compat-tariboy", Tag: "latest"}, store, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

// TestTboxTariboyEnvelope keeps the envelope tbox imports in its compat
// tests a tariboy export: a fresh tariboy build of the fixture must still
// have the digest the committed envelope records. -update-tbox-compat
// rewrites the envelope with tariboy's Export.
func TestTboxTariboyEnvelope(t *testing.T) {
	base := t.TempDir()
	manifest := buildTariboyFixture(t, base)
	if len(manifest.Plugins) != 1 || manifest.Plugins[0].Name != "whoami" {
		t.Fatalf("plugins = %+v", manifest.Plugins)
	}
	path := filepath.Join(tboxCompatDir, "v2-tariboy.tariboy-image.tar.gz")
	if *updateTboxCompat {
		var buf bytes.Buffer
		if err := (Service{BaseDir: base}).Export(context.Background(), "compat-tariboy:latest", &buf); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	service := Service{BaseDir: other, StagingRoot: filepath.Join(other, "imports")}
	preview, err := service.Preview(context.Background(), bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Ref != "compat-tariboy:latest" || preview.Digest != manifest.Digest {
		t.Fatalf("committed envelope %s %s, fresh build %s; run go test ./internal/imageportable -run TestTboxTariboyEnvelope -update-tbox-compat", preview.Ref, preview.Digest, manifest.Digest)
	}
}
