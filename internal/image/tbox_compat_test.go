package image

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// readTboxList returns the lines of a list file in the tbox compat fixtures.
func readTboxList(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tboxCompatDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

// TestTboxSkillCategories keeps tbox/testdata/compat/skill-categories.txt
// equal to the case labels of validSkillCategory, read from the source so a
// new category cannot slip past. When this fails, update the file and
// tbox's skillCategories together.
func TestTboxSkillCategories(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "portable_validate.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "validSkillCategory" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, s)
			}
			return true
		})
		return false
	})
	slices.Sort(got)
	want := readTboxList(t, "skill-categories.txt")
	if !slices.Equal(got, want) {
		t.Fatalf("validSkillCategory cases = %v, skill-categories.txt = %v", got, want)
	}
	for _, c := range want {
		if !validSkillCategory(c) {
			t.Fatalf("%q is not a valid category", c)
		}
	}
}

// TestTboxPluginNamePattern keeps tbox/testdata/compat/plugin-name.txt equal
// to the portable plugin name pattern.
func TestTboxPluginNamePattern(t *testing.T) {
	if want := readTboxList(t, "plugin-name.txt"); len(want) != 1 || portablePluginName.String() != want[0] {
		t.Fatalf("pattern %s, plugin-name.txt %v", portablePluginName, want)
	}
}
