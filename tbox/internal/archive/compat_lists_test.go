package archive

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func compatList(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(compatDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

// TestFormatListsMatchCompatFiles keeps the lists copied from tariboy equal
// to the files tariboy's own tests (TestTboxSkillCategories,
// TestTboxPluginNamePattern in internal/image) check against its code.
func TestFormatListsMatchCompatFiles(t *testing.T) {
	got := slices.Sorted(slices.Values(skillCategories))
	if want := compatList(t, "skill-categories.txt"); !slices.Equal(got, want) {
		t.Fatalf("skillCategories = %v, skill-categories.txt = %v", got, want)
	}
	if want := compatList(t, "plugin-name.txt"); len(want) != 1 || pluginNamePattern.String() != want[0] {
		t.Fatalf("pluginNamePattern = %s, plugin-name.txt = %v", pluginNamePattern, want)
	}
}
