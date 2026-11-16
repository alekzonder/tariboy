package tfile

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExtendsMerge(t *testing.T) {
	root := t.TempDir()
	write(t, root, "base/Tariboyfile.yaml", "schema_version: 2\nimage_version: 0.1.0\nplugins:\n  - name: mail\nskills:\n  - dir: ./skills/a\n  - dir: ./skills/b\nprompts:\n  - file: ./p.md\n  - runtime: goal\n")
	write(t, root, "base/p.md", "same")
	write(t, root, "child/Tariboyfile.yaml", "schema_version: 3\nimage_version: 0.2.0\nextends:\n  - ../base\nplugins:\n  - name: mail\n  - name: goal\nskills:\n  - dir: ./other/a\n  - git: https://x/r\n    path: skills/c\nprompts:\n  - file: ./q.md\n  - runtime: goal\n")
	write(t, root, "child/q.md", "same")
	f, err := Load(root + "/child")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Extends) != 0 || f.ImageVersion != "0.2.0" || f.SchemaVersion != 3 {
		t.Fatalf("unexpected %+v", f)
	}
	if len(f.Plugins) != 2 || f.Plugins[0].Name != "mail" {
		t.Fatalf("plugins %+v", f.Plugins)
	}
	var got []string
	for _, s := range f.Skills {
		got = append(got, s.Dir+s.Path+"@"+s.BaseDir[strings.LastIndex(s.BaseDir, "/")+1:])
	}
	want := "./skills/b@base,./other/a@child,skills/c@child"
	if strings.Join(got, ",") != want {
		t.Fatalf("skills %v", got)
	}
	// q.md has the same content as p.md, runtime goal is repeated.
	if len(f.Prompts) != 2 || f.Prompts[0].File != "./p.md" || f.Prompts[1].Runtime != "goal" {
		t.Fatalf("prompts %+v", f.Prompts)
	}
}

func TestLoadCycle(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/Tariboyfile.yaml", "schema_version: 2\nextends:\n  - ../b\n")
	write(t, root, "b/Tariboyfile.yaml", "schema_version: 2\nextends:\n  - ../a\n")
	if _, err := Load(root + "/a"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("got %v", err)
	}
}

func chain(t *testing.T, n int) string {
	root := t.TempDir()
	for i := 0; i < n; i++ {
		body := "schema_version: 2\n"
		if i+1 < n {
			body += fmt.Sprintf("extends:\n  - ../d%d\n", i+1)
		}
		write(t, root, fmt.Sprintf("d%d/Tariboyfile.yaml", i), body)
	}
	return root + "/d0"
}

func TestLoadDepth(t *testing.T) {
	if _, err := Load(chain(t, 16)); err != nil {
		t.Fatalf("depth 16: %v", err)
	}
	if _, err := Load(chain(t, 17)); err == nil || !strings.Contains(err.Error(), "deeper") {
		t.Fatalf("depth 17: %v", err)
	}
}

func TestLoadDedupesDotDotPromptByContent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "base/Tariboyfile.yaml", "schema_version: 2\nskills:\n  - dir: ./skills/x\nprompts:\n  - file: ./p.md\n")
	write(t, root, "base/p.md", "same")
	write(t, root, "base/skills/x/SKILL.md", "x")
	write(t, root, "child/Tariboyfile.yaml", "schema_version: 2\nextends:\n  - ../base\nskills:\n  - dir: ../base/skills/x\nprompts:\n  - file: ../base/skills/x/p.md\n")
	write(t, root, "base/skills/x/p.md", "same")
	f, err := Load(root + "/child")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Prompts) != 1 || f.Prompts[0].File != "./p.md" {
		t.Fatalf("prompts %+v", f.Prompts)
	}
}

func TestLoadMissingPromptIsAnError(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/Tariboyfile.yaml", "schema_version: 2\nprompts:\n  - file: ./missing.md\n")
	if _, err := Load(root + "/a"); err == nil {
		t.Fatal("expected an error")
	}
}

// TestLoadWithRemap restores ./.agents/skills paths elsewhere: resolution and
// prompt de-duplication read the remapped files, while scope checks and the
// returned source keep the path as written.
func TestLoadWithRemap(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(root, "img")
	restored := filepath.Join(root, "cache", "a")
	write(t, img, Filename, "schema_version: 2\nskills:\n  - dir: ./.agents/skills/a\nprompts:\n  - file: ./.agents/skills/a/a.md\n  - file: ./p.md\n")
	write(t, img, "p.md", "same\n")
	write(t, restored, "SKILL.md", "x")
	write(t, restored, "a.md", "same\n")
	prefix := filepath.Join(img, ".agents", "skills", "a")
	remap := Remap(func(abs string) string {
		if abs == prefix || strings.HasPrefix(abs, prefix+string(filepath.Separator)) {
			return restored + strings.TrimPrefix(abs, prefix)
		}
		return abs
	})
	if _, err := Load(img); err == nil {
		t.Fatal("Load without remap: no error")
	}
	f, err := LoadWith(img, remap)
	if err != nil {
		t.Fatal(err)
	}
	// a.md and p.md have the same content, so p.md is dropped.
	if len(f.Prompts) != 1 || f.Prompts[0].File != "./.agents/skills/a/a.md" {
		t.Fatalf("prompts %+v", f.Prompts)
	}
	dir, src, category, err := ResolveSkillDirWith(f.Skills[0], f.Remap)
	if err != nil || dir != restored || src != "./.agents/skills/a" || category != "source" {
		t.Fatalf("skill %s %s %s %v", dir, src, category, err)
	}
	file, src, _, err := ResolvePromptFileWith(f.Prompts[0], f.Remap, f.SkillDirs()...)
	if err != nil || file != filepath.Join(restored, "a.md") || src != "./.agents/skills/a/a.md" {
		t.Fatalf("prompt %s %s %v", file, src, err)
	}
	if _, _, _, err := ResolveSkillDir(f.Skills[0]); err == nil {
		t.Fatal("ResolveSkillDir without remap: no error")
	}
}
