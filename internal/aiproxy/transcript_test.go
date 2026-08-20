package aiproxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
)

func TestAppendAndGzipTranscript(t *testing.T) {
	agentsDir := t.TempDir()
	l := agentdir.New(agentsDir, "alice")
	if err := l.EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}
	e := TranscriptEntry{
		Meta: AIRequest{
			ID: "air-1", Agent: "alice", Iteration: "alice-1", Model: "claude-opus-4-8",
			GroupID: "alpha", GroupName: "Alpha Team",
		},
		Request:  []byte(`{"model":"claude-opus-4-8"}`),
		Response: []byte(`{"usage":{"input_tokens":1}}`),
	}
	if err := AppendTranscript(agentsDir, e); err != nil {
		t.Fatal(err)
	}
	if err := AppendTranscript(agentsDir, e); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(l.IterationDir("alice-1"), "proxy-transcript.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "\n") != 2 {
		t.Fatalf("expected 2 lines, got %q", data)
	}
	var snapshot TranscriptEntry
	if err := json.Unmarshal(bytes.Split(data, []byte("\n"))[0], &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Meta.GroupID != "alpha" || snapshot.Meta.GroupName != "Alpha Team" {
		t.Fatalf("transcript group snapshot = %q/%q", snapshot.Meta.GroupID, snapshot.Meta.GroupName)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("transcript mode = %v, want 0600", fi.Mode().Perm())
	}
	// Gzip at close (this is what the engine's OnIterationClose hook, wired by
	// the daemon, calls once an iteration is fully finished).
	if err := GzipTranscript(agentsDir, "alice", "alice-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("plain transcript should be removed after gzip")
	}
	gzPath := path + ".gz"
	gzFi, err := os.Stat(gzPath)
	if err != nil {
		t.Fatalf("gzipped transcript missing: %v", err)
	}
	if gzFi.Mode().Perm() != 0o600 {
		t.Fatalf("gz mode = %v, want 0600", gzFi.Mode().Perm())
	}
	// The .gz must be a valid gzip stream that decompresses back to the exact
	// original lines (not just present-and-nonempty).
	gzFile, err := os.Open(gzPath)
	if err != nil {
		t.Fatal(err)
	}
	defer gzFile.Close()
	zr, err := gzip.NewReader(gzFile)
	if err != nil {
		t.Fatalf("not a valid gzip stream: %v", err)
	}
	decompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decompressed, data) {
		t.Fatalf("decompressed content mismatch:\ngot:  %q\nwant: %q", decompressed, data)
	}
}

// TestGzipTranscriptAfterRealProxyCall is the closest-to-production check for
// the M5 gzip-at-close fix: it drives an actual request through *Proxy (the
// same p.persist -> AppendTranscript path production traffic takes), then
// gzips exactly as the daemon's OnIterationClose hook would at iteration
// close, and verifies the plain file is gone and the .gz decompresses back to
// the transcript line the proxy wrote.
func TestGzipTranscriptAfterRealProxyCall(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer up.Close()
	t.Setenv("FAKE_ANTHROPIC_KEY", "real-secret-key")

	agentsDir := t.TempDir()
	l := agentdir.New(agentsDir, "alice")
	if err := l.EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}

	s := newStore(t)
	router := NewRouter()
	router.SetDefault("anthropic", Upstream{BaseURL: up.URL, KeyEnv: "FAKE_ANTHROPIC_KEY"})
	p := New(Config{
		Tokens: NewTokenRegistry(nil), Pricing: &Pricing{table: DefaultPricing()}, Store: s,
		Router: router, AgentsDir: agentsDir,
		Clock: func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) },
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	tok, err := p.Mint(Attribution{Agent: "alice", Iteration: "alice-1", ImageName: "basic", ImageTag: "latest"})
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(`{"model":"claude-opus-4-8"}`))
	req.Header.Set("x-api-key", "real-provider-key")
	p.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("proxy call status = %d body=%s", rr.Code, rr.Body.String())
	}

	path := filepath.Join(l.IterationDir("alice-1"), "proxy-transcript.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("proxy did not write a transcript: %v", err)
	}

	// This is what the daemon's OnIterationClose hook (wired in daemon.go) calls
	// once the loop engine marks this iteration terminally finished.
	if err := GzipTranscript(agentsDir, "alice", "alice-1"); err != nil {
		t.Fatalf("GzipTranscript: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("plain transcript should be removed after gzip at iteration close")
	}
	gzFile, err := os.Open(path + ".gz")
	if err != nil {
		t.Fatalf("gzipped transcript missing: %v", err)
	}
	defer gzFile.Close()
	zr, err := gzip.NewReader(gzFile)
	if err != nil {
		t.Fatalf("not a valid gzip stream: %v", err)
	}
	decompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(decompressed, before) {
		t.Fatalf("decompressed content mismatch:\ngot:  %q\nwant: %q", decompressed, before)
	}
	if !strings.Contains(string(decompressed), `"Agent":"alice"`) {
		t.Fatalf("decompressed transcript missing expected attribution: %q", decompressed)
	}
}

// TestGzipTranscriptMissingIsNoop proves the hook is safe to call for every
// finished iteration, including non-proxy ones that never wrote a transcript:
// GzipTranscript on a nonexistent proxy-transcript.jsonl must not error and
// must not create a stray .gz file.
func TestGzipTranscriptMissingIsNoop(t *testing.T) {
	agentsDir := t.TempDir()
	l := agentdir.New(agentsDir, "bob")
	if err := l.EnsureIteration("bob-1"); err != nil {
		t.Fatal(err)
	}
	if err := GzipTranscript(agentsDir, "bob", "bob-1"); err != nil {
		t.Fatalf("GzipTranscript on missing transcript must be a clean no-op, got: %v", err)
	}
	path := filepath.Join(l.IterationDir("bob-1"), "proxy-transcript.jsonl")
	if _, err := os.Stat(path + ".gz"); !os.IsNotExist(err) {
		t.Fatal("no .gz should be created when there was no transcript to compress")
	}
}

// TestAppendTranscriptOpaqueBodies proves the transcript is a source of truth:
// a non-JSON body (an HTML 502 page) and a truncated/garbage body must STILL
// produce a valid entry whose metadata is intact and whose body round-trips
// byte-for-byte. This fails against a json.RawMessage entry, whose Marshal
// rejects the whole entry (metadata included) when a body is not valid JSON.
func TestAppendTranscriptOpaqueBodies(t *testing.T) {
	cases := []struct {
		name string
		req  []byte
		resp []byte
	}{
		{"non_json_html", []byte(`{"model":"claude-opus-4-8"}`), []byte("<html>oops 502 bad gateway</html>")},
		{"truncated_garbage", []byte("{\"model\":\"claude-op"), []byte{0x00, 0x01, 0xff, 0xfe, '{', '"', 'a'}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agentsDir := t.TempDir()
			l := agentdir.New(agentsDir, "alice")
			if err := l.EnsureIteration("alice-1"); err != nil {
				t.Fatal(err)
			}
			e := TranscriptEntry{
				Meta: AIRequest{
					ID: "air-1", Agent: "alice", Iteration: "alice-1",
					Model: "claude-opus-4-8", InputTokens: 100, OutputTokens: 50,
				},
				Request:  tc.req,
				Response: tc.resp,
			}
			if err := AppendTranscript(agentsDir, e); err != nil {
				t.Fatalf("entry with opaque body must still be written: %v", err)
			}
			path := filepath.Join(l.IterationDir("alice-1"), "proxy-transcript.jsonl")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var got TranscriptEntry
			if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
				t.Fatalf("entry did not round-trip as JSON: %v", err)
			}
			// Metadata present.
			if got.Meta.Agent != "alice" || got.Meta.Iteration != "alice-1" ||
				got.Meta.Model != "claude-opus-4-8" || got.Meta.InputTokens != 100 || got.Meta.OutputTokens != 50 {
				t.Fatalf("metadata lost: %+v", got.Meta)
			}
			// Bodies round-trip to the exact input bytes (base64 in the JSONL).
			if !bytes.Equal(got.Request, tc.req) {
				t.Fatalf("request body not preserved: got %q want %q", got.Request, tc.req)
			}
			if !bytes.Equal(got.Response, tc.resp) {
				t.Fatalf("response body not preserved: got %q want %q", got.Response, tc.resp)
			}
		})
	}
}
