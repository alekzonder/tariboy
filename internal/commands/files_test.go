package commands

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestPullConfinement(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	work := filepath.Join(t.TempDir(), "work")
	os.MkdirAll(work, 0o700)
	as.Create(agent.Agent{Name: "smoke", Cwd: work, OnTimeout: "restart", OnError: "restart"})

	if err := os.WriteFile(filepath.Join(work, "notes.txt"), []byte("hello file"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "agent.pull")(c, registry.Params{"name": "smoke", "path": "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(res.(map[string]any)["content"].(string))
	if string(raw) != "hello file" {
		t.Fatalf("pulled = %q", raw)
	}
	// escape attempt is rejected
	if _, err := h(t, "agent.pull")(c, registry.Params{"name": "smoke", "path": "../../etc/passwd"}); err == nil {
		t.Fatal("path escape not rejected")
	}
}

// TestSymlinkConfinement verifies that a symlink placed inside the agent
// workdir cannot be used to read (pull) outside of it, while a
// legitimate real subdirectory still works and lexical escapes stay rejected.
func TestSymlinkConfinement(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	// A directory that lives OUTSIDE the workdir, holding a secret file.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Symlink INSIDE the workdir that points at the outside directory.
	if err := os.Symlink(outside, filepath.Join(work, "escape")); err != nil {
		t.Fatal(err)
	}
	as.Create(agent.Agent{Name: "smoke", Cwd: work, OnTimeout: "restart", OnError: "restart"})

	// (a) pull THROUGH the symlink is rejected.
	if _, err := h(t, "agent.pull")(c, registry.Params{"name": "smoke", "path": "escape/secret.txt"}); err == nil {
		t.Fatal("pull through symlink escaped confinement")
	}
	// A legitimate real subdirectory remains readable.
	content := base64.StdEncoding.EncodeToString([]byte("safe"))
	if err := os.MkdirAll(filepath.Join(work, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "sub/ok.txt"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := h(t, "agent.pull")(c, registry.Params{"name": "smoke", "path": "sub/ok.txt"})
	if err != nil {
		t.Fatalf("legit real-subdir pull failed: %v", err)
	}
	if got := res.(map[string]any)["content"].(string); got != content {
		t.Fatalf("round-tripped content = %q", got)
	}

	// Lexical paths stay clamped inside the workdir.
	for _, bad := range []string{"../../etc/passwd", "/etc/passwd"} {
		if _, err := h(t, "agent.pull")(c, registry.Params{"name": "smoke", "path": bad}); err == nil {
			t.Fatalf("pull %q unexpectedly escaped the workdir", bad)
		}
	}
}

func TestParseCp(t *testing.T) {
	name, remote, local, up, err := parseCp("./a.txt", "")
	if err != nil || name != "" || remote != "" || local != "./a.txt" || !up {
		t.Fatalf("upload parse: %q %q %q %v %v", name, remote, local, up, err)
	}
	name, remote, local, up, err = parseCp("smoke:out.txt", "./b.txt")
	if err != nil || name != "smoke" || remote != "out.txt" || local != "./b.txt" || up {
		t.Fatalf("download parse: %q %q %q %v %v", name, remote, local, up, err)
	}
	if _, _, _, _, err := parseCp("a.txt", "smoke:in.txt"); err == nil {
		t.Fatal("agent-scoped upload accepted")
	}
	if _, _, _, _, err := parseCp("smoke:out.txt", ""); err == nil {
		t.Fatal("download without destination accepted")
	}
	if _, _, _, _, err := parseCp("a.txt", "b.txt"); err == nil {
		t.Fatal("cp with no agent side should error")
	}
}

func TestServerUploadFiles(t *testing.T) {
	c := &registry.Ctx{BaseDir: t.TempDir()}
	upload := h(t, "files.upload")
	var previous string
	for _, content := range []string{"first", "second"} {
		result, err := upload(c, registry.Params{"name": "notes.txt", "content": base64.StdEncoding.EncodeToString([]byte(content))})
		if err != nil {
			t.Fatal(err)
		}
		abs := result.(map[string]any)["abs"].(string)
		if !filepath.IsAbs(abs) || !strings.HasPrefix(abs, filepath.Join(c.BaseDir, "files")+string(os.PathSeparator)) || filepath.Base(abs) != "notes.txt" || abs == previous {
			t.Fatalf("invalid upload path %q", abs)
		}
		data, err := os.ReadFile(abs)
		if err != nil || string(data) != content {
			t.Fatalf("uploaded data %q: %v", data, err)
		}
		info, err := os.Stat(abs)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("file permissions: %v, %v", info, err)
		}
		if previous != "" {
			data, err := os.ReadFile(previous)
			if err != nil || string(data) != "first" {
				t.Fatalf("previous upload changed: %q, %v", data, err)
			}
		}
		previous = abs
	}
	for _, name := range []string{"", ".", "..", "../outside", "/tmp/outside", "a/b", "a\\b", "bad\x00name", "bad\nname"} {
		if _, err := upload(c, registry.Params{"name": name, "content": "aGk="}); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
	if _, err := upload(c, registry.Params{"name": "bad.txt", "content": "!"}); err == nil {
		t.Fatal("accepted invalid base64")
	}
	if _, err := upload(c, registry.Params{"name": "large.txt", "content": strings.Repeat("A", 24<<20)}); err == nil {
		t.Fatal("legacy JSON route accepted an 18 MiB upload")
	}
	outside := t.TempDir()
	symlinkBase := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(symlinkBase, "files")); err != nil {
		t.Fatal(err)
	}
	if _, err := upload(&registry.Ctx{BaseDir: symlinkBase}, registry.Params{"name": "escape.txt", "content": "aGk="}); err == nil {
		t.Fatal("accepted symlink upload directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("wrote outside base: %v %v", entries, err)
	}
}

func TestServerUploadRejectsDeclaredOversizedHTTPBodyBeforeReading(t *testing.T) {
	c := &registry.Ctx{BaseDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	server := api.NewServer(BuildRegistry(), c)
	body := `{"name":"large.txt","content":"aGk="}`
	reader := strings.NewReader(body)
	request := httptest.NewRequest(http.MethodPut, "/api/files", reader)
	request.ContentLength = serverUpload().HTTP.MaxBodyBytes + 1
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if reader.Len() != len(body) {
		t.Fatal("read a body whose declared size exceeds the limit")
	}
	entries, err := os.ReadDir(c.BaseDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("oversized request wrote files: %v, %v", entries, err)
	}
}

func TestServerUploadRejectsOversizedStreamedHTTPBody(t *testing.T) {
	c := &registry.Ctx{BaseDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	server := api.NewServer(BuildRegistry(), c)
	body := `{"name":"large.txt","content":"` + strings.Repeat("A", 24<<20) + `"}`
	reader := strings.NewReader(body)
	request := httptest.NewRequest(http.MethodPut, "/api/files", reader)
	request.ContentLength = -1
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if reader.Len() == 0 {
		t.Fatal("read the entire oversized body before rejecting it")
	}
	entries, err := os.ReadDir(c.BaseDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("oversized request wrote files: %v, %v", entries, err)
	}
}

func TestCpSharedUpload(t *testing.T) {
	c := &registry.Ctx{BaseDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.Socket = filepath.Join(t.TempDir(), "api.sock")
	listener, err := net.Listen("unix", c.Socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(api.NewServer(BuildRegistry(), c).Handler())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	src := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(src, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := cpCommand().Handler(c, registry.Params{"src": src})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ Abs string }
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(saved.Abs)
	if err != nil || string(contents) != "shared" || !strings.HasPrefix(saved.Abs, filepath.Join(c.BaseDir, "files")+string(os.PathSeparator)) {
		t.Fatalf("upload result %s, contents %q: %v", data, contents, err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/agents/unused/files", strings.NewReader("{}"))
	server.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed && response.Code != http.StatusNotFound {
		t.Fatalf("old upload route remains: %d", response.Code)
	}
}

func TestCpSharedUploadUsesRawBody(t *testing.T) {
	type seenRequest struct {
		path, name, contentType, body string
		contentLength                 int64
	}
	seen := make(chan seenRequest, 1)
	socket := filepath.Join(t.TempDir(), "api.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- seenRequest{r.URL.Path, r.URL.Query().Get("name"), r.Header.Get("Content-Type"), string(body), r.ContentLength}
		api.WriteOK(w, map[string]any{"path": "files/id/notes.txt", "abs": "/server/files/id/notes.txt", "bytes": len(body)})
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()

	src := filepath.Join(t.TempDir(), "notes #1.txt")
	if err := os.WriteFile(src, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cpCommand().Handler(&registry.Ctx{Socket: socket}, registry.Params{"src": src}); err != nil {
		t.Fatal(err)
	}
	got := <-seen
	if got.path != "/api/files/raw" || got.name != "notes #1.txt" || got.contentType != "application/octet-stream" || got.body != "shared" || got.contentLength != 6 {
		t.Fatalf("request = %+v", got)
	}
}
