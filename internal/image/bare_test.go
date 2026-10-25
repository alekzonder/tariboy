package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/version"
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
	if man.ImageVersion != version.Version || man.Digest != RefID(BareRef.Name, version.Version) {
		t.Fatalf("image_version=%q id=%q, want the daemon release %q", man.ImageVersion, man.Digest, version.Version)
	}
	if man.Harness.Type != "" || len(man.Env) != 0 || len(man.RequiresSecrets) != 0 || len(man.Plugins) != 0 {
		t.Fatalf("bare carries runtime settings: %#v", man)
	}
	template, err := s.ReadTemplate(BareRef)
	if err != nil || len(template.Entries) != 0 {
		t.Fatalf("template=%#v err=%v", template, err)
	}
}

// TestEnsureBareIdempotent: one release builds one generation. A restart
// rewrites the same id instead of piling up archives.
func TestEnsureBareIdempotent(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := EnsureBare(s, bareFixedClock); err != nil {
		t.Fatal(err)
	}
	first, err := s.Inspect(BareRef)
	if err != nil {
		t.Fatal(err)
	}
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
	archives, err := filepath.Glob(filepath.Join(s.refs().RefsDir(BareRef.Name), "*.tar.gz"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives = %v err=%v, want exactly one", archives, err)
	}
}

// TestEnsureBareReplacesLegacyV1 reproduces a store seeded before schema v2:
// bare:latest moves to the current release, and the legacy archive stays
// readable for agents pinned to it.
func TestEnsureBareReplacesLegacyV1(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	legacyID := installRawBare(t, s, `{"schema_version":1,"name":"bare","tag":"latest","bare":true,"plugins":[],"layers":[]}`)
	if old, err := s.Inspect(BareRef); err != nil || old.SchemaVersion != 1 {
		t.Fatalf("legacy fixture = %#v err=%v", old, err)
	}

	if err := EnsureBare(s, bareFixedClock); err != nil {
		t.Fatal(err)
	}
	man, err := s.Inspect(BareRef)
	if err != nil {
		t.Fatal(err)
	}
	if man.SchemaVersion != 2 || man.ImageVersion != version.Version || len(man.Plugins) != 0 {
		t.Fatalf("manifest = %#v, want schema v2 of release %s without plugins", man, version.Version)
	}
	if man.Digest == legacyID {
		t.Fatalf("bare:latest still points at the legacy archive %s", legacyID)
	}
	if pinned, err := s.InspectPinned(BareRef, legacyID); err != nil || pinned.SchemaVersion != 1 {
		t.Fatalf("pinned legacy = %#v err=%v, want it kept", pinned, err)
	}
}

// TestEnsureBareMovesPreviousRelease: a bare built by an earlier release is
// replaced by this release's generation, and the earlier one is kept.
func TestEnsureBareMovesPreviousRelease(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	previous, err := buildV2(&imagefile.V2{SchemaVersion: 2, ImageVersion: "0.0.1", Plugins: []imagefile.V2Plugin{}, Prompts: []imagefile.PromptEntry{}}, imagefile.ResolveRoots{}, BareRef, s, bareFixedClock, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureBare(s, bareFixedClock); err != nil {
		t.Fatal(err)
	}
	man, err := s.Inspect(BareRef)
	if err != nil {
		t.Fatal(err)
	}
	if man.Digest != RefID(BareRef.Name, version.Version) {
		t.Fatalf("bare:latest = %s, want release %s", man.Digest, version.Version)
	}
	if pinned, err := s.InspectPinned(BareRef, previous.Digest); err != nil || pinned.ImageVersion != "0.0.1" {
		t.Fatalf("pinned previous = %#v err=%v, want it kept", pinned, err)
	}
}

// installRawBare stores a hand-written v1 archive under its content digest and
// points bare:latest at it, the way an old daemon left it on disk.
func installRawBare(t *testing.T, s *Store, manifest string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"manifest.json": manifest, "PROMPT.md": "", "PROMPT_TAIL.md": "", "BODY.md": ""} {
		if err := writeTarFileMode(tw, name, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	id := hex.EncodeToString(sum[:])
	if err := os.MkdirAll(s.refs().RefsDir(BareRef.Name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.contentPath(BareRef.Name, id), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTag(BareRef, id); err != nil {
		t.Fatal(err)
	}
	return id
}
