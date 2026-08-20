package judge

import (
	"fmt"
	"strings"
)

// CitationResolver deliberately has no access to mutable source iteration data.
// Implementations resolve locators from the assignment's immutable bundle.
type CitationResolver interface{ ResolveCitation(Citation) error }
type CitationResolverFunc func(Citation) error

func (f CitationResolverFunc) ResolveCitation(c Citation) error { return f(c) }

func ValidateAnalysis(a AnalysisResult, r CitationResolver) error {
	if a.SchemaVersion != 1 || (a.Verdict != "pass" && a.Verdict != "fail" && a.Verdict != "uncertain") || a.Score < 0 || a.Score > 1 || a.Confidence < 0 || a.Confidence > 1 || strings.TrimSpace(a.Summary) == "" {
		return ErrInvalidAnalysis
	}
	for _, v := range a.Violations {
		if strings.TrimSpace(v.Criterion) == "" || strings.TrimSpace(v.Description) == "" {
			return ErrInvalidAnalysis
		}
		if err := validateCitations(v.Citations, r); err != nil {
			return err
		}
	}
	for _, s := range a.Strengths {
		if strings.TrimSpace(s.Description) == "" {
			return ErrInvalidAnalysis
		}
		if err := validateCitations(s.Citations, r); err != nil {
			return err
		}
	}
	return nil
}
func validateCitations(cs []Citation, r CitationResolver) error {
	for _, c := range cs {
		if c.BundleHash == "" || c.Artifact == "" || c.Locator == "" || r == nil {
			return ErrInvalidAnalysis
		}
		if err := r.ResolveCitation(c); err != nil {
			return fmt.Errorf("%w: citation: %v", ErrInvalidAnalysis, err)
		}
	}
	return nil
}
func ValidateSummary(s SummaryResult) error {
	if s.SchemaVersion != 1 || strings.TrimSpace(s.ExecutiveConclusion) == "" || s.Coverage == nil {
		return ErrInvalidSummary
	}
	return nil
}
