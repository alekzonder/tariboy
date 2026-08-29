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
	if a.SchemaVersion != 1 {
		return invalidAnalysis("schema_version must be 1")
	}
	if a.Verdict != "pass" && a.Verdict != "fail" && a.Verdict != "uncertain" {
		return invalidAnalysis("verdict must be pass, fail, or uncertain")
	}
	if a.Score < 0 || a.Score > 1 {
		return invalidAnalysis("score must be between 0 and 1")
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		return invalidAnalysis("confidence must be between 0 and 1")
	}
	if strings.TrimSpace(a.Summary) == "" {
		return invalidAnalysis("summary must not be empty")
	}
	for i, v := range a.Violations {
		if strings.TrimSpace(v.Criterion) == "" || strings.TrimSpace(v.Description) == "" {
			return invalidAnalysis(fmt.Sprintf("violations[%d] requires criterion and description", i))
		}
		if err := validateCitations(v.Citations, r, fmt.Sprintf("violations[%d].citations", i)); err != nil {
			return err
		}
	}
	for i, s := range a.Strengths {
		if strings.TrimSpace(s.Description) == "" {
			return invalidAnalysis(fmt.Sprintf("strengths[%d].description must not be empty", i))
		}
		if err := validateCitations(s.Citations, r, fmt.Sprintf("strengths[%d].citations", i)); err != nil {
			return err
		}
	}
	return nil
}
func invalidAnalysis(detail string) error { return fmt.Errorf("%w: %s", ErrInvalidAnalysis, detail) }

func validateCitations(cs []Citation, r CitationResolver, path string) error {
	for i, c := range cs {
		prefix := fmt.Sprintf("%s[%d]", path, i)
		if c.BundleHash == "" {
			return invalidAnalysis(prefix + ".bundle_hash must not be empty")
		}
		if c.Artifact == "" {
			return invalidAnalysis(prefix + ".artifact must not be empty")
		}
		if c.Locator == "" {
			return invalidAnalysis(prefix + ".locator must not be empty")
		}
		if r == nil {
			return invalidAnalysis(prefix + " cannot be resolved")
		}
		if err := r.ResolveCitation(c); err != nil {
			return invalidAnalysis(fmt.Sprintf("%s: %v", prefix, err))
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
