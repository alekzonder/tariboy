package commands

import (
	"encoding/base64"
	"io"
	"log/slog"
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

func TestAgentPush_ReturnsAbsolutePath(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	work := filepath.Join(t.TempDir(), "work")
	os.MkdirAll(work, 0o700)
	as.Create(agent.Agent{Name: "a1", Cwd: work, OnTimeout: "restart", OnError: "restart"})

	content := base64.StdEncoding.EncodeToString([]byte("hi"))
	res, err := h(t, "agent.push")(c, registry.Params{
		"name": "a1", "path": ".tariboy/files/x.txt", "content": content,
	})
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := res.(map[string]any)["abs"].(string)
	want := filepath.Join(work, ".tariboy", "files", "x.txt")
	if !filepath.IsAbs(abs) || abs != want {
		t.Fatalf("abs = %q want %q", abs, want)
	}
	if !strings.HasSuffix(abs, filepath.Join(".tariboy", "files", "x.txt")) {
		t.Fatalf("abs suffix = %q", abs)
	}
}

func TestPushPullConfinement(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	work := filepath.Join(t.TempDir(), "work")
	os.MkdirAll(work, 0o700)
	as.Create(agent.Agent{Name: "smoke", Cwd: work, OnTimeout: "restart", OnError: "restart"})

	content := base64.StdEncoding.EncodeToString([]byte("hello file"))
	if _, err := h(t, "agent.push")(c, registry.Params{"name": "smoke", "path": "notes.txt", "content": content}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(work, "notes.txt")); string(data) != "hello file" {
		t.Fatalf("pushed file = %q", data)
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
// workdir cannot be used to read (pull) or write (push) outside of it, while a
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
	// (b) push THROUGH the symlink is rejected (file does not exist yet).
	content := base64.StdEncoding.EncodeToString([]byte("pwned"))
	if _, err := h(t, "agent.push")(c, registry.Params{"name": "smoke", "path": "escape/planted.txt", "content": content}); err == nil {
		t.Fatal("push through symlink escaped confinement")
	}
	if _, err := os.Stat(filepath.Join(outside, "planted.txt")); err == nil {
		t.Fatal("push through symlink wrote a file outside the workdir")
	}
	// Writing directly to the symlink itself is also rejected.
	if _, err := h(t, "agent.push")(c, registry.Params{"name": "smoke", "path": "escape", "content": content}); err == nil {
		t.Fatal("push onto symlink escaped confinement")
	}

	// (c) a legitimate real subdirectory file still works end to end.
	if _, err := h(t, "agent.push")(c, registry.Params{"name": "smoke", "path": "sub/ok.txt", "content": content}); err != nil {
		t.Fatalf("legit real-subdir push failed: %v", err)
	}
	res, err := h(t, "agent.pull")(c, registry.Params{"name": "smoke", "path": "sub/ok.txt"})
	if err != nil {
		t.Fatalf("legit real-subdir pull failed: %v", err)
	}
	if got := res.(map[string]any)["content"].(string); got != content {
		t.Fatalf("round-tripped content = %q", got)
	}

	// (d) lexical ../ and absolute paths stay clamped inside the workdir (they are
	// anchored via Clean("/"+rel)), so they can never reference anything outside
	// root. Pull of such a clamped, nonexistent path errors; push clamps the write
	// under root rather than escaping.
	for _, bad := range []string{"../../etc/passwd", "/etc/passwd"} {
		if _, err := h(t, "agent.pull")(c, registry.Params{"name": "smoke", "path": bad}); err == nil {
			t.Fatalf("pull %q unexpectedly succeeded (should be clamped to a nonexistent in-root path)", bad)
		}
	}
	if _, err := h(t, "agent.push")(c, registry.Params{"name": "smoke", "path": "/etc/passwd", "content": content}); err != nil {
		t.Fatalf("clamped absolute push failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "etc", "passwd")); err != nil {
		t.Fatalf("absolute path was not clamped inside the workdir: %v", err)
	}
}

func TestParseCp(t *testing.T) {
	name, remote, local, up, err := parseCp("./a.txt", "smoke:in.txt")
	if err != nil || name != "smoke" || remote != "in.txt" || local != "./a.txt" || !up {
		t.Fatalf("upload parse: %q %q %q %v %v", name, remote, local, up, err)
	}
	name, remote, local, up, err = parseCp("smoke:out.txt", "./b.txt")
	if err != nil || name != "smoke" || remote != "out.txt" || local != "./b.txt" || up {
		t.Fatalf("download parse: %q %q %q %v %v", name, remote, local, up, err)
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
		t.Fatal("accepted oversized upload")
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

func TestServerUploadRejectsOversizedHTTPBody(t *testing.T) {
	c := &registry.Ctx{BaseDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	server := api.NewServer(BuildRegistry(), c)
	body := `{"name":"large.txt","content":"` + strings.Repeat("A", 24<<20) + `"}`
	reader := strings.NewReader(body)
	request := httptest.NewRequest(http.MethodPut, "/api/files", reader)
	request.ContentLength = -1 // The bound must also protect streamed requests.
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
