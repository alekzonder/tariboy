package aiproxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const quotaProbeBody = `{"model":"claude-haiku-4-5","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`

func TestForwardRequestStatus(t *testing.T) {
	t.Parallel()
	rateLimit := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Error"}}`))
	}
	cases := []struct {
		name     string
		body     string
		upstream http.HandlerFunc
		want     string
	}{
		{"startup quota probe", quotaProbeBody, rateLimit, "quota_probe"},
		{"quota probe with text block", `{"max_tokens":1,"messages":[{"role":"user","content":[{"type":"text","text":"quota"}]}]}`, rateLimit, "quota_probe"},
		{"real request rate limited", `{"model":"claude-opus-4-8","max_tokens":32000,"messages":[{"role":"user","content":"quota"}]}`, rateLimit, "rate_limited"},
		{"server error", `{"model":"claude-opus-4-8"}`, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}, "upstream_error"},
		{"success", `{"model":"claude-opus-4-8"}`, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"usage":{"input_tokens":1,"output_tokens":1}}`))
		}, "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := httptest.NewServer(tc.upstream)
			defer up.Close()
			p, rows := statusTestProxy(t, up.URL)
			tok, _ := p.Mint(Attribution{Agent: "alice", Iteration: "alice-1"})
			req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(tc.body))
			req.Header.Set("x-api-key", "k")
			p.ServeHTTP(httptest.NewRecorder(), req)
			if got := rows.last(t).Status; got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestForwardClientCancelDuringStreamIsClientClosed(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer up.Close()
	p, rows := statusTestProxy(t, up.URL)
	tok, _ := p.Mint(Attribution{Agent: "alice", Iteration: "alice-1"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(`{"model":"claude-opus-4-8","stream":true}`)).WithContext(ctx)
	req.Header.Set("x-api-key", "k")
	// The session stops once the response is streaming to the client.
	p.ServeHTTP(&cancelOnWrite{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}, req)
	if got := rows.last(t); got.Status != "client_closed" || got.UpstreamStatus != 200 {
		t.Fatalf("status = %q upstream %d, want client_closed with 200", got.Status, got.UpstreamStatus)
	}
}

type cancelOnWrite struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *cancelOnWrite) Write(b []byte) (int, error) {
	w.cancel()
	return w.ResponseRecorder.Write(b)
}

type ingestedRows struct {
	mu   sync.Mutex
	rows []AIRequest
}

func (r *ingestedRows) last(t *testing.T) AIRequest {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.rows) == 0 {
		t.Fatal("no request recorded")
	}
	return r.rows[len(r.rows)-1]
}

func statusTestProxy(t *testing.T, upstreamURL string) (*Proxy, *ingestedRows) {
	t.Helper()
	rows := &ingestedRows{}
	router := NewRouter()
	router.SetDefault("anthropic", Upstream{BaseURL: upstreamURL, KeyEnv: "UNUSED_KEY_ENV"})
	p := New(Config{
		Tokens: NewTokenRegistry(nil), Pricing: &Pricing{table: DefaultPricing()}, Store: newStore(t),
		Router: router, AgentsDir: t.TempDir(),
		Clock: func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Ingest: func(row AIRequest) {
			rows.mu.Lock()
			rows.rows = append(rows.rows, row)
			rows.mu.Unlock()
		},
	})
	return p, rows
}
