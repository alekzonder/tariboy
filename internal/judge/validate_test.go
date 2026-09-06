package judge

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func citedStrength() Strength {
	return Strength{Description: "followed the request", Citations: []Citation{{BundleHash: "bundle", Artifact: "audit", Locator: "line:1"}}}
}

func validReview() AnalysisResult {
	return AnalysisResult{SchemaVersion: 1, Verdict: "pass", Score: .5, Confidence: .5, Summary: "summary", Strengths: []Strength{citedStrength()}}
}

func TestValidateAnalysisEnforcesVerdictEvidenceContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*AnalysisResult)
		want string
	}{
		{"missing strength citation", func(a *AnalysisResult) { a.Strengths[0].Citations = nil }, "strengths[0].citations must not be empty"},
		{"fail without violation", func(a *AnalysisResult) { a.Verdict = "fail"; a.Strengths = nil }, "fail requires a violation"},
		{"uncertain without gap", func(a *AnalysisResult) { a.Verdict = "uncertain"; a.Strengths = nil }, "uncertain requires an evidence gap"},
		{"nonfinite score", func(a *AnalysisResult) { a.Score = math.NaN() }, "score must be finite"},
		{"nonfinite confidence", func(a *AnalysisResult) { a.Confidence = math.Inf(1) }, "confidence must be finite"},
		{"pass with violation", func(a *AnalysisResult) {
			a.Violations = []Violation{{Criterion: "requirement", Description: "action", Citations: []Citation{{BundleHash: "bundle", Artifact: "audit", Locator: "line:2"}}}}
		}, "pass must not include violations"},
		{"blank gap", func(a *AnalysisResult) { a.Verdict = "uncertain"; a.Strengths = nil; a.EvidenceGaps = []string{" "} }, "evidence_gaps[0] must not be empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := validReview()
			tc.edit(&a)
			err := ValidateAnalysis(a, CitationResolverFunc(func(Citation) error { return nil }))
			if !errors.Is(err, ErrInvalidAnalysis) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

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
