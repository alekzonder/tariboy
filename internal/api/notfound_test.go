package api

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

// The daemon no longer serves a UI: every non-/api/ path is a JSON 404, exactly
// like an unknown /api/ route. This is the contract the desktop app relies on —
// it owns the SPA, the daemon owns the API.
func TestNonAPIPathsReturnJSON404(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(registry.Command{
		Path: "daemon.status", Summary: "status",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) { return map[string]any{"ok": 1}, nil },
	})
	srv := NewServer(reg, &registry.Ctx{Version: "test", Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	h := srv.Handler()

	for _, path := range []string{"/", "/agent/foo", "/index.html", "/assets/app.js"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		b, _ := io.ReadAll(rr.Result().Body)
		if rr.Code != 404 || !strings.Contains(string(b), `"not_found"`) {
			t.Fatalf("GET %s = %d %q, want 404 JSON envelope", path, rr.Code, b)
		}
	}

	// Registered API routes are unaffected.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/daemon/status", nil))
	if rr.Code != 200 {
		t.Fatalf("api route status = %d, want 200", rr.Code)
	}
}
