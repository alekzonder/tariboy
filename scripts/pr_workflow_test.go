package scripts

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPullRequestsRunMakeCheckWithLockedDependencies(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	workflowPath := filepath.Join(filepath.Dir(packageDir), ".github", "workflows", "pull-request.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read pull request workflow: %v", err)
	}

	var workflow struct {
		On          map[string]yaml.Node `yaml:"on"`
		Permissions map[string]string    `yaml:"permissions"`
		Jobs        map[string]struct {
			RunsOn      string            `yaml:"runs-on"`
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Uses             string `yaml:"uses"`
				Run              string `yaml:"run"`
				WorkingDirectory string `yaml:"working-directory"`
				With             struct {
					GoVersionFile       string `yaml:"go-version-file"`
					NodeVersion         string `yaml:"node-version"`
					Cache               string `yaml:"cache"`
					CacheDependencyPath string `yaml:"cache-dependency-path"`
				} `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse pull request workflow: %v", err)
	}

	pullRequest, ok := workflow.On["pull_request"]
	if !ok {
		t.Fatal("workflow does not run for pull_request events")
	}
	if pullRequest.Kind != yaml.ScalarNode || pullRequest.Tag != "!!null" {
		t.Fatal("pull_request event must not have branch, path, or activity filters")
	}
	if len(workflow.Permissions) != 1 {
		t.Fatalf("workflow has %d top-level permissions, want exactly contents: read", len(workflow.Permissions))
	}
	if got := workflow.Permissions["contents"]; got != "read" {
		t.Fatalf("contents permission = %q, want read", got)
	}
	job, ok := workflow.Jobs["check"]
	if !ok {
		t.Fatal("workflow has no check job")
	}
	if job.RunsOn != "ubuntu-latest" {
		t.Fatalf("check job runs-on = %q, want ubuntu-latest", job.RunsOn)
	}
	if len(job.Permissions) != 0 {
		t.Fatalf("check job overrides permissions: %v", job.Permissions)
	}

	wantSteps := []struct {
		uses             string
		run              string
		workingDirectory string
	}{
		{uses: "actions/checkout@11d5960a326750d5838078e36cf38b85af677262"},
		{uses: "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"},
		{uses: "actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020"},
		{run: "npm ci", workingDirectory: "ui"},
		{run: "npm ci", workingDirectory: "docs"},
		{run: "mkdir -p \"$RUNNER_TEMP/bin\"\nprintf '#!/bin/sh\\nexit 0\\n' > \"$RUNNER_TEMP/bin/codex\"\nchmod +x \"$RUNNER_TEMP/bin/codex\"\necho \"$RUNNER_TEMP/bin\" >> \"$GITHUB_PATH\"\n"},
		{run: "make check"},
	}
	if len(job.Steps) != len(wantSteps) {
		t.Fatalf("check job has %d steps, want %d", len(job.Steps), len(wantSteps))
	}
	for i, want := range wantSteps {
		got := job.Steps[i]
		if got.Uses != want.uses || got.Run != want.run || got.WorkingDirectory != want.workingDirectory {
			t.Errorf("step %d = uses %q, run %q, working-directory %q; want uses %q, run %q, working-directory %q", i, got.Uses, got.Run, got.WorkingDirectory, want.uses, want.run, want.workingDirectory)
		}
	}

	if got := job.Steps[1].With.GoVersionFile; got != "go.mod" {
		t.Errorf("setup-go go-version-file = %q, want go.mod", got)
	}
	if got := job.Steps[2].With.NodeVersion; got != "22" {
		t.Errorf("setup-node node-version = %q, want 22", got)
	}
	if got := job.Steps[2].With.Cache; got != "npm" {
		t.Errorf("setup-node cache = %q, want npm", got)
	}
	if got, want := job.Steps[2].With.CacheDependencyPath, "ui/package-lock.json\ndocs/package-lock.json\n"; got != want {
		t.Errorf("setup-node cache-dependency-path = %q, want %q", got, want)
	}
}
