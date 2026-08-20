package storeui

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// firstAsset returns the path (relative to dist/) of an arbitrary built asset,
// so tests don't hardcode a specific hashed filename.
func firstAsset(t *testing.T) string {
	t.Helper()
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	var found string
	err = fs.WalkDir(sub, "assets", func(p string, d fs.DirEntry, err error) error {
		if err == nil && d != nil && !d.IsDir() && found == "" {
			found = p
		}
		return nil
	})
	if err != nil || found == "" {
		t.Fatalf("no built asset found in dist/assets: %v", err)
	}
	return found
}

func handler(t *testing.T) http.Handler {
	t.Helper()
	h, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	return h
}

func do(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	res := rr.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestServesIndex(t *testing.T) {
	h := handler(t)
	res, body := do(t, h, "/")
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(body, `id="root"`) {
		t.Fatalf("index body missing root div: %q", body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
}

func TestServesAsset(t *testing.T) {
	h := handler(t)
	asset := firstAsset(t)
	res, body := do(t, h, "/"+asset)
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if strings.Contains(body, `id="root"`) {
		t.Fatalf("asset request fell back to index.html: %q", asset)
	}
	if len(body) == 0 {
		t.Fatalf("asset body empty: %q", asset)
	}
}

func TestSPAFallback(t *testing.T) {
	h := handler(t)
	res, body := do(t, h, "/repo/demo")
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(body, `id="root"`) {
		t.Fatalf("fallback did not serve index: %q", body)
	}
}

func TestTraversalConfined(t *testing.T) {
	h := handler(t)
	res, body := do(t, h, "/../../etc/passwd")
	if res.StatusCode != 200 || strings.Contains(body, "root:") {
		t.Fatalf("traversal leaked or errored: status=%d body=%q", res.StatusCode, body)
	}
}
