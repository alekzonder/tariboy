package stores

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillHashMatchesTboxFixture pins skills-lock.json hash parity from the
// tariboy side: tbox restores skills without Node and compares their content
// with computedHash, so both must hash the shared fixture to the committed
// value.
func TestSkillHashMatchesTboxFixture(t *testing.T) {
	t.Parallel()
	fixture := filepath.Join("..", "..", "tbox", "testdata", "compat", "skills-lock")
	want, err := os.ReadFile(filepath.Join(fixture, "computedHash"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := SkillHash(filepath.Join(fixture, "skill"))
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSpace(string(want)) {
		t.Fatalf("SkillHash = %s, committed tbox fixture %s", got, strings.TrimSpace(string(want)))
	}
}
