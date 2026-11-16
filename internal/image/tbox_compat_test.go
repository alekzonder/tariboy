package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// tboxCompatDir holds the fixtures that pin the archive format tbox shares
// with this package.
var tboxCompatDir = filepath.Join("..", "..", "tbox", "testdata", "compat")

// TestTboxCompatDigest builds the tbox v2-basic fixture with BuildV2 and
// checks the content digest tbox pins for the same source.
func TestTboxCompatDigest(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join(tboxCompatDir, "v2-basic"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := imagefile.ParseV2(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{Dir: t.TempDir()}
	manifest, err := BuildV2(src, imagefile.ResolveRoots{}, Ref{Name: "compat-basic", Tag: "latest"}, store, fixedClock(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(tboxCompatDir, "v2-basic.digest"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Digest != strings.TrimSpace(string(want)) {
		t.Fatalf("digest = %s, v2-basic.digest = %s", manifest.Digest, strings.TrimSpace(string(want)))
	}
}

// TestTboxArchiveImports validates the archive tbox commits for the v3-marked
// fixture the way an import does, with the plugin contract enforced.
func TestTboxArchiveImports(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(tboxCompatDir, "v3-marked.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ValidatePortableArchiveContract(body, Ref{Name: "compat-marked", Tag: "latest"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Digest == "" || manifest.Digest != manifest.ID {
		t.Fatalf("digest = %q, id = %q", manifest.Digest, manifest.ID)
	}
	if len(manifest.Plugins) != 0 {
		t.Fatalf("plugins = %+v, want none", manifest.Plugins)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0].Name != "hello-skill" {
		t.Fatalf("skills = %+v, want hello-skill only", manifest.Skills)
	}
}

// TestTboxRemoteArchiveImports validates the archive tbox builds from a git
// skill and a git prompt the way an import does.
func TestTboxRemoteArchiveImports(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(tboxCompatDir, "v3-remote.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ValidatePortableArchiveContract(body, Ref{Name: "demo", Tag: "latest"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0].Name != "gamma" || manifest.Skills[0].Category != "source" {
		t.Fatalf("skills = %+v", manifest.Skills)
	}
}
