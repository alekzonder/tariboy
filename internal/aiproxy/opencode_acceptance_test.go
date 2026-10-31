package aiproxy_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// Removing lease-specific routing, accounting, or transcript persistence breaks
// this contract even when two agents select the same provider and model IDs.
func TestOpenCodeRoutesAccountAcrossHandoffAndReindex(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	agentsDir := filepath.Join(base, "agents")
	db := storetest.Open(t)
	now := func() time.Time { return time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC) }
	usage := aiproxy.NewStore(db, now)
	budget := aiproxy.NewBudgetCache(usage, now)
	if err := usage.SetAgentBudget("alice", aiproxy.AgentBudget{HourUSD: 31}); err != nil {
		t.Fatal(err)
	}
	prices := aiproxy.NewPricing(map[string]aiproxy.ModelPrice{
		"k3": {InputPerMtok: 1e6, OutputPerMtok: 2e6, CacheWritePerMtok: 3e6, CacheReadPerMtok: 4e6},
	}, nil, nil)
	var aliceCalls, bobCalls atomic.Int32
	upstream := func(calls *atomic.Int32, streamed bool) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.URL.Path != "/coding/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer provider-key" {
				t.Errorf("upstream path/auth = %q/%q", r.URL.Path, r.Header.Get("Authorization"))
			}
			body := `{"model":"k3","usage":{"prompt_tokens":10,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":3},"input_tokens_details":{"cache_write_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}}}`
			if streamed {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Content-Encoding", "gzip")
				gz := gzip.NewWriter(w)
				fmt.Fprintf(gz, "data: %s\n\ndata: [DONE]\n\n", body)
				if err := gz.Close(); err != nil {
					t.Error(err)
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, body)
		}))
	}
	aliceUp, bobUp := upstream(&aliceCalls, false), upstream(&bobCalls, true)
	defer aliceUp.Close()
	defer bobUp.Close()
	handoff := filepath.Join(base, "handoff.json")
	tokens, err := aiproxy.OpenTokenRegistry(handoff, nil)
	if err != nil {
		t.Fatal(err)
	}
	newProxy := func(tokens *aiproxy.TokenRegistry) *aiproxy.Proxy {
		return aiproxy.New(aiproxy.Config{
			Tokens: tokens, Pricing: prices, Store: usage, Router: aiproxy.NewRouter(), Budget: budget,
			AgentsDir: agentsDir, Clock: now, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
			Ingest: func(row aiproxy.AIRequest) {
				if err := usage.Insert(row); err != nil {
					t.Error(err)
				}
			},
			GroupSnapshot: func(string) (string, string, error) { return "team", "Team", nil },
		})
	}
	p := newProxy(tokens)
	leases := map[string]string{}
	for name, upstream := range map[string]string{"alice": aliceUp.URL, "bob": bobUp.URL} {
		if err := os.MkdirAll(filepath.Join(agentsDir, name, "iterations", name+"-1"), 0o700); err != nil {
			t.Fatal(err)
		}
		token, err := p.Mint(aiproxy.Attribution{Agent: name, Iteration: name + "-1", ImageName: "developer", ImageTag: "latest", ImageDigest: "sha256:test", TaskID: "TASK-one", EpicID: "TASK-root"})
		if err != nil {
			t.Fatal(err)
		}
		if err := p.RegisterProviderRoutes(token, map[string]aiproxy.ProviderRoute{"kimi": {BaseURL: upstream + "/coding/v1", Provider: "openai"}}); err != nil {
			t.Fatal(err)
		}
		leases[name] = token
	}
	request := func(p *aiproxy.Proxy, name string, want int) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/_tariboy/"+leases[name]+"/_opencode/kimi/chat/completions", strings.NewReader(`{"model":"k3","stream":true}`))
		r.Header.Set("Authorization", "Bearer provider-key")
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s status = %d, want %d: %s", name, w.Code, want, w.Body.String())
		}
	}
	request(p, "alice", http.StatusOK)
	restored, err := aiproxy.OpenTokenRegistry(handoff, nil)
	if err != nil {
		t.Fatal(err)
	}
	p = newProxy(restored)
	request(p, "bob", http.StatusOK)
	if err := budget.Refresh(); err != nil {
		t.Fatal(err)
	}
	request(p, "alice", http.StatusTooManyRequests)
	if aliceCalls.Load() != 1 || bobCalls.Load() != 1 {
		t.Fatalf("upstream calls alice/bob = %d/%d", aliceCalls.Load(), bobCalls.Load())
	}
	for _, name := range []string{"alice", "bob"} {
		path := filepath.Join(agentsDir, name, "iterations", name+"-1", "proxy-transcript.jsonl")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var entry aiproxy.TranscriptEntry
		if err := json.Unmarshal([]byte(strings.SplitN(string(data), "\n", 2)[0]), &entry); err != nil {
			t.Fatal(err)
		}
		m := entry.Meta
		if m.Agent != name || m.Iteration != name+"-1" || m.TaskID != "TASK-one" || m.EpicID != "TASK-root" || m.GroupID != "team" || m.GroupName != "Team" || m.ImageDigest != "sha256:test" {
			t.Fatalf("attribution = %+v", m)
		}
		if m.InputTokens != 5 || m.OutputTokens != 4 || m.CacheWriteTokens != 2 || m.CacheReadTokens != 3 || m.CostUSD != 31 || m.Model != "k3" || len(entry.Request) == 0 || !strings.Contains(string(entry.Response), `"prompt_tokens":10`) {
			t.Fatalf("accounting/transcript = %+v", entry)
		}
		if strings.Contains(string(entry.Request), leases[name]) || strings.Contains(string(entry.Response), "provider-key") {
			t.Fatal("credentials in transcript bodies")
		}
		if err := aiproxy.GzipTranscript(agentsDir, name, name+"-1"); err != nil {
			t.Fatal(err)
		}
	}
	cmd, ok := commands.BuildRegistry().Get("daemon.reindex")
	if !ok {
		t.Fatal("missing reindex command")
	}
	for range 2 {
		if _, err := registry.Call(&registry.Ctx{Store: db, BaseDir: base}, context.Background(), cmd, nil); err != nil {
			t.Fatal(err)
		}
		var count int
		var cost float64
		if err := db.DB.QueryRow(`SELECT COUNT(*), SUM(cost_usd) FROM ai_requests`).Scan(&count, &cost); err != nil {
			t.Fatal(err)
		}
		if count != 3 || cost != 62 {
			t.Fatalf("reindex count/cost = %d/%g, want 3/62", count, cost)
		}
	}
}
