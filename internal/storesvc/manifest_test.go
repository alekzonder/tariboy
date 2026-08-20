package storesvc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
)

// buildArchive produces a gzip(tar{manifest.json}) blob + its sha256 hex, a valid
// minimal image archive that Repo.Inspect (readFileFromTar "manifest.json") reads.
// Shared with the Task 10 e2e (same package).
func buildArchive(t *testing.T, m image.Manifest) ([]byte, string) {
	t.Helper()
	mj, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o600, Size: int64(len(mj))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(mj); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	b := buf.Bytes()
	s := sha256.Sum256(b)
	return b, hex.EncodeToString(s[:])
}

func TestManifestEndpoint(t *testing.T) {
	s := newTestServer(t) // AnonPull=true, rw token "rw" (from server_test.go)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	m := image.Manifest{
		SchemaVersion:   1,
		Name:            "demo",
		Tag:             "latest",
		BuiltAt:         "2026-07-06T00:00:00Z",
		Parents:         []string{},
		Plugins:         []image.ManifestPlugin{{Name: "status", Version: ">=1.0"}},
		RequiresSecrets: []string{"OPENAI_API_KEY"},
		Harness:         image.ManifestHarness{Type: "claude", Model: "opus", Interactive: false},
		Env:             map[string]string{},
		Evals:           []image.ManifestEval{{Name: "smoke", Type: "prompt", Prompt: "hi"}},
		Layers:          []image.Layer{},
	}
	blob, digest := buildArchive(t, m)

	// Push the real archive (rw token satisfies the write scope).
	put, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/demo/latest", bytes.NewReader(blob))
	put.Header.Set("X-Tariboy-Digest", digest)
	put.Header.Set("Authorization", "Bearer rw")
	pr, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	if pr.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(pr.Body)
		t.Fatalf("PUT = %d (%s)", pr.StatusCode, b)
	}

	// Fetch the parsed manifest (GET is open under AnonPull).
	r, err := http.Get(ts.URL + "/v1/images/demo/latest/manifest")
	if err != nil {
		t.Fatalf("GET manifest: %v", err)
	}
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("GET manifest = %d (%s)", r.StatusCode, b)
	}
	var env struct {
		OK     bool           `json:"ok"`
		Result image.Manifest `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK || env.Result.Name != "demo" || len(env.Result.Plugins) != 1 || env.Result.Plugins[0].Name != "status" {
		t.Fatalf("manifest = %+v", env.Result)
	}
	if len(env.Result.RequiresSecrets) != 1 || env.Result.Harness.Type != "claude" {
		t.Fatalf("manifest details lost: %+v", env.Result)
	}
	if env.Result.Digest != digest {
		t.Fatalf("digest = %q, want %q", env.Result.Digest, digest)
	}

	// A missing tag → 404.
	r2, _ := http.Get(ts.URL + "/v1/images/demo/nope/manifest")
	if r2.StatusCode != http.StatusNotFound {
		t.Fatalf("missing manifest = %d, want 404", r2.StatusCode)
	}
}
