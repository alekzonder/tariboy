package imagefile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func skillMD(name, body string) string {
	return "---\nname: " + name + "\ndescription: " + name + " skill\n---\n" + body + "\n"
}

func TestLayersOrdersParentsFirstAndVisitsSharedAncestorOnce(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"base/Tariboyfile.yaml":  "schema_version: 2\nplugins: []\nskills: []\nprompts: []\n",
		"a/Tariboyfile.yaml":     "schema_version: 2\nextends: [../base]\nplugins: []\nskills: []\nprompts: []\n",
		"b/Tariboyfile.yaml":     "schema_version: 2\nextends: [../base]\nplugins: []\nskills: []\nprompts: []\n",
		"child/Tariboyfile.yaml": "schema_version: 2\nextends: [../a, ../b]\nplugins: []\nskills: []\nprompts: []\n",
	})
	got, err := Layers(filepath.Join(root, "child"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, dir := range got {
		names = append(names, filepath.Base(dir))
	}
	if want := []string{"base", "a", "b", "child"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("layers = %v, want %v", names, want)
	}
}

func TestLayersRejectsCyclesAndMissingParents(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/Tariboyfile.yaml":        "schema_version: 2\nextends: [../b]\nplugins: []\nskills: []\nprompts: []\n",
		"b/Tariboyfile.yaml":        "schema_version: 2\nextends: [../a]\nplugins: []\nskills: []\nprompts: []\n",
		"orphan/Tariboyfile.yaml":   "schema_version: 2\nextends: [../missing]\nplugins: []\nskills: []\nprompts: []\n",
		"relative/Tariboyfile.yaml": "schema_version: 2\nextends: [a]\nplugins: []\nskills: []\nprompts: []\n",
	})
	for dir, want := range map[string]string{"a": "cycle", "orphan": "missing", "relative": "./, ../"} {
		if _, err := Layers(filepath.Join(root, dir)); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Layers(%s) error = %v, want %q", dir, err, want)
		}
	}
}

func TestAssembleMergesLayersInOrder(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"parent/Tariboyfile.yaml": `schema_version: 2
image_version: 1.0.0
plugins: [{name: whoami}]
skills: [{dir: ./skills/demo}, {dir: ./skills/keep}]
prompts: [{file: ./instructions.md}, {runtime: identity}, {file: ./shared.md}]
`,
		"parent/instructions.md":        "parent",
		"parent/shared.md":              "shared",
		"parent/skills/demo/SKILL.md":   skillMD("demo", "parent version"),
		"parent/skills/demo/parent.txt": "parent only",
		"parent/skills/keep/SKILL.md":   skillMD("keep", "keep"),
		"child/Tariboyfile.yaml": `schema_version: 2
image_version: 2.0.0
extends: [../parent]
plugins: [{name: whoami}, {name: context}]
skills: [{dir: ./skills/demo}]
prompts: [{file: ./instructions.md}, {runtime: identity}, {file: ./copy.md}]
`,
		"child/instructions.md":      "child",
		"child/copy.md":              "shared",
		"child/skills/demo/SKILL.md": skillMD("demo", "child version"),
	})
	dir, cleanup, err := Assemble(filepath.Join(root, "child"), t.TempDir(), ResolveRoots{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got, err := ParseV2(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ImageVersion != "2.0.0" || len(got.Extends) != 0 {
		t.Fatalf("assembled header = %q %v", got.ImageVersion, got.Extends)
	}
	if want := []V2Plugin{{Name: "whoami"}, {Name: "context"}}; !reflect.DeepEqual(got.Plugins, want) {
		t.Fatalf("plugins = %#v", got.Plugins)
	}
	if want := []SkillEntry{{Dir: "./skills/keep"}, {Dir: "./skills/demo"}}; !reflect.DeepEqual(got.Skills, want) {
		t.Fatalf("skills = %#v", got.Skills)
	}
	want := []PromptEntry{
		{File: "./.tariboy-extends/0/instructions.md"},
		{Runtime: "identity"},
		{File: "./.tariboy-extends/0/shared.md"},
		{File: "./.tariboy-extends/1/instructions.md"},
	}
	if !reflect.DeepEqual(got.Prompts, want) {
		t.Fatalf("prompts = %#v, want %#v", got.Prompts, want)
	}
	for rel, body := range map[string]string{
		".tariboy-extends/0/instructions.md": "parent",
		".tariboy-extends/1/instructions.md": "child",
		"skills/demo/SKILL.md":               skillMD("demo", "child version"),
	} {
		if data, err := os.ReadFile(filepath.Join(dir, rel)); err != nil || string(data) != body {
			t.Fatalf("%s = %q, %v; want %q", rel, data, err, body)
		}
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("build directory survived cleanup: %v", err)
	}
}

func TestAssembleInstallsEachLayerLockInTheBuildDirectory(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"parent/Tariboyfile.yaml": "schema_version: 2\nplugins: []\nskills: [{dir: ./.agents/skills/demo}]\nprompts: []\n",
		"parent/skills-lock.json": "parent",
		"child/Tariboyfile.yaml":  "schema_version: 2\nextends: [../parent]\nplugins: []\nskills: [{dir: ./.agents/skills/demo}, {dir: ../outside}]\nprompts: []\n",
		"child/skills-lock.json":  "child",
		"outside/SKILL.md":        skillMD("outside", "outside"),
	})
	var installed []string
	install := func(dir string) error {
		lock, err := os.ReadFile(filepath.Join(dir, "skills-lock.json"))
		if err != nil {
			return err
		}
		installed = append(installed, string(lock))
		writeTree(t, dir, map[string]string{".agents/skills/demo/SKILL.md": skillMD("demo", string(lock))})
		return nil
	}
	dir, cleanup, err := Assemble(filepath.Join(root, "child"), t.TempDir(), ResolveRoots{}, install)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if want := []string{"parent", "child"}; !reflect.DeepEqual(installed, want) {
		t.Fatalf("installed = %v, want %v", installed, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills-lock.json")); !os.IsNotExist(err) {
		t.Fatalf("lock survived assembly: %v", err)
	}
	got, err := ParseV2(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []SkillEntry{{Dir: "./.agents/skills/demo"}, {Dir: filepath.Join(root, "outside")}}
	if !reflect.DeepEqual(got.Skills, want) {
		t.Fatalf("skills = %#v, want %#v", got.Skills, want)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".agents/skills/demo/SKILL.md")); !strings.Contains(string(data), "child") {
		t.Fatalf("demo skill = %q, want the child install", data)
	}
}
