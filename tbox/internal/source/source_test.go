package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillMD(name, extra string) string {
	return "---\nname: " + name + "\ndescription: Skill " + name + "\n---\n" + extra + "\n"
}

// newRepo makes a repository on branch main with skill alpha and prompt file.
func newRepo(t *testing.T) string {
	t.Helper()
	isolate(t)
	repo := filepath.Join(t.TempDir(), "origin")
	os.MkdirAll(repo, 0o755)
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "skills/alpha/SKILL.md", skillMD("alpha", "one"))
	write(t, repo, "docs/review.md", "review one\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "first")
	return repo
}

func commitAgain(t *testing.T, repo string) string {
	t.Helper()
	write(t, repo, "skills/alpha/SKILL.md", skillMD("alpha", "two"))
	write(t, repo, "docs/review.md", "review two\n")
	git(t, repo, "commit", "-q", "-am", "second")
	return git(t, repo, "rev-parse", "HEAD")
}

func resolver(t *testing.T) Resolver {
	t.Helper()
	return Resolver{Home: paths.Home(t.TempDir()), Git: gitx.Git{AllowLocal: true, Timeout: 30 * time.Second}}
}

func TestGitSkillResolveThenPinnedOffline(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha", Ref: "main"}
	dir, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "git" || got.Commit != git(t, repo, "rev-parse", "HEAD") || got.SHA256 == "" || got.URL != repo {
		t.Fatalf("%+v", got)
	}
	if filepath.Base(dir) != "alpha" {
		t.Fatalf("dir %s", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	dir2, got2, err := r.SkillDir(context.Background(), e, &got)
	if err != nil || dir2 != dir || got2 != got {
		t.Fatalf("%s %+v %v", dir2, got2, err)
	}
}

func TestGitSkillPinnedCacheMissingExportsPinnedCommit(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha", Ref: "main"}
	_, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	commitAgain(t, repo)
	if err := os.RemoveAll(filepath.Join(string(r.Home), "cache", "sources")); err != nil {
		t.Fatal(err)
	}
	dir, got2, err := r.SkillDir(context.Background(), e, &got)
	if err != nil || got2.Commit != got.Commit {
		t.Fatalf("%+v %v", got2, err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if !strings.Contains(string(body), "one") || strings.Contains(string(body), "two") {
		t.Fatalf("not the pinned commit: %s", body)
	}
}

func TestGitSkillRejectsEscapingPath(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	for _, p := range []string{"../x", "/abs", "a/../../x"} {
		if _, _, err := r.SkillDir(context.Background(), tfile.SkillEntry{Git: repo, Path: p}, nil); err == nil {
			t.Fatalf("path %q accepted", p)
		}
	}
}

func TestGitPromptFile(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.PromptEntry{Git: repo, Path: "docs/review.md"}
	path, got, err := r.PromptFile(context.Background(), e, nil)
	if err != nil || filepath.Base(path) != "review.md" || got.Commit == "" || got.SHA256 == "" {
		t.Fatalf("%s %+v %v", path, got, err)
	}
	if body, _ := os.ReadFile(path); string(body) != "review one\n" {
		t.Fatalf("%q", body)
	}
	os.RemoveAll(repo)
	path2, _, err := r.PromptFile(context.Background(), e, &got)
	if err != nil || path2 != path {
		t.Fatalf("%s %v", path2, err)
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestURLSkillAndChangedBody(t *testing.T) {
	isolate(t)
	var body atomic.Value
	body.Store(tarGz(t, map[string]string{"wolf/SKILL.md": skillMD("wolf", "v1")}))
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Write(body.Load().([]byte))
	}))
	t.Cleanup(srv.Close)
	r := Resolver{Home: paths.Home(t.TempDir()), HTTP: fetch.Client{HTTP: srv.Client()}}
	e := tfile.SkillEntry{URL: srv.URL + "/wolf.tar.gz"}
	dir, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil || got.Kind != "url" || got.SHA256 == "" || filepath.Base(dir) != "wolf" {
		t.Fatalf("%s %+v %v", dir, got, err)
	}
	before := hits.Load()
	dir2, _, err := r.SkillDir(context.Background(), e, &got)
	if err != nil || dir2 != dir || hits.Load() != before {
		t.Fatalf("pinned hit the network: %v hits %d", err, hits.Load())
	}
	os.RemoveAll(filepath.Join(string(r.Home), "cache", "sources"))
	body.Store(tarGz(t, map[string]string{"wolf/SKILL.md": skillMD("wolf", "v2")}))
	_, _, err = r.SkillDir(context.Background(), e, &got)
	if err == nil || !strings.Contains(err.Error(), "tbox build --update") || !strings.Contains(err.Error(), e.URL) {
		t.Fatalf("want update hint naming the address, got %v", err)
	}
	if _, got3, err := r.SkillDir(context.Background(), e, nil); err != nil || got3.SHA256 == got.SHA256 {
		t.Fatalf("%+v %v", got3, err)
	}
}

func TestURLPromptPinned(t *testing.T) {
	isolate(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("hello\n")) }))
	t.Cleanup(srv.Close)
	r := Resolver{Home: paths.Home(t.TempDir()), HTTP: fetch.Client{HTTP: srv.Client()}}
	e := tfile.PromptEntry{URL: srv.URL + "/notes.md"}
	path, got, err := r.PromptFile(context.Background(), e, nil)
	if err != nil || filepath.Base(path) != "notes.md" {
		t.Fatalf("%s %v", path, err)
	}
	srv.Close()
	path2, _, err := r.PromptFile(context.Background(), e, &got)
	if err != nil || path2 != path {
		t.Fatalf("%s %v", path2, err)
	}
}

func TestRewriteURL(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	r.Git.AllowLocal = true
	r.RewriteURL = func(u string) string {
		if u == "https://github.com/acme/skills" {
			return repo
		}
		return u
	}
	e := tfile.SkillEntry{Git: "https://github.com/acme/skills", Path: "skills/alpha"}
	_, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil || got.URL != "https://github.com/acme/skills" {
		t.Fatalf("%+v %v", got, err)
	}
	if lock.Key("git", got.URL, got.Path, got.Ref) == "" {
		t.Fatal("key")
	}
}

func TestPinnedMissingMirrorUnreachable(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha"}
	_, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(string(r.Home), "cache"))
	os.RemoveAll(repo)
	_, _, err = r.SkillDir(context.Background(), e, &got)
	if err == nil || !strings.Contains(err.Error(), repo) {
		t.Fatalf("want error naming %s, got %v", repo, err)
	}
}

func TestConcurrentResolversShareCache(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha"}
	errs := make(chan error, 4)
	dirs := make(chan string, 4)
	for i := 0; i < 4; i++ {
		go func() {
			dir, _, err := r.SkillDir(context.Background(), e, nil)
			dirs <- dir
			errs <- err
		}()
	}
	var first string
	for i := 0; i < 4; i++ {
		d := <-dirs
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = d
		} else if d != first {
			t.Fatalf("dirs differ: %s %s", d, first)
		}
	}
	left, _ := filepath.Glob(filepath.Join(string(r.Home), "cache", "sources", ".stage-*"))
	if len(left) != 0 {
		t.Fatalf("stage leftovers %v", left)
	}
}

func corrupt(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPinnedCorruptedGitSkillIsRepaired(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha"}
	dir, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	corrupt(t, filepath.Join(dir, "SKILL.md"))
	dir2, got2, err := r.SkillDir(context.Background(), e, &got)
	if err != nil || got2 != got {
		t.Fatalf("%+v %v", got2, err)
	}
	if body, _ := os.ReadFile(filepath.Join(dir2, "SKILL.md")); strings.Contains(string(body), "tampered") {
		t.Fatalf("tampered cache returned: %s", body)
	}
}

func TestPinnedCorruptedGitSkillRemoteGoneFails(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha"}
	dir, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	corrupt(t, filepath.Join(dir, "SKILL.md"))
	os.RemoveAll(repo)
	if _, _, err := r.SkillDir(context.Background(), e, &got); err == nil || !strings.Contains(err.Error(), repo) {
		t.Fatalf("want error naming %s, got %v", repo, err)
	}
}

func TestPinnedCorruptedGitPromptIsRepaired(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.PromptEntry{Git: repo, Path: "docs/review.md"}
	file, got, err := r.PromptFile(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	corrupt(t, file)
	file2, _, err := r.PromptFile(context.Background(), e, &got)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(file2); string(body) != "review one\n" {
		t.Fatalf("%q", body)
	}
}

func TestPinnedCorruptedURLEntriesAreRepaired(t *testing.T) {
	isolate(t)
	archive := tarGz(t, map[string]string{"wolf/SKILL.md": skillMD("wolf", "v1")})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, ".md") {
			w.Write([]byte("hello\n"))
			return
		}
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	r := Resolver{Home: paths.Home(t.TempDir()), HTTP: fetch.Client{HTTP: srv.Client()}}
	se := tfile.SkillEntry{URL: srv.URL + "/wolf.tar.gz"}
	dir, sgot, err := r.SkillDir(context.Background(), se, nil)
	if err != nil {
		t.Fatal(err)
	}
	pe := tfile.PromptEntry{URL: srv.URL + "/notes.md"}
	file, pgot, err := r.PromptFile(context.Background(), pe, nil)
	if err != nil {
		t.Fatal(err)
	}
	corrupt(t, filepath.Join(dir, "SKILL.md"))
	corrupt(t, file)
	dir2, _, err := r.SkillDir(context.Background(), se, &sgot)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(filepath.Join(dir2, "SKILL.md")); strings.Contains(string(body), "tampered") {
		t.Fatalf("tampered skill: %s", body)
	}
	file2, _, err := r.PromptFile(context.Background(), pe, &pgot)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(file2); string(body) != "hello\n" {
		t.Fatalf("tampered prompt: %q", body)
	}
}

func TestPinnedWithEmptySHAIsMalformed(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha"}
	_, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	got.SHA256 = ""
	if _, _, err := r.SkillDir(context.Background(), e, &got); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("got %v", err)
	}
}

func TestPinnedEntryMetadataComesFromTheEntry(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	e := tfile.SkillEntry{Git: repo, Path: "skills/alpha"}
	_, got, err := r.SkillDir(context.Background(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	edited := got
	edited.URL, edited.Path, edited.Ref = "https://evil.example/x", "elsewhere", "other"
	_, again, err := r.SkillDir(context.Background(), e, &edited)
	if err != nil || again != got {
		t.Fatalf("%+v %v", again, err)
	}
}

func TestStaleStagesAreSwept(t *testing.T) {
	repo := newRepo(t)
	r := resolver(t)
	stale := filepath.Join(string(r.Home), "cache", "sources", ".stage-old")
	os.MkdirAll(stale, 0o755)
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(stale, old, old)
	if _, _, err := r.SkillDir(context.Background(), tfile.SkillEntry{Git: repo, Path: "skills/alpha"}, nil); err != nil {
		t.Fatal(err)
	}
	if exists(stale) {
		t.Fatal("stale stage not swept")
	}
}
