package tfile

import (
	"fmt"
	"strings"
	"testing"
)

func TestLoadExtendsMerge(t *testing.T) {
	root := t.TempDir()
	write(t, root, "base/Tariboyfile.yaml", "schema_version: 2\nimage_version: 0.1.0\nplugins:\n  - name: mail\nskills:\n  - dir: ./skills/a\n  - dir: ./skills/b\nprompts:\n  - file: ./p.md\n  - runtime: goal\n")
	write(t, root, "base/p.md", "same")
	write(t, root, "child/Tariboyfile.yaml", "schema_version: 3\nimage_version: 0.2.0\nextends:\n  - ../base\nplugins:\n  - name: mail\n  - name: goal\nskills:\n  - dir: ./other/a\n  - git: https://x/r\n    path: skills/c\nprompts:\n  - file: ./q.md\n  - runtime: goal\n")
	write(t, root, "child/q.md", "same")
	f, err := Load(root + "/child")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Extends) != 0 || f.ImageVersion != "0.2.0" || f.SchemaVersion != 3 {
		t.Fatalf("unexpected %+v", f)
	}
	if len(f.Plugins) != 2 || f.Plugins[0].Name != "mail" {
		t.Fatalf("plugins %+v", f.Plugins)
	}
	var got []string
	for _, s := range f.Skills {
		got = append(got, s.Dir+s.Path+"@"+s.BaseDir[strings.LastIndex(s.BaseDir, "/")+1:])
	}
	want := "./skills/b@base,./other/a@child,skills/c@child"
	if strings.Join(got, ",") != want {
		t.Fatalf("skills %v", got)
	}
	// q.md has the same content as p.md, runtime goal is repeated.
	if len(f.Prompts) != 2 || f.Prompts[0].File != "./p.md" || f.Prompts[1].Runtime != "goal" {
		t.Fatalf("prompts %+v", f.Prompts)
	}
}

func TestLoadCycle(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/Tariboyfile.yaml", "schema_version: 2\nextends:\n  - ../b\n")
	write(t, root, "b/Tariboyfile.yaml", "schema_version: 2\nextends:\n  - ../a\n")
	if _, err := Load(root + "/a"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("got %v", err)
	}
}

func chain(t *testing.T, n int) string {
	root := t.TempDir()
	for i := 0; i < n; i++ {
		body := "schema_version: 2\n"
		if i+1 < n {
			body += fmt.Sprintf("extends:\n  - ../d%d\n", i+1)
		}
		write(t, root, fmt.Sprintf("d%d/Tariboyfile.yaml", i), body)
	}
	return root + "/d0"
}

func TestLoadDepth(t *testing.T) {
	if _, err := Load(chain(t, 16)); err != nil {
		t.Fatalf("depth 16: %v", err)
	}
	if _, err := Load(chain(t, 17)); err == nil || !strings.Contains(err.Error(), "deeper") {
		t.Fatalf("depth 17: %v", err)
	}
}
