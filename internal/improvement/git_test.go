package improvement

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateChangedPaths(t *testing.T) {
	if err := ValidateChangedPaths([]string{"skills/review/SKILL.md", "role.md"}, []string{"role.md"}); err != nil {
		t.Fatal(err)
	}
	for _, changed := range [][]string{{"README.md"}, {"../role.md"}, {"/etc/passwd"}} {
		if err := ValidateChangedPaths([]string{"role.md"}, changed); err == nil {
			t.Fatalf("accepted changed paths %v", changed)
		}
	}
}

func TestRepositoryRegistryAndBranchIdentity(t *testing.T) {
	registry := RepositoryRegistry{"images": {ID: "images", URL: "git@example.invalid:images.git", DefaultBranch: "main"}}
	repository, err := registry.Resolve("images")
	if err != nil || repository.ID != "images" {
		t.Fatalf("Resolve() = %+v, %v", repository, err)
	}
	if _, err := registry.Resolve("missing"); err == nil {
		t.Fatal("unknown repository resolved")
	}
	if err := ValidateBranchIdentity("reviewer", "proposal-1", "tariboy/improve/reviewer/proposal-1"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBranchIdentity("reviewer", "proposal-1", "main"); err == nil {
		t.Fatal("wrong branch accepted")
	}
}

func TestGitChangedPaths(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "role.md"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "role.md")
	run("commit", "-qm", "base")
	base := run("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "role.md"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("commit", "-qam", "change")
	head := run("rev-parse", "HEAD")
	paths, err := GitChangedPaths(context.Background(), Repository{ID: "images", CheckoutDir: dir}, base, head)
	if err != nil || len(paths) != 1 || paths[0] != "role.md" {
		t.Fatalf("GitChangedPaths() = %v, %v", paths, err)
	}
}
