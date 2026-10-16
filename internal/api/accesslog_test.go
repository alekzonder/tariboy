package api

import (
	"bytes"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
)

// Every request goes to the access log; the application log gets only slow or
// failed requests, so polling does not flood tariboyd.log.
func TestAccessLogSplitsRequestsFromApplicationLog(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(registry.Command{
		Path: "fast", Summary: "fast",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/fast"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) { return 1, nil },
	})
	_ = reg.Register(registry.Command{
		Path: "slow", Summary: "slow",
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/slow"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			time.Sleep(30 * time.Millisecond)
			return 1, nil
		},
	})
	old := slowRequest
	slowRequest = 20 * time.Millisecond
	defer func() { slowRequest = old }()

	var app, access bytes.Buffer
	srv := NewServer(reg, &registry.Ctx{Version: "test", Log: slog.New(slog.NewTextHandler(&app, nil))})
	srv.SetAccessLog(slog.New(slog.NewTextHandler(&access, nil)))
	h := srv.Handler()
	for _, path := range []string{"/api/fast", "/api/slow", "/api/missing"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		_, _ = io.ReadAll(rr.Result().Body)
	}

	for _, path := range []string{"/api/fast", "/api/slow", "/api/missing"} {
		if !strings.Contains(access.String(), "path="+path) {
			t.Fatalf("access log missing %s:\n%s", path, access.String())
		}
	}
	if strings.Contains(app.String(), "/api/fast") || strings.Contains(app.String(), "/api/missing") {
		t.Fatalf("application log has a fast request:\n%s", app.String())
	}
	if !strings.Contains(app.String(), "level=WARN") || !strings.Contains(app.String(), "path=/api/slow") {
		t.Fatalf("application log misses the slow request:\n%s", app.String())
	}
}
