package imagefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePromptFileSupportsEveryPathForm(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	roots := ResolveRoots{Store: filepath.Join(base, "store"), CurrentVersionStore: filepath.Join(base, "current"), Plugins: filepath.Join(base, "plugins")}
	paths := map[string]string{
		"./local.md": source, "$STORE/shared.md": roots.Store,
		"$CURRENT_VERSION_STORE/builtin.md": roots.CurrentVersionStore,
		"$PLUGINS/acme/1/prompt.md":         roots.Plugins,
	}
	for value, root := range paths {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(value)
		if value[0] == '$' {
			name = filepath.Base(value)
		}
		if value == "$PLUGINS/acme/1/prompt.md" {
			if err := os.MkdirAll(filepath.Join(root, "acme", "1"), 0o700); err != nil {
				t.Fatal(err)
			}
			root = filepath.Join(root, "acme", "1")
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	abs := filepath.Join(base, "absolute.md")
	if err := os.WriteFile(abs, []byte("absolute"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths[abs] = ""
	wantCategory := map[string]string{"./local.md": "source", "$STORE/shared.md": "store", "$CURRENT_VERSION_STORE/builtin.md": "current-store", "$PLUGINS/acme/1/prompt.md": "plugin", abs: "absolute"}
	for value := range paths {
		got, err := ResolvePromptFile(source, value, roots)
		if err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if got.Source != value || got.Category != wantCategory[value] || got.Size <= 0 || got.SHA256 == "" || !filepath.IsAbs(got.Path) {
			t.Fatalf("%s => %#v", value, got)
		}
	}
}

func TestResolvePromptFileRejectsUnsafeInputs(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	store := filepath.Join(base, "store")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(source, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	roots := ResolveRoots{Store: store, CurrentVersionStore: filepath.Join(base, "current"), Plugins: filepath.Join(base, "plugins")}
	for _, value := range []string{"relative.md", "../secret.md", "./missing.md", "./link.md", "./dir", "$STORE/../secret.md", "$UNKNOWN/x.md", "$STORE"} {
		if _, err := ResolvePromptFile(source, value, roots); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestResolveSkillDirectorySupportsEveryPathForm(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	roots := ResolveRoots{
		Store:               filepath.Join(base, "store"),
		CurrentVersionStore: filepath.Join(base, "current"),
		Plugins:             filepath.Join(base, "plugins"),
	}
	tests := []struct {
		value    string
		path     string
		category string
	}{
		{"./skills/local", filepath.Join(source, "skills", "local"), "source"},
		{"$STORE/skills/shared", filepath.Join(roots.Store, "skills", "shared"), "store"},
		{"$CURRENT_VERSION_STORE/skills/builtin", filepath.Join(roots.CurrentVersionStore, "skills", "builtin"), "current-store"},
		{"$PLUGINS/acme/1/skills/plugin", filepath.Join(roots.Plugins, "acme", "1", "skills", "plugin"), "plugin"},
		{filepath.Join(base, "absolute"), filepath.Join(base, "absolute"), "absolute"},
	}
	for _, tc := range tests {
		if err := os.MkdirAll(tc.path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tc.path, "SKILL.md"), []byte("skill"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := ResolveSkillDirectory(source, tc.value, roots)
		if err != nil {
			t.Fatalf("ResolveSkillDirectory(%q): %v", tc.value, err)
		}
		wantPath, err := filepath.Abs(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Path != wantPath || got.Source != tc.value || got.Category != tc.category {
			t.Fatalf("ResolveSkillDirectory(%q) = %#v", tc.value, got)
		}
	}
}

func TestResolveSkillDirectoryRejectsUnsafeInputs(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	store := filepath.Join(base, "store")
	plugins := filepath.Join(base, "plugins")
	for _, dir := range []string{source, store, plugins} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "linked")); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(source, "regular")
	if err := os.WriteFile(regular, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	roots := ResolveRoots{Store: store, CurrentVersionStore: filepath.Join(base, "current"), Plugins: plugins}
	for _, value := range []string{"", "$HOME/x", "$PLUGINS", "$PLUGINS/../outside", "../outside", "./missing", "./linked", "./regular"} {
		if _, err := ResolveSkillDirectory(source, value, roots); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}
