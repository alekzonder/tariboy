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

func TestDesktopMacPackagerRequiresSigningSecrets(t *testing.T) {
	root := repositoryRoot(t)
	baseEnv := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "TAURI_SIGNING_PRIVATE_KEY=") &&
			!strings.HasPrefix(value, "TAURI_SIGNING_PRIVATE_KEY_PASSWORD=") {
			baseEnv = append(baseEnv, value)
		}
	}
	for _, testCase := range []struct {
		name     string
		key      string
		password string
		want     string
	}{
		{name: "private key", password: "fixture", want: "TAURI_SIGNING_PRIVATE_KEY is required"},
		{name: "password", key: "fixture", want: "TAURI_SIGNING_PRIVATE_KEY_PASSWORD is required"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cmd := exec.Command("bash", filepath.Join(root, "scripts", "package-alpha.sh"))
			cmd.Dir = root
			cmd.Env = append(baseEnv,
				"TAURI_SIGNING_PRIVATE_KEY="+testCase.key,
				"TAURI_SIGNING_PRIVATE_KEY_PASSWORD="+testCase.password,
			)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), testCase.want) {
				t.Fatalf("packager did not reject missing %s (err=%v):\n%s", testCase.name, err, output)
			}
		})
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

	type workflowJob = struct {
		RunsOn      string            `yaml:"runs-on"`
		Needs       []string          `yaml:"needs"`
		Permissions map[string]string `yaml:"permissions"`
		Steps       []struct {
			Uses string            `yaml:"uses"`
			With map[string]any    `yaml:"with"`
			Run  string            `yaml:"run"`
			Env  map[string]string `yaml:"env"`
		} `yaml:"steps"`
	}
	var jobs struct {
		Jobs map[string]workflowJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(contents, &jobs); err != nil {
		t.Fatalf("parse Desktop release workflow jobs: %v", err)
	}
	// Every Secret stays on the one step that needs it; collect each job's
	// commands and the env of the steps matching a marker.
	inspect := func(name string, markers ...string) (string, map[string]map[string]string, workflowJob) {
		job, ok := jobs.Jobs[name]
		if !ok {
			t.Fatalf("release workflow has no %s job", name)
		}
		var commands []string
		envs := map[string]map[string]string{}
		for _, step := range job.Steps {
			commands = append(commands, step.Run)
			if strings.HasPrefix(step.Uses, "actions/checkout@") && step.With["persist-credentials"] != false {
				t.Fatalf("%s checkout persist-credentials = %v, want false", name, step.With["persist-credentials"])
			}
			for _, marker := range markers {
				if strings.Contains(step.Run, marker) {
					envs[marker] = step.Env
				}
			}
			for key, value := range step.Env {
				if strings.Contains(value, "secrets.") && !containsAny(step.Run, markers) {
					t.Fatalf("%s step exposes %s outside its build or signing step", name, key)
				}
			}
		}
		return strings.Join(commands, "\n"), envs, job
	}
	requireAll := func(name, commands string, required ...string) {
		for _, want := range required {
			if !strings.Contains(commands, want) {
				t.Errorf("%s job is missing %q", name, want)
			}
		}
	}

	// macOS builds and signs, then only hands the files over: it cannot write.
	macCommands, macEnv, mac := inspect("release", "make desktop-mac")
	if mac.RunsOn != "macos-15" {
		t.Fatalf("release runner = %q, want macos-15", mac.RunsOn)
	}
	if len(mac.Permissions) != 1 || mac.Permissions["contents"] != "read" {
		t.Fatalf("release job permissions = %v, want only contents: read", mac.Permissions)
	}
	requireAll("release", macCommands,
		`GITHUB_REF_NAME#v`,
		`internal/version/version.go`,
		`scripts/release-version.txt`,
		`brew install tmux ripgrep`,
		`make desktop-mac`,
		`scripts/desktop-updater-manifest.py`,
		`desktop/src-tauri/tauri.conf.json`,
		`*.app.tar.gz`,
		`*.app.tar.gz.sig`,
	)
	if strings.Contains(macCommands, "gh release") {
		t.Fatal("macOS build job must not publish")
	}
	buildEnv := macEnv["make desktop-mac"]
	if buildEnv["TAURI_SIGNING_PRIVATE_KEY"] != "${{ secrets.TAURI_SIGNING_PRIVATE_KEY }}" ||
		buildEnv["TAURI_SIGNING_PRIVATE_KEY_PASSWORD"] != "${{ secrets.TAURI_SIGNING_PRIVATE_KEY_PASSWORD }}" {
		t.Fatalf("release signing environment = %v, want signing Secrets on build step", buildEnv)
	}
	cargoCache := false
	for _, step := range mac.Steps {
		if step.Uses == "Swatinem/rust-cache@6323deb102c322ba6fcbdcafc7e3dddab59af2b6" && step.With["workspaces"] == "desktop/src-tauri -> target" {
			cargoCache = true
		}
	}
	if !cargoCache {
		t.Fatal("release workflow has no pinned Cargo cache for the desktop target")
	}

	// The Android job is paused until its signing Secrets are configured: no
	// release may wait on it or upload an APK.
	if _, ok := jobs.Jobs["android"]; ok {
		t.Fatal("release workflow has an android job, want it paused")
	}

	// One job publishes, only after the build, as a draft made public last.
	publishCommands, publishEnv, publish := inspect("publish", "gh release create")
	if len(publish.Needs) != 1 || publish.Needs[0] != "release" {
		t.Fatalf("publish needs = %v, want [release]", publish.Needs)
	}
	if len(publish.Permissions) != 1 || publish.Permissions["contents"] != "write" {
		t.Fatalf("publish job permissions = %v, want only contents: write", publish.Permissions)
	}
	requireAll("publish", publishCommands,
		`gh release create "$GITHUB_REF_NAME"`,
		`--draft`,
		`--generate-notes`,
		`This build is not notarized by Apple.`,
		`If macOS blocks the DMG: **System Settings → Privacy & Security → Open Anyway**`,
		`gh release upload "$GITHUB_REF_NAME" "$release_dir"/*`,
		`gh release edit "$GITHUB_REF_NAME" --draft=false`,
	)
	if strings.Contains(publishCommands, "dist/android") {
		t.Fatal("publish job uploads Android files while the android job is paused")
	}
	env := publishEnv["gh release create"]
	if env["GH_TOKEN"] != "${{ github.token }}" {
		t.Fatalf("release GH_TOKEN = %q, want github.token", env["GH_TOKEN"])
	}
	for key, value := range env {
		if strings.Contains(value, "secrets.") {
			t.Fatalf("publication step exposes %s", key)
		}
	}
}

func containsAny(s string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// A tag run restores only caches of its own tag or of main, so the release must
// restore the cache main warms and never save its own.
func TestDesktopReleaseRestoresTheCacheWarmedOnMain(t *testing.T) {
	root := repositoryRoot(t)
	type step struct {
		Uses string         `yaml:"uses"`
		With map[string]any `yaml:"with"`
		Run  string         `yaml:"run"`
	}
	var release, warm struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]struct {
			RunsOn string `yaml:"runs-on"`
			Steps  []step `yaml:"steps"`
		} `yaml:"jobs"`
	}
	for path, workflow := range map[string]any{"desktop-release.yml": &release, "desktop-cache.yml": &warm} {
		contents, err := os.ReadFile(filepath.Join(root, ".github", "workflows", path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := yaml.Unmarshal(contents, workflow); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
	}
	cache := func(steps []step) map[string]any {
		for _, s := range steps {
			if strings.HasPrefix(s.Uses, "Swatinem/rust-cache@") {
				return s.With
			}
		}
		t.Fatal("no Cargo cache step")
		return nil
	}

	releaseCache := cache(release.Jobs["release"].Steps)
	if releaseCache["shared-key"] != "desktop-release" || releaseCache["save-if"] != false {
		t.Fatalf("release Cargo cache = %v, want shared-key desktop-release and save-if false", releaseCache)
	}

	push, _ := warm.On["push"].(map[string]any)
	if branches, _ := push["branches"].([]any); len(branches) != 1 || branches[0] != "main" {
		t.Fatalf("cache warm push branches = %v, want [main]", push["branches"])
	}
	job, ok := warm.Jobs["warm"]
	if !ok || job.RunsOn != release.Jobs["release"].RunsOn {
		t.Fatalf("cache warm job must run on the release runner %q", release.Jobs["release"].RunsOn)
	}
	warmCache := cache(job.Steps)
	if warmCache["shared-key"] != "desktop-release" || warmCache["workspaces"] != releaseCache["workspaces"] {
		t.Fatalf("cache warm Cargo cache = %v, want the release key and workspace", warmCache)
	}
	if _, ok := warmCache["save-if"]; ok {
		t.Fatalf("cache warm Cargo cache must save, got save-if %v", warmCache["save-if"])
	}
	var commands []string
	for _, s := range job.Steps {
		commands = append(commands, s.Run)
	}
	// The cargo invocation `cargo tauri build` runs for a release.
	if all := strings.Join(commands, "\n"); !strings.Contains(all, "cargo build --bins --release --locked --features tauri/custom-protocol") {
		t.Fatalf("cache warm job does not build like cargo tauri build:\n%s", all)
	}
}
