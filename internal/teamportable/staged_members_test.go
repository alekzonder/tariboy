package teamportable

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// validateStagedTeamMembers accepts only the allowed regular files and keeps
// its error texts: a name outside the set (or a directory) is unsupported, an
// allowed name that is not a regular file is unsafe.
func TestValidateStagedTeamMembersErrorTexts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{"allowed files", func(t *testing.T, root string) {
			write(t, filepath.Join(root, "tariboy-compose.yaml"))
			write(t, filepath.Join(root, ".team-preview.json"))
		}, ""},
		{"unknown file", func(t *testing.T, root string) { write(t, filepath.Join(root, "extra")) },
			`team portable: unsupported staged member "extra"`},
		{"directory", func(t *testing.T, root string) { mkdir(t, filepath.Join(root, "dir")) },
			`team portable: unsupported staged member "dir"`},
		{"unknown symlink", func(t *testing.T, root string) { link(t, "/etc/passwd", filepath.Join(root, "link")) },
			`team portable: unsupported staged member "link"`},
		{"allowed name symlink", func(t *testing.T, root string) {
			link(t, "/etc/passwd", filepath.Join(root, "tariboy-compose.yaml"))
		}, `team portable: unsafe staged member "tariboy-compose.yaml"`},
		{"allowed name fifo", func(t *testing.T, root string) {
			if err := syscall.Mkfifo(filepath.Join(root, ".operation.lease"), 0o600); err != nil {
				t.Skip(err)
			}
		}, `team portable: unsafe staged member ".operation.lease"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)
			err := validateStagedTeamMembers(root)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tc.want {
				t.Fatalf("err = %q, want %q", got, tc.want)
			}
		})
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
