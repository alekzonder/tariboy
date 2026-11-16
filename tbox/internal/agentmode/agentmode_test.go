package agentmode

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/rundir"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
)

type call struct {
	argv0 string
	argv  []string
	env   []string
}

type session struct {
	dir   string
	out   bytes.Buffer
	err   bytes.Buffer
	calls []call
}

func newSession(t *testing.T) *session {
	t.Helper()
	prep, err := skill.Prepare(filepath.Join("..", "..", "testdata", "compat", "skills", "hello-skill"), "dir", "")
	if err != nil {
		t.Fatal(err)
	}
	img, err := archive.Assemble("demo", "1.0.0", time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), []skill.Prepared{prep}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	run, err := rundir.Prepare(paths.Home(t.TempDir()), img, imgstore.Ref{Name: "demo", Tag: "latest"}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	return &session{dir: run.ImageDir}
}

func (s *session) run(imageDir string, args ...string) int {
	s.out.Reset()
	s.err.Reset()
	host := Host{
		Stdout:  &s.out,
		Stderr:  &s.err,
		Environ: []string{"A=1", "TBOX_IMAGE=demo:latest", "TBOX_SKILL_DIR=old"},
		Exec: func(argv0 string, argv, env []string) error {
			s.calls = append(s.calls, call{argv0, argv, env})
			return nil
		},
	}
	return Run(host, imageDir, args)
}

func TestSkillsAndSkill(t *testing.T) {
	s := newSession(t)
	if code := s.run(s.dir, "skills"); code != 0 || !strings.HasPrefix(s.out.String(), "hello-skill\tSays hello.") {
		t.Fatalf("skills: %d %q", code, s.out.String())
	}
	if code := s.run(s.dir, "skill", "hello-skill"); code != 0 {
		t.Fatalf("skill: %d %q", code, s.err.String())
	}
	for _, want := range []string{"name: hello-skill", "scripts/run.sh", "reference.md", "\n\n---\nname: hello-skill", "# Hello skill"} {
		if !strings.Contains(s.out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, s.out.String())
		}
	}
	if code := s.run(s.dir, "skill", "missing"); code != 1 || !strings.Contains(s.err.String(), "hello-skill") {
		t.Fatalf("missing: %d %q", code, s.err.String())
	}
	if code := s.run(s.dir, "skill", "store/hello-skill"); code != 1 || !strings.Contains(s.err.String(), "no store prefix") {
		t.Fatalf("prefixed: %d %q", code, s.err.String())
	}
}

func TestSkillExec(t *testing.T) {
	s := newSession(t)
	if code := s.run(s.dir, "skill", "exec", "hello-skill", "scripts/run.sh", "a", "b"); code != 0 || len(s.calls) != 1 {
		t.Fatalf("exec: %d %q", code, s.err.String())
	}
	c := s.calls[0]
	skillDir := filepath.Join(s.dir, "skills", "hello-skill")
	script := filepath.Join(skillDir, "scripts", "run.sh")
	if c.argv0 != script || !reflect.DeepEqual(c.argv, []string{script, "a", "b"}) {
		t.Fatalf("call %+v", c)
	}
	if !reflect.DeepEqual(c.env, []string{"A=1", "TBOX_IMAGE=demo:latest", "TBOX_SKILL_DIR=" + skillDir}) {
		t.Fatalf("env %v", c.env)
	}
}

func TestSkillExecShellFallback(t *testing.T) {
	s := newSession(t)
	dir := filepath.Join(s.dir, "skills", "hello-skill")
	for name, mode := range map[string]os.FileMode{"plain.sh": 0o600, "plain.txt": 0o600} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("echo hi\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	if code := s.run(s.dir, "skill", "exec", "hello-skill", "plain.sh", "x"); code != 0 || len(s.calls) != 1 {
		t.Fatalf("sh: %d %q", code, s.err.String())
	}
	if c := s.calls[0]; c.argv0 != "/bin/sh" || !reflect.DeepEqual(c.argv, []string{"/bin/sh", filepath.Join(dir, "plain.sh"), "x"}) {
		t.Fatalf("call %+v", c)
	}
	if code := s.run(s.dir, "skill", "exec", "hello-skill", "plain.txt"); code != 1 || len(s.calls) != 1 {
		t.Fatalf("txt: %d %q", code, s.err.String())
	}
}

func TestSkillExecRefusesEscapes(t *testing.T) {
	s := newSession(t)
	outside := filepath.Join(t.TempDir(), "x.sh")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.dir, "skills", "hello-skill")
	if err := os.Symlink(outside, filepath.Join(dir, "link.sh")); err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{"../other/x.sh", outside, "link.sh", "missing.sh", "scripts"} {
		if code := s.run(s.dir, "skill", "exec", "hello-skill", script); code == 0 || len(s.calls) != 0 {
			t.Errorf("%s: code %d calls %d", script, code, len(s.calls))
		}
	}
}

func TestBrokenDirectoryFailsClosed(t *testing.T) {
	s := newSession(t)
	missing := filepath.Join(t.TempDir(), "gone")
	if code := s.run(missing, "skills"); code != 1 || s.err.Len() == 0 {
		t.Fatalf("skills: %d %q", code, s.err.String())
	}
	if code := s.run(missing, "build", "x"); code != 2 {
		t.Fatalf("build: %d", code)
	}
}

func TestOtherCommandsUnavailable(t *testing.T) {
	s := newSession(t)
	if code := s.run(s.dir, "build", "x"); code != 2 || !strings.HasPrefix(s.err.String(), "tbox: build is not available inside the session of image demo:latest\n") {
		t.Fatalf("build: %d %q", code, s.err.String())
	}
	for _, want := range []string{"skills", "skill NAME", "skill exec NAME SCRIPT", "version", "help"} {
		if !strings.Contains(s.err.String(), want) {
			t.Errorf("refusal lacks %q: %q", want, s.err.String())
		}
	}
	host := Host{Stdout: &s.out, Stderr: &s.err}
	s.err.Reset()
	if code := Run(host, s.dir, []string{"run"}); code != 2 || !strings.Contains(s.err.String(), "session of image directory "+s.dir) {
		t.Fatalf("no TBOX_IMAGE: %d %q", code, s.err.String())
	}
}

func TestListingsSanitizeControlCharacters(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills", "evil")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: evil\n---\nbody \x1b[31m kept\n"
	for name, data := range map[string]string{"SKILL.md": body, "a\x1b[2Jb.txt": "x"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := rundir.Run{ImageDir: dir, Skills: []skill.Metadata{{Name: "evil\x1b]0;x\x07", Description: "desc \x1b[31mred", ArchiveRoot: "skills/evil"}}}
	var out, errb bytes.Buffer
	host := Host{Stdout: &out, Stderr: &errb}
	listSkills(host, run)
	if strings.ContainsAny(out.String(), "\x1b\x07") {
		t.Fatalf("skills: %q", out.String())
	}
	out.Reset()
	run.Skills[0].Name = "evil"
	if code := skillCommand(host, run, []string{"evil"}); code != 0 {
		t.Fatalf("skill: %d %q", code, errb.String())
	}
	header, doc, _ := strings.Cut(out.String(), "\n\n")
	if strings.Contains(header, "\x1b") {
		t.Fatalf("header: %q", header)
	}
	if doc != body {
		t.Fatalf("SKILL.md body changed: %q", doc)
	}
}
