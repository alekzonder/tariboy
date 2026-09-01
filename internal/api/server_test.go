package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/registry"
)

func testServer(t *testing.T) (*Server, *http.Client) {
	t.Helper()
	reg := registry.New()
	reg.Register(registry.Command{
		Path: "daemon.status", Summary: "status",
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/daemon/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			return map[string]any{"version": c.Version}, nil
		},
	})
	reg.Register(registry.Command{
		Path: "wild", Summary: "wildcard capture",
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/wild/{ref}/files/{path...}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			return map[string]any{"ref": p["ref"], "path": p["path"]}, nil
		},
	})
	reg.Register(registry.Command{
		Path: "echo", Summary: "echo",
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/echo"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			if p["boom"] == true {
				return nil, UserError{Code: "boom", Msg: "boom requested"}
			}
			if p["crash"] == true {
				return nil, errors.New("internal detail")
			}
			return p, nil
		},
	})
	cctx := &registry.Ctx{Version: "test", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now()}
	srv := NewServer(reg, cctx)

	sock := filepath.Join(t.TempDir(), "d.sock")
	go srv.ServeUnix(sock)
	t.Cleanup(func() { srv.Shutdown(context.Background()) })

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", sock)
		},
	}}
	// wait for socket
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Get("http://unix/api/help.json"); err == nil {
			return srv, client
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return nil, nil
}

func get(t *testing.T, c *http.Client, url string) (int, map[string]any) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, m
}

func TestTrailingWildcardRoute(t *testing.T) {
	_, c := testServer(t)
	code, m := get(t, c, "http://unix/api/wild/demo:latest/files/skills/my/helper.md")
	if code != 200 || m["ok"] != true {
		t.Fatalf("code=%d body=%v", code, m)
	}
	res := m["result"].(map[string]any)
	if res["ref"] != "demo:latest" || res["path"] != "skills/my/helper.md" {
		t.Fatalf("wildcard capture wrong: %v", res)
	}
}

func TestStatusRoute(t *testing.T) {
	_, c := testServer(t)
	code, m := get(t, c, "http://unix/api/daemon/status")
	if code != 200 || m["ok"] != true {
		t.Fatalf("code=%d body=%v", code, m)
	}
	if m["result"].(map[string]any)["version"] != "test" {
		t.Fatalf("bad result: %v", m)
	}
}

func TestPostParamsAndErrors(t *testing.T) {
	_, c := testServer(t)
	resp, err := c.Post("http://unix/api/echo", "application/json", strings.NewReader(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if m["result"].(map[string]any)["x"] != float64(1) {
		t.Fatalf("echo lost params: %v", m)
	}

	resp, _ = c.Post("http://unix/api/echo", "application/json", strings.NewReader(`{"boom":true}`))
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if resp.StatusCode != 400 || m["error"].(map[string]any)["code"] != "boom" {
		t.Fatalf("user error mapping wrong: %d %v", resp.StatusCode, m)
	}

	resp, _ = c.Post("http://unix/api/echo", "application/json", strings.NewReader(`{"crash":true}`))
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if resp.StatusCode != 500 || m["error"].(map[string]any)["code"] != "internal" {
		t.Fatalf("internal error mapping wrong: %d %v", resp.StatusCode, m)
	}
	if strings.Contains(m["error"].(map[string]any)["message"].(string), "internal detail") {
		t.Fatal("internal error message leaked")
	}
}

func TestUnknownRoute(t *testing.T) {
	_, c := testServer(t)
	code, m := get(t, c, "http://unix/api/nope")
	if code != 404 || m["ok"] != false {
		t.Fatalf("code=%d body=%v", code, m)
	}
}

func TestHelpAndOpenAPI(t *testing.T) {
	_, c := testServer(t)
	code, m := get(t, c, "http://unix/api/help.json")
	if code != 200 || m["result"].(map[string]any)["daemon"] == nil {
		t.Fatalf("help.json: %d %v", code, m)
	}
	code, _ = get(t, c, "http://unix/api/openapi.json")
	if code != 200 {
		t.Fatalf("openapi.json: %d", code)
	}
}

func TestOpenAPIRepeatableArgIsArray(t *testing.T) {
	schema := openAPIArgSchema(registry.Arg{Name: "tag", Type: registry.String, Repeatable: true, Help: "target tags"})
	items, _ := schema["items"].(map[string]any)
	if schema["type"] != "array" || items["type"] != "string" {
		t.Fatalf("repeatable schema = %#v, want string array", schema)
	}
}

func TestDispatchMergesPathValue(t *testing.T) {
	reg := registry.New()
	reg.Register(registry.Command{
		Path: "agent.status", Summary: "s",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/status"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) { return map[string]any{"name": p["name"]}, nil },
	})
	cctx := &registry.Ctx{Version: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := NewServer(reg, cctx)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/agents/smoke/status", nil))
	if !strings.Contains(rr.Body.String(), `"name":"smoke"`) {
		t.Fatalf("path value not merged: %s", rr.Body.String())
	}
}

type fakeEventSource struct{ ch chan events.Event }

func (f *fakeEventSource) Subscribe(string, []string) (<-chan events.Event, func()) {
	return f.ch, func() {}
}

func TestSSEStream(t *testing.T) {
	src := &fakeEventSource{ch: make(chan events.Event, 1)}
	reg := registry.New()
	cctx := &registry.Ctx{Version: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := NewServer(reg, cctx)
	srv.SetEventSource(src)

	req := httptest.NewRequest("GET", "/api/agents/smoke/events?types=message", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() { srv.Handler().ServeHTTP(rr, req); close(done) }()
	src.ch <- events.Event{Agent: "smoke", Type: "message", Data: map[string]any{"id": "m1"}}
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
	if !strings.Contains(rr.Body.String(), "data:") || !strings.Contains(rr.Body.String(), "m1") {
		t.Fatalf("SSE body = %q", rr.Body.String())
	}
}

func TestAccessLogRecordsStatus(t *testing.T) {
	var buf bytes.Buffer
	reg := registry.New()
	cctx := &registry.Ctx{Version: "t", Log: slog.New(slog.NewTextHandler(&buf, nil))}
	srv := NewServer(reg, cctx)

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/nope", nil))
	if !strings.Contains(buf.String(), "status=404") {
		t.Fatalf("access log missing status: %q", buf.String())
	}
}

// The plugin-facing routes must all be mounted on the mux, not just handled by
// the plugin API's internal switch: a handler that exists but is never
// registered here 404s before it runs. GET /api/plugin/watches (the §6.2 pull
// path providers reconcile from at startup) regressed exactly this way.
func TestPluginRoutesMounted(t *testing.T) {
	reg := registry.New()
	cctx := &registry.Ctx{Version: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := NewServer(reg, cctx)
	var seen []string
	srv.SetPluginAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		WriteOK(w, map[string]any{"ok": true})
	}))
	h := srv.Handler()
	cases := []struct{ method, path string }{
		{"POST", "/api/plugin/publish"},
		{"GET", "/api/plugin/subscriptions"},
		{"GET", "/api/plugin/watches"},
	}
	for _, c := range cases {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(c.method, c.path, nil))
		if rr.Code == http.StatusNotFound {
			t.Fatalf("route %s %s not mounted (404) — plugin handler never reached", c.method, c.path)
		}
	}
	if len(seen) != len(cases) {
		t.Fatalf("plugin handler saw %v, want all of %+v", seen, cases)
	}
}
