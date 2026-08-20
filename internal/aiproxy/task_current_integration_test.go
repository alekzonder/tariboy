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

// TestTaskCurrentTagsProxiedRows is the integration proof for the `tools task
// current` chain (epic dev-t-3e1 §1): it drives real proxied requests through
// the proxy's forward+persist pipeline and asserts that the ingested ai_requests
// row carries task_id/epic_id exactly per the current-task tag on the live token.
// Native Tasks validation and root resolution are covered in internal/loop; this
// test covers the proxy half of the manager's SetTask chain: untagged, tagged,
// and cleared requests.
func TestTaskCurrentTagsProxiedRows(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":5,` +
			`"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`))
	}))
	defer up.Close()

	router := NewRouter()
	router.SetDefault("anthropic", Upstream{BaseURL: up.URL, KeyEnv: "UNUSED_KEY_ENV"})

	// Ingest captures each persisted row synchronously (persist calls Ingest
	// inline), so we can read the row immediately after each proxied request.
	var rows []AIRequest
	p := New(Config{
		Tokens: NewTokenRegistry(nil), Pricing: &Pricing{table: DefaultPricing()},
		Store: newStore(t), Router: router, AgentsDir: t.TempDir(),
		Clock:  func() time.Time { return time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC) },
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Ingest: func(row AIRequest) { rows = append(rows, row) },
	})
	tok, _ := p.Mint(Attribution{Agent: "alice", Iteration: "alice-1", ImageName: "basic", ImageTag: "latest"})

	// call drives one proxied request on the tokenized path and returns the row
	// the proxy persisted for it.
	call := func() AIRequest {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages",
			strings.NewReader(`{"model":"claude-opus-4-8"}`))
		req.Header.Set("x-api-key", "real-secret-key")
		p.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("proxied request status = %d body=%s", rr.Code, rr.Body.String())
		}
		return rows[len(rows)-1]
	}

	// 1. Untagged: first request lands with empty task/epic.
	if r := call(); r.TaskID != "" || r.EpicID != "" {
		t.Fatalf("untagged row = task=%q epic=%q, want empty", r.TaskID, r.EpicID)
	}

	// 2. Tag a valid id: the next request carries task_id + resolved epic_id.
	p.UpdateTask("alice-1", "SUPER-3", "SUPER-1")
	if r := call(); r.TaskID != "SUPER-3" || r.EpicID != "SUPER-1" {
		t.Fatalf("tagged row = task=%q epic=%q, want SUPER-3/SUPER-1", r.TaskID, r.EpicID)
	}

	// 3. Clear: the next request is untagged again.
	p.UpdateTask("alice-1", "", "")
	if r := call(); r.TaskID != "" || r.EpicID != "" {
		t.Fatalf("cleared row = task=%q epic=%q, want empty", r.TaskID, r.EpicID)
	}
}
