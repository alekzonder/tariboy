package imagecontract

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestTboxRuntimeList keeps tbox/testdata/compat/runtimes.txt, the runtime
// placeholders tbox accepts, equal to the runtime capabilities here. When
// this fails, update the file and tbox's runtimeNames together.
func TestTboxRuntimeList(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tbox", "testdata", "compat", "runtimes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(string(data))
	got := make([]string, 0, len(runtimes))
	for name := range runtimes {
		got = append(got, name)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("runtimes = %v, runtimes.txt = %v", got, want)
	}
}
