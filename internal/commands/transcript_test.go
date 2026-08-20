package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestTranscriptHandler(t *testing.T) {
	base := t.TempDir()
	agent := "a1"
	iter := "a1-20260710-1"
	dir := agentdir.New(filepath.Join(base, "agents"), agent).IterationDir(iter)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"meta":{"TS":"2026-07-10T00:00:00Z","Provider":"anthropic","Model":"m","InputTokens":3},"request":"eyJzeXN0ZW0iOiJTMSIsIm1lc3NhZ2VzIjpbeyJyb2xlIjoidXNlciIsImNvbnRlbnQiOiJoaSJ9XX0=","response":"eyJzdG9wX3JlYXNvbiI6ImVuZF90dXJuIiwiY29udGVudCI6W3sidHlwZSI6InRleHQiLCJ0ZXh0Ijoib2sifV19"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "proxy-transcript.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := transcriptCommand()
	ctx := &registry.Ctx{BaseDir: base}

	out, err := cmd.Handler(ctx, registry.Params{"name": agent, "id": iter})
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["count"].(int) != 1 {
		t.Fatalf("want 1 call, got %v", m["count"])
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"instructions":"S1"`) {
		t.Fatalf("marshaled output missing instructions: %s", b)
	}
	if !strings.Contains(string(b), `"text":"ok"`) {
		t.Fatalf("marshaled output missing response text: %s", b)
	}

	rawOut, err := cmd.Handler(ctx, registry.Params{"name": agent, "id": iter, "raw": "true"})
	if err != nil {
		t.Fatal(err)
	}
	rm := rawOut.(map[string]any)
	if rm["count"].(int) != 1 {
		t.Fatalf("want 1 raw call, got %v", rm["count"])
	}
	rb, err := json.Marshal(rm)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rb), `{\"system\":\"S1\"`) {
		t.Fatalf("raw output missing decoded request: %s", rb)
	}
	if !strings.Contains(string(rb), `{\"stop_reason\":\"end_turn\"`) {
		t.Fatalf("raw output missing decoded response: %s", rb)
	}
}
