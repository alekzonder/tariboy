package image

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// A ref whose name or tag is a dot path component must never place staging
// or content files outside the store directory.
func TestPublishWithDotRefWritesNothingOutsideStore(t *testing.T) {
	for _, ref := range []Ref{{Name: "..", Tag: "v1"}, {Name: ".", Tag: "v1"}, {Name: "app", Tag: ".."}} {
		parent := t.TempDir()
		store := &Store{Dir: filepath.Join(parent, "images")}
		if err := os.Mkdir(store.Dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: t.TempDir()}, imagefile.ResolveRoots{}, ref, store, fixedClock(), nil); err == nil {
			t.Errorf("BuildV2(%v) succeeded", ref)
		}
		if _, err := store.stageBytes(ref, "install", []byte("x")); err == nil {
			t.Errorf("stageBytes(%v) succeeded", ref)
		}
		if err := store.install(ref, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", filepath.Join(parent, "missing")); err == nil {
			t.Errorf("install(%v) succeeded", ref)
		}
		entries, err := os.ReadDir(parent)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "images" {
			t.Errorf("ref %v wrote outside the store: %v", ref, entries)
		}
		inside, err := os.ReadDir(store.Dir)
		if err != nil {
			t.Fatal(err)
		}
		if ref.Name == "." && len(inside) != 0 {
			t.Errorf("ref %v wrote into the store root: %v", ref, inside)
		}
	}
}
