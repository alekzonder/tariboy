package stores

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
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

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitRepo makes a repository with one commit on main and returns its path.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, filepath.Join(dir, "prompts/a.md"), "a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "one")
	return dir
}

func commit(t *testing.T, dir, file string) string {
	t.Helper()
	write(t, filepath.Join(dir, file), file)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", file)
	return git(t, dir, "rev-parse", "HEAD")
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func newRegistry(t *testing.T, handler http.Handler) (Registry, string) {
	t.Helper()
	isolate(t)
	home := t.TempDir()
	r := Registry{Home: paths.Home(home), Git: gitx.Git{AllowLocal: true}, Cwd: t.TempDir()}
	if handler != nil {
		srv := httptest.NewTLSServer(handler)
		t.Cleanup(srv.Close)
		r.HTTP = fetch.Client{HTTP: srv.Client()}
		return r, srv.URL
	}
	return r, ""
}

func archiveHandler(t *testing.T, body *[]byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(*body) })
}

func TestAddEachKindAndList(t *testing.T) {
	archive := tarGz(t, map[string]string{"top/prompts/p.md": "p\n"})
	r, base := newRegistry(t, archiveHandler(t, &archive))
	repo := gitRepo(t)
	dir := t.TempDir()
	ctx := context.Background()

	g, err := r.Add(ctx, "g", "file://"+repo, "")
	if err != nil || g.Kind != KindGit || g.Commit == "" || g.Source != "file://"+repo {
		t.Fatalf("git add %+v %v", g, err)
	}
	u, err := r.Add(ctx, "u", base+"/x.tar.gz", "")
	if err != nil || u.Kind != KindURL {
		t.Fatalf("url add %+v %v", u, err)
	}
	d, err := r.Add(ctx, "d", dir, "")
	if err != nil || d.Kind != KindDir || d.Source != dir {
		t.Fatalf("dir add %+v %v", d, err)
	}
	list, err := r.List()
	if err != nil || len(list) != 3 {
		t.Fatalf("list %+v %v", list, err)
	}
	if got, err := r.Get("g"); err != nil || got != g {
		t.Fatalf("get %+v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(r.Root(u), "prompts/p.md")); err != nil {
		t.Fatalf("archive not extracted at root: %v", err)
	}
	if r.Root(d) != dir || r.Root(g) != filepath.Join(string(r.Home), "stores", "g") {
		t.Fatalf("roots %q %q", r.Root(d), r.Root(g))
	}
}

func TestAddRejections(t *testing.T) {
	r, _ := newRegistry(t, nil)
	repo := gitRepo(t)
	dir := t.TempDir()
	ctx := context.Background()
	for _, bad := range []string{"", "Bad", "-x", ".x", "a/b", "a b", "../x"} {
		if _, err := r.Add(ctx, bad, dir, ""); err == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
	if _, err := r.Add(ctx, "ok", dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(ctx, "ok", "file://"+repo, ""); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := r.Add(ctx, "withref", dir, "main"); err == nil {
		t.Fatal("--ref on a dir store accepted")
	}
	if _, err := r.Add(ctx, "missing", filepath.Join(dir, "nope"), ""); err == nil {
		t.Fatal("missing directory accepted")
	}
	if _, err := r.Add(ctx, "unreach", "file://"+filepath.Join(t.TempDir(), "no-repo"), ""); err == nil {
		t.Fatal("unreachable git accepted")
	}
	if _, err := r.Add(ctx, "badref", "file://"+repo, "nope"); err == nil {
		t.Fatal("unknown ref accepted")
	}
	list, _ := r.List()
	if len(list) != 1 || list[0].Name != "ok" {
		t.Fatalf("registered after failures: %+v", list)
	}
	for _, n := range []string{"unreach", "badref", "missing"} {
		if _, err := os.Lstat(filepath.Join(string(r.Home), "stores", n)); err == nil {
			t.Errorf("leftover directory %s", n)
		}
	}
}

func TestAddRelativeDirIsStoredAbsolute(t *testing.T) {
	r, _ := newRegistry(t, nil)
	if err := os.MkdirAll(filepath.Join(r.Cwd, "sub/store"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := r.Add(context.Background(), "rel", "sub/store", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(r.Cwd, "sub/store"); s.Source != want {
		t.Fatalf("source %q want %q", s.Source, want)
	}
}

func TestRefreshGitMovesCommit(t *testing.T) {
	r, _ := newRegistry(t, nil)
	repo := gitRepo(t)
	ctx := context.Background()
	s, err := r.Add(ctx, "g", "file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	next := commit(t, repo, "prompts/b.md")
	got, err := r.Refresh(ctx, "g")
	if err != nil || got.Commit != next || got.Commit == s.Commit {
		t.Fatalf("refresh %+v %v", got, err)
	}
	if again, _ := r.Get("g"); again.Commit != next {
		t.Fatalf("registry not updated: %+v", again)
	}
	if _, err := r.Refresh(ctx, "zzz"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestRefreshURLSwapsAndKeepsOldOnFailure(t *testing.T) {
	archive := tarGz(t, map[string]string{"top/prompts/old.md": "o\n"})
	r, base := newRegistry(t, archiveHandler(t, &archive))
	ctx := context.Background()
	s, err := r.Add(ctx, "u", base+"/x.tgz", "")
	if err != nil {
		t.Fatal(err)
	}
	archive = tarGz(t, map[string]string{"top/prompts/new.md": "n\n"})
	if _, err := r.Refresh(ctx, "u"); err != nil {
		t.Fatal(err)
	}
	root := r.Root(s)
	if _, err := os.Stat(filepath.Join(root, "prompts/new.md")); err != nil {
		t.Fatalf("new tree missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "prompts/old.md")); err == nil {
		t.Fatal("old tree still present")
	}
	archive = []byte("not an archive")
	if _, err := r.Refresh(ctx, "u"); err == nil {
		t.Fatal("bad archive accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "prompts/new.md")); err != nil {
		t.Fatalf("old tree lost after failed refresh: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(string(r.Home), "stores"))
	if len(entries) != 1 {
		t.Fatalf("temporary directories left: %v", entries)
	}
}

func TestRefreshDirChangesNothing(t *testing.T) {
	r, _ := newRegistry(t, nil)
	dir := t.TempDir()
	want, _ := r.Add(context.Background(), "d", dir, "")
	got, err := r.Refresh(context.Background(), "d")
	if err != nil || got != want {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestSetRef(t *testing.T) {
	r, _ := newRegistry(t, nil)
	repo := gitRepo(t)
	first := git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "tag", "v1")
	commit(t, repo, "prompts/b.md")
	ctx := context.Background()
	if _, err := r.Add(ctx, "g", "file://"+repo, ""); err != nil {
		t.Fatal(err)
	}
	s, err := r.SetRef(ctx, "g", "v1")
	if err != nil || s.Ref != "v1" || s.Commit != first {
		t.Fatalf("tag %+v %v", s, err)
	}
	if _, err := r.SetRef(ctx, "g", "nope"); err == nil {
		t.Fatal("unknown ref accepted")
	}
	if cur, _ := r.Get("g"); cur != s {
		t.Fatalf("registration changed: %+v", cur)
	}
	s, err = r.SetRef(ctx, "g", "")
	if err != nil || s.Ref != "" || s.Commit == first {
		t.Fatalf("default %+v %v", s, err)
	}
	d, _ := r.Add(ctx, "d", t.TempDir(), "")
	if _, err := r.SetRef(ctx, d.Name, "x"); err == nil {
		t.Fatal("ref on dir store accepted")
	}
}

func TestRemove(t *testing.T) {
	r, _ := newRegistry(t, nil)
	repo := gitRepo(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "keep.txt"), "k")
	ctx := context.Background()
	g, _ := r.Add(ctx, "g", "file://"+repo, "")
	if _, err := r.Add(ctx, "d", dir, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Remove("d"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("dir store content touched: %v", err)
	}
	if err := r.Remove("g"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(r.Root(g)); err == nil {
		t.Fatal("clone not removed")
	}
	if list, _ := r.List(); len(list) != 0 {
		t.Fatalf("still registered: %+v", list)
	}
	if err := r.Remove("g"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestRemoveDoesNotFollowSymlinkOrEscape(t *testing.T) {
	r, _ := newRegistry(t, nil)
	repo := gitRepo(t)
	if _, err := r.Add(context.Background(), "g", "file://"+repo, ""); err != nil {
		t.Fatal(err)
	}
	victim := t.TempDir()
	write(t, filepath.Join(victim, "precious"), "p")
	link := filepath.Join(string(r.Home), "stores", "g")
	if err := os.RemoveAll(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := r.Remove("g"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(victim, "precious")); err != nil {
		t.Fatalf("symlink target touched: %v", err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("symlink left behind")
	}
	// A hand-edited registry with an escaping name is refused.
	write(t, filepath.Join(string(r.Home), "stores.json"), `{"version":1,"stores":[{"name":"../evil","kind":"git","source":"x"}]}`)
	if err := r.Remove("../evil"); err == nil {
		t.Fatal("escaping name removed")
	}
}

func TestParseSelector(t *testing.T) {
	for in, want := range map[string]bool{"a/b": true, "a": false, "a/b/c": false, "/b": false, "a/": false, "": false} {
		sel, ok := ParseSelector(in)
		if ok != want {
			t.Errorf("%q ok=%v", in, ok)
		}
		if ok && (sel.Store != "a" || sel.Name != "b") {
			t.Errorf("%q -> %+v", in, sel)
		}
	}
}

func TestPathLikeSourceNeverBecomesOwnerRepo(t *testing.T) {
	r, _ := newRegistry(t, nil)
	for _, src := range []string{"./nope", "../nope", "~/nope", "/nope/nope"} {
		_, err := r.Add(context.Background(), "x", src, "")
		if err == nil || !strings.Contains(err.Error(), "directory not found") {
			t.Errorf("%q: %v", src, err)
		}
	}
	if list, _ := r.List(); len(list) != 0 {
		t.Fatalf("registered %+v", list)
	}
}

func TestCorruptRegistryFailsAndStaysUntouched(t *testing.T) {
	r, _ := newRegistry(t, nil)
	file := filepath.Join(string(r.Home), "stores.json")
	write(t, file, "{not json")
	if _, err := r.List(); err == nil {
		t.Fatal("List accepted corrupt registry")
	}
	if _, err := r.Add(context.Background(), "d", t.TempDir(), ""); err == nil {
		t.Fatal("Add accepted corrupt registry")
	}
	if b, _ := os.ReadFile(file); string(b) != "{not json" {
		t.Fatalf("registry rewritten: %q", b)
	}
}

func TestConcurrentAddKeepsBoth(t *testing.T) {
	r, _ := newRegistry(t, nil)
	errs := make(chan error, 2)
	for _, n := range []string{"one", "two"} {
		go func() {
			_, err := r.Add(context.Background(), n, t.TempDir(), "")
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if list, _ := r.List(); len(list) != 2 {
		t.Fatalf("%+v", list)
	}
}

func TestFailedGitRefreshKeepsTreeAndRegistration(t *testing.T) {
	r, _ := newRegistry(t, nil)
	repo := gitRepo(t)
	s, err := r.Add(context.Background(), "g", "file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Refresh(context.Background(), "g"); err == nil {
		t.Fatal("refresh of a deleted remote succeeded")
	}
	if _, err := os.Stat(filepath.Join(r.Root(s), "prompts/a.md")); err != nil {
		t.Fatalf("tree lost: %v", err)
	}
	if got, _ := r.Get("g"); got != s {
		t.Fatalf("registration changed: %+v", got)
	}
}

func TestRemoveKeepsTreeWhenSaveFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	r, _ := newRegistry(t, nil)
	s, err := r.Add(context.Background(), "g", "file://"+gitRepo(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(string(r.Home), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(string(r.Home), 0o700) })
	if err := r.Remove("g"); err == nil {
		t.Fatal("remove succeeded with unwritable registry")
	}
	if _, err := os.Stat(filepath.Join(r.Root(s), "prompts/a.md")); err != nil {
		t.Fatalf("tree deleted although registration remains: %v", err)
	}
}

func TestAddSymlinkedDirIsStoredCanonical(t *testing.T) {
	isolate(t)
	real := t.TempDir()
	cwd := t.TempDir()
	if err := os.Symlink(real, filepath.Join(cwd, "link")); err != nil {
		t.Fatal(err)
	}
	reg := Registry{Home: paths.Home(t.TempDir()), Cwd: cwd}
	s, err := reg.Add(context.Background(), "s", "./link", "")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(real)
	if s.Source != want || reg.Root(s) != want {
		t.Fatalf("source %q root %q, want %q", s.Source, reg.Root(s), want)
	}
}

func TestAddArchiveWithOnlyLayoutDirKeepsIt(t *testing.T) {
	for _, top := range []string{"skills", "images", "prompts", "workflows"} {
		t.Run(top, func(t *testing.T) {
			body := tarGz(t, map[string]string{top + "/x/SKILL.md": "s"})
			r, srvURL := newRegistry(t, archiveHandler(t, &body))
			s, err := r.Add(context.Background(), "a", srvURL+"/s.tar.gz", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(r.Root(s), top, "x", "SKILL.md")); err != nil {
				t.Fatalf("%s/ was stripped: %v", top, err)
			}
		})
	}
	body := tarGz(t, map[string]string{"repo-main/skills/x/SKILL.md": "s"})
	r, srvURL := newRegistry(t, archiveHandler(t, &body))
	s, err := r.Add(context.Background(), "a", srvURL+"/s.tar.gz", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.Root(s), "skills", "x", "SKILL.md")); err != nil {
		t.Fatalf("top directory was not stripped: %v", err)
	}
}

func TestAddSourceKindMessages(t *testing.T) {
	isolate(t)
	r := Registry{Home: paths.Home(t.TempDir()), Cwd: t.TempDir()}
	ctx := context.Background()
	for source, want := range map[string]string{
		"not a store":                          "want a git repository",
		"http://example.com/s.git":             "https is required",
		"http://example.com/s.tar.gz":          "https is required",
		"https://example.com/s.tar.gz?token=1": "archive URL must not carry a query",
		"https://example.com/s.zip#frag":       "archive URL must not carry a query",
	} {
		_, err := r.Add(ctx, "s", source, "")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", source, err, want)
		}
		if err != nil && strings.Contains(err.Error(), "token=1") {
			t.Errorf("%q: error echoes the query: %v", source, err)
		}
	}
}
