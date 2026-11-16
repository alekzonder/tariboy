package skill

import (
	"crypto/sha256"
	"encoding/hex"
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

// fileOf builds a File with its size and digest.
func fileOf(rel, body string, executable bool) File {
	sum := sha256.Sum256([]byte(body))
	return File{RelativePath: rel, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), Executable: executable, Body: []byte(body)}
}

// preparedOf returns metadata that matches files, as an archive carries it.
func preparedOf(t *testing.T, name string, files []File) Metadata {
	t.Helper()
	hash, err := treeHash(files)
	if err != nil {
		t.Fatal(err)
	}
	var size int64
	for _, f := range files {
		size += f.Size
	}
	return Metadata{Name: name, Description: "A demo skill.", Source: "./skills/demo", Category: "source",
		ArchiveRoot: "skills/" + name, FileCount: len(files), Size: size, TreeSHA256: hash}
}

func TestValidatePrepared(t *testing.T) {
	good := []File{fileOf("SKILL.md", goodSkill, false), fileOf("scripts/run.sh", "#!/bin/sh\n", true)}
	if err := ValidatePrepared(preparedOf(t, "demo", good), good); err != nil {
		t.Fatalf("valid skill: %v", err)
	}
	// An archive skill is judged by its members: a root-level evals member
	// is kept and hashed, not skipped.
	withEvals := []File{fileOf("SKILL.md", goodSkill, false), fileOf("evals/case.md", "case", false)}
	if err := ValidatePrepared(preparedOf(t, "demo", withEvals), withEvals); err != nil {
		t.Fatalf("skill with evals: %v", err)
	}
	cases := map[string]struct {
		edit func(*Metadata, []File) []File
		want string
	}{
		"no files":         {func(m *Metadata, f []File) []File { return nil }, "invalid file count"},
		"archive root":     {func(m *Metadata, f []File) []File { m.ArchiveRoot = "skills/other"; return f }, "invalid archive root"},
		"bad path":         {func(m *Metadata, f []File) []File { f[1].RelativePath = "../run.sh"; return f }, "invalid member path"},
		"unsorted":         {func(m *Metadata, f []File) []File { f[0], f[1] = f[1], f[0]; return f }, "duplicate or unsorted"},
		"size":             {func(m *Metadata, f []File) []File { f[1].Size++; return f }, "invalid size"},
		"digest":           {func(m *Metadata, f []File) []File { f[1].Body = []byte("#!/bin/xx\n"); return f }, "integrity mismatch"},
		"missing SKILL.md": {func(m *Metadata, f []File) []File { return f[1:] }, "missing SKILL.md"},
		"description":      {func(m *Metadata, f []File) []File { m.Description = "Other."; return f }, "frontmatter does not match manifest"},
		"name":             {func(m *Metadata, f []File) []File { m.Name, m.ArchiveRoot = "other", "skills/other"; return f }, "frontmatter does not match manifest"},
		"file count":       {func(m *Metadata, f []File) []File { m.FileCount = 3; return f }, "aggregate metadata mismatch"},
		"tree hash":        {func(m *Metadata, f []File) []File { m.TreeSHA256 = strings.Repeat("0", 64); return f }, "tree integrity mismatch"},
		"executable bit":   {func(m *Metadata, f []File) []File { f[1].Executable = false; return f }, "tree integrity mismatch"},
		"bad frontmatter":  {func(m *Metadata, f []File) []File { f[0] = fileOf("SKILL.md", "no frontmatter", false); return f }, "frontmatter"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			files := append([]File(nil), good...)
			meta := preparedOf(t, "demo", files)
			files = tc.edit(&meta, files)
			err := ValidatePrepared(meta, files)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
