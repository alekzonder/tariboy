package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/skillslock"
)

func skillDoc(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\nbody\n"
}

// gitEnv isolates git from the user's configuration.
func gitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// storeHarness runs the tbox test binary against a scratch TBOX_HOME with a
// fake claude that lists the session's skills.
type storeHarness struct {
	t    *testing.T
	self string
	env  []string
}

func newStoreHarness(t *testing.T) storeHarness {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	put(t, filepath.Join(bin, "tbox"), "#!/bin/sh\nTBOX_E2E_CHILD=1 TBOX_E2E_LOCAL=1 exec \""+self+"\" \"$@\"\n")
	put(t, filepath.Join(bin, "claude"), "#!/bin/sh\nif [ \"$1\" = --version ]; then echo '2.1.325 (Claude Code)'; exit 0; fi\ntbox skills\n")
	for _, n := range []string{"tbox", "claude"} {
		if err := os.Chmod(filepath.Join(bin, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := append(childEnviron(), "TBOX_HOME="+t.TempDir(), "PATH="+bin+":"+os.Getenv("PATH"), "TBOX_E2E_CHILD=1", "TBOX_E2E_LOCAL=1")
	return storeHarness{t: t, self: self, env: env}
}

// tbox runs one command and returns stdout and stderr together.
func (h storeHarness) tbox(args ...string) string {
	h.t.Helper()
	cmd := exec.Command(h.self, args...)
	cmd.Env = h.env
	cmd.Dir = h.t.TempDir()
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("tbox %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestStoreImageEndToEnd(t *testing.T) {
	gitEnv(t)
	h := newStoreHarness(t)
	ext := t.TempDir()
	put(t, filepath.Join(ext, "skills/remote-skill/SKILL.md"), skillDoc("remote-skill", "From a second repository"))
	gitIn(t, ext, "init", "-q", "-b", "main")
	gitIn(t, ext, "add", ".")
	gitIn(t, ext, "commit", "-q", "-m", "ext")

	repo := t.TempDir()
	put(t, filepath.Join(repo, "images/demo/Tariboyfile.yaml"), fmt.Sprintf(`schema_version: 3
image_version: 1.0.0
skills:
  - dir: ../../skills/local-skill
  - git: %s
    path: skills/remote-skill
  - dir: ../../skills/secret-skill
    only: tariboy
`, ext))
	put(t, filepath.Join(repo, "skills/local-skill/SKILL.md"), skillDoc("local-skill", "From the Store"))
	put(t, filepath.Join(repo, "skills/secret-skill/SKILL.md"), skillDoc("secret-skill", "Only tariboy"))
	put(t, filepath.Join(repo, "prompts/process.md"), "process\n")
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "one")

	h.tbox("store", "add", "s", "file://"+repo)
	if out := h.tbox("prompts"); out != "s/process\n" {
		t.Fatalf("prompts %q", out)
	}
	if out := h.tbox("images"); !regexp.MustCompile(`s/demo\s+1\.0\.0\s+not built`).MatchString(out) {
		t.Fatalf("images before build:\n%s", out)
	}
	out := h.tbox("run", "s/demo", "claude")
	if !strings.Contains(out, "built demo:latest ") {
		t.Fatalf("first run has no build line:\n%s", out)
	}
	var skills []string
	for _, l := range strings.Split(out, "\n") {
		if name, _, ok := strings.Cut(l, "\t"); ok {
			skills = append(skills, name)
		}
	}
	if strings.Join(skills, ",") != "local-skill,remote-skill" {
		t.Fatalf("session skills %q in\n%s", skills, out)
	}
	if out := h.tbox("images"); !regexp.MustCompile(`s/demo\s+1\.0\.0\s+built`).MatchString(out) {
		t.Fatalf("images after build:\n%s", out)
	}
	if out := h.tbox("run", "s/demo", "claude"); strings.Contains(out, "built ") {
		t.Fatalf("second run built again:\n%s", out)
	}

	put(t, filepath.Join(repo, "skills/local-skill/SKILL.md"), skillDoc("local-skill", "Changed in the Store"))
	gitIn(t, repo, "commit", "-q", "-am", "two")
	h.tbox("store", "refresh", "s")
	if out := h.tbox("run", "s/demo", "claude"); !strings.Contains(out, "built demo:latest ") || !strings.Contains(out, "Changed in the Store") {
		t.Fatalf("run after refresh:\n%s", out)
	}
}

func TestStoreSchemaV2ImageWithSkillsLock(t *testing.T) {
	gitEnv(t)
	h := newStoreHarness(t)
	repo := t.TempDir()
	put(t, filepath.Join(repo, "skills/b/SKILL.md"), skillDoc("b", "Restored from the lock"))
	hash, err := skillslock.Hash(filepath.Join(repo, "skills/b"))
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(repo, "images/old/Tariboyfile.yaml"), "schema_version: 2\nimage_version: 1.0.0\nplugins:\n  - name: mail\nskills:\n  - dir: ./.agents/skills/b\n")
	put(t, filepath.Join(repo, "images/old/"+skillslock.Filename), fmt.Sprintf(`{"version":1,"skills":{"b":{"source":"../../skills/b","sourceType":"local","computedHash":%q}}}`, hash))
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "one")

	h.tbox("store", "add", "s", "file://"+repo)
	out := h.tbox("run", "s/old", "claude")
	if !strings.Contains(out, "warning: image declares plugins") || !strings.Contains(out, "b\tRestored from the lock") {
		t.Fatalf("run output:\n%s", out)
	}
}
