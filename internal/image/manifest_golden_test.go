package image

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// TestManifestWithSkillsMatchesGolden pins the on-disk and API JSON of a
// manifest built with skills. The golden files were captured before skill
// metadata moved to agentskills.Metadata, so any drift in field names, order
// or omitempty rules fails here. Set TARIBOY_UPDATE_GOLDEN=1 to rewrite them.
func TestManifestWithSkillsMatchesGolden(t *testing.T) {
	source := t.TempDir()
	writeTestSkill(t, filepath.Join(source, "skills"), "code-review")
	src := &imagefile.V2{SchemaVersion: 2, ImageVersion: "1.2.3", Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/code-review"}}}
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "reviewer", Tag: "v1"}
	built, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, fixedClock(), nil)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := store.ReadFile(ref, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := ValidateV2Detailed(src, imagefile.ResolveRoots{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"built": built, "inspected": inspected, "validated": validated.Skills} {
		got, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		checkManifestGolden(t, "manifest-skills-"+name+".json", got)
	}
	checkManifestGolden(t, "manifest-skills-archived.json", archived)
}

func checkManifestGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("TARIBOY_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s drifted:\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}
