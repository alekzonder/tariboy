package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	srcpkg "github.com/alekzonder/tariboy/tbox/internal/source"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRemote(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	repo := filepath.Join(t.TempDir(), "origin")
	write(t, filepath.Join(repo, "skills/gamma/SKILL.md"), skillBody("gamma", "one"))
	write(t, filepath.Join(repo, "docs/review.md"), "review one\n")
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "first")
	return repo
}

func advance(t *testing.T, repo string) string {
	t.Helper()
	write(t, filepath.Join(repo, "skills/gamma/SKILL.md"), skillBody("gamma", "two"))
	gitIn(t, repo, "commit", "-q", "-am", "second")
	return gitIn(t, repo, "rev-parse", "HEAD")
}

func tarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := skillBody("wolf", "wolf")
	tw.WriteHeader(&tar.Header{Name: "wolf/SKILL.md", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write([]byte(body))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

type remoteEnv struct {
	repo   string
	srvURL string
	dir    string
	store  imgstore.Store
	opts   Options
}

func newRemoteEnv(t *testing.T, extra string) remoteEnv {
	t.Helper()
	repo := newRemote(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(tarball(t)) }))
	t.Cleanup(srv.Close)
	body := fmt.Sprintf(`schema_version: 3
image_version: 1.0.0
skills:
  - git: %s
    path: skills/gamma
  - url: %s/wolf.tar.gz
  - dir: ./skills/beta
prompts:
  - file: ./prompt.md
  - git: %s
    path: docs/review.md
%s`, repo, srv.URL, repo, extra)
	dir := source(t, body)
	store := imgstore.Store{Dir: t.TempDir()}
	o := opts(dir, store)
	o.Resolver = &srcpkg.Resolver{
		Home: paths.Home(t.TempDir()),
		Git:  gitx.Git{AllowLocal: true, Timeout: 30 * time.Second},
		HTTP: fetch.Client{HTTP: srv.Client()},
	}
	return remoteEnv{repo: repo, srvURL: srv.URL, dir: dir, store: store, opts: o}
}

func readLock(t *testing.T, dir string) lock.File {
	t.Helper()
	f, err := lock.Read(filepath.Join(dir, lock.Filename))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func commits(f lock.File) []string {
	var out []string
	for _, e := range f.Entries {
		if e.Kind == "git" {
			out = append(out, e.Commit)
		}
	}
	slices.Sort(out)
	return out
}

func TestRemoteBuildWritesLockAndStaysUpToDate(t *testing.T) {
	env := newRemoteEnv(t, "")
	res, err := Build(env.opts)
	if err != nil || !res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	lk := readLock(t, env.dir)
	var git, url int
	for _, e := range lk.Entries {
		switch e.Kind {
		case "git":
			git++
		case "url":
			url++
		}
	}
	if git != 2 || url != 1 {
		t.Fatalf("lock %+v", lk)
	}
	if err := os.RemoveAll(env.repo); err != nil {
		t.Fatal(err)
	}
	env.opts.Resolver.Git.Timeout = 5 * time.Second
	res2, err := Build(env.opts)
	if err != nil || res2.Built || res2.Digest != res.Digest {
		t.Fatalf("second build %+v %v", res2, err)
	}
}

func TestRemoteNewCommitNeedsUpdate(t *testing.T) {
	env := newRemoteEnv(t, "")
	first, err := Build(env.opts)
	if err != nil {
		t.Fatal(err)
	}
	oldCommits := commits(readLock(t, env.dir))
	newCommit := advance(t, env.repo)
	res, err := Build(env.opts)
	if err != nil || res.Built {
		t.Fatalf("without --update %+v %v", res, err)
	}
	env.opts.Update = true
	res, err = Build(env.opts)
	if err != nil || !res.Built || res.Digest == first.Digest {
		t.Fatalf("with --update %+v %v", res, err)
	}
	slices.Sort(res.Moved)
	if !slices.Equal(res.Moved, []string{"1.0.0", "latest"}) {
		t.Fatalf("moved %v", res.Moved)
	}
	if got := commits(readLock(t, env.dir)); slices.Equal(got, oldCommits) || !slices.Contains(got, newCommit) {
		t.Fatalf("lock commits %v, old %v, new %s", got, oldCommits, newCommit)
	}
	if len(res.Updates) != 2 || res.Updates[0].Old == res.Updates[0].New {
		t.Fatalf("updates %+v", res.Updates)
	}
	env.opts.Update = false
	if res, err = Build(env.opts); err != nil || res.Built {
		t.Fatalf("after update %+v %v", res, err)
	}
}

func TestRemoteUpdateWithoutChangeIsUpToDate(t *testing.T) {
	env := newRemoteEnv(t, "")
	if _, err := Build(env.opts); err != nil {
		t.Fatal(err)
	}
	env.opts.Update = true
	res, err := Build(env.opts)
	if err != nil || res.Built || len(res.Updates) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestRemoteTariboyOnlyEntryNeverResolved(t *testing.T) {
	dead := filepath.Join(t.TempDir(), "gone")
	env := newRemoteEnv(t, fmt.Sprintf("  - git: %s\n    path: nope.md\n    only: tariboy\n", dead))
	res, err := Build(env.opts)
	if err != nil || !res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	if len(readLock(t, env.dir).Entries) != 3 {
		t.Fatalf("lock holds an only: tariboy entry")
	}
}

func TestRemoteNilResolverNamesEntry(t *testing.T) {
	env := newRemoteEnv(t, "")
	env.opts.Resolver = nil
	_, err := Build(env.opts)
	if err == nil || !strings.Contains(err.Error(), env.repo) {
		t.Fatalf("want error naming %s, got %v", env.repo, err)
	}
}

func TestRemoteLockPresentCacheMissingRemoteGone(t *testing.T) {
	env := newRemoteEnv(t, "")
	if _, err := Build(env.opts); err != nil {
		t.Fatal(err)
	}
	// A new store has no fingerprint, so the build must resolve from the
	// lock; the cache and the remote are gone.
	env.opts.Store = imgstore.Store{Dir: t.TempDir()}
	env.opts.Resolver.Home = paths.Home(t.TempDir())
	env.opts.Resolver.Git.Timeout = 5 * time.Second
	if err := os.RemoveAll(env.repo); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := Build(env.opts)
	if err == nil || !strings.Contains(err.Error(), env.repo) {
		t.Fatalf("want error naming %s, got %v", env.repo, err)
	}
	if time.Since(start) > 20*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
}
