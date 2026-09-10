package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDesktopMacTargetRunsTheReleasePackager(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("make", "-n", "desktop-mac")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run desktop-mac: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "./scripts/package-alpha.sh") {
		t.Fatalf("desktop-mac does not run the existing release packager:\n%s", output)
	}
	legacy := exec.Command("make", "-n", "desktop-alpha")
	legacy.Dir = root
	if output, err := legacy.CombinedOutput(); err == nil {
		t.Fatalf("legacy desktop-alpha target still exists:\n%s", output)
	}
}

func TestDesktopReleaseWorkflowPublishesCheckedTagArtifacts(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, ".github", "workflows", "desktop-release.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Desktop release workflow: %v", err)
	}

	var workflow struct {
		On struct {
			Push struct {
				Tags []string `yaml:"tags"`
			} `yaml:"push"`
		} `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			RunsOn      string            `yaml:"runs-on"`
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Uses string            `yaml:"uses"`
				With map[string]any    `yaml:"with"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(contents, &workflow); err != nil {
		t.Fatalf("parse Desktop release workflow: %v", err)
	}
	if got, want := workflow.On.Push.Tags, []string{"v[0-9]+.[0-9]+.[0-9]+"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("release tags = %v, want %v", got, want)
	}
	if len(workflow.Permissions) != 0 {
		t.Fatalf("top-level release permissions = %v, want job-scoped permissions", workflow.Permissions)
	}

	job, ok := workflow.Jobs["release"]
	if !ok {
		t.Fatal("release workflow has no release job")
	}
	if job.RunsOn != "macos-15" {
		t.Fatalf("release runner = %q, want macos-15", job.RunsOn)
	}
	if len(job.Permissions) != 1 || job.Permissions["contents"] != "write" {
		t.Fatalf("release job permissions = %v, want only contents: write", job.Permissions)
	}
	commands := make([]string, 0, len(job.Steps))
	var publishEnv map[string]string
	for _, step := range job.Steps {
		commands = append(commands, step.Run)
		if strings.HasPrefix(step.Uses, "actions/checkout@") && step.With["persist-credentials"] != false {
			t.Fatalf("checkout persist-credentials = %v, want false", step.With["persist-credentials"])
		}
		if strings.Contains(step.Run, "gh release create") {
			publishEnv = step.Env
		}
	}
	allCommands := strings.Join(commands, "\n")
	for _, required := range []string{
		`GITHUB_REF_NAME#v`,
		`internal/version/version.go`,
		`scripts/release-version.txt`,
		`brew install tmux ripgrep qemu`,
		`make desktop-mac`,
		`gh release create "$GITHUB_REF_NAME"`,
		`Tariboy_${version}_aarch64.dmg`,
		`SHA256SUMS`,
		`release.json`,
	} {
		if !strings.Contains(allCommands, required) {
			t.Errorf("release workflow is missing %q", required)
		}
	}
	if got := strings.Count(allCommands, `"$release_dir/`); got != 3 {
		t.Fatalf("release publishes %d assets, want exactly 3", got)
	}
	if publishEnv["GH_TOKEN"] != "${{ github.token }}" {
		t.Fatalf("release GH_TOKEN = %q, want github.token", publishEnv["GH_TOKEN"])
	}
}
