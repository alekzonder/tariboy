package portablearchive

import (
	"archive/tar"
	"bytes"
	"path/filepath"
	"testing"
)

// A compose archive carries several image archives, so it may be larger than
// the default 64 MiB, but not larger than 1 GiB.
func TestComposeLimitsAdmitLargerArchivesUpToOneGiB(t *testing.T) {
	body := []byte("x")
	archive := makeArchive(t, fileManifest("compose.yaml", body), []tarEntry{{name: "compose.yaml", typeflag: tar.TypeReg, body: body}})
	limits := ComposeLimits()
	if _, err := Stage(bytes.NewReader(archive), 100<<20, filepath.Join(t.TempDir(), "staged"), limits); err != nil {
		t.Fatalf("stage of a declared 100 MiB archive: %v", err)
	}
	if _, err := Stage(bytes.NewReader(archive), 1<<30+1, filepath.Join(t.TempDir(), "staged"), limits); err == nil {
		t.Fatal("stage of an archive over 1 GiB succeeded")
	}
	if ComposeMemberBytes != 64<<20 {
		t.Fatalf("member limit = %d, want 64 MiB", ComposeMemberBytes)
	}
}
