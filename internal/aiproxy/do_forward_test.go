package aiproxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// recordedUpstream captures what doForward sent and answers with handler.
type recordedUpstream struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
}

func (u *recordedUpstream) server(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.requests = append(u.requests, r)
		u.bodies = append(u.bodies, string(body))
		u.mu.Unlock()
		if handler != nil {
			handler(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (u *recordedUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func (u *recordedUpstream) last(t *testing.T) (*http.Request, string) {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.requests) == 0 {
		t.Fatal("upstream was not called")
	}
	return u.requests[len(u.requests)-1], u.bodies[len(u.bodies)-1]
}

func forwardProxy(t *testing.T, provider, baseURL, keyEnv string) *Proxy {
	t.Helper()
	p := testProxy(t)
	p.cfg.Router = NewRouter()
	p.cfg.Router.SetDefault(provider, Upstream{BaseURL: baseURL, KeyEnv: keyEnv})
	return p
}

func forwardExchange(provider, method, path, body string) (*Exchange, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	return &Exchange{R: r, W: w, Provider: provider, Path: r.URL.Path, ReqBody: []byte(body), Token: "attr-token"}, w
}

// truncatedUpstream promises more bytes than it sends, so the client sees a
// mid-body read error after the listed partial body.
func truncatedUpstream(contentType, partial string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, partial)
	}
}

func TestDoForwardRejectsTokenInRoutedRequestHeaders(t *testing.T) {
	up := &recordedUpstream{}
	srv := up.server(t, nil)
	p := forwardProxy(t, "openai", srv.URL, "")
	ex, w := forwardExchange("openai", http.MethodPost, "/chat/completions", "{}")
	ex.Route = &ProviderRoute{BaseURL: srv.URL, Provider: "openai"}
	ex.R.Header.Set("x-custom", "prefix attr-token suffix")
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	if ex.Status != "auth_error" || w.Code != http.StatusUnauthorized || w.Body.Len() != 0 || up.count() != 0 {
		t.Fatalf("status %q code %d body %q upstream calls %d", ex.Status, w.Code, w.Body.String(), up.count())
	}
}

func TestDoForwardRejectsMissingAuthorizationBeforeUpstream(t *testing.T) {
	up := &recordedUpstream{}
	srv := up.server(t, nil)
	p := forwardProxy(t, "openai", srv.URL, "")
	ex, w := forwardExchange("openai", http.MethodPost, "/v1/chat/completions", "{}")
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	want := `{"type":"error","error":{"type":"authentication_error","message":"missing upstream authorization"}}`
	if ex.Status != "auth_error" || w.Code != http.StatusUnauthorized || w.Body.String() != want || up.count() != 0 {
		t.Fatalf("status %q code %d body %q upstream calls %d", ex.Status, w.Code, w.Body.String(), up.count())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
}

func TestDoForwardTargetAndRequestFailures(t *testing.T) {
	for _, tc := range []struct {
		name, baseURL, method string
	}{
		{name: "unparsable upstream URL", baseURL: "http://bad host", method: http.MethodPost},
		{name: "invalid method", baseURL: "http://127.0.0.1:1", method: "BAD METHOD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := forwardProxy(t, "anthropic", tc.baseURL, "")
			ex, w := forwardExchange("anthropic", http.MethodPost, "/v1/messages", "{}")
			ex.R.Method = tc.method
			if err := p.doForward(ex); err != nil {
				t.Fatal(err)
			}
			if ex.Status != "upstream_error" || w.Code != http.StatusBadGateway || w.Body.Len() != 0 {
				t.Fatalf("status %q code %d body %q", ex.Status, w.Code, w.Body.String())
			}
		})
	}
}

func TestDoForwardReplacesAttributionTokenWithDaemonCredential(t *testing.T) {
	t.Setenv("TEST_FORWARD_KEY", "daemon-key")
	for _, tc := range []struct{ name, header, value string }{
		{name: "bearer", header: "Authorization", value: "Bearer attr-token"},
		{name: "api key", header: "x-api-key", value: "attr-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := &recordedUpstream{}
			srv := up.server(t, nil)
			p := forwardProxy(t, "anthropic", srv.URL, "TEST_FORWARD_KEY")
			ex, _ := forwardExchange("anthropic", http.MethodPost, "/v1/messages", "{}")
			ex.R.Header.Set(tc.header, tc.value)
			if err := p.doForward(ex); err != nil {
				t.Fatal(err)
			}
			r, _ := up.last(t)
			if got := r.Header.Get("Authorization"); got != "Bearer daemon-key" {
				t.Fatalf("Authorization = %q", got)
			}
			if got := r.Header.Get("x-api-key"); got != "" {
				t.Fatalf("x-api-key = %q", got)
			}
		})
	}
}

func TestDoForwardFailsClosedWithoutDaemonCredential(t *testing.T) {
	t.Setenv("TEST_FORWARD_KEY", "")
	up := &recordedUpstream{}
	srv := up.server(t, nil)
	p := forwardProxy(t, "anthropic", srv.URL, "TEST_FORWARD_KEY")
	ex, w := forwardExchange("anthropic", http.MethodPost, "/v1/messages", "{}")
	ex.R.Header.Set("Authorization", "Bearer attr-token")
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	want := `{"type":"error","error":{"type":"api_error","message":"upstream credential unavailable"}}`
	if ex.Status != "upstream_error" || w.Code != http.StatusBadGateway || w.Body.String() != want || up.count() != 0 {
		t.Fatalf("status %q code %d body %q upstream calls %d", ex.Status, w.Code, w.Body.String(), up.count())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	if ex.UpstreamStatus != 0 {
		t.Fatalf("upstream status = %d", ex.UpstreamStatus)
	}
}

func TestDoForwardShapesTheOutgoingRequest(t *testing.T) {
	up := &recordedUpstream{}
	srv := up.server(t, nil)
	p := forwardProxy(t, "anthropic", srv.URL+"/", "")
	ex, _ := forwardExchange("anthropic", http.MethodPost, "/v1/messages?beta=true", `{"model":"m"}`)
	ex.R.Header.Set("Accept-Encoding", "br")
	ex.R.Header.Set("Authorization", "Bearer harness-key")
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	r, body := up.last(t)
	if r.URL.Path != "/v1/messages" || r.URL.RawQuery != "beta=true" || body != `{"model":"m"}` {
		t.Fatalf("path %q query %q body %q", r.URL.Path, r.URL.RawQuery, body)
	}
	if r.Host != strings.TrimPrefix(srv.URL, "http://") {
		t.Fatalf("host = %q", r.Host)
	}
	if got := r.Header.Get("Accept-Encoding"); got != "gzip" {
		t.Fatalf("Accept-Encoding = %q, want the transport's own gzip", got)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer harness-key" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestDoForwardStreamUsageOnlyForRoutedOpenAIChat(t *testing.T) {
	const body = `{"stream":true}`
	for _, tc := range []struct {
		name, provider, path string
		routed, wantChanged  bool
	}{
		{name: "routed chat", provider: "openai", path: "/chat/completions", routed: true, wantChanged: true},
		{name: "routed versioned chat", provider: "openai", path: "/v1/chat/completions", routed: true, wantChanged: true},
		{name: "routed other path", provider: "openai", path: "/v1/responses", routed: true},
		{name: "unrouted chat", provider: "openai", path: "/chat/completions"},
		{name: "routed Anthropic", provider: "anthropic", path: "/chat/completions", routed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := &recordedUpstream{}
			srv := up.server(t, nil)
			p := forwardProxy(t, tc.provider, srv.URL, "")
			ex, _ := forwardExchange(tc.provider, http.MethodPost, tc.path, body)
			ex.R.Header.Set("Authorization", "Bearer harness-key")
			if tc.routed {
				ex.Route = &ProviderRoute{BaseURL: srv.URL, Provider: tc.provider}
			}
			if err := p.doForward(ex); err != nil {
				t.Fatal(err)
			}
			_, got := up.last(t)
			if changed := got != body; changed != tc.wantChanged {
				t.Fatalf("forwarded body = %s", got)
			}
			if tc.wantChanged && !strings.Contains(got, "include_usage") {
				t.Fatalf("forwarded body = %s", got)
			}
		})
	}
}

func TestDoForwardPassthroughStreamsRequestBody(t *testing.T) {
	up := &recordedUpstream{}
	srv := up.server(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "reply") })
	p := forwardProxy(t, "cursor", srv.URL, "")
	ex, w := forwardExchange("cursor", http.MethodPost, "/rpc", "from-stream")
	ex.ReqBody = []byte("ignored-buffer")
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	r, body := up.last(t)
	if body != "from-stream" || r.ContentLength != int64(len("from-stream")) {
		t.Fatalf("body %q content length %d", body, r.ContentLength)
	}
	if !ex.Streamed || ex.Status != "ok" || w.Body.String() != "reply" || ex.RespBody != nil {
		t.Fatalf("streamed %v status %q body %q captured %q", ex.Streamed, ex.Status, w.Body.String(), ex.RespBody)
	}
}

func TestDoForwardUnreachableUpstream(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	p := forwardProxy(t, "anthropic", url, "")

	ex, w := forwardExchange("anthropic", http.MethodPost, "/v1/messages", "{}")
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	want := `{"type":"error","error":{"type":"api_error","message":"upstream unreachable"}}`
	if ex.Status != "upstream_error" || w.Code != http.StatusBadGateway || w.Body.String() != want {
		t.Fatalf("status %q code %d body %q", ex.Status, w.Code, w.Body.String())
	}

	ex, _ = forwardExchange("anthropic", http.MethodPost, "/v1/messages", "{}")
	ctx, cancel := context.WithCancel(ex.R.Context())
	cancel()
	ex.R = ex.R.WithContext(ctx)
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	if ex.Status != "client_closed" {
		t.Fatalf("cancelled status = %q", ex.Status)
	}
}

func TestDoForwardBodyRelayAndFinalStatus(t *testing.T) {
	const usage = `{"model":"claude-x","usage":{"input_tokens":3,"output_tokens":4}}`
	sse := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = io.WriteString(w, "data: {}\n\n")
	}
	status := func(code int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Upstream", "kept")
			w.WriteHeader(code)
			_, _ = io.WriteString(w, body)
		}
	}
	for _, tc := range []struct {
		name         string
		provider     string
		handler      http.HandlerFunc
		reqBody      string
		wantStatus   string
		wantCode     int
		wantBody     string
		wantCaptured string
		wantStreamed bool
		wantInput    int
	}{
		{name: "buffered success parses usage", provider: "anthropic", handler: status(200, usage), wantStatus: "ok", wantCode: 200, wantBody: usage, wantCaptured: usage, wantInput: 3},
		{name: "buffered upstream failure", provider: "anthropic", handler: status(500, "nope"), wantStatus: "upstream_error", wantCode: 500, wantBody: "nope", wantCaptured: "nope"},
		{name: "rate limit", provider: "anthropic", handler: status(429, "slow"), wantStatus: "rate_limited", wantCode: 429, wantBody: "slow", wantCaptured: "slow"},
		{name: "quota probe", provider: "anthropic", handler: status(429, "slow"), reqBody: `{"max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`, wantStatus: "quota_probe", wantCode: 429, wantBody: "slow", wantCaptured: "slow"},
		{name: "truncated buffered body stays partial and skips usage", provider: "anthropic", handler: truncatedUpstream("application/json", usage), wantStatus: "upstream_error", wantCode: 200, wantBody: usage, wantCaptured: usage},
		{name: "event stream", provider: "anthropic", handler: sse, wantStatus: "ok", wantCode: 200, wantBody: "data: {}\n\n", wantStreamed: true, wantCaptured: "data: {}\n\n"},
		{name: "truncated event stream", provider: "anthropic", handler: truncatedUpstream("text/event-stream", "data: {}\n\n"), wantStatus: "upstream_error", wantCode: 200, wantBody: "data: {}\n\n", wantStreamed: true, wantCaptured: "data: {}\n\n"},
		{name: "passthrough", provider: "cursor", handler: status(200, "bytes"), wantStatus: "ok", wantCode: 200, wantBody: "bytes", wantStreamed: true},
		{name: "passthrough wins over event stream", provider: "cursor", handler: sse, wantStatus: "ok", wantCode: 200, wantBody: "data: {}\n\n", wantStreamed: true},
		{name: "truncated passthrough", provider: "cursor", handler: truncatedUpstream("application/octet-stream", "bytes"), wantStatus: "upstream_error", wantCode: 200, wantBody: "bytes", wantStreamed: true},
		{name: "passthrough upstream failure", provider: "cursor", handler: status(503, "down"), wantStatus: "upstream_error", wantCode: 503, wantBody: "down", wantStreamed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := &recordedUpstream{}
			srv := up.server(t, tc.handler)
			p := forwardProxy(t, tc.provider, srv.URL, "")
			body := tc.reqBody
			if body == "" {
				body = "{}"
			}
			ex, w := forwardExchange(tc.provider, http.MethodPost, "/v1/messages", body)
			if err := p.doForward(ex); err != nil {
				t.Fatal(err)
			}
			if ex.Status != tc.wantStatus || w.Code != tc.wantCode || ex.UpstreamStatus != tc.wantCode {
				t.Fatalf("status %q code %d upstream %d", ex.Status, w.Code, ex.UpstreamStatus)
			}
			if w.Body.String() != tc.wantBody || string(ex.RespBody) != tc.wantCaptured {
				t.Fatalf("body %q captured %q", w.Body.String(), ex.RespBody)
			}
			if ex.Streamed != tc.wantStreamed || ex.Usage.InputTokens != tc.wantInput {
				t.Fatalf("streamed %v usage %+v", ex.Streamed, ex.Usage)
			}
		})
	}
}

func TestDoForwardClientGoneAfterResponseIsClientClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	up := &recordedUpstream{}
	srv := up.server(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
	})
	p := forwardProxy(t, "anthropic", srv.URL, "")
	ex, _ := forwardExchange("anthropic", http.MethodPost, "/v1/messages", "{}")
	ex.R = ex.R.WithContext(ctx)
	ex.W = cancellingRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	if err := p.doForward(ex); err != nil {
		t.Fatal(err)
	}
	if ex.Status != "client_closed" {
		t.Fatalf("status = %q", ex.Status)
	}
}

// cancellingRecorder ends the request context as soon as the response starts.
type cancellingRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r cancellingRecorder) WriteHeader(code int) {
	r.ResponseRecorder.WriteHeader(code)
	r.cancel()
}
