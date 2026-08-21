package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func tariboyDeveloperRoot(t *testing.T) string {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve scripts package directory: %v", err)
	}
	return filepath.Dir(packageDir)
}

func TestTariboyDeveloperPackagesGitHubPRWorkflow(t *testing.T) {
	root := tariboyDeveloperRoot(t)
	imagePath := filepath.Join(root, "store", "images", "tariboy-developer", "Tariboyfile.yaml")
	image, err := imagefile.ParseV2(imagePath)
	if err != nil {
		t.Fatalf("parse tariboy-developer image: %v", err)
	}

	const workflowSkill = "./skills/github-pr-workflow"
	declared := false
	for _, skill := range image.Skills {
		if skill.Dir == workflowSkill {
			declared = true
			break
		}
	}
	if !declared {
		t.Fatalf("tariboy-developer image does not declare %q", workflowSkill)
	}

	resolved, err := imagefile.ResolveSkillDirectory(image.Dir, workflowSkill, imagefile.ResolveRoots{})
	if err != nil {
		t.Fatalf("resolve %s: %v", workflowSkill, err)
	}
	prepared, err := agentskills.Prepare(resolved)
	if err != nil {
		t.Fatalf("prepare %s: %v", workflowSkill, err)
	}
	if got, want := prepared.Metadata.Name, "github-pr-workflow"; got != want {
		t.Fatalf("prepared skill name = %q, want %q", got, want)
	}

	utility := filepath.Join(resolved.Path, "scripts", "github-pr.py")
	info, err := os.Stat(utility)
	if err != nil {
		t.Fatalf("stat github PR utility: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("github PR utility %s is not executable (mode %04o)", utility, info.Mode().Perm())
	}
}

func TestGitHubPRWorkflowUtility(t *testing.T) {
	root := tariboyDeveloperRoot(t)
	testPath := filepath.Join(root, "store", "images", "tariboy-developer", "skills", "github-pr-workflow", "tests", "test_github_pr.py")
	cmd := exec.Command("python3", testPath)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("github PR workflow utility contracts failed: %v\n%s", err, output)
	}
}
