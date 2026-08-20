package image

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func seed(t *testing.T, st *Store, name string) {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "b.md"), []byte("BODY "+name), 0o600); err != nil {
		t.Fatal(err)
	}
	im := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "context"}},
		Prompts:       []imagefile.Prompt{{Filepath: filepath.Join(src, "b.md")}},
		Dir:           src,
	}
	if _, err := Build(im, Ref{Name: name, Tag: "latest"}, st, fixedClock()); err != nil {
		t.Fatal(err)
	}
}

func TestStoreListAndRender(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	if l, err := st.List(); err != nil || len(l) != 0 {
		t.Fatalf("empty list: %v %v", l, err)
	}
	seed(t, st, "bbb")
	seed(t, st, "aaa")
	list, err := st.List()
	if err != nil || len(list) != 2 || list[0].Name != "aaa" || list[1].Name != "bbb" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	prompt, err := st.RenderPrompt(Ref{Name: "aaa", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "BODY aaa") || !strings.Contains(prompt, "i-am-done") {
		t.Fatalf("render missing content: %q", prompt)
	}
	// tail renders after the body (recency principle)
	if strings.LastIndex(prompt, "i-am-done") < strings.Index(prompt, "BODY aaa") {
		t.Fatalf("tail not after body: %q", prompt)
	}
}

func TestStoreRemoveAndUnpack(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	seed(t, st, "app")
	dest := t.TempDir()
	if err := st.Unpack(Ref{Name: "app", Tag: "latest"}, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		t.Fatalf("unpack missing manifest: %v", err)
	}
	if err := st.Remove(Ref{Name: "app", Tag: "latest"}); err != nil {
		t.Fatal(err)
	}
	if st.Exists(Ref{Name: "app", Tag: "latest"}) {
		t.Fatal("image still present after Remove")
	}
	if err := st.Remove(Ref{Name: "app", Tag: "latest"}); err == nil {
		t.Fatal("removing absent image should error")
	}
}

func TestBuildNeverOverwritesAnExistingRef(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "app", Tag: "latest"}
	seed(t, st, ref.Name)

	beforeArchive, err := os.ReadFile(st.tarPath(ref))
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}

	src := t.TempDir()
	prompt := filepath.Join(src, "changed.md")
	if err := os.WriteFile(prompt, []byte("CHANGED BODY"), 0o600); err != nil {
		t.Fatal(err)
	}
	im := &imagefile.Imagefile{
		SchemaVersion: 1,
		Prompts:       []imagefile.Prompt{{Filepath: prompt}},
		Dir:           src,
	}
	if _, err := Build(im, ref, st, fixedClock()); !errors.Is(err, ErrExists) {
		t.Fatalf("rebuild error = %v, want ErrExists", err)
	}

	afterArchive, err := os.ReadFile(st.tarPath(ref))
	if err != nil {
		t.Fatal(err)
	}
	after, err := st.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterArchive) != string(beforeArchive) || after.Digest != before.Digest {
		t.Fatal("failed immutable rebuild changed the existing image")
	}
}

func TestReservedDefaultRefsCannotBeRemoved(t *testing.T) {
	for _, ref := range []Ref{{Name: "bare", Tag: "latest"}, {Name: "basic", Tag: "latest"}} {
		st := &Store{Dir: t.TempDir()}
		if err := os.MkdirAll(st.refDir(ref), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(st.tarPath(ref), []byte("managed"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := st.Remove(ref); !errors.Is(err, ErrReserved) {
			t.Fatalf("Remove(%s) error = %v, want ErrReserved", ref, err)
		}
		if !st.Exists(ref) {
			t.Fatalf("reserved image %s was removed", ref)
		}
	}

	ordinary := Ref{Name: "basic", Tag: "custom"}
	st := &Store{Dir: t.TempDir()}
	if err := os.MkdirAll(st.refDir(ordinary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(st.tarPath(ordinary), []byte("ordinary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.Remove(ordinary); err != nil {
		t.Fatalf("Remove(%s): %v", ordinary, err)
	}
}

// TestBuildPacksSkillsDir asserts that a Tariboyfile's skills: directory is
// packed into the image tarball under skills/<name>/... (Task 6 carry-forward:
// the packing code in writeArchive existed but had no coverage).
func TestBuildPacksSkillsDir(t *testing.T) {
	src := t.TempDir()
	skillDir := filepath.Join(src, "myskill")
	if err := os.MkdirAll(filepath.Join(skillDir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# my skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "sub", "helper.md"), []byte("helper content"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := filepath.Join(src, "task.md")
	if err := os.WriteFile(task, []byte("DO THE TASK"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := &Store{Dir: t.TempDir()}
	im := &imagefile.Imagefile{
		SchemaVersion: 1,
		Plugins:       []imagefile.Plugin{{Name: "context"}},
		Prompts:       []imagefile.Prompt{{Filepath: task}},
		Skills:        []string{skillDir},
		Dir:           src,
	}
	ref := Ref{Name: "withskill", Tag: "latest"}
	if _, err := Build(im, ref, st, fixedClock()); err != nil {
		t.Fatal(err)
	}

	names := tarEntryNames(t, st.tarPath(ref))
	want := []string{"skills/myskill/SKILL.md", "skills/myskill/sub/helper.md"}
	for _, w := range want {
		if !names[w] {
			t.Fatalf("archive missing %q; entries: %v", w, names)
		}
	}
	body, err := readFileFromTar(st.tarPath(ref), "skills/myskill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# my skill" {
		t.Fatalf("skill file contents = %q", body)
	}
}

func TestStoreListFiles(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	if _, err := st.ListFiles(Ref{Name: "nope", Tag: "latest"}); err == nil {
		t.Fatal("ListFiles on absent image should error")
	}
	seed(t, st, "app")
	entries, err := st.ListFiles(Ref{Name: "app", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]FileEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	for _, want := range []string{"manifest.json", "PROMPT.md", "PROMPT_TAIL.md", "BODY.md"} {
		e, ok := byPath[want]
		if !ok {
			t.Fatalf("ListFiles missing %q; got %+v", want, entries)
		}
		if e.IsDir {
			t.Fatalf("%q reported as dir", want)
		}
		if e.Size <= 0 {
			t.Fatalf("%q has non-positive size %d", want, e.Size)
		}
	}
	// entries come back sorted by path
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Path > entries[i].Path {
			t.Fatalf("entries not sorted: %+v", entries)
		}
	}
}

func TestStoreReadFile(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	if _, err := st.ReadFile(Ref{Name: "nope", Tag: "latest"}, "BODY.md"); err == nil {
		t.Fatal("ReadFile on absent image should error")
	}
	seed(t, st, "app")
	ref := Ref{Name: "app", Tag: "latest"}

	body, err := st.ReadFile(ref, "BODY.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "BODY app" {
		t.Fatalf("BODY.md = %q", body)
	}

	// missing in-archive file → error
	if _, err := st.ReadFile(ref, "does-not-exist.md"); err == nil {
		t.Fatal("ReadFile of absent member should error")
	}

	// path traversal is rejected before any lookup
	for _, bad := range []string{"../etc/passwd", "skills/../../secret", "/etc/passwd", ".."} {
		if _, err := st.ReadFile(ref, bad); err == nil {
			t.Fatalf("ReadFile(%q) should be rejected", bad)
		}
	}
}

// tarEntryNames lists every member name present in the archive.
func tarEntryNames(t *testing.T, archive string) map[string]bool {
	t.Helper()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out[h.Name] = true
	}
	return out
}
