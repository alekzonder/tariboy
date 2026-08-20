package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/loop"
	"github.com/alekzonder/tariboy/internal/shim"
)

const auditShimTestProxyToken = "sk-tariboy-abcdef0123456789abcdef0123456789abcdef0123456789"

type shimAuditRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *shimAuditRecorder) Record(_, _, _ string, data map[string]any) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if line, ok := data["line"].(string); ok {
		r.lines = append(r.lines, line)
	}
	return int64(len(r.lines))
}

func (r *shimAuditRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func exactCodexAuditArgv(t *testing.T, binDir string) []string {
	t.Helper()
	prompt := filepath.Join(t.TempDir(), "PROMPT.md")
	if err := os.WriteFile(prompt, []byte("test prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	adapter, err := harness.Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	argv, _, err := adapter.Command(t.TempDir(), prompt, harness.Config{
		ProxyURL: "http://127.0.0.1:5555/_tariboy/" + auditShimTestProxyToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	return argv
}

func assertShimAuditSafe(t *testing.T, rec *shimAuditRecorder) {
	t.Helper()
	lines := strings.Join(rec.snapshot(), "\n")
	if strings.Contains(lines, auditShimTestProxyToken) {
		t.Fatal("tailed shim audit data contains the raw proxy token")
	}
	if !strings.Contains(lines, "model_providers.tariboy.base_url=") ||
		!strings.Contains(lines, "/_tariboy/***") {
		t.Fatal("tailed shim audit data does not preserve the redacted provider URL shape")
	}
}

func TestTailerKeepsCodexProxyTokenOutOfExecShimAudit(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := shim.Run(shim.Options{
		IterationDir: dir, Agent: "codex-agent", IterationID: "codex-exec-audit", HardTimeoutS: 10,
		HarnessArgv: exactCodexAuditArgv(t, binDir),
	}); err != nil {
		t.Fatal(err)
	}
	rec := &shimAuditRecorder{}
	tailer := loop.StartTailer(rec, "codex-exec-audit", logsDir, time.Hour, false)
	tailer.Stop()
	assertShimAuditSafe(t, rec)
}

func TestTailerKeepsCodexProxyTokenOutOfTmuxShimAudit(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := shim.Run(shim.Options{
		IterationDir: dir, Agent: "codex-agent", IterationID: "codex-tmux-audit", HardTimeoutS: 10,
		TmuxSession: "codex-agent", HarnessArgv: exactCodexAuditArgv(t, binDir),
	})
	if err == nil {
		t.Fatal("shim.Run should return the fake tmux error")
	}
	rec := &shimAuditRecorder{}
	tailer := loop.StartTailer(rec, "codex-tmux-audit", logsDir, time.Hour, true)
	tailer.Stop()
	assertShimAuditSafe(t, rec)
}
