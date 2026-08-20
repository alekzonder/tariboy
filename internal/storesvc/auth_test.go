package storesvc

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func authServer(t *testing.T, anonPull bool) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(Config{AllowInsecure: true, AnonPull: anonPull, DataDir: dir, DBPath: filepath.Join(dir, "store.db")})
	if err != nil {
		t.Fatal(err)
	}
	s.SeedToken("ro-token", ScopeRead)
	s.SeedToken("rw-token", ScopeReadWrite)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close() })
	return s, ts
}

func put(t *testing.T, url, token string) int {
	t.Helper()
	blob := []byte("bytes")
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(blob))
	req.Header.Set("X-Tariboy-Digest", sha256hex(blob))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return r.StatusCode
}

func TestAuthMatrix(t *testing.T) {
	_, ts := authServer(t, false)
	u := ts.URL + "/v1/images/demo/latest"

	if got := put(t, u, ""); got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated PUT = %d, want 401", got)
	}
	if got := put(t, u, "ro-token"); got != http.StatusForbidden {
		t.Fatalf("read-scope PUT = %d, want 403", got)
	}
	if got := put(t, u, "rw-token"); got != http.StatusOK {
		t.Fatalf("readwrite PUT = %d, want 200", got)
	}
	// Reads require a token when anon-pull is off.
	req, _ := http.NewRequest(http.MethodHead, u, nil)
	r, _ := http.DefaultClient.Do(req)
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated HEAD (no anon) = %d, want 401", r.StatusCode)
	}
}

func TestAnonPullAllowsReads(t *testing.T) {
	_, ts := authServer(t, true)
	u := ts.URL + "/v1/images/demo/latest"
	// A read with no token is allowed under --anon-pull...
	r, _ := http.Head(u)
	if r.StatusCode != http.StatusNotFound { // 404 (absent), NOT 401
		t.Fatalf("anon HEAD = %d, want 404", r.StatusCode)
	}
	// ...but a write still requires a readwrite token.
	if got := put(t, u, ""); got != http.StatusUnauthorized {
		t.Fatalf("anon PUT = %d, want 401", got)
	}
}

// TestUnknownScopeDenies is the Task-3 carry-forward hardening: SeedToken does no
// scope-value validation, so a typo'd/unrecognized stored scope must fail closed —
// it grants NEITHER read nor write (403), never silently passing as read access.
func TestUnknownScopeDenies(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{AllowInsecure: true, DataDir: dir, DBPath: filepath.Join(dir, "store.db")})
	if err != nil {
		t.Fatal(err)
	}
	// A stored scope that is neither "read" nor "readwrite" (e.g. a typo).
	s.SeedToken("bogus-token", "reed")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); s.Close() })
	u := ts.URL + "/v1/images/demo/latest"

	// A safe read with the bogus-scope token must be denied (403), not passed.
	req, _ := http.NewRequest(http.MethodHead, u, nil)
	req.Header.Set("Authorization", "Bearer bogus-token")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusForbidden {
		t.Fatalf("unknown-scope HEAD = %d, want 403", r.StatusCode)
	}
	// And it certainly must not authorize a mutation.
	if got := put(t, u, "bogus-token"); got != http.StatusForbidden {
		t.Fatalf("unknown-scope PUT = %d, want 403", got)
	}
}
