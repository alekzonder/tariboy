package image

import (
	"testing"
	"time"
)

func bareFixedClock() time.Time { return time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC) }

func TestEnsureBareSeedsImage(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := EnsureBare(s, bareFixedClock); err != nil {
		t.Fatal(err)
	}
	man, err := s.Inspect(BareRef)
	if err != nil {
		t.Fatal(err)
	}
	if man.SchemaVersion != 2 || man.Bare {
		t.Fatalf("manifest = %#v, want transparent schema v2", man)
	}
	if man.Name != "bare" || man.Tag != "latest" {
		t.Fatalf("ref = %s:%s, want bare:latest", man.Name, man.Tag)
	}
	if man.Harness.Type != "" || len(man.Env) != 0 || len(man.RequiresSecrets) != 0 {
		t.Fatalf("bare carries runtime settings: %#v", man)
	}
	template, err := s.ReadTemplate(BareRef)
	if err != nil || len(template.Entries) != 0 {
		t.Fatalf("template=%#v err=%v", template, err)
	}
}

func TestEnsureBareIdempotent(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := EnsureBare(s, bareFixedClock); err != nil {
		t.Fatal(err)
	}
	first, err := s.Inspect(BareRef)
	if err != nil {
		t.Fatal(err)
	}
	// Second call must be a no-op (existing image untouched, digest unchanged).
	if err := EnsureBare(s, func() time.Time { return bareFixedClock().Add(time.Hour) }); err != nil {
		t.Fatal(err)
	}
	second, err := s.Inspect(BareRef)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == "" || first.Digest != second.Digest {
		t.Fatalf("digest changed: %q -> %q", first.Digest, second.Digest)
	}
}
