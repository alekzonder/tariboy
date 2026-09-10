package auditexport

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/audit"
)

func TestWriteZIPScopesIterationAndIncludesReadableAndRawRecords(t *testing.T) {
	agentsDir := t.TempDir()
	layout := agentdir.New(agentsDir, "codex-agent")
	clock := time.Date(2026, 8, 18, 16, 29, 10, 0, time.UTC)
	log := audit.Open(layout.AuditLog(), func() time.Time { return clock })
	log.Record("iteration_started", "system", "iter-1", map[string]any{"trigger": "manual"})
	log.Record("status", "system", "iter-1", map[string]any{"message": "reviewing audit UI"})
	log.Record("iteration_started", "system", "iter-2", map[string]any{"trigger": "timer"})
	for _, id := range []string{"iter-1", "iter-2", "iter-with-transcript-only"} {
		if err := layout.EnsureIteration(id); err != nil {
			t.Fatal(err)
		}
		entry := aiproxy.TranscriptEntry{
			Meta:     aiproxy.AIRequest{ID: "air-" + id, TS: clock.Format(time.RFC3339), Agent: "codex-agent", Iteration: id, Provider: "openai", Model: "gpt-5.6-sol"},
			Request:  []byte(`{"instructions":"secret prompt","input":"inspect"}`),
			Response: []byte(`{"status":"completed","output":[{"type":"function_call","name":"exec_command","call_id":"call-1","arguments":"{\"cmd\":\"rg --files\"}"}]}`),
		}
		if err := aiproxy.AppendTranscript(agentsDir, entry); err != nil {
			t.Fatal(err)
		}
	}

	var selected bytes.Buffer
	if err := WriteZIP(context.Background(), &selected, agentsDir, "codex-agent", "iter-1"); err != nil {
		t.Fatal(err)
	}
	markdown, jsonl := zipContents(t, selected.Bytes())
	for _, want := range []string{"# Audit log — codex-agent", "Iteration `iter-1`", "reviewing audit UI", "Command", "rg --files"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("audit.md missing %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "iter-2") || strings.Contains(jsonl, "iter-2") {
		t.Fatalf("iteration export leaked another iteration:\n%s\n%s", markdown, jsonl)
	}
	for _, want := range []string{`"record_type":"audit_event"`, `"record_type":"proxy_transcript"`, `secret prompt`, `rg --files`} {
		if !strings.Contains(jsonl, want) {
			t.Fatalf("audit.jsonl missing %q:\n%s", want, jsonl)
		}
	}

	var all bytes.Buffer
	if err := WriteZIP(context.Background(), &all, agentsDir, "codex-agent", ""); err != nil {
		t.Fatal(err)
	}
	allMarkdown, allJSONL := zipContents(t, all.Bytes())
	if !strings.Contains(allMarkdown, "iter-1") || !strings.Contains(allMarkdown, "iter-2") ||
		!strings.Contains(allMarkdown, "iter-with-transcript-only") || !strings.Contains(allJSONL, "iter-with-transcript-only") {
		t.Fatalf("full export is incomplete:\n%s\n%s", allMarkdown, allJSONL)
	}
}

func TestWriteZIPPreservesLargeTranscriptOrderAndBodies(t *testing.T) {
	agentsDir := t.TempDir()
	layout := agentdir.New(agentsDir, "large-agent")
	if err := layout.EnsureIteration("iter-large"); err != nil {
		t.Fatal(err)
	}
	const count = 2000
	for i := 0; i < count; i++ {
		entry := aiproxy.TranscriptEntry{
			Meta:     aiproxy.AIRequest{ID: fmt.Sprintf("request-%04d", i), Agent: "large-agent", Iteration: "iter-large", Provider: "openai", Model: "gpt-test"},
			Request:  []byte(fmt.Sprintf(`{"input":"request-%04d"}`, i)),
			Response: []byte(fmt.Sprintf(`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"response-%04d"}]}]}`, i)),
		}
		if err := aiproxy.AppendTranscript(agentsDir, entry); err != nil {
			t.Fatal(err)
		}
	}

	var output bytes.Buffer
	if err := WriteZIP(context.Background(), &output, agentsDir, "large-agent", "iter-large"); err != nil {
		t.Fatal(err)
	}
	_, jsonl := zipContents(t, output.Bytes())
	lines := strings.Split(strings.TrimSpace(jsonl), "\n")
	if len(lines) != count {
		t.Fatalf("JSONL records = %d, want %d", len(lines), count)
	}
	for i, line := range lines {
		if !strings.Contains(line, fmt.Sprintf(`"call_index":%d`, i)) || !strings.Contains(line, fmt.Sprintf("request-%04d", i)) || !strings.Contains(line, fmt.Sprintf("response-%04d", i)) {
			t.Fatalf("record %d lost order or bodies: %s", i, line)
		}
	}
}

func TestWriteMarkdownStopsBetweenAuditEventsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	writer := &cancelAfterWriter{cancel: cancel, needle: "first"}
	events := []audit.Event{
		{IterationID: "iter-1", TS: "t1", Type: "status", Data: map[string]any{"message": "first"}},
		{IterationID: "iter-1", TS: "t2", Type: "status", Data: map[string]any{"message": "second"}},
	}
	err := writeMarkdown(ctx, writer, t.TempDir(), "agent", events, []string{"iter-1"})
	if !errors.Is(err, context.Canceled) || strings.Contains(writer.String(), "second") {
		t.Fatalf("error = %v, output = %q; want cancellation before second event", err, writer.String())
	}
}

type cancelAfterWriter struct {
	bytes.Buffer
	cancel func()
	needle string
}

func (w *cancelAfterWriter) Write(body []byte) (int, error) {
	n, err := w.Buffer.Write(body)
	if strings.Contains(w.String(), w.needle) {
		w.cancel()
	}
	return n, err
}

func TestToolDetailRendersLocalShellCommand(t *testing.T) {
	got := toolDetail([]byte(`{"type":"exec","command":["bash","-lc","make check"]}`))
	if got != "bash -lc make check" {
		t.Fatalf("toolDetail() = %q, want readable command", got)
	}
}

func TestIterationIDsIgnoreUnsafeAuditEventPaths(t *testing.T) {
	ids, err := iterationIDs([]audit.Event{{IterationID: "../outside"}, {IterationID: "iter-1"}}, "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "iter-1" {
		t.Fatalf("iterationIDs() = %#v, want only the safe ID", ids)
	}
}

func zipContents(t *testing.T, raw []byte) (string, string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, file := range zr.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(file.Name)] = string(body)
	}
	if len(files) != 2 {
		t.Fatalf("zip files = %#v", files)
	}
	return files["audit.md"], files["audit.jsonl"]
}
