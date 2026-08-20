package storesvc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func uiServer(t *testing.T, anonPull bool) *Server {
	t.Helper()
	dir := t.TempDir()
	s, err := New(Config{
		AllowInsecure: true, AnonPull: anonPull, Version: "9.9-test",
		DataDir: dir, DBPath: filepath.Join(dir, "store.db"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.SetUI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<div id="root">store ui</div>`))
	}))
	s.SeedToken("rw", ScopeReadWrite)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUIServedAndAPIStillGated(t *testing.T) {
	s := uiServer(t, false) // anon-pull OFF: /v1 reads require a token
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Non-/v1 path → the public SPA (no auth).
	r, _ := http.Get(ts.URL + "/repo/demo")
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !contains(string(b), `id="root"`) {
		t.Fatalf("SPA route = %d %q", r.StatusCode, b)
	}

	// Root "/" also serves the public SPA (no auth).
	rr, _ := http.Get(ts.URL + "/")
	br, _ := io.ReadAll(rr.Body)
	if rr.StatusCode != 200 || !contains(string(br), `id="root"`) {
		t.Fatalf("GET / = %d %q", rr.StatusCode, br)
	}

	// /v1/info is PUBLIC even with anon-pull off (login needs it pre-auth).
	ri, _ := http.Get(ts.URL + "/v1/info")
	if ri.StatusCode != 200 {
		t.Fatalf("/v1/info = %d, want 200 (public)", ri.StatusCode)
	}
	var env struct {
		Result struct {
			Version  string `json:"version"`
			AnonPull bool   `json:"anon_pull"`
		} `json:"result"`
	}
	json.NewDecoder(ri.Body).Decode(&env)
	if env.Result.Version != "9.9-test" || env.Result.AnonPull != false {
		t.Fatalf("/v1/info result = %+v", env.Result)
	}

	// /v1/images STILL requires a token (anon-pull off) — the SPA is public, the
	// API is not.
	ra, _ := http.Get(ts.URL + "/v1/images")
	if ra.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /v1/images (no token) = %d, want 401", ra.StatusCode)
	}
}

// TestUIInfoLeaksOnlyVersionAndAnonPull asserts the PUBLIC /v1/info endpoint
// exposes exactly {version, anon_pull} inside the ok envelope — no token, no
// internal paths, no catalog data.
func TestUIInfoLeaksOnlyVersionAndAnonPull(t *testing.T) {
	s := uiServer(t, false)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ri, _ := http.Get(ts.URL + "/v1/info")
	if ri.StatusCode != 200 {
		t.Fatalf("/v1/info = %d, want 200", ri.StatusCode)
	}
	var env struct {
		OK     bool                   `json:"ok"`
		Result map[string]interface{} `json:"result"`
	}
	if err := json.NewDecoder(ri.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK {
		t.Fatalf("/v1/info ok=false: %+v", env)
	}
	if len(env.Result) != 2 {
		t.Fatalf("/v1/info result should carry exactly {version,anon_pull}, got %d keys: %+v", len(env.Result), env.Result)
	}
	if _, ok := env.Result["version"]; !ok {
		t.Fatalf("/v1/info result missing version: %+v", env.Result)
	}
	if _, ok := env.Result["anon_pull"]; !ok {
		t.Fatalf("/v1/info result missing anon_pull: %+v", env.Result)
	}
}

// TestUIUnauthMutationStill401 is the load-bearing security assertion: adding a
// public SPA + public /v1/info must NOT leave any mutating API route reachable
// without a readwrite bearer. An unauthenticated PUT must be 401.
func TestUIUnauthMutationStill401(t *testing.T) {
	for _, anon := range []bool{false, true} {
		s := uiServer(t, anon)
		ts := httptest.NewServer(s.Handler())

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/x/latest", strings.NewReader("body"))
		req.Header.Set("X-Tariboy-Digest", "sha256:deadbeef")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ts.Close()
			t.Fatalf("PUT: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			ts.Close()
			t.Fatalf("unauth PUT /v1/images/x/latest (anon=%v) = %d, want 401 (mutations are never anon)", anon, resp.StatusCode)
		}
		ts.Close()
	}
}

// TestUIAnonPullReadsButNotWrites asserts that with --anon-pull ON, reads
// succeed without a token but a PUT is STILL 401 — anon-pull relaxes reads only.
func TestUIAnonPullReadsButNotWrites(t *testing.T) {
	s := uiServer(t, true) // anon-pull ON
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Catalog read succeeds with no token under anon-pull.
	ra, _ := http.Get(ts.URL + "/v1/images")
	if ra.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/images (anon-pull, no token) = %d, want 200", ra.StatusCode)
	}
	ra.Body.Close()

	// A mutation is still gated.
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/x/latest", strings.NewReader("body"))
	req.Header.Set("X-Tariboy-Digest", "sha256:deadbeef")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth PUT under anon-pull = %d, want 401", resp.StatusCode)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
