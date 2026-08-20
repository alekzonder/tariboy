package agentskills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeDiscoveredSkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: Duplicate scope fixture.\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindScopeDuplicatesReturnsOnlyRequestedNamesAndCategories(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "cwd")
	home := filepath.Join(base, "home")
	writeDiscoveredSkill(t, filepath.Join(cwd, ".agents", "skills"), "code-review")
	writeDiscoveredSkill(t, filepath.Join(cwd, ".claude", "skills"), "code-review")
	writeDiscoveredSkill(t, filepath.Join(cwd, ".opencode", "skills"), "cwd-only")
	writeDiscoveredSkill(t, filepath.Join(home, ".claude", "skills"), "code-review")
	writeDiscoveredSkill(t, filepath.Join(home, ".codex", "skills"), "global-only")
	writeDiscoveredSkill(t, filepath.Join(home, ".config", "opencode", "skills"), "not-requested")

	got := FindScopeDuplicates([]string{"global-only", "code-review"}, cwd, home)
	want := []ScopeDuplicate{
		{Name: "code-review", Scope: "cwd"},
		{Name: "code-review", Scope: "global"},
		{Name: "global-only", Scope: "global"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("duplicates = %#v, want %#v", got, want)
	}
}

func TestFindScopeDuplicatesSkipsInvalidOrLinkedSkills(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "cwd")
	home := filepath.Join(base, "home")
	invalid := filepath.Join(cwd, ".agents", "skills", "invalid")
	if err := os.MkdirAll(invalid, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalid, "SKILL.md"), []byte("no frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "outside")
	writeDiscoveredSkill(t, target, "linked")
	linkedRoot := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(linkedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "linked"), filepath.Join(linkedRoot, "linked")); err != nil {
		t.Fatal(err)
	}
	if got := FindScopeDuplicates([]string{"invalid", "linked"}, cwd, home); len(got) != 0 {
		t.Fatalf("unsafe duplicates = %#v", got)
	}
}
