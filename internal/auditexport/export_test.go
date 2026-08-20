package auditexport

import (
	"archive/zip"
	"bytes"
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
	if err := WriteZIP(&selected, agentsDir, "codex-agent", "iter-1"); err != nil {
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
	if err := WriteZIP(&all, agentsDir, "codex-agent", ""); err != nil {
		t.Fatal(err)
	}
	allMarkdown, allJSONL := zipContents(t, all.Bytes())
	if !strings.Contains(allMarkdown, "iter-1") || !strings.Contains(allMarkdown, "iter-2") ||
		!strings.Contains(allMarkdown, "iter-with-transcript-only") || !strings.Contains(allJSONL, "iter-with-transcript-only") {
		t.Fatalf("full export is incomplete:\n%s\n%s", allMarkdown, allJSONL)
	}
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
