package tfile

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRuntimeNamesMatchCompatList keeps runtimeNames equal to
// testdata/compat/runtimes.txt, which tariboy's TestTboxRuntimeList
// (internal/imagecontract) checks against its runtime capabilities.
func TestRuntimeNamesMatchCompatList(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "compat", "runtimes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Sorted(slices.Values(runtimeNames))
	if want := strings.Fields(string(data)); !slices.Equal(got, want) {
		t.Fatalf("runtimeNames = %v, runtimes.txt = %v", got, want)
	}
}
