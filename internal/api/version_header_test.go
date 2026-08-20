package api

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/version"
)

// Every operator-API response — success, user error and 404 alike — carries the
// daemon's build version, so a client of any age can notice it is talking to a
// daemon it does not match (SUPER-224 §4).
func TestOperatorAPIStampsVersionHeader(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(registry.Command{
		Path: "daemon.status", Summary: "status",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) { return map[string]any{"ok": 1}, nil },
	})
	_ = reg.Register(registry.Command{
		Path: "daemon.boom", Summary: "boom",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/boom"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) { return nil, UserError{Code: "nope", Msg: "no"} },
	})
	srv := NewServer(reg, &registry.Ctx{Version: "test", Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	h := srv.Handler()

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/daemon/status", 200},
		{"/api/daemon/boom", 400},
		{"/api/does-not-exist", 404},
		{"/not-an-api-path", 404},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", tc.path, nil))
		if rr.Code != tc.want {
			t.Fatalf("GET %s = %d, want %d", tc.path, rr.Code, tc.want)
		}
		if got := rr.Result().Header.Get(version.Header); got != version.Version {
			t.Fatalf("GET %s header = %q, want %q", tc.path, got, version.Version)
		}
	}
}
