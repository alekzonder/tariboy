package improvement

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
)

func CanonicalHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func ValidateDraft(draft ProposalDraft) error {
	if draft.Target.Repository == "" || draft.Target.BaseCommit == "" || draft.Target.Image == "" || draft.Target.ImageDigest == "" {
		return fmt.Errorf("%w: target repository, commit, image, and digest are required", ErrInvalidProposal)
	}
	if len(draft.SubjectIDs) == 0 || len(draft.Findings) == 0 || len(draft.Changes) == 0 || len(draft.Acceptance) == 0 {
		return fmt.Errorf("%w: subjects, findings, changes, and acceptance criteria are required", ErrInvalidProposal)
	}
	for _, finding := range draft.Findings {
		if finding.Severity == "" || finding.Criterion == "" || finding.Observation == "" || len(finding.Evidence) == 0 {
			return fmt.Errorf("%w: every finding requires severity, criterion, observation, and evidence", ErrInvalidProposal)
		}
		for _, citation := range finding.Evidence {
			if len(citation.BundleHash) != 64 || citation.Artifact == "" || citation.Locator == "" || strings.ContainsAny(citation.Locator, `/\\`) {
				return fmt.Errorf("%w: invalid evidence citation", ErrInvalidProposal)
			}
		}
	}
	seen := map[string]bool{}
	for _, change := range draft.Changes {
		clean := path.Clean(change.File)
		if change.File == "" || change.Intent == "" || strings.HasPrefix(change.File, "/") || clean != change.File || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(change.File, `\`) || seen[change.File] {
			return fmt.Errorf("%w: invalid or duplicate change path %q", ErrInvalidProposal, change.File)
		}
		seen[change.File] = true
	}
	for _, criterion := range draft.Acceptance {
		if strings.TrimSpace(criterion) == "" {
			return fmt.Errorf("%w: empty acceptance criterion", ErrInvalidProposal)
		}
	}
	if draft.Risk != "low" && draft.Risk != "medium" && draft.Risk != "high" {
		return fmt.Errorf("%w: risk must be low, medium, or high", ErrInvalidProposal)
	}
	rollback, err := image.ParseRef(draft.RollbackImage)
	if err != nil || rollback.Tag == "latest" {
		return fmt.Errorf("%w: rollback image must be an immutable ref", ErrInvalidProposal)
	}
	return nil
}
