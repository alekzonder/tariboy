package judge

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAnalysisReportsInvalidField(t *testing.T) {
	a := AnalysisResult{SchemaVersion: 1, Verdict: "uncertain", Score: .5, Confidence: .5, Summary: "summary"}
	a.EvidenceGaps = []string{"gap"}
	a.Recommendations = []Recommendation{{Description: "next step"}}
	a.Strengths = []Strength{{Description: "strength", Citations: []Citation{{BundleHash: "bundle", Artifact: "audit"}}}}

	err := ValidateAnalysis(a, CitationResolverFunc(func(Citation) error { return nil }))
	if !errors.Is(err, ErrInvalidAnalysis) {
		t.Fatalf("error = %v, want ErrInvalidAnalysis", err)
	}
	if !strings.Contains(err.Error(), "strengths[0].citations[0].locator") {
		t.Fatalf("error = %q, want field path", err)
	}
}
