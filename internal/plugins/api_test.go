package plugins

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
)

func newAPI(t *testing.T) (*API, *bus.Bus, *TokenRegistry) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	b := bus.New(s, func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) })
	reg := NewTokenRegistry(nil)
	a := NewAPI(reg, b, slog.New(slog.NewTextHandler(io.Discard, nil)), func(string, string, string) {})
	return a, b, reg
}

func TestPublishScoped(t *testing.T) {
	a, b, reg := newAPI(t)
	tok, _ := reg.Mint(Identity{Name: "echo", Publish: []string{"chat:*"}, Subscribe: []string{"chat:in"}})

	do := func(token, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/plugin/publish", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		a.ServeHTTP(rr, req)
		return rr
	}
	// Allowed channel -> 200 and a bus message with plugin attribution.
	rr := do(tok, `{"channel":"chat:echo-out","type":"echo.reply","text":"hi"}`)
	if rr.Code != 200 {
		t.Fatalf("publish code = %d body=%s", rr.Code, rr.Body)
	}
	msgs, _ := b.Tail("chat:echo-out", 10)
	if len(msgs) != 1 || msgs[0].Source != "plugin:echo" || msgs[0].ProducedByPlugin != "echo" {
		t.Fatalf("published msg = %+v", msgs)
	}
	// Stable plugin-local keys deduplicate retries without colliding with other
	// plugins or daemon publishers.
	if rr := do(tok, `{"channel":"chat:echo-out","text":"once","idempotency_key":"update-9"}`); rr.Code != 200 {
		t.Fatalf("idempotent publish code = %d body=%s", rr.Code, rr.Body)
	}
	if rr := do(tok, `{"channel":"chat:echo-out","text":"duplicate","idempotency_key":"update-9"}`); rr.Code != 200 {
		t.Fatalf("idempotent retry code = %d body=%s", rr.Code, rr.Body)
	}
	msgs, _ = b.Tail("chat:echo-out", 10)
	if len(msgs) != 2 || msgs[1].Text != "once" {
		t.Fatalf("idempotent messages = %+v", msgs)
	}
	// Out-of-scope channel -> 403.
	if rr := do(tok, `{"channel":"user:ops","type":"x"}`); rr.Code != 403 {
		t.Fatalf("out-of-scope code = %d", rr.Code)
	}
	// Bad token -> 401.
	if rr := do("plg-bogus", `{"channel":"chat:x"}`); rr.Code != 401 {
		t.Fatalf("bad token code = %d", rr.Code)
	}
	// No token -> 401.
	if rr := do("", `{"channel":"chat:x"}`); rr.Code != 401 {
		t.Fatalf("no token code = %d", rr.Code)
	}
}

func TestSubscriptions(t *testing.T) {
	a, _, reg := newAPI(t)
	tok, _ := reg.Mint(Identity{Name: "echo", Subscribe: []string{"chat:in", "chat:in2"}})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/plugin/subscriptions", nil)
	req.Header.Set("X-Plugin-Token", tok)
	a.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("subs code = %d", rr.Code)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Channels []string `json:"channels"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Channels) != 2 || env.Data.Channels[0] != "chat:in" {
		t.Fatalf("subs = %+v", env.Data.Channels)
	}
}

// TestPluginWatchesPull covers GET /api/plugin/watches (spec §6.2 pull path):
// it returns the full current watch list for exactly the plugin's provided
// channels, scoped by the plugin token.
func TestPluginWatchesPull(t *testing.T) {
	a, b, reg := newAPI(t)
	tok, _ := reg.Mint(Identity{Name: "issue-provider", Provide: []string{"issue-provider:query", "issue-provider:ticket"}})

	// Two agents share one query watch; a third watches a ticket.
	if _, err := b.SubscribeParams("dev-manager", "issue-provider:query", bus.Matcher{}, nil,
		map[string]any{"query": "PROJ"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SubscribeParams("dev-worker", "issue-provider:query", bus.Matcher{}, nil,
		map[string]any{"query": "PROJ"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SubscribeParams("dev-lead", "issue-provider:ticket", bus.Matcher{}, nil,
		map[string]any{"ticket": "PROJ-1"}); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/plugin/watches", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	a.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("watches code=%d body=%s", rr.Code, rr.Body)
	}
	var resp struct {
		Result struct {
			Channels []ChannelWatchesDTO `json:"channels"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rr.Body)
	}
	byCh := map[string]ChannelWatchesDTO{}
	for _, c := range resp.Result.Channels {
		byCh[c.Channel] = c
	}
	if len(resp.Result.Channels) != 2 {
		t.Fatalf("want 2 provided channels, got %d: %+v", len(resp.Result.Channels), resp.Result.Channels)
	}
	q := byCh["issue-provider:query"]
	if len(q.Watches) != 1 || len(q.Watches[0].Subscribers) != 2 {
		t.Fatalf("query watch = %+v", q.Watches)
	}
	if q.Watches[0].Params["query"] != "PROJ" {
		t.Fatalf("query params not carried: %+v", q.Watches[0].Params)
	}
	if tk := byCh["issue-provider:ticket"]; len(tk.Watches) != 1 {
		t.Fatalf("ticket watch = %+v", tk.Watches)
	}

	// Unknown token → 401.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/plugin/watches", nil)
	req.Header.Set("Authorization", "Bearer plg-bogus")
	a.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("bad token code=%d", rr.Code)
	}
}
