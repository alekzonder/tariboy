package judge

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestImageContractJudgeRubricIsOneLocalLayerAndChangesBuiltDigest(t *testing.T) {
	sourceDir := t.TempDir()
	for _, name := range []string{"Tariboyfile.yaml", "instructions.md", "rubric.md"} {
		body, err := os.ReadFile(filepath.Join("../../store/images/llm-as-judge", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := &image.Store{Dir: t.TempDir()}
	ref := image.Ref{Name: "llm-as-judge", Tag: "latest"}
	build := func() (image.Manifest, image.PromptTemplate) {
		t.Helper()
		source, err := imagefile.ParseV2(sourceDir)
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := image.BuildV2Mutable(source, imagefile.ResolveRoots{
			Store: filepath.Clean("../../store"), CurrentVersionStore: filepath.Clean("../../store"),
		}, ref, store, func() time.Time { return time.Unix(0, 0) }, nil)
		if err != nil {
			t.Fatal(err)
		}
		template, err := store.ReadTemplate(ref)
		if err != nil {
			t.Fatal(err)
		}
		return manifest, template
	}

	first, template := build()
	wantHash := "85e6735491cbf3900df7eda4bbeb656b15b0430caa2994a5a406b526ffb88fa3"
	count := 0
	for _, entry := range template.Entries {
		if entry.Source == "./rubric.md" {
			count++
			if entry.SHA256 != wantHash {
				t.Fatalf("rubric hash = %s", entry.SHA256)
			}
		}
	}
	if count != 1 {
		t.Fatalf("rubric layers = %d", count)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "rubric.md"), []byte("changed rubric\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, _ := build()
	if first.Digest == second.Digest || first.PromptTemplateSHA256 == second.PromptTemplateSHA256 {
		t.Fatalf("local rubric change retained digest %s/template %s", first.Digest, first.PromptTemplateSHA256)
	}
}
