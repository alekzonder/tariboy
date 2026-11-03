package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/stores"
)

// fakeNpx is a stand-in for `npx skills experimental_install`. It copies each
// skill directory of $NPX_SKILLS_SRC that skills-lock.json names into
// .agents/skills and, like the real installer, skips a skill whose SKILL.md
// already exists. $NPX_EXTRA installs one more skill that no lock names.
const fakeNpx = `#!/bin/sh
test -f skills-lock.json || exit 0
for src in "$NPX_SKILLS_SRC"/*/; do
  name=$(basename "$src")
  grep -q "\"$name\"" skills-lock.json || continue
  test -f ".agents/skills/$name/SKILL.md" && continue
  mkdir -p .agents/skills && cp -R "$src" ".agents/skills/$name"
done
if test -n "${NPX_EXTRA-}"; then
  mkdir -p ".agents/skills/$NPX_EXTRA"
  printf -- '---\nname: %s\ndescription: extra\n---\n' "$NPX_EXTRA" >".agents/skills/$NPX_EXTRA/SKILL.md"
fi
`

// autoBuildFixture is a local Git Store with one image whose lock pins the
// cli-text skill, plus the fake installer and its skill source.
type autoBuildFixture struct {
	c      *registry.Ctx
	store  string
	skills string
}

func skillBody(description string) string {
	return "---\nname: cli-text\ndescription: " + description + "\n---\nbody\n"
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func lockFor(t *testing.T, hashes map[string]string) string {
	t.Helper()
	parts := make([]string, 0, len(hashes))
	for name, hash := range hashes {
		parts = append(parts, `"`+name+`":{"source":"../../skills/`+name+`","sourceType":"local","computedHash":"`+hash+`"}`)
	}
	return `{"version":1,"skills":{` + strings.Join(parts, ",") + "}}\n"
}

func installFakeNpx(t *testing.T, skillsSrc string) {
	t.Helper()
	tools := t.TempDir()
	if err := os.WriteFile(filepath.Join(tools, "npx"), []byte(fakeNpx), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NPX_SKILLS_SRC", skillsSrc)
	t.Setenv("NPX_EXTRA", "")
}

func newAutoBuildFixture(t *testing.T) autoBuildFixture {
	t.Helper()
	f := autoBuildFixture{c: localCtx(t), store: t.TempDir(), skills: t.TempDir()}
	writeFiles(t, f.skills, map[string]string{"cli-text/SKILL.md": skillBody("fresh")})
	fresh, err := stores.SkillHash(filepath.Join(f.skills, "cli-text"))
	if err != nil {
		t.Fatal(err)
	}
	installFakeNpx(t, f.skills)
	writeFiles(t, f.store, map[string]string{
		".gitignore":                       ".agents/\n__pycache__/\n",
		"skills-lock.json":                 lockFor(t, nil),
		"images/reviewer/Tariboyfile.yaml": "schema_version: 2\nimage_version: 1.0.0\nplugins: []\nskills: [{dir: ./.agents/skills/cli-text}]\nprompts: [{file: ./instructions.md}]\n",
		"images/reviewer/instructions.md":  "review",
		"images/reviewer/skills-lock.json": lockFor(t, map[string]string{"cli-text": fresh}),
	})
	gitIn(t, f.store, "init", "-q", "-b", "main")
	gitIn(t, f.store, "add", "-A")
	gitIn(t, f.store, "commit", "-q", "-m", "store")
	if _, err := cmdHandler(t, "store.add")(f.c, registry.Params{"name": "team", "source": f.store}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f autoBuildFixture) autoBuild(t *testing.T) (any, error) {
	t.Helper()
	return cmdHandler(t, "image.build")(f.c, registry.Params{"source": "team/reviewer", "initiator": "auto"})
}

func snapshotOf(t *testing.T, c *registry.Ctx, ref string) imagesnapshot.Snapshot {
	t.Helper()
	snapshot, ok, err := imageSnapshotStore(c).Lookup(context.Background(), ref)
	if err != nil || !ok {
		t.Fatalf("snapshot %s: ok %v, err %v", ref, ok, err)
	}
	return snapshot
}

func snapshotFiles(t *testing.T, c *registry.Ctx, snapshot imagesnapshot.Snapshot) map[string]string {
	t.Helper()
	dir, err := imageSnapshotStore(c).Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		body, err := os.ReadFile(path)
		files[filepath.ToSlash(rel)] = string(body)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func userErrorCode(t *testing.T, err error, code, text string) {
	t.Helper()
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != code || !strings.Contains(userErr.Msg, text) {
		t.Fatalf("error = %#v, want code %s with %q", err, code, text)
	}
}

func TestAutoBuildUsesOnlyCommitContent(t *testing.T) {
	// Sequential: sets PATH and NPX_* with t.Setenv.
	f := newAutoBuildFixture(t)
	writeFiles(t, f.store, map[string]string{
		"images/reviewer/.agents/skills/cli-text/SKILL.md": skillBody("stale"),
		"images/reviewer/__pycache__/x.pyc":                "bytecode",
	})

	if _, err := f.autoBuild(t); err != nil {
		t.Fatal(err)
	}
	manifest, err := imageStore(f.c).Inspect(image.Ref{Name: "reviewer", Tag: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0].Description != "fresh" {
		t.Fatalf("image skills = %#v, want the locked cli-text", manifest.Skills)
	}
	auto := snapshotOf(t, f.c, "reviewer:1.0.0")
	for rel, body := range snapshotFiles(t, f.c, auto) {
		if strings.Contains(rel, "__pycache__") || strings.Contains(body, "stale") {
			t.Fatalf("image source holds working-copy file %s", rel)
		}
	}
	if got, err := os.ReadFile(filepath.Join(f.store, "images/reviewer/.agents/skills/cli-text/SKILL.md")); err != nil || !strings.Contains(string(got), "stale") {
		t.Fatalf("automatic build changed the Store working copy: %q, %v", got, err)
	}
	if status := gitIn(t, f.store, "status", "--porcelain"); status != "" {
		t.Fatalf("Store working copy is dirty: %s", status)
	}

	// Reference: a clean git archive of the same commit, the same lock
	// installation, built from its path.
	clean := t.TempDir()
	archive := exec.Command("sh", "-c", `git -C "$1" archive --format=tar HEAD | tar -x -C "$2"`, "sh", f.store, clean)
	if out, err := archive.CombinedOutput(); err != nil {
		t.Fatalf("git archive: %v\n%s", err, out)
	}
	for _, dir := range []string{clean, filepath.Join(clean, "images", "reviewer")} {
		if err := stores.RestoreLock(context.Background(), dir, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cmdHandler(t, "image.build")(f.c, registry.Params{"name": "reference", "path": filepath.Join(clean, "images", "reviewer")}); err != nil {
		t.Fatal(err)
	}
	reference := snapshotOf(t, f.c, "reference:1.0.0")
	if auto.SourceDigest != reference.SourceDigest {
		t.Fatalf("automatic build source %s, clean archive build source %s", auto.SourceDigest, reference.SourceDigest)
	}
	if entries, _ := os.ReadDir(filepath.Join(f.c.BaseDir, "image-build")); len(entries) != 0 {
		t.Fatalf("build directories survived: %v", entries)
	}
}

func TestAutoBuildRefusesSkillsThatDifferFromLock(t *testing.T) {
	// Sequential: sets PATH and NPX_* with t.Setenv.
	f := newAutoBuildFixture(t)
	if _, err := f.autoBuild(t); err != nil {
		t.Fatal(err)
	}
	latest := image.Ref{Name: "reviewer", Tag: "latest"}
	before, err := imageStore(f.c).Inspect(latest)
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, f.store, map[string]string{"images/reviewer/Tariboyfile.yaml": "schema_version: 2\nimage_version: 1.0.1\nplugins: []\nskills: [{dir: ./.agents/skills/cli-text}]\nprompts: [{file: ./instructions.md}]\n"})
	gitIn(t, f.store, "commit", "-q", "-am", "bump")

	t.Run("changed content", func(t *testing.T) {
		writeFiles(t, f.skills, map[string]string{"cli-text/SKILL.md": skillBody("changed upstream")})
		defer writeFiles(t, f.skills, map[string]string{"cli-text/SKILL.md": skillBody("fresh")})
		_, err := f.autoBuild(t)
		userErrorCode(t, err, "skills_lock_mismatch", "Store team image reviewer: skills-lock.json images/reviewer/skills-lock.json: skill cli-text: installed content sha256:")
		userErrorCode(t, err, "skills_lock_mismatch", "does not match computedHash")
	})
	t.Run("extra skill", func(t *testing.T) {
		t.Setenv("NPX_EXTRA", "intruder")
		_, err := f.autoBuild(t)
		// The fake installer adds the extra skill in the Store root and the image.
		userErrorCode(t, err, "skills_lock_mismatch", "Store team image reviewer: skills-lock.json ")
		userErrorCode(t, err, "skills_lock_mismatch", "skills-lock.json images/reviewer/skills-lock.json: skill intruder: is installed but not in skills-lock.json")
	})
	if imageStore(f.c).Exists(image.Ref{Name: "reviewer", Tag: "1.0.1"}) {
		t.Fatal("a lock mismatch published a version")
	}
	if after, err := imageStore(f.c).Inspect(latest); err != nil || after.Digest != before.Digest {
		t.Fatalf("latest moved: %s -> %#v, %v", before.Digest, after.Digest, err)
	}
}

func TestAutoBuildRefusesStoreWithoutOwnGit(t *testing.T) {
	t.Parallel()
	for name, nested := range map[string]bool{"plain directory": false, "inside another repository": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := localCtx(t)
			parent := t.TempDir()
			source := parent
			if nested {
				gitIn(t, parent, "init", "-q")
				source = filepath.Join(parent, "store")
			}
			writeImageV2(t, filepath.Join(source, "images", "reviewer"), "1.0.0")
			if _, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source}); err != nil {
				t.Fatal(err)
			}
			_, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/reviewer", "initiator": "auto"})
			userErrorCode(t, err, "store_not_git", "Store team image reviewer: Store root "+source+" is not the root of a Git repository")
			if imageStore(c).Exists(image.Ref{Name: "reviewer", Tag: "latest"}) {
				t.Fatal("a Store without Git published an image")
			}
		})
	}
}

func TestAutoBuildRefusesSymlinkOutsideCommit(t *testing.T) {
	// Sequential: sets PATH and NPX_* with t.Setenv.
	f := newAutoBuildFixture(t)
	// A link inside the commit is extracted like git archive | tar does.
	if err := os.Symlink("../README.md", filepath.Join(f.store, "docs-link")); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, f.store, map[string]string{"README.md": "readme"})
	gitIn(t, f.store, "add", "-A")
	gitIn(t, f.store, "commit", "-q", "-m", "inside link")
	if _, err := f.autoBuild(t); err == nil || !strings.Contains(err.Error(), "docs-link") {
		// ../README.md from the root escapes it.
		t.Fatalf("link outside the commit: %v", err)
	}
	if err := os.Remove(filepath.Join(f.store, "docs-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("README.md", filepath.Join(f.store, "docs-link")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, f.store, "commit", "-q", "-am", "inside link")
	if _, err := f.autoBuild(t); err != nil {
		t.Fatalf("link inside the commit: %v", err)
	}

	if err := os.Symlink("../../../outside", filepath.Join(f.store, "images", "reviewer", "escape")); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, f.store, map[string]string{"images/reviewer/Tariboyfile.yaml": "schema_version: 2\nimage_version: 1.0.1\nplugins: []\nskills: [{dir: ./.agents/skills/cli-text}]\nprompts: [{file: ./instructions.md}]\n"})
	gitIn(t, f.store, "add", "-A")
	gitIn(t, f.store, "commit", "-q", "-m", "escape")
	_, err := f.autoBuild(t)
	userErrorCode(t, err, "bad_store", "Store team image reviewer: symlink images/reviewer/escape points outside the Store commit")
	if imageStore(f.c).Exists(image.Ref{Name: "reviewer", Tag: "1.0.1"}) {
		t.Fatal("an escaping link published a version")
	}
}

func lockLine(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return rel + "\x00" + hex.EncodeToString(sum[:]) + "\n"
}

func digestOf(lines ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(lines, "")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestAutoBuildRecordsCommitAndLockDigest(t *testing.T) {
	// Sequential: sets PATH and NPX_* with t.Setenv.
	f := newAutoBuildFixture(t)
	if _, err := f.autoBuild(t); err != nil {
		t.Fatal(err)
	}
	shown, err := cmdHandler(t, "image.provenance")(f.c, registry.Params{"ref": "reviewer:latest"})
	if err != nil {
		t.Fatal(err)
	}
	got := shown.(ImageProvenance)
	want := digestOf(lockLine(t, f.store, "skills-lock.json"), lockLine(t, f.store, "images/reviewer/skills-lock.json"))
	if got.GitCommit == nil || *got.GitCommit != gitIn(t, f.store, "rev-parse", "HEAD") {
		t.Fatalf("git_commit = %v", got.GitCommit)
	}
	if got.LockDigest == nil || *got.LockDigest != want {
		t.Fatalf("lock_digest = %v, want %s", got.LockDigest, want)
	}
	if !got.SourceCWD.Valid || got.SourceCWD.Value != filepath.Join(f.store, "images", "reviewer") {
		t.Fatalf("source_cwd = %#v", got.SourceCWD)
	}
}

func TestAutoBuildExtendsLockDigestAndContainment(t *testing.T) {
	// Sequential: sets PATH and NPX_* with t.Setenv.
	f := newAutoBuildFixture(t)
	fresh, err := stores.SkillHash(filepath.Join(f.skills, "cli-text"))
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, f.store, map[string]string{
		"images/parent/Tariboyfile.yaml": "schema_version: 2\nplugins: []\nskills: [{dir: ./.agents/skills/cli-text}]\nprompts: []\n",
		"images/parent/skills-lock.json": lockFor(t, map[string]string{"cli-text": fresh}),
		"images/child/Tariboyfile.yaml":  "schema_version: 2\nimage_version: 2.0.0\nextends: [../parent]\nplugins: []\nskills: []\nprompts: [{file: ./instructions.md}]\n",
		"images/child/instructions.md":   "child",
		"images/child/skills-lock.json":  lockFor(t, map[string]string{"cli-text": fresh}),
	})
	gitIn(t, f.store, "add", "-A")
	gitIn(t, f.store, "commit", "-q", "-m", "extends")
	if _, err := cmdHandler(t, "image.build")(f.c, registry.Params{"source": "team/child", "initiator": "auto"}); err != nil {
		t.Fatal(err)
	}
	want := digestOf(lockLine(t, f.store, "skills-lock.json"), lockLine(t, f.store, "images/parent/skills-lock.json"), lockLine(t, f.store, "images/child/skills-lock.json"))
	if got := snapshotOf(t, f.c, "child:2.0.0").LockDigest; got != want {
		t.Fatalf("lock_digest = %s, want %s", got, want)
	}

	outside := t.TempDir()
	writeImageV2(t, outside, "9.9.9")
	writeFiles(t, f.store, map[string]string{"images/child/Tariboyfile.yaml": "schema_version: 2\nimage_version: 2.0.1\nextends: [" + outside + "]\nplugins: []\nskills: []\nprompts: []\n"})
	gitIn(t, f.store, "commit", "-q", "-am", "outside")
	_, err = cmdHandler(t, "image.build")(f.c, registry.Params{"source": "team/child", "initiator": "auto"})
	userErrorCode(t, err, "bad_store", "Store team image child: extends layer "+outside+" points outside the Store commit")
}
