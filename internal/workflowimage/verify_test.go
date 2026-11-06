package workflowimage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// VerifyArchive recomputes the digest of an exported workflow image without a
// store and leaves nothing behind.
func TestVerifyArchiveReturnsTheExportedDigest(t *testing.T) {
	src, _ := newRegistry(t)
	m, _, err := src.Publish(writeSource(t, "1.0.0"), t0)
	if err != nil {
		t.Fatal(err)
	}
	archive := exportArchive(t, src, "demo", "1.0.0")
	staging := filepath.Join(t.TempDir(), "verify")
	info, err := VerifyArchive(bytes.NewReader(archive), int64(len(archive)), staging)
	if err != nil {
		t.Fatal(err)
	}
	if info != (ArchiveInfo{Name: "demo", Version: "1.0.0", Digest: m.Digest}) {
		t.Fatalf("info = %+v, want demo 1.0.0 %s", info, m.Digest)
	}
	if entries, err := os.ReadDir(staging); err != nil || len(entries) != 0 {
		t.Fatalf("staging left behind: %v, %v", entries, err)
	}
}

func TestVerifyArchiveRefusesDigestMismatch(t *testing.T) {
	src, _ := newRegistry(t)
	m, _, err := src.Publish(writeSource(t, "1.0.0"), t0)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	meta := archiveMetadata{Name: m.Name, Version: m.Version, Digest: "0000000000000000000000000000000000000000000000000000000000000000", Executable: []string{"scripts/check.sh"}}
	if err := writeArchive(&buf, src.Store.ContentDir(m.Name, m.Digest), m.Files, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyArchive(bytes.NewReader(buf.Bytes()), int64(buf.Len()), t.TempDir()); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("err = %v; want ErrDigestMismatch", err)
	}
}
