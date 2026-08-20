package storesvc

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/storeui"
)

func TestStoreUIE2EOverTLS(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{
		AllowInsecure: true, // TLS is provided by httptest.NewTLSServer below
		AnonPull:      true, // reads open; writes still need a readwrite token
		Version:       "e2e",
		DataDir:       dir, DBPath: filepath.Join(dir, "store.db"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	ui, err := storeui.Handler()
	if err != nil {
		t.Fatalf("storeui.Handler: %v", err)
	}
	s.SetUI(ui)
	s.SeedToken("rw", ScopeReadWrite)

	ts := httptest.NewTLSServer(s.Handler()) // real TLS, self-signed
	defer ts.Close()
	client := ts.Client() // trusts the server's cert (NO InsecureSkipVerify)

	// 1) The SPA is served (publicly) over TLS at /.
	r, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), `id="root"`) {
		t.Fatalf("GET / = %d, body=%q", r.StatusCode, b)
	}

	// 2) /v1/info is public and reports the version + anon_pull.
	ri, _ := client.Get(ts.URL + "/v1/info")
	var infoEnv struct {
		Result struct {
			Version  string `json:"version"`
			AnonPull bool   `json:"anon_pull"`
		} `json:"result"`
	}
	json.NewDecoder(ri.Body).Decode(&infoEnv)
	if infoEnv.Result.Version != "e2e" || !infoEnv.Result.AnonPull {
		t.Fatalf("/v1/info = %+v", infoEnv.Result)
	}

	// 3) Push a real image (readwrite token) — buildArchive is from manifest_test.go.
	m := image.Manifest{
		SchemaVersion: 1, Name: "demo", Tag: "latest", BuiltAt: "2026-07-06T00:00:00Z",
		Parents: []string{}, Plugins: []image.ManifestPlugin{{Name: "status"}},
		RequiresSecrets: []string{}, Harness: image.ManifestHarness{Type: "claude"},
		Env: map[string]string{}, Evals: []image.ManifestEval{}, Layers: []image.Layer{},
	}
	blob, digest := buildArchive(t, m)
	put, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/images/demo/latest", bytes.NewReader(blob))
	put.Header.Set("X-Tariboy-Digest", digest)
	put.Header.Set("Authorization", "Bearer rw")
	pr, err := client.Do(put)
	if err != nil || pr.StatusCode != http.StatusOK {
		body := ""
		if pr != nil {
			bb, _ := io.ReadAll(pr.Body)
			body = string(bb)
		}
		t.Fatalf("PUT: err=%v status=%v body=%s", err, pr, body)
	}

	// 4) Catalog lists the repo (public read under anon-pull).
	rc, _ := client.Get(ts.URL + "/v1/images")
	var catEnv struct {
		Result struct {
			Repos []struct {
				Name string   `json:"name"`
				Tags []string `json:"tags"`
			} `json:"repos"`
		} `json:"result"`
	}
	json.NewDecoder(rc.Body).Decode(&catEnv)
	if len(catEnv.Result.Repos) != 1 || catEnv.Result.Repos[0].Name != "demo" {
		t.Fatalf("catalog = %+v", catEnv.Result.Repos)
	}

	// 5) Tags carry the digest history.
	rt, _ := client.Get(ts.URL + "/v1/images/demo/tags")
	var tagEnv struct {
		Result struct {
			Tags []PushRow `json:"tags"`
		} `json:"result"`
	}
	json.NewDecoder(rt.Body).Decode(&tagEnv)
	if len(tagEnv.Result.Tags) != 1 || tagEnv.Result.Tags[0].Digest != digest {
		t.Fatalf("tags = %+v", tagEnv.Result.Tags)
	}

	// 6) Manifest endpoint surfaces the parsed plugins/harness.
	rm, _ := client.Get(ts.URL + "/v1/images/demo/latest/manifest")
	var manEnv struct {
		Result image.Manifest `json:"result"`
	}
	json.NewDecoder(rm.Body).Decode(&manEnv)
	if manEnv.Result.Harness.Type != "claude" || len(manEnv.Result.Plugins) != 1 {
		t.Fatalf("manifest = %+v", manEnv.Result)
	}
}
