package commands

import (
	"path/filepath"
	"testing"
)

// resolveCwd expands relative/~ against the fs root and honors absolute paths,
// leaving existence/dir checks to agent.ValidateCwd.
func TestResolveCwd(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TARIBOY_FS_ROOT", root)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"relative resolves against root", "tmp/", filepath.Join(root, "tmp")},
		{"relative nested", "a/b/c", filepath.Join(root, "a", "b", "c")},
		{"tilde alone is the root", "~", root},
		{"tilde slash is relative to root", "~/work", filepath.Join(root, "work")},
		{"absolute is honored as-is", "/opt/data", "/opt/data"},
		{"absolute is cleaned", "/opt//data/", "/opt/data"},
		{"whitespace trimmed", "  tmp  ", filepath.Join(root, "tmp")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveCwd(tc.in)
			if err != nil {
				t.Fatalf("resolveCwd(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("resolveCwd(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
