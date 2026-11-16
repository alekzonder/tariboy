package harness

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func claudeHost(version string) *fakeHost {
	return &fakeHost{
		installed: map[string]bool{"claude": true},
		output: func(_ []string, name string, args []string) ([]byte, error) {
			if name == "/fake/bin/claude" && slices.Equal(args, []string{"--version"}) {
				return []byte(version + " (Claude Code)\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}
}

func TestClaudePrepareTwoSkills(t *testing.T) {
	run := twoSkillRun(t, "be helpful\n")
	f := claudeHost("2.1.325")
	launch, err := claude{}.Prepare(context.Background(), f.host(), run, []string{"--model", "opus"})
	if err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(run.Dir, "plugin")
	wantArgv := []string{"claude", "--plugin-dir", plugin, "--append-system-prompt-file", run.PromptPath, "--model", "opus"}
	if launch.Path != "/fake/bin/claude" || !slices.Equal(launch.Argv, wantArgv) {
		t.Fatalf("launch %+v", launch)
	}
	wantEnv := []string{"TBOX_IMAGE_DIR=" + run.ImageDir, "TBOX_IMAGE=demo:latest"}
	if !slices.Equal(launch.Env, wantEnv) {
		t.Fatalf("env %v", launch.Env)
	}
	manifest, err := os.ReadFile(filepath.Join(plugin, ".claude-plugin", "plugin.json"))
	if err != nil || string(manifest) != "{\n  \"name\": \"demo\",\n  \"version\": \"1.2.0\"\n}\n" {
		t.Fatalf("plugin.json %q %v", manifest, err)
	}
	body, err := os.ReadFile(filepath.Join(plugin, "skills", "beta", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "beta body") {
		t.Fatalf("beta skill %q %v", body, err)
	}
	info, err := os.Stat(filepath.Join(plugin, "skills", "alpha", "scripts", "run.sh"))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("run.sh %v %v", info, err)
	}
	err = filepath.WalkDir(plugin, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type()&fs.ModeSymlink != 0 {
			t.Errorf("symlink in bridge: %s", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClaudePrepareIsIdempotent(t *testing.T) {
	run := twoSkillRun(t, "p\n")
	f := claudeHost("2.1.325")
	if _, err := (claude{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(run.Dir, "plugin", ".claude-plugin", "plugin.json")
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(manifest, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := (claude{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(manifest)
	if err != nil || !info.ModTime().Equal(old) {
		t.Fatalf("plugin rewritten on second run: %v %v", info, err)
	}
}

func TestClaudeEmptyPromptAndNoVersion(t *testing.T) {
	run := twoSkillRun(t, "")
	run.ImageVersion = ""
	launch, err := claude{}.Prepare(context.Background(), claudeHost("2.1.227").host(), run, nil)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(launch.Argv, "--append-system-prompt-file") {
		t.Fatalf("empty prompt passed: %v", launch.Argv)
	}
	manifest, _ := os.ReadFile(filepath.Join(run.Dir, "plugin", ".claude-plugin", "plugin.json"))
	if !strings.Contains(string(manifest), `"version": "0.0.0"`) {
		t.Fatalf("plugin.json %s", manifest)
	}
}

func TestClaudeOldVersion(t *testing.T) {
	run := twoSkillRun(t, "p\n")
	_, err := claude{}.Prepare(context.Background(), claudeHost("2.1.100").host(), run, nil)
	var unsupported ErrUnsupportedVersion
	if !errors.As(err, &unsupported) || unsupported.Harness != "claude" || unsupported.Found != "2.1.100" || unsupported.Minimum != "2.1.227" {
		t.Fatalf("error %#v", err)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "plugin")); !os.IsNotExist(err) {
		t.Fatalf("plugin written for unsupported version: %v", err)
	}
}

func TestClaudeMissingBinary(t *testing.T) {
	f := &fakeHost{}
	_, err := claude{}.Prepare(context.Background(), f.host(), twoSkillRun(t, ""), nil)
	if err == nil || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("error %v", err)
	}
}

func TestClaudeSimultaneousFirstLaunches(t *testing.T) {
	run := twoSkillRun(t, "p\n")
	f := claudeHost("2.1.325")
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = claude{}.Prepare(context.Background(), f.host(), run, nil)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "plugin", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeSweepsStaleStaging(t *testing.T) {
	run := twoSkillRun(t, "p\n")
	stale := filepath.Join(run.Dir, ".plugin-123")
	writeFile(t, filepath.Join(stale, "x"), "x", 0o600)
	if _, err := (claude{}).Prepare(context.Background(), claudeHost("2.1.325").host(), run, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale staging kept: %v", err)
	}
}

func TestClaudePluginNameIsKebab(t *testing.T) {
	run := twoSkillRun(t, "")
	run.Name = "my.img_x"
	if _, err := (claude{}).Prepare(context.Background(), claudeHost("2.1.325").host(), run, nil); err != nil {
		t.Fatal(err)
	}
	manifest, _ := os.ReadFile(filepath.Join(run.Dir, "plugin", ".claude-plugin", "plugin.json"))
	if !strings.Contains(string(manifest), `"name": "my-img-x"`) {
		t.Fatalf("plugin.json %s", manifest)
	}
}
