package builtinimages

import (
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
)

func generatedBundle(t *testing.T, source, version string) fs.FS {
	t.Helper()
	out := filepath.Join(t.TempDir(), "generated")
	if err := Generate(source, out, version); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(out, "basic.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"generated/basic.tar.gz": &fstest.MapFile{Data: archive, Mode: 0o600},
		"generated/VERSION":      &fstest.MapFile{Data: []byte(version + "\n"), Mode: 0o600},
	}
}

func basicSource(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "internal", "builtinimages", "source")
}

func TestEnsureBasicInstallsNoOpsAndRefreshes(t *testing.T) {
	store := &image.Store{Dir: t.TempDir()}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ref := image.Ref{Name: "basic", Tag: "latest"}

	if err := ensureBasicFS(generatedBundle(t, basicSource(t), "1.0.0"), store, log); err != nil {
		t.Fatal(err)
	}
	first, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(store.Dir, "basic", "latest.tar.gz")
	before, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureBasicFS(generatedBundle(t, basicSource(t), "1.0.0"), store, log); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("same-version activation replaced the basic archive")
	}

	if err := ensureBasicFS(generatedBundle(t, basicSource(t), "1.1.0"), store, log); err != nil {
		t.Fatal(err)
	}
	refreshed, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(after, refreshed) {
		t.Fatal("new-version activation did not replace the basic archive")
	}
	second, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != second.Name || second.Name != "basic" || second.Tag != "latest" {
		t.Fatalf("ref changed during refresh: before=%+v after=%+v", first, second)
	}
}

func TestEnsureBasicRefreshPreservesPreviouslyPinnedArchive(t *testing.T) {
	store := &image.Store{Dir: t.TempDir()}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ref := image.Ref{Name: "basic", Tag: "latest"}

	if err := ensureBasicFS(generatedBundle(t, basicSource(t), "1.0.0"), store, log); err != nil {
		t.Fatal(err)
	}
	first, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)) + 10*time.Millisecond)
	if err := ensureBasicFS(generatedBundle(t, basicSource(t), "1.1.0"), store, log); err != nil {
		t.Fatal(err)
	}
	current, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if current.Digest == first.Digest {
		t.Fatal("test setup did not replace the managed basic archive")
	}
	pinned, err := store.InspectPinned(ref, first.Digest)
	if err != nil {
		t.Fatalf("previously pinned basic archive was lost during upgrade: %v", err)
	}
	if pinned.Digest != first.Digest || pinned.PromptTemplateSHA256 != first.PromptTemplateSHA256 {
		t.Fatalf("pinned archive = %+v, want digest %s template %s", pinned, first.Digest, first.PromptTemplateSHA256)
	}
}

func TestEnsureBasicPreservesValidArchiveWhenBundleIsCorrupt(t *testing.T) {
	store := &image.Store{Dir: t.TempDir()}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := ensureBasicFS(generatedBundle(t, basicSource(t), "1.0.0"), store, log); err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "basic", Tag: "latest"}
	before, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := fstest.MapFS{
		"generated/basic.tar.gz": &fstest.MapFile{Data: []byte("not an archive")},
		"generated/VERSION":      &fstest.MapFile{Data: []byte("2.0.0\n")},
	}
	if err := ensureBasicFS(corrupt, store, log); err == nil {
		t.Fatal("corrupt embedded archive was accepted")
	}
	after, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if after.Digest != before.Digest {
		t.Fatalf("corrupt refresh changed valid archive: %s -> %s", before.Digest, after.Digest)
	}
}

func TestEnsureBasicRepairsMissingArchiveWithoutTouchingOtherImages(t *testing.T) {
	store := &image.Store{Dir: t.TempDir()}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	bundle := generatedBundle(t, basicSource(t), "1.0.0")
	if err := ensureBasicFS(bundle, store, log); err != nil {
		t.Fatal(err)
	}
	otherDir := filepath.Join(store.Dir, "other")
	if err := os.MkdirAll(otherDir, 0o700); err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(otherDir, "latest.tar.gz")
	if err := os.WriteFile(otherPath, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.Dir, "basic", "latest.tar.gz")); err != nil {
		t.Fatal(err)
	}
	if err := ensureBasicFS(bundle, store, log); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect(image.Ref{Name: "basic", Tag: "latest"}); err != nil {
		t.Fatalf("basic archive was not repaired: %v", err)
	}
	if got, err := os.ReadFile(otherPath); err != nil || string(got) != "unrelated" {
		t.Fatalf("unrelated image changed: %q, %v", got, err)
	}
}
