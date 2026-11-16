package skillslock

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/source"
)

const fixture = "../../testdata/compat/skills-lock"

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		write(t, filepath.Join(to, rel), string(body))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestHashMatchesTariboy pins Hash to the computedHash that tariboy's
// stores.SkillHash produced for the fixture. .git and node_modules cannot be
// committed, so the test adds them to a copy: both are skipped.
func TestHashMatchesTariboy(t *testing.T) {
	want, err := os.ReadFile(filepath.Join(fixture, "computedHash"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "skill")
	copyTree(t, filepath.Join(fixture, "skill"), dir)
	write(t, filepath.Join(dir, ".git", "HEAD"), "skip\n")
	write(t, filepath.Join(dir, "node_modules", "x", "i.js"), "skip\n")
	got, err := Hash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSpace(string(want)) {
		t.Fatalf("hash %s, tariboy %s", got, want)
	}
}

func TestHashRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "SKILL.md"), "x")
	if err := os.Symlink("/etc/hostname", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Hash(dir); err == nil || !strings.Contains(err.Error(), "link") {
		t.Fatalf("err %v", err)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, Filename)
	write(t, p, `{"version":1,"skills":{"a":{"source":"o/r","sourceType":"github","skillPath":"skills/a/SKILL.md","computedHash":"h"},"b":{"source":"../b","sourceType":"local","computedHash":"g"}}}`)
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Skill{
		"a": {Source: "o/r", SourceType: "github", SkillPath: "skills/a/SKILL.md", ComputedHash: "h"},
		"b": {Source: "../b", SourceType: "local", ComputedHash: "g"},
	}
	if len(got) != 2 || got["a"] != want["a"] || got["b"] != want["b"] {
		t.Fatalf("got %+v", got)
	}
	if got, err := Read(filepath.Join(dir, "missing.json")); err != nil || got != nil {
		t.Fatalf("missing: %v %v", got, err)
	}
	write(t, p, `{"version":2,"skills":{}}`)
	if _, err := Read(p); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Fatalf("version err %v", err)
	}
	write(t, p, `{"version":1,"skills":{"../x":{"source":"o/r","sourceType":"github"}}}`)
	if _, err := Read(p); err == nil || !strings.Contains(err.Error(), `"../x"`) {
		t.Fatalf("name err %v", err)
	}
}

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

func skillMD(name string) string {
	return "---\nname: " + name + "\ndescription: Skill " + name + "\n---\nbody\n"
}

// newRepo makes a repository with skill alpha under skills/alpha and returns
// it with a resolver whose RewriteURL serves it for github.com/o/r.
func newRepo(t *testing.T) (string, *source.Resolver) {
	t.Helper()
	isolate(t)
	repo := filepath.Join(t.TempDir(), "origin")
	write(t, filepath.Join(repo, "skills/alpha/SKILL.md"), skillMD("alpha"))
	write(t, filepath.Join(repo, "skills/alpha/notes.md"), "notes\n")
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "first")
	r := &source.Resolver{
		Home: paths.Home(t.TempDir()),
		Git:  gitx.Git{AllowLocal: true, Timeout: 30 * time.Second},
		RewriteURL: func(u string) string {
			if u == "https://github.com/o/r.git" {
				return repo
			}
			return u
		},
	}
	return repo, r
}

func TestRestoreGithubAndLocal(t *testing.T) {
	repo, r := newRepo(t)
	root := t.TempDir()
	lockDir := filepath.Join(root, "images", "img")
	write(t, filepath.Join(lockDir, Filename), "{}")
	write(t, filepath.Join(root, "skills/beta/SKILL.md"), skillMD("beta"))
	betaHash, err := Hash(filepath.Join(root, "skills/beta"))
	if err != nil {
		t.Fatal(err)
	}
	skills := map[string]Skill{
		"alpha": {Source: "o/r", SourceType: "github", SkillPath: "skills/alpha/SKILL.md", ComputedHash: "wrong"},
		"beta":  {Source: "../../skills/beta", SourceType: "local", ComputedHash: betaHash},
	}
	got, err := Restore(context.Background(), r, lockDir, skills, nil)
	if err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(got.Dirs["alpha"], "notes.md")); err != nil || string(body) != "notes\n" {
		t.Fatalf("alpha dir %q: %q %v", got.Dirs["alpha"], body, err)
	}
	if want := filepath.Join(root, "skills", "beta"); got.Dirs["beta"] != want {
		t.Fatalf("beta dir %q, want %q", got.Dirs["beta"], want)
	}
	if !slices.Equal(got.Warnings, []string{"skill alpha: content differs from skills-lock.json"}) {
		t.Fatalf("warnings %q", got.Warnings)
	}
	key := lock.Key("git", "https://github.com/o/r.git", "skills/alpha", "")
	entry, ok := got.Entries[key]
	if !ok || len(got.Entries) != 1 || entry.Commit != git(t, repo, "rev-parse", "HEAD") || entry.SHA256 == "" {
		t.Fatalf("entries %+v", got.Entries)
	}
	if got.Keys[got.Dirs["alpha"]] != key {
		t.Fatalf("keys %+v", got.Keys)
	}

	// Pinned and cached: no network, the repository may be gone.
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	pin := func(k string) *lock.Entry {
		if k == key {
			return &entry
		}
		return nil
	}
	again, err := Restore(context.Background(), r, lockDir, map[string]Skill{"alpha": skills["alpha"]}, pin)
	if err != nil || again.Dirs["alpha"] != got.Dirs["alpha"] {
		t.Fatalf("pinned restore %+v %v", again, err)
	}
}

func TestRestoreErrors(t *testing.T) {
	_, r := newRepo(t)
	dir := t.TempDir()
	cases := map[string]Skill{
		`unsupported sourceType "npm"`:                 {Source: "x", SourceType: "npm"},
		`invalid github source "o/../r"`:               {Source: "o/../r", SourceType: "github", SkillPath: "skills/alpha/SKILL.md"},
		`skillPath "skills/alpha/README.md"`:           {Source: "o/r", SourceType: "github", SkillPath: "skills/alpha/README.md"},
		`skillPath "skills/other/SKILL.md"`:            {Source: "o/r", SourceType: "github", SkillPath: "skills/other/SKILL.md"},
		`local source "./missing/alpha"`:               {Source: "./missing/alpha", SourceType: "local"},
		`local source "./elsewhere" must be directory`: {Source: "./elsewhere", SourceType: "local"},
	}
	if err := os.MkdirAll(filepath.Join(dir, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	for want, s := range cases {
		_, err := Restore(context.Background(), r, dir, map[string]Skill{"alpha": s}, nil)
		if err == nil || !strings.Contains(err.Error(), "skill alpha: ") || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v", want, err)
		}
	}
	if _, err := Restore(context.Background(), nil, dir, map[string]Skill{"alpha": {Source: "o/r", SourceType: "github", SkillPath: "skills/alpha/SKILL.md"}}, nil); err == nil {
		t.Error("github without resolver: no error")
	}
}

func TestRemapper(t *testing.T) {
	dirs := map[string]string{"a": "/cache/a"}
	m := Remapper([]string{"/s", "/s/images/i"}, func(name string) (string, bool) {
		d, ok := dirs[name]
		return d, ok
	})
	cases := map[string]string{
		"/s/images/i/.agents/skills/a":        "/cache/a",
		"/s/images/i/.agents/skills/a/x/y.md": "/cache/a/x/y.md",
		"/s/.agents/skills/a/p.md":            "/cache/a/p.md",
		"/s/images/i/.agents/skills/b":        "/s/images/i/.agents/skills/b",
		"/s/images/i/.agents/skills":          "/s/images/i/.agents/skills",
		"/s/images/i/skills/a":                "/s/images/i/skills/a",
		"/other/.agents/skills/a":             "/other/.agents/skills/a",
	}
	for in, want := range cases {
		if got := m(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}
