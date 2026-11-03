package imageportable

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/portablearchive"
	"github.com/alekzonder/tariboy/internal/version"
)

// buildInner builds a one-prompt image declaring image_version 1.0.0 and
// returns its runnable archive.
func buildInner(t *testing.T, ref image.Ref, prompt string) []byte {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "p.md"), []byte(prompt), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &image.Store{Dir: t.TempDir()}
	if _, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, ImageVersion: "1.0.0", Dir: source, Prompts: []imagefile.PromptEntry{{File: "./p.md"}}}, imagefile.ResolveRoots{}, ref, store, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	archive, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

// rewriteManifest rewrites manifest.json of a runnable archive.
func rewriteManifest(t *testing.T, archive []byte, rewrite func(map[string]any)) []byte {
	t.Helper()
	in, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	tr := tar.NewReader(in)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "manifest.json" {
			var fields map[string]any
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatal(err)
			}
			rewrite(fields)
			if body, err = json.MarshalIndent(fields, "", "  "); err != nil {
				t.Fatal(err)
			}
		}
		header := *h
		header.Size = int64(len(body))
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// exportInner wraps a runnable archive the way Export does, recording digest
// as the producer's image digest.
func exportInner(t *testing.T, ref image.Ref, inner []byte, digest string) []byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.tar.gz"), inner, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(inner)
	meta, err := json.Marshal(metadata{Ref: ref.String(), Digest: digest, ProducerVersion: version.Version, Plugins: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	file := portablearchive.File{Path: "image.tar.gz", Size: int64(len(inner)), SHA256: hex.EncodeToString(sum[:])}
	if err := portablearchive.Write(&out, dir, portablearchive.Manifest{Format: "tariboy-portable", Version: 1, Kind: "image-artifact", Files: []portablearchive.File{file}, Metadata: meta}, []string{"image.tar.gz"}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func manifestID(t *testing.T, ref image.Ref, inner []byte) string {
	t.Helper()
	manifest, err := image.ValidatePortableArchive(inner, ref)
	if err != nil {
		t.Fatal(err)
	}
	return manifest.Digest
}

func TestArtifactImportRejectsChangedMemberUnderContentID(t *testing.T) {
	ref := image.Ref{Name: "demo", Tag: "v1"}
	inner := buildInner(t, ref, "portable")
	id := manifestID(t, ref, inner)
	tampered := rewriteManifest(t, inner, func(fields map[string]any) { fields["role"] = map[string]any{"text": "substituted"} })
	artifact := exportInner(t, ref, tampered, id)
	base := t.TempDir()
	importer := Service{BaseDir: base, StagingRoot: filepath.Join(base, "imports")}
	if _, err := importer.Preview(context.Background(), bytes.NewReader(artifact), int64(len(artifact))); err == nil || !strings.Contains(err.Error(), "does not match content digest") {
		t.Fatalf("Preview err = %v, want content digest mismatch", err)
	}
}

func TestArtifactImportAcceptsLegacyExport(t *testing.T) {
	ref := image.Ref{Name: "demo", Tag: "v1"}
	legacyID := image.RefID(ref.Name, "1.0.0")
	legacy := rewriteManifest(t, buildInner(t, ref, "portable"), func(fields map[string]any) { fields["id"] = legacyID })
	artifact := exportInner(t, ref, legacy, legacyID)
	base := t.TempDir()
	importer := Service{BaseDir: base, StagingRoot: filepath.Join(base, "imports")}
	preview, err := importer.Preview(context.Background(), bytes.NewReader(artifact), int64(len(artifact)))
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Apply(context.Background(), preview.ImportID, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Digest != legacyID {
		t.Fatalf("imported digest = %s, want the legacy id %s", result.Digest, legacyID)
	}
}

func TestArtifactImportRefusesConflictingImageVersion(t *testing.T) {
	ref := image.Ref{Name: "demo", Tag: "v1"}
	base := t.TempDir()
	importer := Service{BaseDir: base, StagingRoot: filepath.Join(base, "imports")}
	for i, prompt := range []string{"first", "second"} {
		inner := buildInner(t, ref, prompt)
		artifact := exportInner(t, ref, inner, manifestID(t, ref, inner))
		preview, err := importer.Preview(context.Background(), bytes.NewReader(artifact), int64(len(artifact)))
		if err != nil {
			t.Fatal(err)
		}
		_, err = importer.Apply(context.Background(), preview.ImportID, "")
		var conflict *image.VersionConflictError
		if i == 0 && err != nil {
			t.Fatal(err)
		}
		if i == 1 && !errors.As(err, &conflict) {
			t.Fatalf("second import err = %v, want VersionConflictError", err)
		}
	}
	prompt, err := (&image.Store{Dir: filepath.Join(base, "images")}).RenderPrompt(ref)
	if err != nil || prompt != "first" {
		t.Fatalf("tagged prompt = %q err = %v, want the first import", prompt, err)
	}
}
