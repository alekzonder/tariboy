package image

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// versionedSource builds a minimal schema-v2 source declaring image_version.
func versionedSource(t *testing.T, imageVersion, body string) *imagefile.V2 {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PROMPT.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return &imagefile.V2{
		SchemaVersion: 2,
		ImageVersion:  imageVersion,
		Dir:           dir,
		Plugins:       []imagefile.V2Plugin{},
		Prompts:       []imagefile.PromptEntry{{File: "./PROMPT.md"}},
	}
}

// TestRebuildingAnExistingRefMovesItsTags is the regression for the refusal
// that blocked publication: every build path must be able to rebuild a ref
// that already exists.
func TestRebuildingAnExistingRefMovesItsTags(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "worker", Tag: "latest"}
	src := versionedSource(t, "1.0.0", "first\n")

	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, func() time.Time { return time.Unix(1, 0) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, func() time.Time { return time.Unix(2, 0) }, nil)
	if err != nil {
		t.Fatalf("rebuilding an existing ref failed: %v", err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("same image_version produced two ids: %s and %s", first.Digest, second.Digest)
	}
	if want := RefID(ref.Name, "1.0.0"); second.Digest != want {
		t.Fatalf("ref id = %s, want %s", second.Digest, want)
	}
	current, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if current.Digest != second.Digest {
		t.Fatalf("tag points at %s, want %s", current.Digest, second.Digest)
	}
}

// TestBumpingImageVersionCreatesAnotherRef keeps pinned agents working: the
// previous ref id stays readable and unpackable after the tag moves.
func TestBumpingImageVersionCreatesAnotherRef(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "worker", Tag: "latest"}
	clock := func() time.Time { return time.Unix(1, 0) }

	old, err := BuildV2(versionedSource(t, "1.0.0", "old\n"), imagefile.ResolveRoots{}, ref, store, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := BuildV2(versionedSource(t, "1.1.0", "new\n"), imagefile.ResolveRoots{}, ref, store, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	if old.Digest == next.Digest {
		t.Fatal("bumping image_version reused the same ref id")
	}
	pinned, err := store.InspectPinned(ref, old.Digest)
	if err != nil {
		t.Fatalf("pinned generation is unavailable: %v", err)
	}
	if pinned.ImageVersion != "1.0.0" {
		t.Fatalf("pinned image_version = %q, want 1.0.0", pinned.ImageVersion)
	}
	dest := t.TempDir()
	if err := store.UnpackPinned(ref, old.Digest, dest); err != nil {
		t.Fatalf("unpack pinned generation: %v", err)
	}
}

// TestTagsArePointersToOneRef is the publication-after-merge contract: the
// versioned tag and latest must name the same id without a second build.
func TestTagsArePointersToOneRef(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	versioned := Ref{Name: "worker", Tag: "2.0.0"}
	latest := Ref{Name: "worker", Tag: "latest"}
	src := versionedSource(t, "2.0.0", "body\n")

	built, err := BuildV2(src, imagefile.ResolveRoots{}, versioned, store, func() time.Time { return time.Unix(1, 0) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTag(latest, built.Digest); err != nil {
		t.Fatalf("move latest onto the built ref: %v", err)
	}
	moved, err := store.Inspect(latest)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Digest != built.Digest {
		t.Fatalf("latest = %s, want %s", moved.Digest, built.Digest)
	}
	if moved.Tag != "latest" {
		t.Fatalf("inspected tag = %q, want latest", moved.Tag)
	}
}

// TestMigrateAdoptsLegacyLayout covers the one-way upgrade of a store written
// before tags became pointers: every archive keeps its content digest as its
// ref id, so agents pinned by digest keep resolving.
func TestMigrateAdoptsLegacyLayout(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	staging := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "worker", Tag: "latest"}
	// A pre-ref-model archive declares no id, so it stays content-addressed.
	if _, err := BuildV2(versionedSource(t, "", "legacy\n"), imagefile.ResolveRoots{}, ref, staging, func() time.Time { return time.Unix(1, 0) }, nil); err != nil {
		t.Fatal(err)
	}
	archive, err := staging.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	legacyDigest := sha256.Sum256(archive)
	legacyID := hex.EncodeToString(legacyDigest[:])

	// Lay the bytes out the way the pre-ref-model store did.
	if err := os.MkdirAll(filepath.Join(store.Dir, ref.Name), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"latest.tar.gz":  archive,
		"latest.digest":  []byte(legacyID + "\n"),
		"latest.mutable": []byte("mutable\n"),
	} {
		if err := os.WriteFile(filepath.Join(store.Dir, ref.Name, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	history := filepath.Join(store.Dir, ".mutable", ref.Name, ref.Tag)
	if err := os.MkdirAll(history, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(history, legacyID+".tar.gz"), archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store.Dir, ".publications"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, ".publications", "crashed.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
	current, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if current.Digest != legacyID {
		t.Fatalf("migrated id = %s, want the legacy digest %s", current.Digest, legacyID)
	}
	if _, err := store.InspectPinned(ref, legacyID); err != nil {
		t.Fatalf("digest pinned before migration no longer resolves: %v", err)
	}
	for _, gone := range []string{".mutable", ".managed", ".publications"} {
		if _, err := os.Stat(filepath.Join(store.Dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s survived migration: %v", gone, err)
		}
	}
	// A rebuild after migration adopts the version-derived id.
	rebuilt, err := BuildV2(versionedSource(t, "1.0.0", "rebuilt\n"), imagefile.ResolveRoots{}, ref, store, func() time.Time { return time.Unix(2, 0) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Digest != RefID(ref.Name, "1.0.0") {
		t.Fatalf("rebuilt id = %s, want %s", rebuilt.Digest, RefID(ref.Name, "1.0.0"))
	}
	if _, err := store.InspectPinned(ref, legacyID); err != nil {
		t.Fatalf("migrated generation lost after rebuild: %v", err)
	}
}
