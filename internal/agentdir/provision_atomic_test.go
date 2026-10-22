package agentdir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/image"
)

func TestProvisionUnpackFailureLeavesNoImageDir(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	imgStore := &image.Store{Dir: filepath.Join(base, "images")}
	buildImage(t, imgStore, "basic")
	ref := image.Ref{Name: "basic", Tag: "latest"}
	archive, err := imgStore.ArchivePath(ref)
	if err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("not a gzip archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	l := New(filepath.Join(base, "agents"), "smoke")
	a := agent.Agent{Name: "smoke", ImageRef: "basic:latest", Plugins: []string{"loop"}}
	if err := Provision(l, a, imgStore, ref); err == nil {
		t.Fatal("Provision from a broken archive succeeded")
	}
	assertAbsent(t, l.ImageDir())

	if err := os.WriteFile(archive, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Provision(l, a, imgStore, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(l.ImageDir(), "prompt", "template.json")); err != nil {
		t.Fatalf("image not unpacked: %v", err)
	}
}

func TestProvisionAgainLeavesNoStaleImageFiles(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	imgStore := &image.Store{Dir: filepath.Join(base, "images")}
	buildImage(t, imgStore, "basic")
	ref := image.Ref{Name: "basic", Tag: "latest"}
	l := New(filepath.Join(base, "agents"), "smoke")
	a := agent.Agent{Name: "smoke", ImageRef: "basic:latest", Plugins: []string{"loop"}}
	if err := Provision(l, a, imgStore, ref); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(l.ImageDir(), "skills", "gone")
	writeMarker(t, stale, "stale")
	// Leftovers of an unpack interrupted by a crash.
	leftovers := []string{filepath.Join(l.Root, ".unpack-123456"), filepath.Join(l.Root, ".unpack-backup-image")}
	for _, dir := range leftovers {
		writeMarker(t, dir, "leftover")
	}

	if err := Provision(l, a, imgStore, ref); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, stale)
	for _, dir := range leftovers {
		assertAbsent(t, dir)
	}
	if _, err := os.Stat(filepath.Join(l.ImageDir(), "prompt", "template.json")); err != nil {
		t.Fatalf("image not unpacked: %v", err)
	}
}
