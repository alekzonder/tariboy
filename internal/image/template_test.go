package image

import (
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestReadTemplatePinnedRetainsImageIdentityAfterMutableRebuild(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "judge", Tag: "latest"}
	source := &imagefile.V2{SchemaVersion: 2, Prompts: []imagefile.PromptEntry{{Runtime: "identity"}}}
	first, err := BuildV2Mutable(source, imagefile.ResolveRoots{}, ref, s, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	source.Prompts = []imagefile.PromptEntry{{Runtime: "messages"}}
	second, err := BuildV2Mutable(source, imagefile.ResolveRoots{}, ref, s, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range []Manifest{first, second} {
		template, err := s.ReadTemplatePinned(ref, manifest.Digest)
		if err != nil {
			t.Fatal(err)
		}
		if template.SHA256 != manifest.PromptTemplateSHA256 {
			t.Fatalf("template %s does not belong to digest %s", template.SHA256, manifest.Digest)
		}
	}
	if _, err := s.ReadTemplatePinned(ref, "unverifiable"); err == nil {
		t.Fatal("invalid digest accepted")
	}
}

func TestValidatePromptTemplateAcceptsGoalRuntime(t *testing.T) {
	template := PromptTemplate{SchemaVersion: 2, Entries: []TemplateEntry{{Kind: "runtime", Runtime: "goal"}}}
	template.SHA256, _ = PromptTemplateHash(template.Entries)
	if err := ValidatePromptTemplate(template); err != nil {
		t.Fatal(err)
	}
}
