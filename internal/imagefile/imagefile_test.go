package imagefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write lays down a Tariboyfile plus its referenced files in a temp dir.
func write(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Tariboyfile.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ctx.md"), []byte("override"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", "deploy"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

const fullYAML = `
schema_version: 1
from: basic:latest
plugins:
  - { name: schedule }
  - { name: status, version: ">=0.3" }
requires_secrets: [JIRA_TOKEN]
env: { APP_ENV: prod }
policy:
  tools_deny: [scripts.*]
prompts:
  - task.md
  - { name: "system:context", filepath: ctx.md }
skills:
  - ./skills/deploy
`

func TestParseFull(t *testing.T) {
	dir := write(t, fullYAML)
	im, err := Parse(dir) // directory form
	if err != nil {
		t.Fatal(err)
	}
	if im.SchemaVersion != 1 || im.From != "basic:latest" {
		t.Fatalf("header: %+v", im)
	}
	if len(im.Plugins) != 2 || im.Plugins[1].Name != "status" || im.Plugins[1].Version != ">=0.3" {
		t.Fatalf("plugins: %+v", im.Plugins)
	}
	if im.Env["APP_ENV"] != "prod" || len(im.Policy.ToolsDeny) != 1 {
		t.Fatalf("env/policy: %+v %+v", im.Env, im.Policy)
	}
	if len(im.Prompts) != 2 {
		t.Fatalf("prompts: %+v", im.Prompts)
	}
	if im.Prompts[0].Name != "" || !filepath.IsAbs(im.Prompts[0].Filepath) {
		t.Fatalf("body prompt not resolved absolute: %+v", im.Prompts[0])
	}
	if im.Prompts[1].Name != "system:context" || filepath.Base(im.Prompts[1].Filepath) != "ctx.md" {
		t.Fatalf("override prompt: %+v", im.Prompts[1])
	}
	if len(im.Skills) != 1 || !filepath.IsAbs(im.Skills[0]) {
		t.Fatalf("skills: %+v", im.Skills)
	}
	if filepath.Base(im.Dir) == "" {
		t.Fatalf("Dir not set")
	}
}

func TestParseHarnessDefaults(t *testing.T) {
	dir := write(t, `schema_version: 1
harness:
  type: codex
  model: gpt-5
  effort: high
  interactive: true
prompts: [task.md]
`)
	im, err := Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if im.Harness.Type != "codex" || im.Harness.Model != "gpt-5" ||
		im.Harness.Effort != "high" || !im.Harness.Interactive {
		t.Fatalf("harness defaults = %+v", im.Harness)
	}
}

func TestParseHarnessExplicitFalse(t *testing.T) {
	dir := write(t, `schema_version: 1
harness:
  type: claude
  interactive: false
`)
	im, err := Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if im.Harness.Interactive {
		t.Fatalf("interactive = true, want explicit false")
	}
}

func TestParseHarnessRejectsUnknownField(t *testing.T) {
	dir := write(t, "schema_version: 1\nharness:\n  type: claude\n  temperature: 1\n")
	_, err := Parse(dir)
	if err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Fatalf("unknown harness field error = %v", err)
	}
}

func TestParseHarnessRejectsUnsupportedType(t *testing.T) {
	dir := write(t, "schema_version: 1\nharness:\n  type: bard\n")
	_, err := Parse(dir)
	if err == nil || !strings.Contains(err.Error(), `unsupported harness type "bard"`) {
		t.Fatalf("unsupported harness error = %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(dir, "Tariboyfile.yaml")) {
		t.Fatalf("unsupported harness error is not path-qualified: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"unknown field":       "schema_version: 1\nbogus: 1\n",
		"newer schema":        "schema_version: 2\n",
		"zero schema":         "from: x\n",
		"unsupported harness": "schema_version: 1\nharness: { type: bard }\n",
		"missing prompt":      "schema_version: 1\nprompts: [nope.md]\n",
		"missing skill":       "schema_version: 1\nskills: [./nope]\n",
	}
	for name, yaml := range cases {
		t.Run(name, func(t *testing.T) {
			dir := write(t, yaml)
			if _, err := Parse(dir); err == nil {
				t.Fatalf("%s: expected error", name)
			}
		})
	}
}

func TestParseFileForm(t *testing.T) {
	dir := write(t, "schema_version: 1\nprompts: [task.md]\n")
	if _, err := Parse(filepath.Join(dir, "Tariboyfile.yaml")); err != nil {
		t.Fatalf("file form: %v", err)
	}
}
