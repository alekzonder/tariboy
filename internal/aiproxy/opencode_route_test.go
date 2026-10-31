package aiproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProviderRouteRestoredFromHandoff(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/coding/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer provider-key" {
			t.Errorf("upstream path=%q authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		options, _ := body["stream_options"].(map[string]any)
		if options["include_usage"] != true {
			t.Errorf("stream options = %#v", options)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"model\":\"k3\",\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":40,\"prompt_tokens_details\":{\"cached_tokens\":25}}}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "handoff.json")
	state := fmt.Sprintf(`{"tokens":{"token":{"Agent":"alice","Iteration":"iteration","Routes":{"kimi":{"BaseURL":%q,"Provider":"openai"}}}}}`, upstream.URL+"/coding/v1")
	if err := os.WriteFile(path, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := OpenTokenRegistry(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := testProxy(t)
	p.cfg.Tokens = registry
	p.cfg.Router = NewRouter()
	p.cfg.Router.SetDefault("anthropic", Upstream{BaseURL: upstream.URL})
	p.cfg.Pricing = &Pricing{table: map[string]ModelPrice{"k3": {InputPerMtok: 1_000_000, OutputPerMtok: 2_000_000, CacheReadPerMtok: 3_000_000}}}
	var rows []AIRequest
	p.cfg.Ingest = func(row AIRequest) { rows = append(rows, row) }
	r := httptest.NewRequest(http.MethodPost, "/_tariboy/token/_opencode/kimi/chat/completions", strings.NewReader(`{"model":"k3","stream":true}`))
	r.Header.Set("Authorization", "Bearer provider-key")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(rows) != 1 || rows[0].Model != "k3" || rows[0].InputTokens != 75 || rows[0].OutputTokens != 40 || rows[0].CacheReadTokens != 25 || rows[0].CostUSD != 230 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestProviderLeaseCannotUseUnboundRoutes(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	p := testProxy(t)
	p.cfg.Router = NewRouter()
	p.cfg.Router.SetDefault("openai", Upstream{BaseURL: upstream.URL})
	p.cfg.Router.SetDefault("anthropic", Upstream{BaseURL: upstream.URL})
	token, err := p.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.RegisterProviderRoutes(token, map[string]ProviderRoute{"kimi": {BaseURL: upstream.URL, Provider: "openai"}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/messages", "/_opencode/other/chat/completions", "/_opencode/kimi/../chat/completions", "/_opencode/kimi/%2e%2e/chat/completions"} {
		r := httptest.NewRequest(http.MethodPost, "/_tariboy/"+token+path, strings.NewReader(`{"model":"k3"}`))
		r.Header.Set("Authorization", "Bearer provider-key")
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code < 400 {
			t.Errorf("path %q status %d", path, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("unbound requests reached upstream %d times", calls.Load())
	}
}

func TestInvalidRestoredProviderRouteFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handoff.json")
	if err := os.WriteFile(path, []byte(`{"tokens":{"token":{"Routes":{"x":{"BaseURL":"https://secret@example.com/v1","Provider":"openai"}}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenTokenRegistry(path, nil); err == nil {
		t.Fatal("accepted route containing credentials")
	}
	if err := os.WriteFile(path, []byte(`{"tokens":{"token":{"Routes":{}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenTokenRegistry(path, nil); err == nil {
		t.Fatal("accepted empty provider route set")
	}
}

func TestProviderStreamWithoutResponseModelUsesRequestModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2}}\n\n")
	}))
	defer upstream.Close()
	p := testProxy(t)
	p.cfg.Router = NewRouter()
	var rows []AIRequest
	p.cfg.Ingest = func(row AIRequest) { rows = append(rows, row) }
	token, err := p.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.RegisterProviderRoutes(token, map[string]ProviderRoute{"kimi": {BaseURL: upstream.URL, Provider: "openai"}}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/_tariboy/"+token+"/_opencode/kimi/chat/completions", strings.NewReader(`{"model":"k3","stream":true}`))
	r.Header.Set("Authorization", "Bearer provider-key")
	p.ServeHTTP(httptest.NewRecorder(), r)
	if len(rows) != 1 || rows[0].Model != "k3" || rows[0].InputTokens != 5 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestProviderRoutesCannotBeMutatedAfterRegistration(t *testing.T) {
	reg := NewTokenRegistry(nil)
	token, err := reg.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]ProviderRoute{"kimi": {BaseURL: "https://example.com/v1", Provider: "openai"}}
	if err := reg.RegisterProviderRoutes(token, routes); err != nil {
		t.Fatal(err)
	}
	routes["kimi"] = ProviderRoute{BaseURL: "https://attacker.example", Provider: "openai"}
	a, _ := reg.Resolve(token)
	a.Routes["kimi"] = ProviderRoute{BaseURL: "https://attacker.example", Provider: "openai"}
	b, _ := reg.Resolve(token)
	if b.Routes["kimi"].BaseURL != "https://example.com/v1" {
		t.Fatal("route mutated outside registry")
	}
}

func TestProviderRouteNeverForwardsAttributionToken(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	p := testProxy(t)
	p.cfg.Router = NewRouter()
	token, _ := p.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	if err := p.RegisterProviderRoutes(token, map[string]ProviderRoute{"kimi": {BaseURL: upstream.URL, Provider: "openai"}}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/_tariboy/"+token+"/_opencode/kimi/chat/completions", strings.NewReader(`{"model":"k3"}`))
	r.Header.Add("Authorization", "Bearer provider-key")
	r.Header.Add("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code < 400 || calls.Load() != 0 {
		t.Fatalf("status %d, calls %d", w.Code, calls.Load())
	}
	r = httptest.NewRequest(http.MethodPost, "/_tariboy/"+token+"/_opencode/kimi/chat/completions", strings.NewReader(`{"model":"k3"}`))
	r.Header.Set("x-provider-key", token)
	w = httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code < 400 || calls.Load() != 0 {
		t.Fatalf("custom header status %d, calls %d", w.Code, calls.Load())
	}
}

func TestProviderTruncatedStreamRecordsUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: 1000\r\n\r\ndata: {}\n\n")
	}))
	defer upstream.Close()
	p := testProxy(t)
	p.cfg.Router = NewRouter()
	var rows []AIRequest
	p.cfg.Ingest = func(row AIRequest) { rows = append(rows, row) }
	token, _ := p.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	if err := p.RegisterProviderRoutes(token, map[string]ProviderRoute{"kimi": {BaseURL: upstream.URL, Provider: "openai"}}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/_tariboy/"+token+"/_opencode/kimi/chat/completions", strings.NewReader(`{"model":"k3","stream":true}`))
	r.Header.Set("Authorization", "Bearer provider-key")
	p.ServeHTTP(httptest.NewRecorder(), r)
	if len(rows) != 1 || rows[0].Status != "upstream_error" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRegisteredRoutesPersistAndRevokeWithLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handoff.json")
	reg, err := OpenTokenRegistry(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := reg.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterProviderRoutes(token, map[string]ProviderRoute{"kimi": {BaseURL: "https://example.com/v1", Provider: "openai"}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenTokenRegistry(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := reopened.Resolve(token)
	if !ok || a.Routes["kimi"].BaseURL != "https://example.com/v1" {
		t.Fatalf("restored = %+v, %v", a, ok)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("handoff permissions: %v, %v", info, err)
	}
	if err := reopened.Revoke(token); err != nil {
		t.Fatal(err)
	}
	again, err := OpenTokenRegistry(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Resolve(token); ok {
		t.Fatal("revoked lease restored")
	}
}

func TestRegisterProviderRoutesRejectsInvalidTargets(t *testing.T) {
	reg := NewTokenRegistry(nil)
	token, _ := reg.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	for _, route := range []ProviderRoute{
		{BaseURL: "https://user:pass@example.com", Provider: "openai"},
		{BaseURL: "https://example.com/v1/../private", Provider: "openai"},
		{BaseURL: "https://example.com/v1?key=secret", Provider: "openai"},
		{BaseURL: "file:///etc/passwd", Provider: "openai"},
		{BaseURL: "https://example.com", Provider: "unsupported"},
	} {
		if err := reg.RegisterProviderRoutes(token, map[string]ProviderRoute{"p": route}); err == nil {
			t.Errorf("accepted invalid route %+v", route)
		}
	}
	if err := reg.RegisterProviderRoutes("missing", map[string]ProviderRoute{"p": {BaseURL: "https://example.com", Provider: "openai"}}); err == nil {
		t.Fatal("accepted missing lease")
	}
}

func TestProviderRoutePreservesCustomAuthorization(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-provider-key") != "provider-secret" {
			t.Error("provider credential missing")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	p := testProxy(t)
	p.cfg.Router = NewRouter()
	token, _ := p.Mint(Attribution{Agent: "alice", Iteration: "iteration"})
	if err := p.RegisterProviderRoutes(token, map[string]ProviderRoute{"custom": {BaseURL: upstream.URL, Provider: "openai"}}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/_tariboy/"+token+"/_opencode/custom/chat/completions", strings.NewReader(`{"model":"model"}`))
	r.Header.Set("x-provider-key", "provider-secret")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
}
