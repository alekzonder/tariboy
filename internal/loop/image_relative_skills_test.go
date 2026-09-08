package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
)

func TestAgentImageV2ConfinesSkillSources(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "work")
	source := filepath.Join(work, "images", "reviewer")
	inside := filepath.Join(work, "skills", "review")
	outside := filepath.Join(base, "outside", "review")
	for _, dir := range []string{source, inside, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{inside, outside} {
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: Review changes.\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, path string
		allowed    bool
	}{
		{"absolute-outside", outside, false},
		{"relative-outside", "../../../outside/review", false},
		{"relative-inside", "../../skills/review", true},
		{"absolute-inside", inside, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := fmt.Sprintf("schema_version: 2\nplugins: []\nskills:\n  - dir: %q\nprompts: []\n", tc.path)
			if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			store := &image.Store{Dir: filepath.Join(t.TempDir(), "images")}
			_, err := buildImageForAgent(store, work, "reviewer", "v1", source)
			if tc.allowed && err != nil {
				t.Fatal(err)
			}
			if !tc.allowed && (err == nil || !strings.Contains(err.Error(), "workdir")) {
				t.Fatalf("expected workdir rejection, got %v", err)
			}
			if !tc.allowed && store.Exists(image.Ref{Name: "reviewer", Tag: "v1"}) {
				t.Fatal("rejected skill published an image")
			}
		})
	}
}
