package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowStep struct {
	Name string `yaml:"name"`
	ID   string `yaml:"id"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
	With struct {
		GoVersionFile       string `yaml:"go-version-file"`
		NodeVersion         string `yaml:"node-version"`
		Cache               string `yaml:"cache"`
		CacheDependencyPath string `yaml:"cache-dependency-path"`
		Packages            string `yaml:"packages"`
		Version             string `yaml:"version"`
		Path                string `yaml:"path"`
		Key                 string `yaml:"key"`
		RestoreKeys         string `yaml:"restore-keys"`
	} `yaml:"with"`
}

type workflowJob struct {
	Name        string            `yaml:"name"`
	RunsOn      string            `yaml:"runs-on"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
}

const (
	checkoutAction  = "actions/checkout@11d5960a326750d5838078e36cf38b85af677262"
	setupGoAction   = "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"
	setupNodeAction = "actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020"
	aptCacheAction  = "awalsh128/cache-apt-pkgs-action@553a35bb8ebd9fcabcb1c9451aa4c98e1b4ca8a9"
	cacheAction     = "actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830"
	codexHarnessRun = "mkdir -p \"$RUNNER_TEMP/bin\"\nprintf '#!/bin/sh\\nexit 0\\n' > \"$RUNNER_TEMP/bin/codex\"\nchmod +x \"$RUNNER_TEMP/bin/codex\"\necho \"$RUNNER_TEMP/bin\" >> \"$GITHUB_PATH\"\n"
	rustVersionRun  = "echo \"version=$(rustc --version | cut -d' ' -f2)\" >> \"$GITHUB_OUTPUT\"\n"
)

func TestPullRequestsRunCheckGroupsAsParallelCachedJobs(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "pull-request.yml"))
	if err != nil {
		t.Fatalf("read pull request workflow: %v", err)
	}
	var workflow struct {
		On          map[string]yaml.Node   `yaml:"on"`
		Permissions map[string]string      `yaml:"permissions"`
		Jobs        map[string]workflowJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse pull request workflow: %v", err)
	}

	checkEvents(t, workflow.On)
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatalf("top-level permissions = %v, want exactly contents: read", workflow.Permissions)
	}
	jobNames := make([]string, 0, len(workflow.Jobs))
	for name, job := range workflow.Jobs {
		jobNames = append(jobNames, name)
		if job.Name != name {
			t.Errorf("job %s has name %q, want %q", name, job.Name, name)
		}
		if job.RunsOn != "ubuntu-latest" {
			t.Errorf("job %s runs-on = %q, want ubuntu-latest", name, job.RunsOn)
		}
		if len(job.Permissions) != 0 {
			t.Errorf("job %s overrides permissions: %v", name, job.Permissions)
		}
		for i, step := range job.Steps {
			if step.Uses != "" && !regexp.MustCompile(`@[0-9a-f]{40}$`).MatchString(step.Uses) {
				t.Errorf("job %s step %d uses %q, want a full commit SHA pin", name, i, step.Uses)
			}
			if strings.Contains(step.Run, "apt-get") {
				t.Errorf("job %s step %d runs apt-get directly; use the apt package cache", name, i)
			}
		}
	}
	slices.Sort(jobNames)
	// Each job runs one make target; together they must equal the groups of `make check`.
	if groups := makeCheckGroups(t, root); !slices.Equal(jobNames, groups) {
		t.Fatalf("jobs = %v, want the make check groups %v", jobNames, groups)
	}

	checkBackendJob(t, root, workflow.Jobs["backend-check"])
	checkFrontendJob(t, workflow.Jobs["frontend-check"])
}

func checkEvents(t *testing.T, on map[string]yaml.Node) {
	t.Helper()
	pullRequest, ok := on["pull_request"]
	if !ok {
		t.Fatal("workflow does not run for pull_request events")
	}
	if pullRequest.Kind != yaml.ScalarNode || pullRequest.Tag != "!!null" {
		t.Fatal("pull_request event must not have branch, path, or activity filters")
	}
	// A push to main saves caches that every pull request can restore.
	push, ok := on["push"]
	if !ok {
		t.Fatal("workflow does not run for push events on main")
	}
	var pushFilter struct {
		Branches []string `yaml:"branches"`
	}
	if err := push.Decode(&pushFilter); err != nil {
		t.Fatalf("decode push event: %v", err)
	}
	if !slices.Equal(pushFilter.Branches, []string{"main"}) {
		t.Fatalf("push branches = %v, want [main]", pushFilter.Branches)
	}
	if len(on) != 2 {
		t.Fatalf("workflow has %d events, want pull_request and push", len(on))
	}
}

func checkSteps(t *testing.T, job string, got []workflowStep, want []workflowStep) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("job %s has %d steps, want %d", job, len(got), len(want))
	}
	for i := range want {
		if got[i].Uses != want[i].Uses || got[i].Run != want[i].Run {
			t.Errorf("job %s step %d = uses %q, run %q; want uses %q, run %q", job, i, got[i].Uses, got[i].Run, want[i].Uses, want[i].Run)
		}
	}
}

func checkBackendJob(t *testing.T, root string, job workflowJob) {
	t.Helper()
	checkSteps(t, "backend-check", job.Steps, []workflowStep{
		{Uses: checkoutAction},
		{Uses: setupGoAction},
		{Run: codexHarnessRun},
		{Uses: aptCacheAction},
		{Run: rustVersionRun},
		{Uses: cacheAction},
		{Run: "make backend-check"},
	})
	if got := job.Steps[1].With.GoVersionFile; got != "go.mod" {
		t.Errorf("setup-go go-version-file = %q, want go.mod", got)
	}
	apt := job.Steps[3].With
	if got, want := strings.Fields(apt.Packages), []string{"ripgrep", "libwebkit2gtk-4.1-dev", "libxdo-dev", "libssl-dev", "libayatana-appindicator3-dev", "librsvg2-dev"}; !slices.Equal(got, want) {
		t.Errorf("apt cache packages = %v, want %v", got, want)
	}
	if apt.Version == "" {
		t.Error("apt cache has no version")
	}
	if job.Steps[4].ID != "rust" {
		t.Errorf("rust version step id = %q, want rust", job.Steps[4].ID)
	}

	cargo := job.Steps[5].With
	paths := strings.Fields(cargo.Path)
	if len(paths) == 0 || paths[0] != cargoCheckDir(t, root) {
		t.Errorf("cargo cache paths = %v, want the desktop-rust-check target %q first", paths, cargoCheckDir(t, root))
	}
	for _, part := range []string{"${{ steps.rust.outputs.version }}", "${{ hashFiles('desktop/src-tauri/Cargo.lock') }}"} {
		if !strings.Contains(cargo.Key, part) {
			t.Errorf("cargo cache key %q does not contain %s", cargo.Key, part)
		}
	}
	if !strings.Contains(cargo.RestoreKeys, "${{ steps.rust.outputs.version }}") {
		t.Errorf("cargo cache restore-keys %q must stay within one Rust version", cargo.RestoreKeys)
	}
}

func checkFrontendJob(t *testing.T, job workflowJob) {
	t.Helper()
	checkSteps(t, "frontend-check", job.Steps, []workflowStep{
		{Uses: checkoutAction},
		{Uses: setupNodeAction},
		{Run: "make frontend-check"},
	})
	node := job.Steps[1].With
	if node.NodeVersion != "22" {
		t.Errorf("setup-node node-version = %q, want 22", node.NodeVersion)
	}
	if node.Cache != "npm" {
		t.Errorf("setup-node cache = %q, want npm", node.Cache)
	}
	if want := "ui/package-lock.json\ndocs/package-lock.json\n"; node.CacheDependencyPath != want {
		t.Errorf("setup-node cache-dependency-path = %q, want %q", node.CacheDependencyPath, want)
	}
}

// makeCheckGroups returns the sorted run_group names of the Makefile check recipe.
func makeCheckGroups(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	recipe := regexp.MustCompile(`(?m)^check:\n((?:\t.*\n)+)`).FindSubmatch(data)
	if recipe == nil {
		t.Fatal("Makefile has no check recipe")
	}
	var groups []string
	for _, m := range regexp.MustCompile(`run_group "([^"]+)"`).FindAllSubmatch(recipe[1], -1) {
		groups = append(groups, string(m[1]))
	}
	slices.Sort(groups)
	return groups
}

// cargoCheckDir returns the default desktop-rust-check target directory with $HOME as ~.
func cargoCheckDir(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	m := regexp.MustCompile(`TARIBOY_CARGO_CHECK_DIR:-\$\$HOME(/[^}]+)}`).FindSubmatch(data)
	if m == nil {
		t.Fatal("Makefile has no TARIBOY_CARGO_CHECK_DIR default")
	}
	return "~" + string(m[1])
}
