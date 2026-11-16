package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodSkill = "---\nname: demo\ndescription: A demo skill.\n---\n\nbody\n"

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// skillDir returns <tmp>/<name> populated by setup.
func skillDir(t *testing.T, name string, setup func(dir string)) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	setup(dir)
	return dir
}

func TestPrepareValid(t *testing.T) {
	dir := skillDir(t, "demo", func(dir string) {
		writeFile(t, filepath.Join(dir, "SKILL.md"), goodSkill, 0o644)
		writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "#!/bin/sh\n", 0o755)
		writeFile(t, filepath.Join(dir, "evals", "case.md"), "ignored", 0o644)
	})
	got, err := Prepare(dir, "local", "custom")
	if err != nil {
		t.Fatal(err)
	}
	m := got.Metadata
	if m.Name != "demo" || m.Description != "A demo skill." || m.Source != "local" || m.Category != "custom" {
		t.Fatalf("metadata = %+v", m)
	}
	if m.ArchiveRoot != "skills/demo" || m.FileCount != 2 || len(got.Files) != 2 {
		t.Fatalf("metadata = %+v files = %d", m, len(got.Files))
	}
	if want := int64(len(goodSkill) + len("#!/bin/sh\n")); m.Size != want {
		t.Fatalf("size = %d, want %d", m.Size, want)
	}
	if got.Files[0].RelativePath != "SKILL.md" || got.Files[0].Executable {
		t.Fatalf("file0 = %+v", got.Files[0])
	}
	if got.Files[1].RelativePath != "scripts/run.sh" || !got.Files[1].Executable {
		t.Fatalf("file1 = %+v", got.Files[1])
	}
	for _, f := range got.Files {
		if strings.HasPrefix(f.RelativePath, "evals/") {
			t.Fatalf("evals not excluded: %s", f.RelativePath)
		}
	}
}

func TestPrepareRejects(t *testing.T) {
	cases := []struct {
		name  string
		dir   string
		setup func(dir string)
		want  string
	}{
		{"name mismatch", "other", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), goodSkill, 0o644)
		}, "must match source directory"},
		{"bad name", "Bad_Name", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: Bad_Name\ndescription: d\n---\n", 0o644)
		}, "invalid Agent Skill name"},
		{"missing frontmatter", "demo", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), "# no frontmatter\n", 0o644)
		}, "must start with YAML frontmatter"},
		{"missing SKILL.md", "demo", func(dir string) {
			writeFile(t, filepath.Join(dir, "other.md"), "x", 0o644)
		}, "must contain regular SKILL.md"},
		{"symlink member", "demo", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), goodSkill, 0o644)
			if err := os.Symlink("SKILL.md", filepath.Join(dir, "link.md")); err != nil {
				t.Fatal(err)
			}
		}, "link.md"},
		{"hardlink member", "demo", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), goodSkill, 0o644)
			if err := os.Link(filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "copy.md")); err != nil {
				t.Fatal(err)
			}
		}, "hardlink"},
		{"file over limit", "demo", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), goodSkill, 0o644)
			f, err := os.Create(filepath.Join(dir, "big.bin"))
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Truncate(MaxFileBytes + 1); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}, "exceeds"},
		{"too many files", "demo", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), goodSkill, 0o644)
			for i := 0; i < MaxFilesPerSkill; i++ {
				writeFile(t, filepath.Join(dir, "f", strings.Repeat("a", 1)+string(rune('a'+i%26)), string(rune('a'+i/26))+".txt"), "x", 0o644)
			}
		}, "more than"},
		{"fifo", "demo", func(dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), goodSkill, 0o644)
			mkfifo(t, filepath.Join(dir, "pipe"))
		}, "not a regular file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := skillDir(t, tc.dir, tc.setup)
			_, err := Prepare(dir, "local", "custom")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateSet(t *testing.T) {
	a := Prepared{Metadata: Metadata{Name: "a", Size: 1}}
	b := Prepared{Metadata: Metadata{Name: "b", Size: 1}}
	if err := ValidateSet([]Prepared{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSet([]Prepared{a, a}); err == nil || !strings.Contains(err.Error(), "duplicate skill name") {
		t.Fatalf("duplicate err = %v", err)
	}
	if err := ValidateSet([]Prepared{{}}); err == nil {
		t.Fatal("empty name must be rejected")
	}
}

func TestTreeHashMatchesTariboy(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "compat", "skills", "hello-skill.tree_sha256"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Prepare(filepath.Join("..", "..", "testdata", "compat", "skills", "hello-skill"), "compat", "custom")
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.TreeSHA256 != strings.TrimSpace(string(want)) {
		t.Fatalf("tree hash = %s, want %s", got.Metadata.TreeSHA256, strings.TrimSpace(string(want)))
	}
	if got.Metadata.FileCount != 3 {
		t.Fatalf("file count = %d", got.Metadata.FileCount)
	}
}
