package regclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/storesvc"
)

func sha256hex(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func TestPushThenPullMovesBytesAndVerifies(t *testing.T) {
	storeDir := t.TempDir()
	srv, err := storesvc.New(storesvc.Config{AllowInsecure: true, DataDir: storeDir, DBPath: filepath.Join(storeDir, "s.db")})
	if err != nil {
		t.Fatal(err)
	}
	srv.SeedToken("rw", storesvc.ScopeReadWrite)
	ts := httptest.NewServer(srv.Handler())
	defer func() { ts.Close(); srv.Close() }()

	cl, err := NewClient(ts.URL, "rw", "")
	if err != nil {
		t.Fatal(err)
	}
	srcImages := t.TempDir()
	ref, digest := buildImage(t, srcImages)
	pres, err := Push(srcImages, ref, cl)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if pres["digest"] != digest || pres["pushed"] != true {
		t.Fatalf("Push result = %+v", pres)
	}
	// Second push is a no-op (HEAD-skip).
	pres2, _ := Push(srcImages, ref, cl)
	if pres2["skipped"] != true {
		t.Fatalf("second Push should skip, got %+v", pres2)
	}

	// Pull into a FRESH image dir; the installed sidecar digest must match.
	dstImages := t.TempDir()
	if _, err := Pull(dstImages, ref, cl); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dstImages, "demo", "latest.digest"))
	if err != nil {
		t.Fatalf("pulled sidecar: %v", err)
	}
	if strings.TrimSpace(string(got)) != digest {
		t.Fatalf("pulled digest = %s, want %s", strings.TrimSpace(string(got)), digest)
	}
}

// TestPullRejectsDigestMismatch is the load-bearing security invariant: a
// tampered/corrupt download (bytes whose sha256 disagrees with the server's
// advertised digest) MUST be rejected before install — no .tar.gz, no sidecar,
// no temp left behind, and nothing retrievable from the local store.
func TestPullRejectsDigestMismatch(t *testing.T) {
	tampered := []byte("tampered-bytes")
	// Server lies: advertises a digest that does NOT match the body it serves.
	bogus := sha256hex([]byte("what-you-expected"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Tariboy-Digest", bogus)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tampered)
	}))
	defer srv.Close()

	cl, err := NewClient(srv.URL, "rw", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "demo", Tag: "latest"}

	dst := t.TempDir()
	_, err = Pull(dst, ref, cl)
	if err == nil {
		t.Fatal("Pull must reject a digest mismatch, got nil error")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected a digest-mismatch error, got %v", err)
	}

	// Nothing must be installed: no archive, no sidecar, and the refDir must
	// hold no leftover temp files.
	if _, err := os.Stat(filepath.Join(dst, "demo", "latest.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("rejected pull must NOT install the archive (stat err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "demo", "latest.digest")); !os.IsNotExist(err) {
		t.Fatalf("rejected pull must NOT write a sidecar (stat err=%v)", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dst, "demo"))
	for _, e := range entries {
		t.Fatalf("rejected pull left a leftover file: %s", e.Name())
	}

	// And it must not be retrievable from the local store.
	if (&image.Store{Dir: dst}).Exists(ref) {
		t.Fatal("rejected pull must not be locally retrievable")
	}
}

func TestPullRejectsInvalidArchiveWithoutReplacingExisting(t *testing.T) {
	bad := []byte("not an image archive")
	digest := sha256hex(bad)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tariboy-Digest", digest)
		_, _ = w.Write(bad)
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL, "rw", "")
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	ref, _ := buildImage(t, dst)
	archive := filepath.Join(dst, ref.Name, ref.Tag+".tar.gz")
	want, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(dst, ref, client); err == nil {
		t.Fatal("correct-digest invalid archive was installed")
	}
	got, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("invalid pull replaced the existing image")
	}
}

// TestPullRejectsMissingDigest is the fail-closed counterpart to
// TestPullRejectsDigestMismatch: when the store OMITS the
// X-Tariboy-Digest response header entirely, Pull must NOT fall back to
// "skip verification" — it must refuse to install unverified bytes, exactly
// as if the digest had mismatched.
func TestPullRejectsMissingDigest(t *testing.T) {
	body := []byte("valid-looking-archive-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Deliberately do NOT set X-Tariboy-Digest.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cl, err := NewClient(srv.URL, "rw", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "demo", Tag: "latest"}

	dst := t.TempDir()
	_, err = Pull(dst, ref, cl)
	if err == nil {
		t.Fatal("Pull must reject a missing digest header, got nil error")
	}
	if !strings.Contains(err.Error(), "no digest") {
		t.Fatalf("expected a missing-digest error, got %v", err)
	}

	// Nothing must be installed: no archive, no sidecar, and the refDir must
	// hold no leftover temp files.
	if _, err := os.Stat(filepath.Join(dst, "demo", "latest.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("rejected pull must NOT install the archive (stat err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "demo", "latest.digest")); !os.IsNotExist(err) {
		t.Fatalf("rejected pull must NOT write a sidecar (stat err=%v)", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dst, "demo"))
	for _, e := range entries {
		t.Fatalf("rejected pull left a leftover file: %s", e.Name())
	}

	// And it must not be retrievable from the local store.
	if (&image.Store{Dir: dst}).Exists(ref) {
		t.Fatal("rejected pull must not be locally retrievable")
	}
}
