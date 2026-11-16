package stores

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func inv(t *testing.T, root string) Inventory {
	t.Helper()
	r, _ := newRegistry(t, nil)
	got, err := r.Inventory(Store{Name: "s", Kind: KindDir, Source: root})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

const goodSkill = "---\nname: hello\ndescription: Says hello\n---\nbody\n"

func TestInventoryStandardLayout(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "images/zeta/Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.2.3\n")
	write(t, filepath.Join(root, "images/alpha/Tariboyfile.yaml"), "schema_version: 2\n")
	write(t, filepath.Join(root, "images/broken/Tariboyfile.yaml"), "schema_version: [\n")
	write(t, filepath.Join(root, "skills/hello/SKILL.md"), goodSkill)
	write(t, filepath.Join(root, "skills/bad/SKILL.md"), "no frontmatter\n")
	write(t, filepath.Join(root, "skills/empty/note.txt"), "no SKILL.md here")
	write(t, filepath.Join(root, "images/noyaml/readme.txt"), "x")
	write(t, filepath.Join(root, "prompts/b.md"), "b")
	write(t, filepath.Join(root, "prompts/a.md"), "a")
	write(t, filepath.Join(root, "prompts/notes.txt"), "x")
	write(t, filepath.Join(root, "workflows/w.yaml"), "x")
	got := inv(t, root)

	if len(got.Images) != 3 || got.Images[0].Name != "alpha" || got.Images[1].Name != "broken" || got.Images[2].Name != "zeta" {
		t.Fatalf("images %+v", got.Images)
	}
	if a := got.Images[0]; a.Schema != 2 || a.Err != "" || a.Dir != filepath.Join(root, "images/alpha") {
		t.Fatalf("alpha %+v", a)
	}
	if got.Images[1].Err == "" {
		t.Fatal("broken image has no Err")
	}
	if z := got.Images[2]; z.Version != "1.2.3" || z.Schema != 3 {
		t.Fatalf("zeta %+v", z)
	}
	if len(got.Skills) != 2 || got.Skills[0].Name != "bad" || got.Skills[0].Err == "" ||
		got.Skills[1].Name != "hello" || got.Skills[1].Description != "Says hello" || got.Skills[1].Err != "" {
		t.Fatalf("skills %+v", got.Skills)
	}
	if len(got.Prompts) != 2 || got.Prompts[0].Name != "a" || got.Prompts[0].Path != filepath.Join(root, "prompts/a.md") || got.Prompts[1].Name != "b" {
		t.Fatalf("prompts %+v", got.Prompts)
	}
}

func TestInventoryRootSkill(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "SKILL.md"), goodSkill)
	got := inv(t, root)
	if len(got.Skills) != 1 || got.Skills[0].Name != "hello" || got.Skills[0].Dir != root {
		t.Fatalf("%+v", got.Skills)
	}
	if len(got.Images) != 0 || len(got.Prompts) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestInventoryKnownSkillDirs(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".claude/skills/x/SKILL.md"), "---\nname: x\ndescription: from claude\n---\n")
	write(t, filepath.Join(root, ".agents/skills/y/SKILL.md"), "---\nname: y\ndescription: from agents\n---\n")
	got := inv(t, root)
	if len(got.Skills) != 2 || got.Skills[0].Name != "x" || got.Skills[1].Name != "y" {
		t.Fatalf("%+v", got.Skills)
	}
}

func TestInventorySkillsDirWinsOverFallbacks(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "skills/hello/SKILL.md"), goodSkill)
	write(t, filepath.Join(root, ".claude/skills/x/SKILL.md"), "---\nname: x\ndescription: d\n---\n")
	write(t, filepath.Join(root, "SKILL.md"), goodSkill)
	if got := inv(t, root); len(got.Skills) != 1 {
		t.Fatalf("%+v", got.Skills)
	}
}

func TestInventoryIgnoresSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, filepath.Join(outside, "SKILL.md"), goodSkill)
	write(t, filepath.Join(outside, "Tariboyfile.yaml"), "schema_version: 3\n")
	write(t, filepath.Join(outside, "p.md"), "p")
	write(t, filepath.Join(root, "skills/good/SKILL.md"), goodSkill)
	for link, target := range map[string]string{
		"skills/evil":                outside,
		"images/evil":                outside,
		"prompts/evil.md":            filepath.Join(outside, "p.md"),
		"skills/good2/SKILL.md":      filepath.Join(outside, "SKILL.md"),
		"images/ok/Tariboyfile.yaml": filepath.Join(outside, "Tariboyfile.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, link)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	got := inv(t, root)
	for _, s := range got.Skills {
		if s.Name == "evil" || (s.Name == "good2" && s.Err == "") {
			t.Fatalf("followed a symlink: %+v", s)
		}
	}
	if len(got.Prompts) != 0 {
		t.Fatalf("prompt symlink listed: %+v", got.Prompts)
	}
	for _, i := range got.Images {
		if i.Name == "evil" || (i.Name == "ok" && i.Err == "") {
			t.Fatalf("followed a symlink: %+v", i)
		}
	}
}

func TestInventoryMissingDirs(t *testing.T) {
	got := inv(t, t.TempDir())
	if len(got.Images)+len(got.Skills)+len(got.Prompts) != 0 {
		t.Fatalf("%+v", got)
	}
	r, _ := newRegistry(t, nil)
	if _, err := r.Inventory(Store{Name: "s", Kind: KindDir, Source: filepath.Join(t.TempDir(), "gone")}); err == nil {
		t.Fatal("missing root accepted")
	}
}

func TestInventorySkillNameMustMatchDirectory(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "skills/other/SKILL.md"), goodSkill)
	got := inv(t, root)
	if len(got.Skills) != 1 || got.Skills[0].Name != "other" || got.Skills[0].Err == "" {
		t.Fatalf("%+v", got.Skills)
	}
}

func TestInventoryCapsRows(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxRows+5; i++ {
		write(t, filepath.Join(root, fmt.Sprintf("prompts/p%04d.md", i)), "p")
		write(t, filepath.Join(root, fmt.Sprintf("images/i%04d/Tariboyfile.yaml", i)), "schema_version: 3\n")
	}
	got := inv(t, root)
	if len(got.Images) != maxRows+1 || got.Images[maxRows].Err != "… 5 more not shown" {
		t.Fatalf("images %d %+v", len(got.Images), got.Images[maxRows])
	}
	if len(got.Prompts) != maxRows+1 || got.Prompts[maxRows].Err != "… 5 more not shown" {
		t.Fatalf("prompts %d", len(got.Prompts))
	}
}

func TestInventoryDuplicateSkillKeepsFirst(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".agents/skills/x/SKILL.md"), "---\nname: x\ndescription: from agents\n---\n")
	write(t, filepath.Join(root, ".claude/skills/x/SKILL.md"), "---\nname: x\ndescription: from claude\n---\n")
	got := inv(t, root).Skills
	if len(got) != 2 || got[0].Err != "" || got[0].Description != "from agents" ||
		got[1].Name != "x" || !strings.Contains(got[1].Err, "duplicate skill name") {
		t.Fatalf("%+v", got)
	}
}
