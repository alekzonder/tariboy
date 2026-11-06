package agentskills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func memberFile(rel, body string) File {
	sum := sha256.Sum256([]byte(body))
	return File{RelativePath: rel, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), Body: []byte(body)}
}

func portableSkill(t *testing.T, name string, files ...File) Prepared {
	t.Helper()
	skill := memberFile("SKILL.md", "---\nname: "+name+"\ndescription: Use this skill for tests.\n---\n")
	all := append([]File{skill}, files...)
	var total int64
	for _, file := range all {
		total += file.Size
	}
	hash, err := treeHash(all)
	if err != nil {
		t.Fatal(err)
	}
	return Prepared{
		Metadata: Metadata{
			Name: name, Description: "Use this skill for tests.", ArchiveRoot: "skills/" + name,
			FileCount: len(all), Size: total, TreeSHA256: hash,
		},
		Files: all,
	}
}

func TestValidatePreparedErrorTextAndOrder(t *testing.T) {
	tests := map[string]struct {
		mutate func(*Prepared)
		want   string
	}{
		"valid": {mutate: func(*Prepared) {}},
		"no files": {
			mutate: func(p *Prepared) { p.Files = nil; p.Metadata.ArchiveRoot = "wrong" },
			want:   `skill "demo" has invalid file count`,
		},
		"too many files": {
			mutate: func(p *Prepared) { p.Files = make([]File, MaxFilesPerSkill+1) },
			want:   `skill "demo" has invalid file count`,
		},
		"archive root before members": {
			mutate: func(p *Prepared) { p.Metadata.ArchiveRoot = "wrong"; p.Files[0].RelativePath = "../x" },
			want:   `skill "demo" has invalid archive root`,
		},
		"empty path":     {mutate: func(p *Prepared) { p.Files[0].RelativePath = "" }, want: `skill "demo" has invalid member path ""`},
		"unclean path":   {mutate: func(p *Prepared) { p.Files[0].RelativePath = "a//b" }, want: `skill "demo" has invalid member path "a//b"`},
		"dot path":       {mutate: func(p *Prepared) { p.Files[0].RelativePath = "." }, want: `skill "demo" has invalid member path "."`},
		"parent path":    {mutate: func(p *Prepared) { p.Files[0].RelativePath = "../x" }, want: `skill "demo" has invalid member path "../x"`},
		"absolute path":  {mutate: func(p *Prepared) { p.Files[0].RelativePath = "/x" }, want: `skill "demo" has invalid member path "/x"`},
		"backslash path": {mutate: func(p *Prepared) { p.Files[0].RelativePath = `a\b` }, want: `skill "demo" has invalid member path "a\\b"`},
		"path before size": {
			mutate: func(p *Prepared) { p.Files[0].RelativePath = "../x"; p.Files[0].Size = -1 },
			want:   `skill "demo" has invalid member path "../x"`,
		},
		"duplicate": {
			mutate: func(p *Prepared) { p.Files = append(p.Files, memberFile("SKILL.md", "x")) },
			want:   `skill "demo" members are duplicate or unsorted`,
		},
		"unsorted before size": {
			mutate: func(p *Prepared) { p.Files = append(p.Files, File{RelativePath: "A", Size: -1}) },
			want:   `skill "demo" members are duplicate or unsorted`,
		},
		"negative size": {mutate: func(p *Prepared) { p.Files[0].Size = -1 }, want: `skill "demo" member "SKILL.md" has invalid size`},
		"oversize": {
			mutate: func(p *Prepared) {
				p.Files = append(p.Files, File{RelativePath: "z-large", Size: MaxFileBytes + 1, Body: make([]byte, MaxFileBytes+1)})
			},
			want: `skill "demo" member "z-large" has invalid size`,
		},
		"body shorter than size": {mutate: func(p *Prepared) { p.Files[0].Size++ }, want: `skill "demo" member "SKILL.md" has invalid size`},
		"aggregate before integrity": {
			mutate: func(p *Prepared) {
				for i := range 3 {
					p.Files = append(p.Files, memberFile(fmt.Sprintf("z-part-%d", i), strings.Repeat("x", MaxFileBytes)))
				}
				p.Files = append(p.Files, File{RelativePath: "z-part-3", Size: MaxFileBytes, Body: make([]byte, MaxFileBytes)})
			},
			want: fmt.Sprintf(`skill "demo" exceeds %d aggregate bytes`, MaxSkillBytes),
		},
		"integrity": {mutate: func(p *Prepared) { p.Files[0].SHA256 = "00" }, want: `skill "demo" member "SKILL.md" integrity mismatch`},
		"integrity before missing SKILL.md": {
			mutate: func(p *Prepared) { p.Files = []File{{RelativePath: "other", Size: 1, Body: []byte("x")}} },
			want:   `skill "demo" member "other" integrity mismatch`,
		},
		"missing SKILL.md": {
			mutate: func(p *Prepared) { p.Files = []File{memberFile("other", "x")} },
			want:   `skill "demo" is missing SKILL.md`,
		},
		"nil SKILL.md body": {
			mutate: func(p *Prepared) { p.Files[0] = memberFile("SKILL.md", ""); p.Files[0].Body = nil },
			want:   `skill "demo" is missing SKILL.md`,
		},
		"frontmatter name": {
			mutate: func(p *Prepared) {
				p.Files[0] = memberFile("SKILL.md", "---\nname: other\ndescription: Use this skill for tests.\n---\n")
			},
			want: `skill "demo" frontmatter does not match manifest`,
		},
		"frontmatter before aggregate metadata": {
			mutate: func(p *Prepared) { p.Metadata.Description = "x"; p.Metadata.Size = 0 },
			want:   `skill "demo" frontmatter does not match manifest`,
		},
		"file count metadata": {mutate: func(p *Prepared) { p.Metadata.FileCount++ }, want: `skill "demo" aggregate metadata mismatch`},
		"size metadata":       {mutate: func(p *Prepared) { p.Metadata.Size++ }, want: `skill "demo" aggregate metadata mismatch`},
		"aggregate before tree": {
			mutate: func(p *Prepared) { p.Metadata.Size++; p.Metadata.TreeSHA256 = "x" },
			want:   `skill "demo" aggregate metadata mismatch`,
		},
		"tree": {mutate: func(p *Prepared) { p.Metadata.TreeSHA256 = "x" }, want: `skill "demo" tree integrity mismatch`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			skill := portableSkill(t, "demo", memberFile("references/a.md", "a"))
			tc.mutate(&skill)
			err := ValidatePrepared(skill)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidatePreparedReturnsFrontmatterError(t *testing.T) {
	skill := portableSkill(t, "demo")
	skill.Files[0] = memberFile("SKILL.md", "no frontmatter")
	_, want := parseFrontmatter(skill.Files[0].Body)
	if want == nil {
		t.Fatal("frontmatter parsed")
	}
	if err := ValidatePrepared(skill); err == nil || err.Error() != want.Error() {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func prepareDir(dir string, hook func(string) error) (Prepared, error) {
	return prepareWithHook(imagefile.ResolvedDirectory{Path: dir}, hook)
}

func TestWalkVisitsMembersSortedAndSkipsOnlyRootEvalsDirectory(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "walk-skill")
	writeSkillFile(t, dir, "evals/case.json", "{}", 0o600)
	writeSkillFile(t, dir, "references/evals/case.json", "{}", 0o600)
	writeSkillFile(t, dir, "references/b.md", "b", 0o600)

	var visited []string
	got, err := prepareDir(dir, func(rel string) error {
		visited = append(visited, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"SKILL.md", "references", "references/b.md", "references/evals", "references/evals/case.json"}
	if !reflect.DeepEqual(visited, want) {
		t.Fatalf("visited = %#v, want %#v", visited, want)
	}
	if !strings.HasSuffix(got.Files[1].SourcePath, filepath.FromSlash("walk-skill/references/b.md")) {
		t.Fatalf("source path = %q", got.Files[1].SourcePath)
	}
}

func TestWalkDoesNotSkipRootEvalsThatIsNotADirectory(t *testing.T) {
	t.Run("regular file is packaged", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "evals-file")
		writeSkillFile(t, dir, "evals", "x", 0o600)
		got, err := prepareDir(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Files) != 2 || got.Files[0].RelativePath != "SKILL.md" || got.Files[1].RelativePath != "evals" {
			t.Fatalf("files = %#v", got.Files)
		}
	})
	t.Run("symlink is refused", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "evals-link")
		if err := os.Symlink(t.TempDir(), filepath.Join(dir, "evals")); err != nil {
			t.Fatal(err)
		}
		_, err := prepareDir(dir, nil)
		if err == nil || !strings.HasPrefix(err.Error(), `open skill member "evals": `) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestWalkReturnsHookErrorUnwrappedBeforeOpening(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "hook-skill")
	hookErr := errors.New("stop here")
	_, err := prepareDir(dir, func(string) error { return hookErr })
	if err != hookErr {
		t.Fatalf("err = %v", err)
	}
}

func TestWalkErrorTextForMembers(t *testing.T) {
	t.Run("symlink inside subdirectory", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "nested-link")
		writeSkillFile(t, dir, "references/a.md", "a", 0o600)
		if err := os.Symlink(filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "references", "b.md")); err != nil {
			t.Fatal(err)
		}
		_, err := prepareDir(dir, nil)
		if err == nil || !strings.HasPrefix(err.Error(), `open skill member "references/b.md": `) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "hard-link")
		writeSkillFile(t, dir, "a.md", "a", 0o600)
		if err := os.Link(filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")); err != nil {
			t.Fatal(err)
		}
		_, err := prepareDir(dir, nil)
		if err == nil || err.Error() != `skill member "a.md" is a hardlink` {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("oversized file", func(t *testing.T) {
		dir := validSkill(t, t.TempDir(), "large-file")
		writeSkillFile(t, dir, "large.bin", strings.Repeat("x", MaxFileBytes+1), 0o600)
		_, err := prepareDir(dir, nil)
		want := fmt.Sprintf(`read skill member "large.bin": skill file exceeds %d bytes`, MaxFileBytes)
		if err == nil || err.Error() != want {
			t.Fatalf("err = %v, want %q", err, want)
		}
	})
}

func TestWalkChecksHardlinkThenFileCountBeforeReading(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "many-files")
	for i := range MaxFilesPerSkill - 1 {
		writeSkillFile(t, dir, fmt.Sprintf("a-%04d", i), "x", 0o600)
	}
	if _, err := prepareDir(dir, nil); err != nil {
		t.Fatalf("exact limit rejected: %v", err)
	}

	// z-large would fail the size check, so reaching the count error proves the
	// count is checked before the member is read.
	writeSkillFile(t, dir, "z-large", strings.Repeat("x", MaxFileBytes+1), 0o600)
	_, err := prepareDir(dir, nil)
	want := fmt.Sprintf("skill has more than %d files", MaxFilesPerSkill)
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}

	// A hardlink past the limit still reports the link.
	if err := os.Remove(filepath.Join(dir, "z-large")); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, dir, "z-one", "x", 0o600)
	if err := os.Link(filepath.Join(dir, "z-one"), filepath.Join(dir, "z-two")); err != nil {
		t.Fatal(err)
	}
	_, err = prepareDir(dir, nil)
	if err == nil || err.Error() != `skill member "z-one" is a hardlink` {
		t.Fatalf("err = %v", err)
	}
}

func TestWalkEnforcesAggregateBytesAcrossDirectories(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "aggregate-skill")
	for i := range 3 {
		writeSkillFile(t, dir, fmt.Sprintf("d%d/part", i), strings.Repeat("x", MaxFileBytes), 0o600)
	}
	if _, err := prepareDir(dir, nil); err != nil {
		t.Fatalf("under limit rejected: %v", err)
	}
	writeSkillFile(t, dir, "d3/part", strings.Repeat("x", MaxFileBytes), 0o600)
	_, err := prepareDir(dir, nil)
	want := fmt.Sprintf("skill exceeds %d aggregate bytes", MaxSkillBytes)
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestWalkDetectsDirectoryChangedDuringTraversal(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "changing-skill")
	writeSkillFile(t, dir, "references/a.md", "a", 0o600)
	// Backdate the directory: adding a member must move its mtime even when
	// both writes fall inside one filesystem timestamp tick.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "references"), past, past); err != nil {
		t.Fatal(err)
	}
	added := false
	_, err := prepareDir(dir, func(rel string) error {
		if rel != "references/a.md" || added {
			return nil
		}
		added = true
		return os.WriteFile(filepath.Join(dir, "references", "new.md"), []byte("x"), 0o600)
	})
	if err == nil || err.Error() != "skill directory changed while reading" {
		t.Fatalf("err = %v", err)
	}
}

func TestWalkVerifiesMemberByNameAfterReading(t *testing.T) {
	dir := validSkill(t, t.TempDir(), "swap-skill")
	writeSkillFile(t, dir, "references/a.md", "a", 0o600)
	original := filepath.Join(dir, "references")
	swapped := false
	_, err := prepareDir(dir, func(rel string) error {
		if rel != "references/a.md" || swapped {
			return nil
		}
		swapped = true
		return os.Rename(original, original+"-old")
	})
	if err == nil || !strings.HasPrefix(err.Error(), `verify skill member "references": `) {
		t.Fatalf("err = %v", err)
	}
}
