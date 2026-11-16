package imgstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/flock"
)

func image(t *testing.T, version, body string) archive.Image {
	t.Helper()
	prompts := []archive.PromptLayer{{Source: "./p.md", Category: "source", Base: "p.md", Body: []byte(body)}}
	img, err := archive.Assemble("demo", version, time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), nil, prompts, nil)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestParseRef(t *testing.T) {
	for in, want := range map[string]Ref{
		"demo":         {"demo", "latest"},
		"demo:0.3.0":   {"demo", "0.3.0"},
		"my.image:dev": {"my.image", "dev"},
	} {
		got, err := ParseRef(in)
		if err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	for _, in := range []string{"My Image", "a/b", "", "x:", "..", "a:b/c", ":tag", "a:B"} {
		if _, err := ParseRef(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestPublishResolveAndMove(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	one := image(t, "1.0.0", "one")
	moved, err := s.Publish("demo", one, []string{"1.0.0", "latest"})
	if err != nil || len(moved) != 0 {
		t.Fatalf("moved %v err %v", moved, err)
	}
	for _, ref := range []string{"demo", "demo:latest", "demo:1.0.0"} {
		r, _ := ParseRef(ref)
		digest, path, err := s.Resolve(r)
		if err != nil || digest != one.Manifest.Digest || filepath.Base(path) != digest+".tar.gz" {
			t.Fatalf("%s: %q %q %v", ref, digest, path, err)
		}
	}
	if _, err := s.Open(Ref{"demo", "latest"}); err != nil {
		t.Fatal(err)
	}
	two := image(t, "1.0.0", "two")
	moved, err = s.Publish("demo", two, []string{"1.0.0", "latest"})
	if err != nil || len(moved) != 2 {
		t.Fatalf("moved %v err %v", moved, err)
	}
	if _, err := os.Stat(s.refPath("demo", one.Manifest.Digest)); err != nil {
		t.Fatalf("old ref gone: %v", err)
	}
	moved, _ = s.Publish("demo", two, []string{"latest"})
	if len(moved) != 0 {
		t.Fatalf("same content moved %v", moved)
	}
}

func TestRemoveKeepsTaggedRef(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	img := image(t, "1.0.0", "one")
	if _, err := s.Publish("demo", img, []string{"1.0.0", "latest"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(Ref{"demo", "latest"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Resolve(Ref{"demo", "1.0.0"}); err != nil {
		t.Fatalf("still tagged ref lost: %v", err)
	}
	if err := s.Remove(Ref{"demo", "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.nameDir("demo")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("name dir left: %v", err)
	}
	if err := s.Remove(Ref{"demo", "1.0.0"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if entries, _ := s.List(); len(entries) != 0 {
		t.Fatalf("%v", entries)
	}
}

func TestListAndImport(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	img := image(t, "2.0.0", "x")
	var buf bytes.Buffer
	if err := img.Write(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := s.ImportArchive("other", &buf, []string{"2.0.0", "latest"})
	if err != nil || got.Manifest.Digest != img.Manifest.Digest {
		t.Fatalf("%v %v", got.Manifest.Digest, err)
	}
	entries, err := s.List()
	if err != nil || len(entries) != 2 || entries[0].Tag != "2.0.0" || entries[0].ImageVersion != "2.0.0" || entries[0].Digest != img.Manifest.Digest {
		t.Fatalf("%+v %v", entries, err)
	}
	if _, err := s.ImportArchive("other", bytes.NewReader([]byte("junk")), nil); err == nil {
		t.Fatal("junk accepted")
	}
}

func TestFingerprintRoundTrip(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if _, _, ok := s.Fingerprint("demo"); ok {
		t.Fatal("unexpected record")
	}
	if err := s.SetFingerprint("demo", "fp", "dg"); err != nil {
		t.Fatal(err)
	}
	if fp, dg, ok := s.Fingerprint("demo"); !ok || fp != "fp" || dg != "dg" {
		t.Fatalf("%q %q %v", fp, dg, ok)
	}
	if err := s.SetFingerprint("../x", "a", "b"); err == nil {
		t.Fatal("bad name accepted")
	}
}

func TestSourceDirIsKeptAcrossFingerprintUpdates(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if _, ok := s.SourceDir("demo"); ok {
		t.Fatal("source dir of an unbuilt image")
	}
	if err := s.SetSourceDir("demo", "/a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.SourceDir("demo"); ok {
		t.Fatal("source dir recorded without a fingerprint")
	}
	if err := s.SetFingerprint("demo", "fp1", "d1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSourceDir("demo", "/a"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFingerprint("demo", "fp2", "d2"); err != nil {
		t.Fatal(err)
	}
	if dir, ok := s.SourceDir("demo"); !ok || dir != "/a" {
		t.Fatalf("source dir %q %v", dir, ok)
	}
	if fp, d, ok := s.Fingerprint("demo"); !ok || fp != "fp2" || d != "d2" {
		t.Fatalf("fingerprint %q %q %v", fp, d, ok)
	}
}

func TestCheckNameAndTagRejectLeadingPunctuation(t *testing.T) {
	for _, name := range []string{".demo", "-demo", "_demo", ".", ".."} {
		if CheckName(name) == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	for _, name := range []string{"demo", "0demo", "my.image", "a-b_c"} {
		if err := CheckName(name); err != nil {
			t.Errorf("name %q: %v", name, err)
		}
	}
	for _, tag := range []string{".hidden", ".tmp-1", ".", ".."} {
		if CheckTag(tag) == nil {
			t.Errorf("tag %q accepted", tag)
		}
	}
	for _, tag := range []string{"latest", "1.0.0", "-dev", "_x"} {
		if err := CheckTag(tag); err != nil {
			t.Errorf("tag %q: %v", tag, err)
		}
	}
}

func TestOpenRejectsArchiveOfOtherDigest(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	one, two := image(t, "1.0.0", "one"), image(t, "1.0.0", "two")
	if _, err := s.Publish("demo", one, []string{"latest"}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := two.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.refPath("demo", one.Manifest.Digest), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(Ref{"demo", "latest"}); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("err = %v", err)
	}
}

func TestRemoveChecksNameAndTag(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if _, err := s.Publish("demo", image(t, "1.0.0", "one"), []string{"latest"}); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []Ref{{"../demo", "latest"}, {"demo", "../tags/latest"}, {"demo", ".latest"}} {
		if err := s.Remove(ref); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("%v: err = %v", ref, err)
		}
	}
	if _, _, err := s.Resolve(Ref{"demo", "latest"}); err != nil {
		t.Fatal(err)
	}
}

func TestListReadsOnlyTheManifest(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	img := image(t, "1.0.0", "one")
	if _, err := s.Publish("demo", img, []string{"1.0.0", "latest"}); err != nil {
		t.Fatal(err)
	}
	// Corrupt everything after manifest.json: a listing still shows the tag,
	// an Open still refuses it.
	broken := img
	broken.Members = append([]archive.Member{}, img.Members...)
	broken.Members[1].Body = []byte("{}")
	var buf bytes.Buffer
	if err := broken.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.refPath("demo", img.Manifest.Digest), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	reads := 0
	manifestReadHook = func(string) { reads++ }
	t.Cleanup(func() { manifestReadHook = nil })
	entries, err := s.List()
	if err != nil || len(entries) != 2 || entries[1].Tag != "latest" || entries[1].Digest != img.Manifest.Digest || entries[1].ImageVersion != "1.0.0" || entries[1].BuiltAt != img.Manifest.BuiltAt {
		t.Fatalf("%+v %v", entries, err)
	}
	if reads != 1 {
		t.Fatalf("manifest read %d times for one digest", reads)
	}
	if _, err := s.Open(Ref{"demo", "latest"}); err == nil {
		t.Fatal("broken archive opened")
	}
}

func TestPruneUntagged(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	one, two := image(t, "1.0.0", "one"), image(t, "1.0.0", "two")
	if _, err := s.Publish("demo", one, []string{"latest"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish("demo", two, []string{"latest"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish("other", one, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish("other", two, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	// A temporary file of an interrupted write is not an archive.
	if err := os.WriteFile(filepath.Join(s.refsDir("demo"), ".tmp-x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := s.PruneUntagged("demo")
	if err != nil || len(removed) != 1 || removed[0] != one.Manifest.Digest {
		t.Fatalf("removed %v err %v", removed, err)
	}
	if _, err := os.Stat(s.refPath("demo", one.Manifest.Digest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("untagged archive left: %v", err)
	}
	if _, err := s.Open(Ref{"demo", "latest"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.refPath("other", one.Manifest.Digest)); err != nil {
		t.Fatalf("other image pruned: %v", err)
	}
	if _, err := s.PruneUntagged("../demo"); err == nil {
		t.Fatal("bad name accepted")
	}
	all, err := s.PruneAllUntagged()
	if err != nil || len(all) != 1 || all[0] != (Pruned{Name: "other", Digest: one.Manifest.Digest}) {
		t.Fatalf("all %v err %v", all, err)
	}
	if again, err := s.PruneAllUntagged(); err != nil || len(again) != 0 {
		t.Fatalf("again %v err %v", again, err)
	}
	if _, err := os.Stat(filepath.Join(s.refsDir("demo"), ".tmp-x")); err != nil {
		t.Fatalf("temporary file touched: %v", err)
	}
}

// legacyImage publishes img as demo and renames its directory to name, the
// way an image built before CheckName was tightened sits on disk.
func legacyImage(t *testing.T, s Store, name string, img archive.Image) {
	t.Helper()
	if _, err := s.Publish("demo", img, []string{"latest"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(s.nameDir("demo"), s.nameDir(name)); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyNamesStayListableAndRemovable(t *testing.T) {
	for _, name := range []string{"_old", "-old", ".old"} {
		s := Store{Dir: t.TempDir()}
		img := image(t, "1.0.0", "one")
		legacyImage(t, s, name, img)
		entries, err := s.List()
		if err != nil || len(entries) != 1 || entries[0].Name != name {
			t.Fatalf("%s: list %+v %v", name, entries, err)
		}
		ref, err := ParseExistingRef(name)
		if err != nil || ref != (Ref{name, "latest"}) {
			t.Fatalf("%s: parse %v %v", name, ref, err)
		}
		if _, _, err := s.Resolve(ref); err != nil {
			t.Fatalf("%s: resolve %v", name, err)
		}
		if removed, err := s.PruneUntagged(name); err != nil || len(removed) != 0 {
			t.Fatalf("%s: prune %v %v", name, removed, err)
		}
		if pruned, err := s.PruneAllUntagged(); err != nil || len(pruned) != 0 {
			t.Fatalf("%s: prune all %v %v", name, pruned, err)
		}
		if err := s.Remove(ref); err != nil {
			t.Fatalf("%s: remove %v", name, err)
		}
		if _, err := os.Stat(s.nameDir(name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: dir left: %v", name, err)
		}
		if _, err := s.Publish(name, img, []string{"latest"}); err == nil {
			t.Fatalf("%s: new image with a legacy name published", name)
		}
		if _, err := ParseRef(name); err == nil {
			t.Fatalf("%s: ParseRef accepted a legacy name", name)
		}
	}
	for _, in := range []string{".", "..", "a/b", "A", ".:latest", "x:.hidden"} {
		if _, err := ParseExistingRef(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	s := Store{Dir: t.TempDir()}
	for _, ref := range []Ref{{".", "latest"}, {"..", "latest"}} {
		if err := s.Remove(ref); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("%v: err = %v", ref, err)
		}
	}
}

// heldName takes the lock of name in s and returns its release.
func heldName(t *testing.T, s Store, name string) func() {
	t.Helper()
	unlock, err := flock.Lock(s.lockPath(name))
	if err != nil {
		t.Fatal(err)
	}
	return unlock
}

func TestStoreWritersWaitForTheNameLock(t *testing.T) {
	one, two := image(t, "1.0.0", "one"), image(t, "1.0.0", "two")
	var buf bytes.Buffer
	if err := two.Write(&buf); err != nil {
		t.Fatal(err)
	}
	ops := map[string]func(Store) error{
		"publish": func(s Store) error { _, err := s.Publish("demo", two, []string{"latest"}); return err },
		"import": func(s Store) error {
			_, err := s.ImportArchive("demo", bytes.NewReader(buf.Bytes()), []string{"latest"})
			return err
		},
		"remove": func(s Store) error { return s.Remove(Ref{"demo", "latest"}) },
		"prune":  func(s Store) error { _, err := s.PruneUntagged("demo"); return err },
	}
	for name, op := range ops {
		s := Store{Dir: t.TempDir()}
		if _, err := s.Publish("demo", one, []string{"latest"}); err != nil {
			t.Fatal(err)
		}
		unlock := heldName(t, s, "demo")
		done := make(chan error, 1)
		go func() { done <- op(s) }()
		select {
		case err := <-done:
			unlock()
			t.Fatalf("%s ran while the name was locked: %v", name, err)
		case <-time.After(150 * time.Millisecond):
		}
		unlock()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s never ran", name)
		}
	}
}
