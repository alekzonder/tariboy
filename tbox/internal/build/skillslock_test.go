package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/skillslock"
	srcpkg "github.com/alekzonder/tariboy/tbox/internal/source"
)

const v2Image = `schema_version: 2
image_version: 1.0.0
%s
skills:
  - dir: ./.agents/skills/a
  - dir: ./.agents/skills/b
  - dir: ./.agents/skills/c
prompts:
  - file: ./.agents/skills/a/a.md
  - file: ./own.md
`

// v2Env is a Store checkout under $TBOX_HOME/stores/acme with image img,
// whose skills come from skills-lock.json: a from github (served by a local
// repository), b from a local path of the image lock (which wins over the
// Store root's b), c from a local path of the Store root lock.
type v2Env struct {
	home  paths.Home
	root  string
	img   string
	repo  string
	store imgstore.Store
	opts  Options
}

func newV2Env(t *testing.T, plugins string) v2Env {
	t.Helper()
	repo := newRemote(t)
	write(t, filepath.Join(repo, "skills/a/SKILL.md"), skillBody("a", "remote a"))
	write(t, filepath.Join(repo, "skills/a/a.md"), "prompt of a\n")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "skill a")
	home := paths.Home(t.TempDir())
	root := filepath.Join(home.StoresDir(), "acme")
	img := filepath.Join(root, "images", "img")
	write(t, filepath.Join(img, "Tariboyfile.yaml"), fmt.Sprintf(v2Image, plugins))
	write(t, filepath.Join(img, "own.md"), "own prompt\n")
	write(t, filepath.Join(root, "skills/b/SKILL.md"), skillBody("b", "image b"))
	write(t, filepath.Join(root, "store-skills/b/SKILL.md"), skillBody("b", "store b"))
	write(t, filepath.Join(root, "skills/c/SKILL.md"), skillBody("c", "store c"))
	bHash, err := skillslock.Hash(filepath.Join(root, "skills/b"))
	if err != nil {
		t.Fatal(err)
	}
	cHash, err := skillslock.Hash(filepath.Join(root, "skills/c"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(img, skillslock.Filename), fmt.Sprintf(`{
  "version": 1,
  "skills": {
    "a": {"source": "acme/skills", "sourceType": "github", "skillPath": "skills/a/SKILL.md", "computedHash": "stale"},
    "b": {"source": "../../skills/b", "sourceType": "local", "computedHash": %q}
  }
}
`, bHash))
	// unused is never restored: only skills the Tariboyfile reaches are.
	write(t, filepath.Join(root, skillslock.Filename), fmt.Sprintf(`{"version":1,"skills":{
  "b": {"source": "./store-skills/b", "sourceType": "local", "computedHash": "x"},
  "c": {"source": "./skills/c", "sourceType": "local", "computedHash": %q},
  "unused": {"source": "nobody/nothing", "sourceType": "github", "skillPath": "skills/unused/SKILL.md", "computedHash": "x"}
}}
`, cHash))
	age(t, root)
	store := imgstore.Store{Dir: home.ImagesDir()}
	o := opts(img, store)
	o.StoreRoot = root
	missing := filepath.Join(t.TempDir(), "missing")
	o.Resolver = &srcpkg.Resolver{
		Home: home,
		Git:  gitx.Git{AllowLocal: true, Timeout: 30 * time.Second},
		RewriteURL: func(u string) string {
			if u == "https://github.com/acme/skills.git" {
				return repo
			}
			return missing
		},
	}
	return v2Env{home: home, root: root, img: img, repo: repo, store: store, opts: o}
}

// treeState lists every path under dir with its mode and content hash.
func treeState(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s %v %d", p, info.Mode(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(body)
			line += " " + hex.EncodeToString(sum[:])
		}
		out = append(out, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestV2SkillsLockBuild(t *testing.T) {
	env := newV2Env(t, "")
	before := treeState(t, env.root)
	res, err := Build(env.opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Built {
		t.Fatalf("not built %+v", res)
	}
	if !slices.Equal(res.Warnings, []string{"skill a: content differs from skills-lock.json"}) {
		t.Fatalf("warnings %q", res.Warnings)
	}
	if after := treeState(t, env.root); !slices.Equal(before, after) {
		t.Fatalf("Store changed:\nbefore %q\nafter  %q", before, after)
	}
	img, err := env.store.Open(imgstore.Ref{Name: "img", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"a": "remote a", "b": "image b", "c": "store c"} {
		meta, members, ok := img.Skill(name)
		if !ok || meta.Source != "./.agents/skills/"+name || meta.Category != "source" {
			t.Fatalf("skill %s: %+v %v", name, meta, ok)
		}
		var found bool
		for _, m := range members {
			found = found || strings.Contains(string(m.Body), want)
		}
		if !found {
			t.Fatalf("skill %s does not hold %q", name, want)
		}
	}
	var sources []string
	for _, e := range img.Template.Entries {
		sources = append(sources, e.Source)
	}
	if !slices.Equal(sources, []string{"./.agents/skills/a/a.md", "./own.md"}) {
		t.Fatalf("prompts %q", sources)
	}

	lk, err := lock.Read(lock.PathFor(env.img, env.home))
	if err != nil {
		t.Fatal(err)
	}
	key := lock.Key("git", "https://github.com/acme/skills.git", "skills/a", "")
	if e, ok := lk.Entries[key]; !ok || len(lk.Entries) != 1 || e.Commit != gitIn(t, env.repo, "rev-parse", "HEAD") {
		t.Fatalf("lock %+v", lk)
	}

	// Pinned: the second build needs no network.
	if err := os.RemoveAll(env.repo); err != nil {
		t.Fatal(err)
	}
	env.opts.Resolver.Git.Timeout = 5 * time.Second
	again, err := Build(env.opts)
	if err != nil || again.Built || again.Digest != res.Digest {
		t.Fatalf("second build %+v %v", again, err)
	}
	if after := treeState(t, env.root); !slices.Equal(before, after) {
		t.Fatal("Store changed by the second build")
	}
}

func TestV2SkillsLockUpdateFollowsDefaultBranch(t *testing.T) {
	env := newV2Env(t, "")
	first, err := Build(env.opts)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(env.repo, "skills/a/SKILL.md"), skillBody("a", "remote a two"))
	gitIn(t, env.repo, "commit", "-q", "-am", "a two")
	if res, err := Build(env.opts); err != nil || res.Built {
		t.Fatalf("without --update %+v %v", res, err)
	}
	env.opts.Update = true
	res, err := Build(env.opts)
	if err != nil || !res.Built || res.Digest == first.Digest || len(res.Updates) != 1 {
		t.Fatalf("with --update %+v %v", res, err)
	}
}

func TestV2SkillsLockPluginsWarning(t *testing.T) {
	env := newV2Env(t, "plugins:\n  - name: mail")
	res, err := Build(env.opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(res.Warnings, "image declares plugins; skills that need the tariboy daemon will not work here") {
		t.Fatalf("warnings %q", res.Warnings)
	}
	if res, err = Build(env.opts); err != nil || res.Built || len(res.Warnings) != 2 {
		t.Fatalf("up-to-date build keeps the warnings: %+v %v", res, err)
	}
}

func TestV2SkillMissingEverywhereFailsNamingIt(t *testing.T) {
	env := newV2Env(t, "")
	body := strings.Replace(fmt.Sprintf(v2Image, ""), "./.agents/skills/c\n", "./.agents/skills/c\n  - dir: ./.agents/skills/d\n", 1)
	write(t, filepath.Join(env.img, "Tariboyfile.yaml"), body)
	if _, err := Build(env.opts); err == nil || !strings.Contains(err.Error(), "./.agents/skills/d") {
		t.Fatalf("err %v", err)
	}
}

func TestV2PluginsPathFailsNamingEntry(t *testing.T) {
	dir := source(t, "schema_version: 2\nskills:\n  - dir: $PLUGINS/mail/skills/mail\n")
	if _, err := Build(opts(dir, imgstore.Store{Dir: t.TempDir()})); err == nil || !strings.Contains(err.Error(), "$PLUGINS/mail/skills/mail") {
		t.Fatalf("err %v", err)
	}
	dir = source(t, "schema_version: 2\nprompts:\n  - file: $PLUGINS/mail/p.md\n")
	if _, err := Build(opts(dir, imgstore.Store{Dir: t.TempDir()})); err == nil || !strings.Contains(err.Error(), "$PLUGINS/mail/p.md") {
		t.Fatalf("err %v", err)
	}
}

func TestV2LocalOnlyLockNeedsNoResolver(t *testing.T) {
	dir := source(t, "schema_version: 2\nskills:\n  - dir: ./.agents/skills/alpha\n")
	write(t, filepath.Join(dir, skillslock.Filename), `{"version":1,"skills":{"alpha":{"source":"./skills/alpha","sourceType":"local","computedHash":"x"}}}`)
	res, err := Build(opts(dir, imgstore.Store{Dir: t.TempDir()}))
	if err != nil || !res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(dir, lock.Filename)); err == nil {
		t.Fatal("a local-only skills lock wrote tbox-lock.json")
	}
}
