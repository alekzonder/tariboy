package storesvc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newTestServer builds a store for the functional (non-auth) tests. Auth itself
// is exercised in auth_test.go; here we keep reads open via AnonPull and seed a
// readwrite token ("rw") for the mutating PUTs so those requests satisfy the
// scope gate without obscuring what each test actually verifies.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	s, err := New(Config{AllowInsecure: true, AnonPull: true, DataDir: dir, DBPath: filepath.Join(dir, "store.db")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.SeedToken("rw", ScopeReadWrite)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBlobRoundTripAndDigestGuard(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	blob := []byte("archive-bytes")
	digest := sha256hex(blob)

	// HEAD on a missing blob -> 404.
	resp, _ := http.Head(ts.URL + "/v1/images/demo/latest")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("HEAD missing = %d, want 404", resp.StatusCode)
	}

	// PUT with a wrong claimed digest -> 400 digest_mismatch.
	bad, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/demo/latest", bytes.NewReader(blob))
	bad.Header.Set("X-Tariboy-Digest", sha256hex([]byte("other")))
	bad.Header.Set("Authorization", "Bearer rw")
	r1, _ := http.DefaultClient.Do(bad)
	if r1.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad PUT = %d, want 400", r1.StatusCode)
	}

	// PUT with the correct digest -> 200.
	ok, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/demo/latest", bytes.NewReader(blob))
	ok.Header.Set("X-Tariboy-Digest", digest)
	ok.Header.Set("Authorization", "Bearer rw")
	r2, _ := http.DefaultClient.Do(ok)
	if r2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(r2.Body)
		t.Fatalf("good PUT = %d (%s)", r2.StatusCode, body)
	}

	// HEAD now reports the digest.
	r3, _ := http.Head(ts.URL + "/v1/images/demo/latest")
	if r3.StatusCode != http.StatusOK || r3.Header.Get("X-Tariboy-Digest") != digest {
		t.Fatalf("HEAD present = %d,%q", r3.StatusCode, r3.Header.Get("X-Tariboy-Digest"))
	}

	// GET streams the bytes back with the digest header.
	r4, _ := http.Get(ts.URL + "/v1/images/demo/latest")
	back, _ := io.ReadAll(r4.Body)
	if !bytes.Equal(back, blob) || r4.Header.Get("X-Tariboy-Digest") != digest {
		t.Fatalf("GET body/digest mismatch")
	}
}

func TestRejectsTraversalRef(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	// An uppercase / illegal name never matches ParseRef -> 400, never a path.
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/BAD/latest", bytes.NewReader([]byte("x")))
	req.Header.Set("X-Tariboy-Digest", sha256hex([]byte("x")))
	req.Header.Set("Authorization", "Bearer rw")
	r, _ := http.DefaultClient.Do(req)
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("illegal name = %d, want 400", r.StatusCode)
	}
}

// TestRejectsDotComponentRef is the load-bearing security test beyond the brief:
// image.ParseRef's regex ([a-z0-9._-]+) ACCEPTS bare "." and ".." — a one-level
// path-traversal escape. The store takes name/tag over the network from
// authenticated-but-untrusted pushers, so parseRef must additionally reject pure
// dot components. We drive handlePut directly with crafted PathValues (bypassing
// net/http's path cleaning) to prove the guard is at the ref-validation layer,
// and assert no blob escaped the data dir.
func TestRejectsDotComponentRef(t *testing.T) {
	s := newTestServer(t)
	blob := []byte("x")
	digest := sha256hex(blob)
	cases := []struct{ name, tag string }{
		{"..", "latest"},
		{"demo", ".."},
		{".", "latest"},
		{"demo", "."},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPut, "/v1/images/x/y", bytes.NewReader(blob))
		req.SetPathValue("name", tc.name)
		req.SetPathValue("tag", tc.tag)
		req.Header.Set("X-Tariboy-Digest", digest)
		rec := httptest.NewRecorder()
		s.handlePut(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("PUT name=%q tag=%q = %d, want 400", tc.name, tc.tag, rec.Code)
		}
	}
	// Defense-in-depth: a ".." name would have written a sibling of DataDir.
	escaped := filepath.Join(filepath.Dir(s.cfg.DataDir), "latest.tar.gz")
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		t.Fatalf("traversal escaped data dir: %s (err=%v)", escaped, err)
	}
}

func TestCatalogAndTags(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	blob := []byte("archive-bytes")
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/demo/latest", bytes.NewReader(blob))
	req.Header.Set("X-Tariboy-Digest", sha256hex(blob))
	req.Header.Set("Authorization", "Bearer rw")
	http.DefaultClient.Do(req)

	r, _ := http.Get(ts.URL + "/v1/images/demo/tags")
	var env struct {
		Result struct {
			Tags []PushRow `json:"tags"`
		} `json:"result"`
	}
	json.NewDecoder(r.Body).Decode(&env)
	if len(env.Result.Tags) != 1 || env.Result.Tags[0].Digest != sha256hex(blob) {
		t.Fatalf("tags = %+v", env.Result.Tags)
	}
}
