package api

import (
	"archive/zip"
	"bytes"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestAuditExportRouteDownloadsIterationZIP(t *testing.T) {
	base := t.TempDir()
	agentsDir := paths.New(base).AgentsDir()
	layout := agentdir.New(agentsDir, "alice")
	log := audit.Open(layout.AuditLog(), func() time.Time { return time.Date(2026, 8, 18, 16, 0, 0, 0, time.UTC) })
	log.Record("status", "system", "iter-1", map[string]any{"message": "running tests"})
	log.Record("status", "system", "iter-2", map[string]any{"message": "other iteration"})

	srv := NewServer(registry.New(), &registry.Ctx{BaseDir: base, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/agents/alice/audit-export?iteration=iter-1", nil))

	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type = %q", got)
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, `filename="alice-iter-1-audit.zip"`) {
		t.Fatalf("content disposition = %q", got)
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("files = %d", len(zr.File))
	}
	file, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "running tests") || strings.Contains(string(body), "other iteration") {
		t.Fatalf("iteration scoping failed: %s", body)
	}

	markdown := httptest.NewRecorder()
	srv.Handler().ServeHTTP(markdown, httptest.NewRequest("GET", "/api/agents/alice/audit-export?iteration=iter-1&format=markdown", nil))
	if markdown.Code != 200 || markdown.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Fatalf("markdown response: status=%d type=%q body=%s", markdown.Code, markdown.Header().Get("Content-Type"), markdown.Body.String())
	}
	if !strings.Contains(markdown.Body.String(), "running tests") || strings.Contains(markdown.Body.String(), "other iteration") {
		t.Fatalf("markdown iteration scoping failed: %s", markdown.Body.String())
	}
}

func TestAuditExportRejectsInvalidAgentAndIterationNames(t *testing.T) {
	srv := NewServer(registry.New(), &registry.Ctx{BaseDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, path := range []string{
		"/api/agents/bad$name/audit-export",
		"/api/agents/alice/audit-export?iteration=../secret",
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != 400 || !strings.Contains(rr.Body.String(), `"code":"bad_request"`) {
			t.Fatalf("path %q: status=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}
}
