package build

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillBody(name, extra string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: Skill %s\n---\n%s\n", name, name, extra)
}

// source writes a small image source and returns its directory.
func source(t *testing.T, tfileBody string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "demo")
	write(t, filepath.Join(dir, "Tariboyfile.yaml"), tfileBody)
	write(t, filepath.Join(dir, "skills/alpha/SKILL.md"), skillBody("alpha", "one"))
	write(t, filepath.Join(dir, "skills/beta/SKILL.md"), skillBody("beta", "one"))
	write(t, filepath.Join(dir, "prompt.md"), "prompt text\n")
	age(t, dir)
	return dir
}

// age moves every mtime under dir an hour back so the stat cache trusts it.
func age(t *testing.T, dir string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	err := filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, old, old)
	})
	if err != nil {
		t.Fatal(err)
	}
}

const basic = `schema_version: 3
image_version: 1.0.0
skills:
  - dir: ./skills/alpha
  - dir: ./skills/beta
    only: tariboy
prompts:
  - file: ./prompt.md
`

func opts(dir string, store imgstore.Store) Options {
	return Options{SourceDir: dir, Store: store, Now: func() time.Time { return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC) }}
}

func refFiles(t *testing.T, store imgstore.Store, name string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(store.Dir, name, "refs"))
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestBuildFirstAndUnchanged(t *testing.T) {
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	first, err := Build(opts(dir, store))
	if err != nil || !first.Built || first.Name != "demo" || !slices.Equal(first.Tags, []string{"1.0.0", "latest"}) {
		t.Fatalf("%+v %v", first, err)
	}
	second, err := Build(opts(dir, store))
	if err != nil || second.Built || second.Digest != first.Digest {
		t.Fatalf("%+v %v", second, err)
	}
	if got := refFiles(t, store, "demo"); len(got) != 1 {
		t.Fatalf("refs %v", got)
	}
}

func TestBuildSkillEditMovesTags(t *testing.T) {
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	first, _ := Build(opts(dir, store))
	write(t, filepath.Join(dir, "skills/alpha/SKILL.md"), skillBody("alpha", "two"))
	second, err := Build(opts(dir, store))
	if err != nil || !second.Built || second.Digest == first.Digest {
		t.Fatalf("%+v %v", second, err)
	}
	slices.Sort(second.Moved)
	if !slices.Equal(second.Moved, []string{"1.0.0", "latest"}) {
		t.Fatalf("moved %v", second.Moved)
	}
	if got := refFiles(t, store, "demo"); len(got) != 2 {
		t.Fatalf("refs %v", got)
	}
}

func TestBuildIgnoresTariboyOnlySkill(t *testing.T) {
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	if _, err := Build(opts(dir, store)); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "skills/beta/SKILL.md"), skillBody("beta", "changed"))
	res, err := Build(opts(dir, store))
	if err != nil || res.Built {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestBuildTouchWithoutChange(t *testing.T) {
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	if _, err := Build(opts(dir, store)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, p := range []string{"skills/alpha/SKILL.md", "prompt.md", "Tariboyfile.yaml"} {
		if err := os.Chtimes(filepath.Join(dir, p), now, now); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Build(opts(dir, store))
	if err != nil || res.Built {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestBuildChangedPromptAndVersion(t *testing.T) {
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	if _, err := Build(opts(dir, store)); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "prompt.md"), "changed\n")
	if res, _ := Build(opts(dir, store)); !res.Built {
		t.Fatal("prompt change not built")
	}
	write(t, filepath.Join(dir, "Tariboyfile.yaml"), basic[:len("schema_version: 3\n")]+"image_version: 1.1.0\n"+basic[len("schema_version: 3\nimage_version: 1.0.0\n"):])
	res, err := Build(opts(dir, store))
	if err != nil || !res.Built || !slices.Contains(res.Tags, "1.1.0") {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestBuildNewTagRebuildsPointers(t *testing.T) {
	dir := source(t, basic)
	store := imgstore.Store{Dir: t.TempDir()}
	first, _ := Build(opts(dir, store))
	o := opts(dir, store)
	o.Tags = []string{"stable"}
	res, err := Build(o)
	if err != nil || !res.Built || res.Digest != first.Digest || len(res.Moved) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestBuildDropsRuntimeAndPlugins(t *testing.T) {
	dir := source(t, `schema_version: 3
plugins:
  - name: some-plugin
skills:
  - dir: ./skills/alpha
prompts:
  - file: ./prompt.md
  - runtime: identity
`)
	store := imgstore.Store{Dir: t.TempDir()}
	res, err := Build(opts(dir, store))
	if err != nil || !res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	img, err := store.Open(imgstore.Ref{Name: "demo", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Manifest.Plugins) != 0 {
		t.Fatalf("plugins %v", img.Manifest.Plugins)
	}
	for _, e := range img.Template.Entries {
		if e.Kind != "file" || e.Runtime != "" {
			t.Fatalf("entry %+v", e)
		}
	}
}

func TestBuildRejectsBadNames(t *testing.T) {
	store := imgstore.Store{Dir: t.TempDir()}
	for _, name := range []string{"My Image", "Demo", "a/b", ".."} {
		dir := source(t, basic)
		o := opts(dir, store)
		o.Name = name
		if _, err := Build(o); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	// The rule allows lowercase letters, digits, dot, underscore, hyphen.
	o := opts(source(t, basic), store)
	o.Name = "my.image_1-x"
	if _, err := Build(o); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
	// A source directory named like "My Image" fails without --name.
	bad := filepath.Join(t.TempDir(), "My Image")
	write(t, filepath.Join(bad, "Tariboyfile.yaml"), "schema_version: 3\nskills: []\nprompts: []\n")
	if _, err := Build(opts(bad, store)); err == nil {
		t.Error("directory name My Image accepted")
	}
	o = opts(source(t, basic), store)
	o.Tags = []string{"Bad Tag"}
	if _, err := Build(o); err == nil {
		t.Error("bad tag accepted")
	}
}

func TestBuildImageReadsBack(t *testing.T) {
	dir := source(t, basic)
	write(t, filepath.Join(dir, "ROLE.md"), "a role\n")
	store := imgstore.Store{Dir: t.TempDir()}
	if _, err := Build(opts(dir, store)); err != nil {
		t.Fatal(err)
	}
	img, err := store.Open(imgstore.Ref{Name: "demo", Tag: "1.0.0"})
	if err != nil || img.Manifest.Role == nil || len(img.Manifest.Skills) != 1 {
		t.Fatalf("%+v %v", img.Manifest, err)
	}
	var _ archive.Image = img
}

// trustFreshFiles lets the stat cache trust files changed just now: age()
// sets an old mtime, but the change time of every file is fresh.
func trustFreshFiles(t *testing.T) {
	prev := racyWindow
	racyWindow = 0
	t.Cleanup(func() { racyWindow = prev })
}

func TestBuildUnchangedIsFast(t *testing.T) {
	trustFreshFiles(t)
	dir := source(t, basic)
	for i := 0; i < 200; i++ {
		write(t, filepath.Join(dir, "skills/alpha/refs", fmt.Sprintf("f%03d.md", i)), fmt.Sprintf("file %d\n", i))
	}
	age(t, dir)
	store := imgstore.Store{Dir: t.TempDir()}
	if res, err := Build(opts(dir, store)); err != nil || !res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	reads := 0
	hashHook = func(string) { reads++ }
	defer func() { hashHook = nil }()
	start := time.Now()
	res, err := Build(opts(dir, store))
	elapsed := time.Since(start)
	if err != nil || res.Built {
		t.Fatalf("%+v %v", res, err)
	}
	if reads != 0 {
		t.Fatalf("second build hashed %d file bodies", reads)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("unchanged build took %s", elapsed)
	}
	t.Logf("unchanged build of 200 skill files: %s", elapsed)
}

func TestFingerprintIsPathIndependent(t *testing.T) {
	a, b := source(t, basic), source(t, basic)
	fa, fb := fingerprintOf(t, a), fingerprintOf(t, b)
	if fa != fb {
		t.Fatalf("%s != %s", fa, fb)
	}
}

func fingerprintOf(t *testing.T, dir string) string {
	t.Helper()
	loaded, err := tfile.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := Fingerprint(loaded.ForTbox())
	if err != nil {
		t.Fatal(err)
	}
	return fp
}
