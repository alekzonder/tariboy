package storesvc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func sha256hex(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func validBlob(t *testing.T, ref image.Ref) ([]byte, string) {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "Tariboyfile.yaml"), []byte("schema_version: 1\nprompts:\n  - task.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "task.md"), []byte("do it\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := imagefile.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest, err := image.Build(f, ref, &image.Store{Dir: dir}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(filepath.Join(dir, ref.Name, ref.Tag+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return blob, manifest.Digest
}

func TestRepoPutVerifiesAndStores(t *testing.T) {
	r := NewRepo(t.TempDir())
	ref := image.Ref{Name: "demo", Tag: "latest"}
	blob, digest := validBlob(t, ref)

	got, err := r.Put(ref, bytes.NewReader(blob), digest)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got != digest {
		t.Fatalf("Put digest = %s, want %s", got, digest)
	}
	head, ok := r.Head(ref)
	if !ok || head != digest {
		t.Fatalf("Head = %q,%v, want %s,true", head, ok, digest)
	}
	rc, hd, err := r.Open(ref)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	back, _ := io.ReadAll(rc)
	if !bytes.Equal(back, blob) || hd != digest {
		t.Fatalf("Open returned %q,%s", back, hd)
	}
}

func TestRepoPutRejectsInvalidArchiveWithoutReplacingExisting(t *testing.T) {
	r := NewRepo(t.TempDir())
	ref := image.Ref{Name: "demo", Tag: "latest"}
	valid, digest := validBlob(t, ref)
	if _, err := r.Put(ref, bytes.NewReader(valid), digest); err != nil {
		t.Fatal(err)
	}
	bad := []byte("not an image archive")
	if _, err := r.Put(ref, bytes.NewReader(bad), sha256hex(bad)); err == nil {
		t.Fatal("correct-digest invalid archive was accepted")
	}
	rc, _, err := r.Open(ref)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, valid) {
		t.Fatal("invalid upload replaced the existing image")
	}
}

func TestRepoPutRejectsDigestMismatch(t *testing.T) {
	r := NewRepo(t.TempDir())
	ref := image.Ref{Name: "demo", Tag: "latest"}
	_, err := r.Put(ref, bytes.NewReader([]byte("abc")), sha256hex([]byte("DIFFERENT")))
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("want ErrDigestMismatch, got %v", err)
	}
	if _, ok := r.Head(ref); ok {
		t.Fatal("a rejected blob must not be stored")
	}
}

func TestRepoHeadMissing(t *testing.T) {
	r := NewRepo(t.TempDir())
	if _, ok := r.Head(image.Ref{Name: "nope", Tag: "latest"}); ok {
		t.Fatal("Head on a missing blob must be false")
	}
}
