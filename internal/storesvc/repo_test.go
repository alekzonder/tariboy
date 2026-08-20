package storesvc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
)

func sha256hex(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func TestRepoPutVerifiesAndStores(t *testing.T) {
	r := NewRepo(t.TempDir())
	ref := image.Ref{Name: "demo", Tag: "latest"}
	blob := []byte("not-really-a-tarball-but-bytes-are-bytes")
	digest := sha256hex(blob)

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
