package aiproxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxyEmitsMetadataOnly(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"claude-opus-4-8","usage":{"input_tokens":100,"output_tokens":50,
			"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`))
	}))
	defer up.Close()
	t.Setenv("FAKE_KEY", "real")

	s := newStore(t)
	router := NewRouter()
	router.SetDefault("anthropic", Upstream{BaseURL: up.URL, KeyEnv: "FAKE_KEY"})
	var gotAgent string
	var gotData map[string]any
	p := New(Config{
		Tokens: NewTokenRegistry(nil), Pricing: &Pricing{table: DefaultPricing()}, Store: s,
		Router: router, AgentsDir: t.TempDir(),
		Clock: func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Emit:  func(agent string, data map[string]any) { gotAgent = agent; gotData = data },
	})
	// Ensure the iteration dir exists so the transcript append succeeds.
	tok, _ := p.Mint(Attribution{Agent: "alice", Iteration: "alice-1", ImageName: "basic", ImageTag: "latest"})
	mkIter(t, p.cfg.AgentsDir, "alice", "alice-1")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(`{"model":"claude-opus-4-8"}`))
	req.Header.Set("x-api-key", "real-provider-key")
	p.ServeHTTP(rr, req)

	if gotAgent != "alice" {
		t.Fatalf("emit agent = %q", gotAgent)
	}
	if gotData["model"] != "claude-opus-4-8" || gotData["input_tokens"].(int) != 100 || gotData["status"] != "ok" {
		t.Fatalf("emit data = %+v", gotData)
	}
	if gotData["request_id"] == "" || gotData["iteration_id"] != "alice-1" {
		t.Fatalf("emit attribution = %+v", gotData)
	}
	// Must NOT carry bodies.
	for k := range gotData {
		if k == "request" || k == "response" || k == "body" {
			t.Fatalf("proxy event leaked a body field %q", k)
		}
	}
}
