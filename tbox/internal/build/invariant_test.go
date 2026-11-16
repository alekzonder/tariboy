package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
)

// mustBuild builds dir into store and fails the test on an error.
func mustBuild(t *testing.T, dir string, store imgstore.Store) Result {
	t.Helper()
	res, err := Build(opts(dir, store))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestBuildSeesSameSizeEditWithPreservedMtime rewrites a prompt with a body
// of the same length and puts the old mtime back, as cp -p, rsync -t and tar
// extraction do. The build must still see the change.
func TestBuildSeesSameSizeEditWithPreservedMtime(t *testing.T) {
	trustFreshFiles(t)
	for _, mode := range []string{"in place", "replaced"} {
		t.Run(mode, func(t *testing.T) {
			dir := source(t, basic)
			store := imgstore.Store{Dir: t.TempDir()}
			first := mustBuild(t, dir, store)
			path := filepath.Join(dir, "prompt.md")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			body := "PROMPT TEXT\n"
			if len(body) != int(info.Size()) {
				t.Fatalf("body length %d, file %d", len(body), info.Size())
			}
			if mode == "in place" {
				write(t, path, body)
			} else {
				write(t, path+".new", body)
				if err := os.Rename(path+".new", path); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			second := mustBuild(t, dir, store)
			if !second.Built || second.Digest == first.Digest {
				t.Fatalf("same-size edit not seen: %+v", second)
			}
		})
	}
}

// TestBuildRebuildsOnEveryContentChange pins the central invariant: any
// change to what the image packages triggers a rebuild.
func TestBuildRebuildsOnEveryContentChange(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"exec bit": func(t *testing.T, dir string) {
			if err := os.Chmod(filepath.Join(dir, "skills/alpha/SKILL.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"ROLE.md added": func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "ROLE.md"), "You review code.\n")
		},
		"ROLE.md changed": func(t *testing.T, dir string) {
			// Set up below with a ROLE.md; the same length and mtime.
			path := filepath.Join(dir, "ROLE.md")
			info, _ := os.Stat(path)
			write(t, path, "You TEST code.\n")
			_ = os.Chtimes(path, info.ModTime(), info.ModTime())
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			dir := source(t, basic)
			if name == "ROLE.md changed" {
				write(t, filepath.Join(dir, "ROLE.md"), "You test code.\n")
				age(t, dir)
			}
			store := imgstore.Store{Dir: t.TempDir()}
			first := mustBuild(t, dir, store)
			change(t, dir)
			second := mustBuild(t, dir, store)
			if !second.Built || second.Digest == first.Digest {
				t.Fatalf("change not seen: %+v", second)
			}
		})
	}
}

// extendsSource writes a parent layer with a skill and a prompt and a child
// that extends it, and returns both directories.
func extendsSource(t *testing.T) (parent, child string) {
	t.Helper()
	root := t.TempDir()
	parent, child = filepath.Join(root, "parent"), filepath.Join(root, "demo")
	write(t, filepath.Join(parent, "Tariboyfile.yaml"), "schema_version: 3\nskills:\n  - dir: ./skills/base\nprompts:\n  - file: ./base.md\n")
	write(t, filepath.Join(parent, "skills/base/SKILL.md"), skillBody("base", "one"))
	write(t, filepath.Join(parent, "base.md"), "base prompt\n")
	write(t, filepath.Join(child, "Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.0.0\nextends:\n  - ../parent\nprompts:\n  - file: ./child.md\n")
	write(t, filepath.Join(child, "child.md"), "child prompt\n")
	age(t, root)
	return parent, child
}

func TestBuildRebuildsOnParentLayerEdit(t *testing.T) {
	cases := map[string]string{
		"parent skill":  "skills/base/SKILL.md",
		"parent prompt": "base.md",
	}
	for name, rel := range cases {
		t.Run(name, func(t *testing.T) {
			parent, child := extendsSource(t)
			store := imgstore.Store{Dir: t.TempDir()}
			first := mustBuild(t, child, store)
			if again := mustBuild(t, child, store); again.Built {
				t.Fatal("unchanged source rebuilt")
			}
			path := filepath.Join(parent, rel)
			body, _ := os.ReadFile(path)
			write(t, path, string(body)+"more\n")
			second := mustBuild(t, child, store)
			if !second.Built || second.Digest == first.Digest {
				t.Fatalf("parent edit not seen: %+v", second)
			}
		})
	}
}

// TestBuildNamesTariboyOnlySkillOfPrompt: a ../ prompt inside a skill marked
// only: tariboy passes Load, which sees every skill, and then has no skill
// directory left in the tbox view. The error says why.
func TestBuildNamesTariboyOnlySkillOfPrompt(t *testing.T) {
	dir := source(t, `schema_version: 3
skills:
  - dir: ./skills/alpha
  - dir: ../shared/beta
    only: tariboy
prompts:
  - file: ./prompt.md
  - file: ../shared/beta/notes.md
`)
	write(t, filepath.Join(dir, "../shared/beta/SKILL.md"), skillBody("beta", "one"))
	write(t, filepath.Join(dir, "../shared/beta/notes.md"), "notes\n")
	_, err := Build(opts(dir, imgstore.Store{Dir: t.TempDir()}))
	if err == nil {
		t.Fatal("build passed")
	}
	for _, want := range []string{"prompt ../shared/beta/notes.md is inside skill ../shared/beta", "only: tariboy", "mark the prompt only: tariboy too"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want %q", err, want)
		}
	}
}

// TestUpToDateKeepsStatCache: an up-to-date check that read nothing new does
// not rewrite the stat cache.
func TestUpToDateKeepsStatCache(t *testing.T) {
	trustFreshFiles(t)
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	mustBuild(t, dir, store)
	if res := mustBuild(t, dir, store); res.Built {
		t.Fatal("rebuilt")
	}
	path := store.StatCachePath("demo")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if res := mustBuild(t, dir, store); res.Built {
		t.Fatal("rebuilt")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("stat cache rewritten by an up-to-date check")
	}
}

// TestOldStatCacheFormatIsIgnored: entries written before the cache kept the
// change time and inode never vouch for a file.
func TestOldStatCacheFormatIsIgnored(t *testing.T) {
	trustFreshFiles(t)
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	first := mustBuild(t, dir, store)
	cachePath := store.StatCachePath("demo")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		delete(e, "ctime_ns")
		delete(e, "inode")
	}
	data, _ = json.Marshal(entries)
	write(t, cachePath, string(data))
	path := filepath.Join(dir, "prompt.md")
	info, _ := os.Stat(path)
	write(t, path, "PROMPT TEXT\n")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	reads := 0
	hashHook = func(string) { reads++ }
	t.Cleanup(func() { hashHook = nil })
	second := mustBuild(t, dir, store)
	if !second.Built || second.Digest == first.Digest || reads < len(entries) {
		t.Fatalf("old cache trusted: %+v, %d reads of %d entries", second, reads, len(entries))
	}
}
