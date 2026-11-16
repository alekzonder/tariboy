package tfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func parseBody(t *testing.T, body string) (*File, error) {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, Filename, body)
	return Parse(dir)
}

func TestParseValid(t *testing.T) {
	f, err := parseBody(t, "schema_version: 2\nimage_version: 1.2.3\nplugins:\n  - name: mail\nskills:\n  - dir: ./s\nprompts:\n  - file: ./a.md\n  - runtime: goal\n")
	if err != nil {
		t.Fatal(err)
	}
	if f.SchemaVersion != 2 || f.ImageVersion != "1.2.3" || f.Skills[0].BaseDir != f.Dir {
		t.Fatalf("unexpected %+v", f)
	}
	v3 := `schema_version: 3
skills:
  - git: https://example.com/r
    path: skills/x
    ref: v1
  - url: https://example.com/s.tar.gz
  - dir: ./s
  - dir: $PLUGINS/p
    only: tariboy
prompts:
  - git: https://example.com/r
    path: p.md
  - url: https://example.com/p.md
  - file: ./a.md
    only: tariboy
`
	f, err = parseBody(t, v3)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Skills) != 4 || f.Skills[0].Ref != "v1" || f.Skills[3].Only != "tariboy" || len(f.Prompts) != 3 {
		t.Fatalf("unexpected %+v", f)
	}
}

func TestParseRejects(t *testing.T) {
	many := "schema_version: 3\nskills:\n"
	for i := 0; i < 129; i++ {
		many += fmt.Sprintf("  - dir: ./s%d\n", i)
	}
	cases := map[string]string{
		"unknown key":      "schema_version: 2\nbogus: 1\n",
		"schema 1":         "schema_version: 1\n",
		"schema 4":         "schema_version: 4\n",
		"two documents":    "schema_version: 2\n---\nschema_version: 2\n",
		"bad semver":       "schema_version: 2\nimage_version: 1.2\n",
		"v2 git":           "schema_version: 2\nskills:\n  - git: https://x\n    path: p\n",
		"v2 only":          "schema_version: 2\nskills:\n  - dir: ./s\n    only: tariboy\n",
		"dir and git":      "schema_version: 3\nskills:\n  - dir: ./s\n    git: https://x\n    path: p\n",
		"git no path":      "schema_version: 3\nskills:\n  - git: https://x\n",
		"only other":       "schema_version: 3\nskills:\n  - dir: ./s\n    only: other\n",
		"129 skills":       many,
		"duplicate rt":     "schema_version: 3\nprompts:\n  - runtime: goal\n  - runtime: goal\n",
		"unknown rt":       "schema_version: 3\nprompts:\n  - runtime: nope\n",
		"bad extends":      "schema_version: 3\nextends:\n  - base\n",
		"plugins no only":  "schema_version: 3\nskills:\n  - dir: $PLUGINS/x\n",
		"prompt two kinds": "schema_version: 3\nprompts:\n  - file: ./a\n    runtime: goal\n",
	}
	for name, body := range cases {
		if _, err := parseBody(t, body); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestForTbox(t *testing.T) {
	f, err := parseBody(t, `schema_version: 3
plugins:
  - name: mail
skills:
  - dir: ./a
  - dir: ./b
    only: tariboy
prompts:
  - file: ./a.md
  - file: ./b.md
    only: tariboy
  - runtime: workdir
`)
	if err != nil {
		t.Fatal(err)
	}
	g := f.ForTbox()
	if len(g.Plugins) != 0 || len(g.Skills) != 1 || g.Skills[0].Dir != "./a" || len(g.Prompts) != 1 || g.Prompts[0].File != "./a.md" {
		t.Fatalf("unexpected %+v", g)
	}
	if len(f.Skills) != 2 || len(f.Plugins) != 1 {
		t.Fatal("ForTbox changed the original")
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "skills/a/SKILL.md", "x")
	write(t, dir, "skills/a/notes.md", "x")
	write(t, dir, "p.md", "x")
	write(t, dir, "sub/img/Tariboyfile.yaml", "")
	if err := os.Symlink(filepath.Join(dir, "p.md"), filepath.Join(dir, "link.md")); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "sub/img")
	canon, _ := filepath.EvalSymlinks(dir)

	path, src, cat, err := ResolveSkillDir(SkillEntry{Dir: "../../skills/a", BaseDir: base})
	if err != nil || path != filepath.Join(canon, "skills/a") || src != "../../skills/a" || cat != "source" {
		t.Fatalf("skill: %q %q %q %v", path, src, cat, err)
	}
	_, _, cat, err = ResolveSkillDir(SkillEntry{Dir: filepath.Join(canon, "skills/a"), BaseDir: base})
	if err != nil || cat != "absolute" {
		t.Fatalf("absolute: %q %v", cat, err)
	}
	_, _, _, err = ResolveSkillDir(SkillEntry{Dir: "$PLUGINS/x", BaseDir: base})
	if err == nil || !strings.Contains(err.Error(), "$PLUGINS/x") || !strings.Contains(err.Error(), "no plugin root") {
		t.Fatalf("plugins: %v", err)
	}
	if _, _, _, err = ResolveSkillDir(SkillEntry{Git: "https://x", Path: "p"}); !errors.Is(err, ErrRemoteUnsupported) {
		t.Fatalf("git: %v", err)
	}
	if _, _, _, err = ResolvePromptFile(PromptEntry{URL: "https://x"}); !errors.Is(err, ErrRemoteUnsupported) {
		t.Fatalf("url: %v", err)
	}
	if _, _, _, err = ResolvePromptFile(PromptEntry{File: "../../p.md", BaseDir: base}); err == nil {
		t.Fatal("../ prompt outside a skill must fail")
	}
	skillDirs := []string{filepath.Join(dir, "skills/a")}
	path, _, cat, err = ResolvePromptFile(PromptEntry{File: "../../skills/a/notes.md", BaseDir: base}, skillDirs...)
	if err != nil || cat != "source" || path != filepath.Join(canon, "skills/a/notes.md") {
		t.Fatalf("below skill: %q %q %v", path, cat, err)
	}
	if _, _, _, err = ResolvePromptFile(PromptEntry{File: "./link.md", BaseDir: dir}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink: %v", err)
	}
	if _, _, _, err = ResolvePromptFile(PromptEntry{File: "./p.md", BaseDir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = ResolvePromptFile(PromptEntry{File: "$PLUGINS/q.md", BaseDir: dir}); err == nil || !strings.Contains(err.Error(), "$PLUGINS/q.md") {
		t.Fatalf("prompt plugins: %v", err)
	}
}

func TestRoleRead(t *testing.T) {
	dir := t.TempDir()
	if r, err := RoleRead(dir); r != nil || err != nil {
		t.Fatalf("missing: %v %v", r, err)
	}
	write(t, dir, "ROLE.md", "Reviewer\n")
	if r, err := RoleRead(dir); err != nil || r == nil || r.Text != "Reviewer\n" {
		t.Fatalf("role: %v %v", r, err)
	}
	write(t, dir, "ROLE.md", strings.Repeat("x", 501))
	if _, err := RoleRead(dir); err == nil {
		t.Fatal("expected a length error")
	}
}
