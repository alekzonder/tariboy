package agentskills

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func writeSkillFile(t *testing.T, dir, rel, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func validSkill(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	writeSkillFile(t, dir, "SKILL.md", "---\nname: "+name+"\ndescription: Use this skill for tests.\n---\n# Skill\n", 0o600)
	return dir
}

func TestPrepareReturnsSortedFilesAndMetadata(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "code-review")
	writeSkillFile(t, dir, "scripts/check.sh", "#!/bin/sh\nexit 0\n", 0o755)
	writeSkillFile(t, dir, "references/rules.md", "rules\n", 0o644)

	resolved := imagefile.ResolvedDirectory{Source: "./skills/code-review", Path: dir, Category: "source"}
	got, err := Prepare(resolved)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"SKILL.md", "references/rules.md", "scripts/check.sh"}
	paths := make([]string, len(got.Files))
	for i, file := range got.Files {
		paths[i] = file.RelativePath
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("paths = %#v, want %#v", paths, wantPaths)
	}
	if got.Metadata.Name != "code-review" || got.Metadata.Description != "Use this skill for tests." || got.Metadata.Source != resolved.Source || got.Metadata.Category != "source" || got.Metadata.ArchiveRoot != "skills/code-review" || got.Metadata.FileCount != 3 || got.Metadata.Size <= 0 || len(got.Metadata.TreeSHA256) != 64 {
		t.Fatalf("metadata = %#v", got.Metadata)
	}
	if !got.Files[2].Executable || got.Files[0].Executable {
		t.Fatalf("executable normalization = %#v", got.Files)
	}
	again, err := Prepare(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if again.Metadata.TreeSHA256 != got.Metadata.TreeSHA256 {
		t.Fatalf("tree hashes differ: %s != %s", again.Metadata.TreeSHA256, got.Metadata.TreeSHA256)
	}
}

func TestPrepareRequiresRegularSkillMarkdown(t *testing.T) {
	parent := t.TempDir()
	missing := filepath.Join(parent, "missing")
	if err := os.Mkdir(missing, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(imagefile.ResolvedDirectory{Path: missing}); err == nil {
		t.Fatal("accepted missing SKILL.md")
	}
	target := filepath.Join(parent, "target.md")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(parent, "linked")
	if err := os.Mkdir(linked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(linked, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(imagefile.ResolvedDirectory{Path: linked}); err == nil {
		t.Fatal("accepted symlink SKILL.md")
	}
}

func TestPrepareRejectsFrontmatterNameDifferentFromBasename(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "directory-name")
	writeSkillFile(t, dir, "SKILL.md", "---\nname: other-name\ndescription: mismatch\n---\n", 0o600)
	if _, err := Prepare(imagefile.ResolvedDirectory{Path: dir}); err == nil {
		t.Fatal("accepted mismatched frontmatter name")
	}
}

func TestPrepareRejectsLinksAndSpecialFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "linked-skill")
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "reference.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := Prepare(imagefile.ResolvedDirectory{Path: dir}); err == nil {
			t.Fatal("accepted symlink")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "hardlinked-skill")
		original := filepath.Join(dir, "original.md")
		writeSkillFile(t, dir, "original.md", "same inode", 0o600)
		if err := os.Link(original, filepath.Join(dir, "linked.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := Prepare(imagefile.ResolvedDirectory{Path: dir}); err == nil {
			t.Fatal("accepted hardlink")
		}
	})
	t.Run("fifo", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "fifo-skill")
		if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Prepare(imagefile.ResolvedDirectory{Path: dir}); err == nil {
			t.Fatal("accepted FIFO")
		}
	})
}

func TestPrepareRejectsIntermediateDirectorySwap(t *testing.T) {
	parent := t.TempDir()
	dir := validSkill(t, parent, "swapped-skill")
	original := filepath.Join(dir, "references")
	writeSkillFile(t, dir, "references/local.md", "local", 0o600)
	outside := t.TempDir()
	writeSkillFile(t, outside, "secret.md", "secret", 0o600)

	swapped := false
	_, err := prepareWithHook(imagefile.ResolvedDirectory{Path: dir}, func(rel string) error {
		if rel != "references" || swapped {
			return nil
		}
		swapped = true
		if err := os.Rename(original, original+"-old"); err != nil {
			return err
		}
		return os.Symlink(outside, original)
	})
	if err == nil {
		t.Fatal("accepted an intermediate directory swapped for a symlink")
	}
}

func TestPrepareRejectsAncestorDirectorySwap(t *testing.T) {
	parent := t.TempDir()
	ancestor := filepath.Join(parent, "ancestor")
	dir := validSkill(t, ancestor, "ancestor-swap-skill")
	outside := t.TempDir()
	validSkill(t, outside, "ancestor-swap-skill")

	resolved := imagefile.ResolvedDirectory{Path: dir}
	if err := os.Rename(ancestor, ancestor+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, ancestor); err != nil {
		t.Fatal(err)
	}
	_, err := Prepare(resolved)
	if err == nil {
		t.Fatal("accepted a skill root reached through a swapped symlink ancestor")
	}
}

func TestPrepareEnforcesPerFileLimit(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "large-skill")
	writeSkillFile(t, dir, "large.bin", strings.Repeat("x", MaxFileBytes+1), 0o600)
	if _, err := Prepare(imagefile.ResolvedDirectory{Path: dir}); err == nil {
		t.Fatal("accepted oversized file")
	}
}

func TestPrepareHashChangesWithPathModeOrBytes(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "hash-skill")
	writeSkillFile(t, dir, "scripts/check.sh", "one", 0o600)
	resolved := imagefile.ResolvedDirectory{Path: dir}
	first, err := Prepare(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "scripts/check.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	modeChanged, err := Prepare(resolved)
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, dir, "scripts/check.sh", "two", 0o700)
	bytesChanged, err := Prepare(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if first.Metadata.TreeSHA256 == modeChanged.Metadata.TreeSHA256 || modeChanged.Metadata.TreeSHA256 == bytesChanged.Metadata.TreeSHA256 {
		t.Fatalf("hashes did not reflect mode/bytes: %s %s %s", first.Metadata.TreeSHA256, modeChanged.Metadata.TreeSHA256, bytesChanged.Metadata.TreeSHA256)
	}
}

func TestValidateSetRejectsDuplicateNamesAndAggregateLimits(t *testing.T) {
	duplicate := Prepared{Metadata: Metadata{Name: "same", Size: 1}}
	if err := ValidateSet([]Prepared{duplicate, duplicate}); err == nil {
		t.Fatal("accepted duplicate skill names")
	}
	tooMany := make([]Prepared, MaxSkills+1)
	for i := range tooMany {
		tooMany[i].Metadata.Name = "skill-" + strings.Repeat("x", i%50) + string(rune('a'+i%26))
	}
	if err := ValidateSet(tooMany); err == nil {
		t.Fatal("accepted too many skills")
	}
	if err := ValidateSet([]Prepared{{Metadata: Metadata{Name: "one", Size: MaxImageSkillBytes}}, {Metadata: Metadata{Name: "two", Size: 1}}}); err == nil {
		t.Fatal("accepted aggregate bytes over limit")
	}
}

func TestValidatePreparedRejectsTamperedPortableMetadata(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "portable-skill")
	prepared, err := Prepare(imagefile.ResolvedDirectory{Source: "./portable-skill", Path: dir, Category: "source"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrepared(prepared); err != nil {
		t.Fatalf("valid prepared skill rejected: %v", err)
	}
	tests := map[string]func(Prepared) Prepared{
		"tree hash":    func(got Prepared) Prepared { got.Metadata.TreeSHA256 = strings.Repeat("0", 64); return got },
		"description":  func(got Prepared) Prepared { got.Metadata.Description = "tampered"; return got },
		"file bytes":   func(got Prepared) Prepared { got.Files[0].Body = []byte("tampered"); return got },
		"archive root": func(got Prepared) Prepared { got.Metadata.ArchiveRoot = "skills/other"; return got },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			got := prepared
			got.Files = append([]File(nil), prepared.Files...)
			if err := ValidatePrepared(mutate(got)); err == nil {
				t.Fatal("accepted tampered prepared skill")
			}
		})
	}
}
