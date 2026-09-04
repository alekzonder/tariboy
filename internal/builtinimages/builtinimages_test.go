package builtinimages

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestShippedImagesOrderActionableRuntimes(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for path, want := range map[string][]imagefile.PromptEntry{
		filepath.Join(root, "internal", "builtinimages", "source"): {
			{Runtime: "one-shot"}, {Runtime: "messages"}, {Runtime: "goal"},
		},
		filepath.Join(root, "store", "images", "tariboy-developer"): {
			{Runtime: "one-shot"}, {Runtime: "messages"}, {Runtime: "goal"},
		},
		filepath.Join(root, "store", "images", "llm-as-judge"): {
			{Runtime: "one-shot"}, {Runtime: "messages"},
		},
	} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parsed, err := imagefile.ParseV2(path)
			if err != nil {
				t.Fatal(err)
			}
			var got []imagefile.PromptEntry
			for _, prompt := range parsed.Prompts {
				if prompt.Runtime == "one-shot" || prompt.Runtime == "messages" || prompt.Runtime == "goal" {
					got = append(got, prompt)
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("actionable runtime prompts = %#v, want %#v", got, want)
			}
		})
	}
}
