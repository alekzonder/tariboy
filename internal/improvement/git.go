package improvement

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

var gitCommit = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

type Repository struct {
	ID            string `json:"id"`
	URL           string `json:"-"`
	DefaultBranch string `json:"default_branch"`
	CheckoutDir   string `json:"-"`
}

type RepositoryRegistry map[string]Repository

func (r RepositoryRegistry) Resolve(id string) (Repository, error) {
	repository, ok := r[id]
	if !ok || repository.ID != id {
		return Repository{}, fmt.Errorf("%w: repository %q", ErrNotFound, id)
	}
	return repository, nil
}

func safeRelativePath(value string) bool {
	clean := path.Clean(value)
	return value != "" && clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(value, "/") && !strings.Contains(value, `\`)
}

func ValidateChangedPaths(approved, changed []string) error {
	allowed := make(map[string]bool, len(approved))
	for _, file := range approved {
		if !safeRelativePath(file) || allowed[file] {
			return fmt.Errorf("invalid approved path %q", file)
		}
		allowed[file] = true
	}
	for _, file := range changed {
		if !safeRelativePath(file) || !allowed[file] {
			return fmt.Errorf("changed path %q is outside the approved scope", file)
		}
	}
	return nil
}

func ValidateBranchIdentity(image, proposalID, branch string) error {
	expected := "tariboy/improve/" + image + "/" + proposalID
	if !safeRelativePath(image) || strings.Contains(image, "/") || branch != expected {
		return fmt.Errorf("branch must be %q", expected)
	}
	return nil
}

func GitChangedPaths(ctx context.Context, repository Repository, base, head string) ([]string, error) {
	if repository.CheckoutDir == "" || !gitCommit.MatchString(base) || !gitCommit.MatchString(head) {
		return nil, fmt.Errorf("invalid repository checkout or commit")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repository.CheckoutDir, "diff", "--name-only", "-z", base+"..."+head, "--")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}
	parts := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	if len(parts) == 1 && len(parts[0]) == 0 {
		return nil, nil
	}
	paths := make([]string, len(parts))
	for i, part := range parts {
		paths[i] = string(part)
	}
	return paths, nil
}
