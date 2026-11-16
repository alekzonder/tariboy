package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
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

func tagDigests(t *testing.T, env remoteEnv) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tag := range []string{"1.0.0", "latest"} {
		d, _, err := env.store.Resolve(imgstore.Ref{Name: "demo", Tag: tag})
		if err != nil {
			t.Fatal(err)
		}
		out[tag] = d
	}
	return out
}

func TestUpdateWithUnreachableRemoteChangesNothing(t *testing.T) {
	env := newRemoteEnv(t, "")
	if _, err := Build(env.opts); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(env.dir, lock.Filename))
	tags := tagDigests(t, env)
	os.RemoveAll(env.repo)
	env.opts.Update = true
	env.opts.Resolver.Git.Timeout = 5 * time.Second
	if _, err := Build(env.opts); err == nil || !strings.Contains(err.Error(), env.repo) {
		t.Fatalf("want error naming the address, got %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(env.dir, lock.Filename))
	if !bytes.Equal(before, after) || !maps.Equal(tags, tagDigests(t, env)) {
		t.Fatal("failed --update changed the lock or the tags")
	}
}

func TestLockFollowsTariboyfileChanges(t *testing.T) {
	env := newRemoteEnv(t, "")
	if _, err := Build(env.opts); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`schema_version: 3
image_version: 1.0.0
skills:
  - git: %s
    path: skills/gamma
    ref: main
  - dir: ./skills/beta
prompts:
  - file: ./prompt.md
`, env.repo)
	write(t, filepath.Join(env.dir, "Tariboyfile.yaml"), body)
	res, err := Build(env.opts)
	if err != nil || !res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	lk := readLock(t, env.dir)
	want := lock.Key("git", env.repo, "skills/gamma", "main")
	if _, ok := lk.Entries[want]; !ok || len(lk.Entries) != 1 {
		t.Fatalf("lock keys %v, want only %s", lk.Entries, want)
	}
}

func TestReadOnlySourceKeepsLockInHome(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only directory")
	}
	env := newRemoteEnv(t, "")
	before, _ := os.ReadDir(env.dir)
	if err := os.Chmod(env.dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(env.dir, 0o755) })
	if _, err := Build(env.opts); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadDir(env.dir)
	if len(after) != len(before) {
		t.Fatalf("source directory changed: %v", after)
	}
	matches, _ := filepath.Glob(filepath.Join(string(env.opts.Resolver.Home), "locks", "*.json"))
	if len(matches) != 1 {
		t.Fatalf("locks %v", matches)
	}
}

func TestCorruptedCacheIsRepairedOrFailsNamingEntry(t *testing.T) {
	env := newRemoteEnv(t, "")
	first, err := Build(env.opts)
	if err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(string(env.opts.Resolver.Home), "cache", "sources", "*", "gamma", "SKILL.md"))
	if len(matches) != 1 {
		t.Fatalf("cache skill %v", matches)
	}
	if err := os.WriteFile(matches[0], []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(env.dir, "prompt.md"), "changed prompt\n")
	second, err := Build(env.opts)
	if err != nil || !second.Built {
		t.Fatalf("repair %+v %v", second, err)
	}
	img, err := env.store.Open(imgstore.Ref{Name: "demo", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	_, members, _ := img.Skill("gamma")
	for _, m := range members {
		if strings.Contains(string(m.Body), "tampered") {
			t.Fatal("packaged tampered content")
		}
	}
	_ = first
	// Remote gone: the local mirror still repairs a further tampering.
	if err := os.WriteFile(matches[0], []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(env.repo)
	write(t, filepath.Join(env.dir, "prompt.md"), "changed once more\n")
	if res, err := Build(env.opts); err != nil || !res.Built {
		t.Fatalf("repair from the mirror %+v %v", res, err)
	}
	// Remote and mirror gone: a further tampering must fail naming the entry.
	if err := os.WriteFile(matches[0], []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(string(env.opts.Resolver.Home), "cache", "git"))
	write(t, filepath.Join(env.dir, "prompt.md"), "changed again\n")
	env.opts.Resolver.Git.Timeout = 5 * time.Second
	if _, err := Build(env.opts); err == nil || !strings.Contains(err.Error(), env.repo) {
		t.Fatalf("want error naming %s, got %v", env.repo, err)
	}
}

func TestAssembleRejectsHashMismatch(t *testing.T) {
	env := newRemoteEnv(t, "")
	if _, err := Build(env.opts); err != nil {
		t.Fatal(err)
	}
	j, err := newJob(env.opts)
	if err != nil {
		t.Fatal(err)
	}
	defer j.close()
	rs, err := resolve(context.Background(), env.opts.Resolver, j.f, j.lock, false)
	if err != nil {
		t.Fatal(err)
	}
	for k, e := range rs.lock.Entries {
		e.SHA256 = strings.Repeat("0", 64)
		rs.lock.Entries[k] = e
	}
	if _, err := assemble(j.f, "demo", j.dir, time.Now(), rs); err == nil || !strings.Contains(err.Error(), env.repo) {
		t.Fatalf("want mismatch error naming %s, got %v", env.repo, err)
	}
}

var updateCompat = flag.Bool("update-compat", false, "regenerate testdata/compat/v3-remote.tar.gz")

// TestRemoteArchiveCompat keeps the committed archive of a source with a git
// skill and a git prompt equal to a fresh build; the root module validates
// the committed file with tariboy's portable image contract.
func TestRemoteArchiveCompat(t *testing.T) {
	t.Setenv("GIT_AUTHOR_DATE", "2026-11-16T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-11-16T00:00:00Z")
	env := newRemoteEnv(t, "")
	const logical = "https://github.com/acme/skills"
	body := fmt.Sprintf("schema_version: 3\nimage_version: 1.0.0\nskills:\n  - git: %s\n    path: skills/gamma\nprompts:\n  - git: %s\n    path: docs/review.md\n", logical, logical)
	write(t, filepath.Join(env.dir, "Tariboyfile.yaml"), body)
	env.opts.Resolver.RewriteURL = func(u string) string {
		if u == logical {
			return env.repo
		}
		return u
	}
	if _, err := Build(env.opts); err != nil {
		t.Fatal(err)
	}
	img, err := env.store.Open(imgstore.Ref{Name: "demo", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Manifest.Skills[0].Category; got != "source" {
		t.Fatalf("category %q", got)
	}
	var buf bytes.Buffer
	if err := img.Write(&buf); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "testdata", "compat", "v3-remote.tar.gz")
	if *updateCompat {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := archive.Read(bytes.NewReader(committed))
	if err != nil || got.Manifest.ID != img.Manifest.ID {
		t.Fatalf("committed archive differs from a fresh build (run go test ./internal/build -update-compat): %v", err)
	}
}

func TestRemoteBuildGitRootSkill(t *testing.T) {
	repo := newRemote(t)
	rootRepo := filepath.Join(t.TempDir(), "rootskill")
	write(t, filepath.Join(rootRepo, "SKILL.md"), skillBody("rootish", "one"))
	gitIn(t, rootRepo, "init", "-q", "-b", "main")
	gitIn(t, rootRepo, "add", ".")
	gitIn(t, rootRepo, "commit", "-q", "-m", "first")
	_ = repo
	dir := source(t, fmt.Sprintf("schema_version: 3\nimage_version: 1.0.0\nskills:\n  - git: %s\n    path: .\n    ref: main\n", rootRepo))
	o := opts(dir, imgstore.Store{Dir: t.TempDir()})
	o.Resolver = &srcpkg.Resolver{Home: paths.Home(t.TempDir()), Git: gitx.Git{AllowLocal: true, Timeout: 30 * time.Second}}
	res, err := Build(o)
	if err != nil || !res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	img, err := o.Store.Open(imgstore.Ref{Name: res.Name, Tag: "latest"})
	if err != nil || len(img.Manifest.Skills) != 1 || img.Manifest.Skills[0].Name != "rootish" {
		t.Fatalf("%v %+v", err, img.Manifest.Skills)
	}
}
