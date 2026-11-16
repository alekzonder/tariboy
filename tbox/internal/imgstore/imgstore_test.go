package imgstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
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
