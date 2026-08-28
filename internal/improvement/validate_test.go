package improvement

import (
	"testing"
)

func validDraft() ProposalDraft {
	return ProposalDraft{
		SubjectIDs: []string{"subject-1"},
		Target:     Target{Repository: "production-agent-images", BaseCommit: "91ab820", Image: "reviewer", ImageDigest: "sha256:image"},
		Findings: []Finding{{
			Severity: "important", Criterion: "review-completeness", Observation: "CI was not checked",
			Evidence: []Citation{{BundleHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Artifact: "transcript", Locator: "req-17"}},
		}},
		Changes:       []Change{{File: "skills/code-review/SKILL.md", Intent: "Require current-head CI verification"}},
		Acceptance:    []string{"Reviewer records the current CI state"},
		Risk:          "medium",
		RollbackImage: "reviewer:v7",
	}
}

func TestCanonicalHashIgnoresMapOrderAndBindsAcceptanceCriteria(t *testing.T) {
	first, err := CanonicalHash(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalHash(map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canonical hashes differ: %q and %q", first, second)
	}
	draft := validDraft()
	before, err := CanonicalHash(draft)
	if err != nil {
		t.Fatal(err)
	}
	draft.Acceptance[0] = "Reviewer records both CI state and commit SHA"
	after, err := CanonicalHash(draft)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("acceptance change did not change canonical hash")
	}
}

func TestValidateDraftRejectsUnscopedOrUncitedChanges(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProposalDraft)
	}{
		{name: "repository", mutate: func(d *ProposalDraft) { d.Target.Repository = "" }},
		{name: "base commit", mutate: func(d *ProposalDraft) { d.Target.BaseCommit = "" }},
		{name: "evidence", mutate: func(d *ProposalDraft) { d.Findings[0].Evidence = nil }},
		{name: "absolute path", mutate: func(d *ProposalDraft) { d.Changes[0].File = "/tmp/role.md" }},
		{name: "escaping path", mutate: func(d *ProposalDraft) { d.Changes[0].File = "../role.md" }},
		{name: "acceptance", mutate: func(d *ProposalDraft) { d.Acceptance = nil }},
		{name: "mutable rollback", mutate: func(d *ProposalDraft) { d.RollbackImage = "reviewer:latest" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			draft := validDraft()
			tc.mutate(&draft)
			if err := ValidateDraft(draft); err == nil {
				t.Fatal("invalid draft accepted")
			}
		})
	}
}
