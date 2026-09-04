package builtinimages

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestCanonicalImagesDeclareGoalRuntime(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, path := range []string{
		filepath.Join(root, "internal", "builtinimages", "source"),
		filepath.Join(root, "store", "images", "tariboy-developer"),
	} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parsed, err := imagefile.ParseV2(path)
			if err != nil {
				t.Fatal(err)
			}
			var got []imagefile.PromptEntry
			for _, prompt := range parsed.Prompts {
				if prompt.Runtime == "goal" {
					got = append(got, prompt)
				}
			}
			if want := []imagefile.PromptEntry{{Runtime: "goal"}}; !reflect.DeepEqual(got, want) {
				t.Fatalf("goal prompts = %#v, want %#v", got, want)
			}
		})
	}
}
