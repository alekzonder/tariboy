package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/rundir"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
)

// fakeHost answers LookPath from a set of installed names and Output from a
// function, and records every Output call.
type fakeHost struct {
	installed map[string]bool
	env       map[string]string
	output    func(name string, args []string) ([]byte, error)
	calls     [][]string
}

func (f *fakeHost) host() Host {
	return Host{
		Getenv: func(k string) string { return f.env[k] },
		LookPath: func(name string) (string, error) {
			if f.installed[name] {
				return "/fake/bin/" + name, nil
			}
			return "", errors.New("executable file not found in $PATH")
		},
		Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			f.calls = append(f.calls, append([]string{name}, args...))
			if f.output == nil {
				return nil, errors.New("unexpected call")
			}
			return f.output(name, args)
		},
	}
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// twoSkillRun lays out an unpacked run directory with skills alpha and beta.
func twoSkillRun(t *testing.T, prompt string) rundir.Run {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "run", "abc", "h")
	image := filepath.Join(dir, "image")
	skills := filepath.Join(image, "skills")
	writeFile(t, filepath.Join(skills, "alpha", "SKILL.md"), "---\nname: alpha\ndescription: First skill.\n---\nalpha body\n", 0o600)
	writeFile(t, filepath.Join(skills, "alpha", "scripts", "run.sh"), "#!/bin/sh\necho alpha\n", 0o700)
	writeFile(t, filepath.Join(skills, "beta", "SKILL.md"), "---\nname: beta\ndescription: Second\n  skill.\n---\nbeta body\n", 0o600)
	writeFile(t, filepath.Join(dir, "PROMPT.md"), prompt, 0o600)
	return rundir.Run{
		Dir:          dir,
		ImageDir:     image,
		SkillsDir:    skills,
		PromptPath:   filepath.Join(dir, "PROMPT.md"),
		Name:         "demo",
		Tag:          "latest",
		Digest:       "abc",
		ImageVersion: "1.2.0",
		Skills: []skill.Metadata{
			{Name: "alpha", Description: "First skill.", ArchiveRoot: "skills/alpha"},
			{Name: "beta", Description: "Second\n  skill.", ArchiveRoot: "skills/beta"},
		},
	}
}

func TestNamesAndGet(t *testing.T) {
	if got := Names(); !slices.Equal(got, []string{"claude", "codex", "opencode"}) {
		t.Fatalf("names %v", got)
	}
	for _, name := range []string{"claude", "opencode"} {
		a, err := Get(name)
		if err != nil || a.Name() != name {
			t.Fatalf("get %s: %v %v", name, a, err)
		}
	}
	if _, err := Get("vim"); err == nil || !strings.Contains(err.Error(), "vim") {
		t.Fatalf("unknown harness: %v", err)
	}
}

func TestInstalledOnlyCodex(t *testing.T) {
	f := &fakeHost{installed: map[string]bool{"codex": true}}
	if got := Installed(f.host()); !slices.Equal(got, []string{"codex"}) {
		t.Fatalf("installed %v", got)
	}
}

func TestCatalogTwoSkills(t *testing.T) {
	run := twoSkillRun(t, "")
	want := "## Skills of image demo\n\n" +
		"- alpha: First skill.\n" +
		"- beta: Second skill.\n\n" +
		"Read a skill with `tbox skill <name>` and run its scripts with `tbox skill exec <name> <script>`.\n"
	if got := Catalog(run); got != want {
		t.Fatalf("catalog:\n%q\nwant:\n%q", got, want)
	}
}

func TestErrUnsupportedVersionMessage(t *testing.T) {
	err := error(ErrUnsupportedVersion{Harness: "claude", Found: "2.1.100", Minimum: "2.1.227"})
	if err.Error() != "claude 2.1.100 is not supported: version 2.1.227 or newer is required" {
		t.Fatalf("message %q", err)
	}
}

func TestParseVersion(t *testing.T) {
	cases := map[string]string{
		"2.1.325 (Claude Code)\n": "2.1.325",
		"1.19.5":                  "1.19.5",
		"v0.166.0-beta":           "0.166.0",
	}
	for in, want := range cases {
		_, got, ok := parseVersion(in)
		if !ok || got != want {
			t.Errorf("parse %q: %q %v", in, got, ok)
		}
	}
	if _, _, ok := parseVersion("nothing"); ok {
		t.Error("parsed a string without a version")
	}
}

func TestCopySkillsRejectsSymlink(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a", "SKILL.md"), "x", 0o600)
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "a", "link")); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, filepath.Join(t.TempDir(), "dst")); err == nil {
		t.Fatal("symlink copied")
	}
}
