package construct

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
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

func skillDoc(name string) string {
	return "---\nname: " + name + "\ndescription: " + name + " skill\n---\nbody\n"
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

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
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
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixture registers a git Store "g", a dir Store "d" and an archive Store "a".
type fixture struct {
	reg     stores.Registry
	home    paths.Home
	gitURL  string
	commit  string
	dirRoot string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	isolate(t)
	home := paths.Home(t.TempDir())
	repo := t.TempDir()
	write(t, filepath.Join(repo, "skills/gs/SKILL.md"), skillDoc("gs"))
	write(t, filepath.Join(repo, "skills/other/SKILL.md"), skillDoc("other"))
	write(t, filepath.Join(repo, "prompts/gp.md"), "git prompt\n")
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "one")

	dir := t.TempDir()
	write(t, filepath.Join(dir, "skills/ds/SKILL.md"), skillDoc("ds"))
	write(t, filepath.Join(dir, "skills/other/SKILL.md"), skillDoc("other"))
	write(t, filepath.Join(dir, "prompts/dp.md"), "dir prompt\n")
	// a dir Store that is a git checkout is still addressed by dir
	git(t, dir, "init", "-q", "-b", "main")

	archive := tarGz(t, map[string]string{
		"top/skills/as/SKILL.md":       skillDoc("as"),
		"top/skills/as/scripts/run.sh": "echo hi\n",
		"top/prompts/ap.md":            "archive prompt\n",
	})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	t.Cleanup(srv.Close)

	reg := stores.Registry{Home: home, Git: gitx.Git{AllowLocal: true}, HTTP: fetch.Client{HTTP: srv.Client()}, Cwd: t.TempDir()}
	ctx := context.Background()
	g, err := reg.Add(ctx, "g", "file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Add(ctx, "d", dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Add(ctx, "a", srv.URL+"/s.tar.gz", ""); err != nil {
		t.Fatal(err)
	}
	return fixture{reg: reg, home: home, gitURL: g.Source, commit: g.Commit, dirRoot: dir}
}

func sel(store, name string) stores.Selector { return stores.Selector{Store: store, Name: name} }

func TestCreateAddressesEachStoreKind(t *testing.T) {
	f := newFixture(t)
	dir, err := Create(f.reg, f.home, Spec{
		Name:    "demo",
		Skills:  []stores.Selector{sel("g", "gs"), sel("d", "ds"), sel("a", "as")},
		Prompts: []stores.Selector{sel("g", "gp"), sel("d", "dp"), sel("a", "ap")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(string(f.home), "sources", "demo") {
		t.Fatalf("dir %s", dir)
	}
	file, err := tfile.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if file.SchemaVersion != 3 || file.ImageVersion != "0.1.0" {
		t.Fatalf("header %+v", file)
	}
	text, _ := os.ReadFile(filepath.Join(dir, "Tariboyfile.yaml"))
	for _, name := range []string{"g", "d", "a"} {
		if strings.Contains(string(text), "stores/"+name) && name != "d" {
			// store directories must not leak
			t.Fatalf("store dir in source:\n%s", text)
		}
	}
	wantSkills := []tfile.SkillEntry{
		{Git: f.gitURL, Path: "skills/gs", Ref: f.commit},
		{Dir: filepath.Join(f.dirRoot, "skills/ds")},
		{Dir: "./skills/as"},
	}
	for i, w := range wantSkills {
		got := file.Skills[i]
		if got.Git != w.Git || got.Path != w.Path || got.Ref != w.Ref || got.Dir != w.Dir {
			t.Fatalf("skill %d: %+v want %+v", i, got, w)
		}
	}
	wantPrompts := []tfile.PromptEntry{
		{Git: f.gitURL, Path: "prompts/gp.md", Ref: f.commit},
		{File: filepath.Join(f.dirRoot, "prompts/dp.md")},
		{File: "./prompts/ap.md"},
	}
	if len(file.Prompts) != 3 {
		t.Fatalf("prompts %+v", file.Prompts)
	}
	for i, w := range wantPrompts {
		got := file.Prompts[i]
		if got.Git != w.Git || got.Path != w.Path || got.Ref != w.Ref || got.File != w.File {
			t.Fatalf("prompt %d: %+v want %+v", i, got, w)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "skills/as/scripts/run.sh")); err != nil || string(b) != "echo hi\n" {
		t.Fatalf("copied skill: %q %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "prompts/ap.md")); err != nil || string(b) != "archive prompt\n" {
		t.Fatalf("copied prompt: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "instructions.md")); err == nil {
		t.Fatal("instructions.md written although prompts were selected")
	}
}

func TestCreateInstructionsPrompt(t *testing.T) {
	f := newFixture(t)
	dir, err := Create(f.reg, f.home, Spec{Name: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := tfile.Parse(dir)
	if err != nil || len(file.Prompts) != 1 || file.Prompts[0].File != "./instructions.md" {
		t.Fatalf("%v %+v", err, file)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "instructions.md"))
	if !strings.HasPrefix(string(body), "# empty\n") || strings.Count(string(body), "\n") < 2 {
		t.Fatalf("instructions: %q", body)
	}

	dir, err = Create(f.reg, f.home, Spec{Name: "both", Prompts: []stores.Selector{sel("d", "dp")}, NewPrompt: true, Version: "2.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	file, _ = tfile.Parse(dir)
	if len(file.Prompts) != 2 || file.Prompts[1].File != "./instructions.md" || file.ImageVersion != "2.0.0" {
		t.Fatalf("%+v", file)
	}
}

func TestCreateErrors(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{"unknown store", Spec{Name: "x", Skills: []stores.Selector{sel("nope", "a")}}, "nope"},
		{"unknown skill", Spec{Name: "x", Skills: []stores.Selector{sel("g", "zzz")}}, "gs, other"},
		{"unknown prompt", Spec{Name: "x", Prompts: []stores.Selector{sel("g", "zzz")}}, "gp"},
		{"duplicate name", Spec{Name: "x", Skills: []stores.Selector{sel("g", "other"), sel("d", "other")}}, "g/other"},
		{"bad name", Spec{Name: "Bad Name"}, "invalid image name"},
		{"bad version", Spec{Name: "x", Version: "nope"}, "version"},
	}
	for _, c := range cases {
		_, err := Create(f.reg, f.home, c.spec)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v want %q", c.name, err, c.want)
		}
	}
	if _, err := os.Stat(filepath.Join(string(f.home), "sources", "x")); err == nil {
		t.Fatal("failed Create left a source")
	}
	entries, _ := os.ReadDir(filepath.Join(string(f.home), "sources"))
	for _, e := range entries {
		t.Errorf("leftover %s", e.Name())
	}
}

func TestCreateBrokenSkillWritesNothing(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.dirRoot, "skills/broken/SKILL.md"), "nothing\n")
	_, err := Create(f.reg, f.home, Spec{Name: "x", Skills: []stores.Selector{sel("d", "broken")}})
	if err == nil || !strings.Contains(err.Error(), "d/broken") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(string(f.home), "sources", "x")); err == nil {
		t.Fatal("source written")
	}
}

func TestCreateExistingAndForce(t *testing.T) {
	f := newFixture(t)
	spec := Spec{Name: "demo", Skills: []stores.Selector{sel("d", "ds")}}
	dir, err := Create(f.reg, f.home, spec)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "stray.txt"), "x")
	if _, err := Create(f.reg, f.home, spec); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stray.txt")); err != nil {
		t.Fatal("existing source changed without Force")
	}
	spec.Force = true
	if _, err := Create(f.reg, f.home, spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stray.txt")); err == nil {
		t.Fatal("stray file survived Force")
	}
	if _, err := tfile.Parse(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRootSkillOfGitStoreIsRefused(t *testing.T) {
	isolate(t)
	home := paths.Home(t.TempDir())
	repo := t.TempDir()
	write(t, filepath.Join(repo, "SKILL.md"), skillDoc("rootskill"))
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "one")
	reg := stores.Registry{Home: home, Git: gitx.Git{AllowLocal: true}, Cwd: t.TempDir()}
	if _, err := reg.Add(context.Background(), "r", "file://"+repo, ""); err != nil {
		t.Fatal(err)
	}
	_, err := Create(reg, home, Spec{Name: "x", Skills: []stores.Selector{sel("r", "rootskill")}})
	if err == nil || !strings.Contains(err.Error(), "root of a Store") {
		t.Fatalf("%v", err)
	}
}
