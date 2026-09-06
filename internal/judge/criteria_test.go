package judge

import (
	"strings"
	"testing"
)

func TestReviewCriteriaReadsBundledRubricAndHashesIt(t *testing.T) {
	text, hash, err := ReviewCriteria()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Instruction-following review v1") || len(hash) != 64 {
		t.Fatalf("criteria text/hash = %q/%q", text, hash)
	}
}
