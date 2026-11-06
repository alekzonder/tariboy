package imagesnapshot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/imagesource"
)

func writeSnapshotFile(t *testing.T, name, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, mode); err != nil {
		t.Fatal(err)
	}
}

func writeSnapshotSkill(t *testing.T, dir, name string) {
	t.Helper()
	writeSnapshotFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: Review changes.\n---\nBody\n", 0o600)
	writeSnapshotFile(t, filepath.Join(dir, "check.sh"), "#!/bin/sh\nexit 0\n", 0o700)
}

// assertNoStaging fails when Freeze left a staging directory under root.
func assertNoStaging(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".staging-") {
			t.Fatalf("staging directory left behind: %s", entry.Name())
		}
	}
}

// TestFreezeDigestIsStable pins the content digest of one tree: file order,
// normalized modes, the metadata exclusion, and external skill members all feed
// it, and a changed digest would republish every snapshot.
func TestFreezeDigestIsStable(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "images", "app")
	writeSnapshotFile(t, filepath.Join(source, "b.txt"), "b\n", 0o644)
	writeSnapshotFile(t, filepath.Join(source, "a", "run.sh"), "#!/bin/sh\n", 0o755)
	writeSnapshotFile(t, filepath.Join(source, imagesource.MetadataFilename), "ignored\n", 0o600)
	writeSnapshotSkill(t, filepath.Join(source, "skills", "inner"), "inner")
	writeSnapshotSkill(t, filepath.Join(base, "skills", "review"), "review")

	root := t.TempDir()
	frozen, err := Store{Root: root}.Freeze(source,
		imagefile.SkillEntry{Dir: "./skills/inner"},
		imagefile.SkillEntry{Dir: "../../skills/review"},
		imagefile.SkillEntry{Dir: "$PLUGINS/company/1.0.0/skills/other"},
	)
	if err != nil {
		t.Fatal(err)
	}
	const want = "sha256:17a5fea1f1524ed92e6ebef740ceaa047ae3f28b105e39ca42071923954317d0"
	if frozen.SourceDigest != want {
		t.Fatalf("digest = %s, want %s", frozen.SourceDigest, want)
	}
	if frozen.RelativeDir != strings.TrimPrefix(want, "sha256:") {
		t.Fatalf("relative dir = %s", frozen.RelativeDir)
	}
	target := filepath.Join(root, frozen.RelativeDir)
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	wantSkills := map[string]string{
		"./skills/inner":      filepath.Join(resolved, "skills", "inner"),
		"../../skills/review": filepath.Join(resolved, ".tariboy-snapshot-skills", "review"),
	}
	if len(frozen.SourceSkills) != len(wantSkills) {
		t.Fatalf("source skills = %#v", frozen.SourceSkills)
	}
	for name, path := range wantSkills {
		if frozen.SourceSkills[name] != path {
			t.Fatalf("source skill %s = %q, want %q", name, frozen.SourceSkills[name], path)
		}
	}
	if _, err := os.Lstat(filepath.Join(target, imagesource.MetadataFilename)); !os.IsNotExist(err) {
		t.Fatalf("metadata file was frozen: %v", err)
	}
	for name, mode := range map[string]os.FileMode{
		"b.txt":    0o600,
		"a/run.sh": 0o700,
		".tariboy-snapshot-skills/review/SKILL.md": 0o600,
		".tariboy-snapshot-skills/review/check.sh": 0o700,
	} {
		info, err := os.Lstat(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("%s = %v, %v, want mode %o", name, info, err, mode)
		}
	}
	assertNoStaging(t, root)
}

func TestFreezeRepeatedSourceReusesThePublishedTree(t *testing.T) {
	source := t.TempDir()
	writeSnapshotFile(t, filepath.Join(source, "a.txt"), "a\n", 0o600)
	root := t.TempDir()
	first, err := Store{Root: root}.Freeze(source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Store{Root: root}.Freeze(source)
	if err != nil {
		t.Fatal(err)
	}
	if first.SourceDigest != second.SourceDigest || first.RelativeDir != second.RelativeDir {
		t.Fatalf("frozen sources differ: %#v, %#v", first, second)
	}
	assertNoStaging(t, root)
}

func TestFreezeRequiresASafeRequest(t *testing.T) {
	if _, err := (Store{}).Freeze(t.TempDir()); err == nil || err.Error() != "image snapshot: incomplete freeze request" {
		t.Fatalf("empty root error = %v", err)
	}
	base := t.TempDir()
	regular := filepath.Join(base, "file")
	writeSnapshotFile(t, regular, "x", 0o600)
	link := filepath.Join(base, "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"missing": filepath.Join(base, "missing"), "file": regular, "symlink": link} {
		root := filepath.Join(t.TempDir(), "root")
		if _, err := (Store{Root: root}).Freeze(source); err == nil || err.Error() != "image snapshot: unsafe source root" {
			t.Fatalf("%s source error = %v", name, err)
		}
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatalf("%s source created the root: %v", name, err)
		}
	}
}

func TestFreezeSkillErrors(t *testing.T) {
	t.Run("missing skill directory", func(t *testing.T) {
		source := t.TempDir()
		_, err := Store{Root: t.TempDir()}.Freeze(source, imagefile.SkillEntry{Dir: "./missing"})
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Freeze error = %v", err)
		}
	})
	t.Run("invalid external skill", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "app")
		writeSnapshotFile(t, filepath.Join(source, "a.txt"), "a", 0o600)
		writeSnapshotFile(t, filepath.Join(base, "skills", "broken", "notes.txt"), "no skill file", 0o600)
		root := filepath.Join(t.TempDir(), "root")
		if _, err := (Store{Root: root}).Freeze(source, imagefile.SkillEntry{Dir: "../skills/broken"}); err == nil {
			t.Fatal("Freeze accepted a directory without SKILL.md")
		}
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatalf("root created before skills were validated: %v", err)
		}
	})
	t.Run("invalid inner skill is not prepared", func(t *testing.T) {
		source := t.TempDir()
		writeSnapshotFile(t, filepath.Join(source, "skills", "broken", "notes.txt"), "no skill file", 0o600)
		if _, err := (Store{Root: t.TempDir()}).Freeze(source, imagefile.SkillEntry{Dir: "./skills/broken"}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("duplicate external skill name", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "app")
		writeSnapshotFile(t, filepath.Join(source, "a.txt"), "a", 0o600)
		writeSnapshotSkill(t, filepath.Join(base, "one", "review"), "review")
		writeSnapshotSkill(t, filepath.Join(base, "two", "review"), "review")
		_, err := Store{Root: t.TempDir()}.Freeze(source, imagefile.SkillEntry{Dir: "../one/review"}, imagefile.SkillEntry{Dir: "../two/review"})
		if err == nil || err.Error() != `skills[1]: duplicate skill name "review"` {
			t.Fatalf("Freeze error = %v", err)
		}
	})
}

func TestFreezeReservesTheSkillRootOnlyWithExternalSkills(t *testing.T) {
	for name, write := range map[string]func(t *testing.T, source string){
		"directory": func(t *testing.T, source string) {
			writeSnapshotFile(t, filepath.Join(source, ".tariboy-snapshot-skills", "x"), "x", 0o600)
		},
		"file": func(t *testing.T, source string) {
			writeSnapshotFile(t, filepath.Join(source, ".tariboy-snapshot-skills"), "x", 0o600)
		},
	} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "app")
			write(t, source)
			writeSnapshotSkill(t, filepath.Join(base, "skills", "review"), "review")
			root := t.TempDir()
			if _, err := (Store{Root: root}).Freeze(source); err != nil {
				t.Fatalf("Freeze without external skills: %v", err)
			}
			_, err := Store{Root: root}.Freeze(source, imagefile.SkillEntry{Dir: "../skills/review"})
			if err == nil || err.Error() != "image snapshot: source uses reserved directory .tariboy-snapshot-skills" {
				t.Fatalf("Freeze error = %v", err)
			}
		})
	}
}

func TestFreezeReportsUnsafeEntriesWithItsPrefix(t *testing.T) {
	source := t.TempDir()
	if err := os.Symlink("outside", filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "root")
	_, err := Store{Root: root}.Freeze(source)
	if err == nil || !strings.HasPrefix(err.Error(), "image snapshot: ") || !strings.Contains(err.Error(), "escape") {
		t.Fatalf("Freeze error = %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("root created for an unsafe source: %v", err)
	}
}

func TestFreezeRootErrors(t *testing.T) {
	source := t.TempDir()
	writeSnapshotFile(t, filepath.Join(source, "a.txt"), "a", 0o600)
	blocked := filepath.Join(t.TempDir(), "file")
	writeSnapshotFile(t, blocked, "x", 0o600)
	if _, err := (Store{Root: filepath.Join(blocked, "root")}).Freeze(source); err == nil || !strings.HasPrefix(err.Error(), "mkdir ") {
		t.Fatalf("Freeze error = %v", err)
	}
}

func TestFreezeTightensRootMode(t *testing.T) {
	source := t.TempDir()
	writeSnapshotFile(t, filepath.Join(source, "a.txt"), "a", 0o600)
	root := filepath.Join(t.TempDir(), "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: root}).Freeze(source); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("root = %v, %v", info, err)
	}
}

func TestFreezeRemovesStagingWhenACopyFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file modes")
	}
	source := t.TempDir()
	writeSnapshotFile(t, filepath.Join(source, "a.txt"), "a", 0o600)
	unreadable := filepath.Join(source, "b.txt")
	writeSnapshotFile(t, unreadable, "b", 0o600)
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	_, err := Store{Root: root}.Freeze(source)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Freeze error = %v", err)
	}
	assertNoStaging(t, root)
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("root after failure = %v, %v", entries, err)
	}
}
