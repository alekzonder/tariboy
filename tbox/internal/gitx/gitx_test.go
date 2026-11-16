package gitx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func write(t *testing.T, root, rel, body string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	repo          string
	first, second string
	devTip        string
}

// newRepo makes a repository with main (two commits, tag v1 on the first)
// and a branch dev one commit ahead of main.
func newRepo(t *testing.T) fixture {
	t.Helper()
	isolate(t)
	repo := filepath.Join(t.TempDir(), "origin")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "README.md", "one", 0o644)
	write(t, repo, "skills/a/SKILL.md", "skill a", 0o644)
	write(t, repo, "skills/a/run.sh", "#!/bin/sh", 0o755)
	write(t, repo, "skills/b/SKILL.md", "skill b", 0o644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "first")
	f := fixture{repo: repo, first: git(t, repo, "rev-parse", "HEAD")}
	git(t, repo, "tag", "-a", "-m", "v1", "v1")
	write(t, repo, "README.md", "two", 0o644)
	git(t, repo, "commit", "-q", "-am", "second")
	f.second = git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "checkout", "-q", "-b", "dev")
	write(t, repo, "dev.txt", "dev", 0o644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "dev")
	f.devTip = git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "checkout", "-q", "main")
	return f
}

func local() Git { return Git{AllowLocal: true, Timeout: 30 * time.Second} }

func TestNormalizeURL(t *testing.T) {
	ok := map[string]string{
		"https://github.com/a/b.git": "https://github.com/a/b.git",
		"git@github.com:a/b.git":     "git@github.com:a/b.git",
		"ssh://git@h/a/b":            "ssh://git@h/a/b",
		"a/b":                        "https://github.com/a/b.git",
		"a/b.git":                    "https://github.com/a/b.git",
		"  a/b  ":                    "https://github.com/a/b.git",
	}
	for in, want := range ok {
		if got, err := NormalizeURL(in, false); err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "https://u:p@h/a", "https://u@h/a", "ssh://u:p@h/a", "http://h/a", "file:///x",
		"ext::sh -c x", "--upload-pack=x", "-x/y", "/abs/path", "./rel", "../x/y", "a b/c",
		"ssh://-oProxyCommand=x/a", "git@-h:a/b", "ftp://h/a", "https:///a", "x::y",
		"git@h:../x", "./b",
	}
	for _, in := range bad {
		if got, err := NormalizeURL(in, false); err == nil {
			t.Errorf("NormalizeURL(%q) = %q, want error", in, got)
		}
	}
	for _, in := range []string{"/abs/path", "file:///x", "http://h/a"} {
		if _, err := NormalizeURL(in, true); err != nil {
			t.Errorf("allowLocal NormalizeURL(%q): %v", in, err)
		}
	}
	for _, in := range []string{"https://u:p@h/a", "ext::sh -c x", "-x"} {
		if _, err := NormalizeURL(in, true); err == nil {
			t.Errorf("allowLocal NormalizeURL(%q) accepted", in)
		}
	}
}

func TestCheckRef(t *testing.T) {
	for _, ok := range []string{"main", "v1.2.3", "feature/x", "0123abc", strings.Repeat("a", 255)} {
		if err := CheckRef(ok); err != nil {
			t.Errorf("CheckRef(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "-x", "a..b", "a b", "a\tb", "a\x00b", "a\nb", "a\x7fb", strings.Repeat("a", 256), "a:b", "a^b", "a@{1}"} {
		if err := CheckRef(bad); err == nil {
			t.Errorf("CheckRef(%q) accepted", bad)
		}
	}
}

func TestMirrorResolve(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	mirror := filepath.Join(t.TempDir(), "m")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"": f.second, "main": f.second, "dev": f.devTip, "v1": f.first, f.first[:7]: f.first, f.devTip: f.devTip}
	for ref, want := range cases {
		if got, err := g.Resolve(ctx, mirror, ref); err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", ref, got, err, want)
		}
	}
	for _, ref := range []string{"nope", "deadbeef"} {
		if _, err := g.Resolve(ctx, mirror, ref); !errors.Is(err, ErrRefNotFound) {
			t.Errorf("Resolve(%q) err = %v, want ErrRefNotFound", ref, err)
		}
	}
	if _, err := g.Resolve(ctx, mirror, "-x"); err == nil || errors.Is(err, ErrRefNotFound) {
		t.Errorf("Resolve(-x) err = %v, want a ref error", err)
	}
}

func TestMirrorUpdates(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	mirror := filepath.Join(t.TempDir(), "m")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	git(t, f.repo, "branch", "-D", "dev")
	write(t, f.repo, "new.txt", "n", 0o644)
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-q", "-m", "third")
	third := git(t, f.repo, "rev-parse", "HEAD")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	if got, err := g.Resolve(ctx, mirror, "main"); err != nil || got != third {
		t.Errorf("after update Resolve(main) = %q, %v; want %q", got, err, third)
	}
	if _, err := g.Resolve(ctx, mirror, "dev"); !errors.Is(err, ErrRefNotFound) {
		t.Errorf("pruned branch still resolves: %v", err)
	}
}

func TestMirrorMissingRemote(t *testing.T) {
	isolate(t)
	g := local()
	dir := filepath.Join(t.TempDir(), "m")
	start := time.Now()
	err := g.Mirror(context.Background(), filepath.Join(t.TempDir(), "absent"), dir)
	if err == nil {
		t.Fatal("Mirror of a missing remote succeeded")
	}
	if time.Since(start) > 20*time.Second {
		t.Error("failure took too long")
	}
	if _, serr := os.Lstat(dir); serr == nil {
		t.Error("mirror dir left behind")
	}
	if err := (Git{}).Mirror(context.Background(), "https://user:secret@example.invalid/a/b", dir); err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("user-info URL: %v", err)
	}
}

func TestMaskCredentials(t *testing.T) {
	got := maskCredentials("fatal: unable to access 'https://user:tok@host/a.git/': nope")
	if strings.Contains(got, "tok") || strings.Contains(got, "user") {
		t.Errorf("masked = %q", got)
	}
}

func TestTimeout(t *testing.T) {
	isolate(t)
	g := Git{AllowLocal: true, Timeout: time.Nanosecond}
	err := g.Mirror(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "m"))
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
}

func TestExport(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	mirror := filepath.Join(t.TempDir(), "m")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := g.Export(ctx, mirror, f.second, "skills/a", dest); err != nil {
		t.Fatal(err)
	}
	var names []string
	filepath.WalkDir(dest, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dest, p)
			names = append(names, rel)
		}
		return err
	})
	if strings.Join(names, ",") != "SKILL.md,run.sh" {
		t.Errorf("exported files = %v", names)
	}
	if fi, err := os.Stat(filepath.Join(dest, "run.sh")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("run.sh mode = %v, %v", fi, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "SKILL.md")); string(b) != "skill a" {
		t.Errorf("SKILL.md = %q", b)
	}

	full := filepath.Join(t.TempDir(), "full")
	if err := g.Export(ctx, mirror, f.first, "", full); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(full, "README.md")); string(b) != "one" {
		t.Errorf("README.md at first = %q", b)
	}
}

func TestExportRejects(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	mirror := filepath.Join(t.TempDir(), "m")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	for _, sub := range []string{"../x", "a/../../x", "/etc", "skills/nope"} {
		dest := filepath.Join(parent, "o")
		if err := g.Export(ctx, mirror, f.second, sub, dest); err == nil {
			t.Errorf("Export subpath %q accepted", sub)
		}
		if _, err := os.Lstat(dest); err == nil {
			t.Errorf("Export subpath %q left dest", sub)
		}
	}
	if err := g.Export(ctx, mirror, "main", "", filepath.Join(parent, "o")); err == nil {
		t.Error("non-hex commit accepted")
	}
	existing := t.TempDir()
	if err := g.Export(ctx, mirror, f.second, "", existing); err == nil {
		t.Error("existing dest accepted")
	}
}

func TestExportFile(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	mirror := filepath.Join(t.TempDir(), "m")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	if b, err := g.ExportFile(ctx, mirror, f.second, "skills/a/SKILL.md", 100); err != nil || string(b) != "skill a" {
		t.Errorf("ExportFile = %q, %v", b, err)
	}
	if _, err := g.ExportFile(ctx, mirror, f.second, "skills/a/SKILL.md", 3); err == nil {
		t.Error("over-cap file accepted")
	}
	if _, err := g.ExportFile(ctx, mirror, f.second, "../x", 100); err == nil {
		t.Error("escaping path accepted")
	}
	if _, err := g.ExportFile(ctx, mirror, f.second, "missing", 100); err == nil {
		t.Error("missing file accepted")
	}
}

func TestClonePull(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	commit, err := g.Clone(ctx, f.repo, "v1", dir)
	if err != nil || commit != f.first {
		t.Fatalf("Clone = %q, %v; want %q", commit, err, f.first)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(b) != "one" {
		t.Errorf("README.md = %q", b)
	}
	if got := git(t, dir, "rev-parse", "HEAD"); got != f.first {
		t.Errorf("HEAD = %s", got)
	}
	if commit, err = g.Pull(ctx, dir, "dev"); err != nil || commit != f.devTip {
		t.Fatalf("Pull dev = %q, %v", commit, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dev.txt")); err != nil {
		t.Errorf("dev.txt missing: %v", err)
	}
	// A new upstream commit on main is picked up by name.
	write(t, f.repo, "up.txt", "u", 0o644)
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-q", "-m", "up")
	up := git(t, f.repo, "rev-parse", "HEAD")
	if commit, err = g.Pull(ctx, dir, ""); err != nil || commit != up {
		t.Errorf("Pull default = %q, %v; want %q", commit, err, up)
	}
	if commit, err = g.Pull(ctx, dir, f.second[:8]); err != nil || commit != f.second {
		t.Errorf("Pull commit id = %q, %v", commit, err)
	}
	if _, err = g.Pull(ctx, dir, "nope"); !errors.Is(err, ErrRefNotFound) {
		t.Errorf("Pull nope err = %v", err)
	}
}

func TestPullDirtyTree(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	if _, err := g.Clone(ctx, f.repo, "main", dir); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "README.md", "local edit", 0o644)
	if _, err := g.Pull(ctx, dir, "v1"); err == nil {
		t.Fatal("Pull on a dirty tree succeeded")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(b) != "local edit" {
		t.Errorf("README.md = %q", b)
	}
	if _, err := g.Pull(ctx, t.TempDir(), ""); err == nil {
		t.Error("Pull on a non-checkout succeeded")
	}
}

func TestCloneFailures(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	if _, err := g.Clone(ctx, f.repo, "nope", dir); !errors.Is(err, ErrRefNotFound) {
		t.Errorf("Clone nope err = %v", err)
	}
	if _, err := os.Lstat(dir); err == nil {
		t.Error("failed clone left dir")
	}
	if _, err := g.Clone(ctx, f.repo, "-x", dir); err == nil {
		t.Error("bad ref accepted")
	}
}

func TestMirrorFollowsRenamedDefaultBranch(t *testing.T) {
	f := newRepo(t)
	g := local()
	ctx := context.Background()
	mirror := filepath.Join(t.TempDir(), "m")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	git(t, f.repo, "checkout", "-q", "-b", "trunk")
	write(t, f.repo, "t.txt", "t", 0o644)
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-q", "-m", "trunk")
	trunk := git(t, f.repo, "rev-parse", "HEAD")
	git(t, f.repo, "branch", "-D", "main")
	if err := g.Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	if got, err := g.Resolve(ctx, mirror, ""); err != nil || got != trunk {
		t.Errorf("Resolve(\"\") = %q, %v; want %q", got, err, trunk)
	}
}

func TestResolveBrokenMirrorIsNotRefNotFound(t *testing.T) {
	isolate(t)
	_, err := local().Resolve(context.Background(), t.TempDir(), "main")
	if err == nil || errors.Is(err, ErrRefNotFound) {
		t.Errorf("err = %v, want a non-ErrRefNotFound error", err)
	}
	_, err = local().Resolve(context.Background(), t.TempDir(), "")
	if err == nil || errors.Is(err, ErrRefNotFound) {
		t.Errorf("default err = %v, want a non-ErrRefNotFound error", err)
	}
}

func TestNormalizeURLRejectsQueryAndFragment(t *testing.T) {
	for _, in := range []string{"https://h/a/b.git?token=x", "https://h/a/b.git#frag", "ssh://git@h/a?x=1", "git@h:a/b.git?x", "git@h:a/b#f", "https://h/a?"} {
		if got, err := NormalizeURL(in, false); err == nil {
			t.Errorf("NormalizeURL(%q) = %q, want error", in, got)
		}
	}
}

func TestGitErrorKeepsOneMaskedLine(t *testing.T) {
	stderr := []byte("Cloning into 'x'...\nfatal: unable to access 'https://user:tok@host/a/': " + strings.Repeat("é", 2000) + "\n\n")
	err := gitError(context.Background(), "clone", errors.New("exit"), stderr)
	msg := err.Error()
	if strings.Contains(msg, "tok") || strings.Contains(msg, "Cloning") || !utf8.ValidString(msg) || len(msg) > 400 {
		t.Errorf("error = %q", msg)
	}
}

func TestDisplayAddressHidesCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://alice:s3cr3t-token@host/x/y.git":   "https://host/x/y.git",
		"https://s3cr3t-token@host/x/y.git?a=b#c":   "https://host/x/y.git",
		"https://host/x.tar.gz?token=s3cr3t-token":  "https://host/x.tar.gz",
		"git@github.com:o/r.git":                    "git@github.com:o/r.git",
		"alice:s3cr3t-token@host:o/r.git":           "host:o/r.git",
		"owner/repo":                                "owner/repo",
		"https://alice:s3cr3t-token@host/%zz?x=1#f": "https://host/%zz",
	} {
		if got := DisplayAddress(in); got != want {
			t.Errorf("DisplayAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMirrorFetchesOnlyBranchesAndTags(t *testing.T) {
	f := newRepo(t)
	git(t, f.repo, "update-ref", "refs/pull/1/head", f.devTip)
	mirror := filepath.Join(t.TempDir(), "m")
	if err := local().Mirror(context.Background(), f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	refs := git(t, mirror, "--git-dir="+mirror, "for-each-ref", "--format=%(refname)")
	if strings.Contains(refs, "refs/pull/") || !strings.Contains(refs, "refs/heads/dev") || !strings.Contains(refs, "refs/tags/v1") {
		t.Fatalf("mirror refs:\n%s", refs)
	}
	if got, err := local().Resolve(context.Background(), mirror, ""); err != nil || got != f.second {
		t.Fatalf("default branch %q %v", got, err)
	}
}

func TestMirrorRepairsBrokenDirectory(t *testing.T) {
	f := newRepo(t)
	mirror := filepath.Join(t.TempDir(), "m")
	write(t, mirror, "junk", "not a repository", 0o644)
	if err := local().Mirror(context.Background(), f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	if got, err := local().Resolve(context.Background(), mirror, "main"); err != nil || got != f.second {
		t.Fatalf("Resolve %q %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(mirror))
	if len(entries) != 1 {
		t.Fatalf("leftovers beside the mirror: %v", entries)
	}
}

func TestMirrorFailureLeavesNothing(t *testing.T) {
	isolate(t)
	parent := t.TempDir()
	if err := local().Mirror(context.Background(), filepath.Join(t.TempDir(), "absent"), filepath.Join(parent, "m")); err == nil {
		t.Fatal("mirror of a missing remote succeeded")
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestTimeoutFromEnvironment(t *testing.T) {
	t.Setenv("TBOX_GIT_TIMEOUT", "7")
	if got := (Git{}).timeout(); got != 7*time.Second {
		t.Fatalf("timeout %v", got)
	}
	if got := (Git{Timeout: time.Second}).timeout(); got != time.Second {
		t.Fatalf("explicit timeout %v", got)
	}
	for _, bad := range []string{"", "x", "0", "-3"} {
		t.Setenv("TBOX_GIT_TIMEOUT", bad)
		if got := (Git{}).timeout(); got != 120*time.Second {
			t.Fatalf("TBOX_GIT_TIMEOUT=%q gives %v", bad, got)
		}
	}
}

func TestHasCommit(t *testing.T) {
	f := newRepo(t)
	mirror := filepath.Join(t.TempDir(), "m")
	ctx := context.Background()
	if local().HasCommit(ctx, mirror, f.first) {
		t.Fatal("missing mirror has a commit")
	}
	if err := local().Mirror(ctx, f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	if !local().HasCommit(ctx, mirror, f.first) {
		t.Fatal("mirror lacks the first commit")
	}
	if local().HasCommit(ctx, mirror, strings.Repeat("ab", 20)) {
		t.Fatal("mirror has an unknown commit")
	}
}

func TestExportIgnoresUserGitConfig(t *testing.T) {
	f := newRepo(t)
	write(t, f.repo, "skills/a/notes.txt", "one\ntwo\n", 0o644)
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-q", "-m", "notes")
	commit := git(t, f.repo, "rev-parse", "HEAD")
	attrs := filepath.Join(t.TempDir(), "attributes")
	write(t, filepath.Dir(attrs), "attributes", "* text eol=crlf\n", 0o644)
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	write(t, filepath.Dir(cfg), "gitconfig", "[core]\n\tautocrlf = true\n\teol = crlf\n\tattributesFile = "+attrs+"\n", 0o644)
	mirror := filepath.Join(t.TempDir(), "m")
	if err := local().Mirror(context.Background(), f.repo, mirror); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	dest := filepath.Join(t.TempDir(), "out")
	if err := local().Export(context.Background(), mirror, commit, "skills/a", dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "notes.txt")); string(b) != "one\ntwo\n" {
		t.Fatalf("notes.txt = %q", b)
	}
}
