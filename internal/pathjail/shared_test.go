package pathjail_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/filebrowser"
	"github.com/alekzonder/tariboy/internal/fsbrowser"
	"github.com/alekzonder/tariboy/internal/pathjail"
)

// TestSymlinkEscapeBothBrowsers runs the same symlink-escape scenario through
// the public API of both jailed packages: a symlink inside the root pointing
// outside must be refused with a bad_path *pathjail.Error.
func TestSymlinkEscapeBothBrowsers(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	checks := map[string]func() error{
		"filebrowser.List": func() error {
			_, err := filebrowser.List(root, "link")
			return err
		},
		"filebrowser.Write": func() error {
			return filebrowser.Write(root, "link/x.txt", []byte("x"))
		},
		"fsbrowser.List": func() error {
			_, err := fsbrowser.List(root, "link")
			return err
		},
		"fsbrowser.List nested": func() error {
			_, err := fsbrowser.List(root, "link/sub")
			return err
		},
	}
	for name, call := range checks {
		err := call()
		var pe *pathjail.Error
		if !errors.As(err, &pe) || pe.Code != "bad_path" {
			t.Errorf("%s: err = %v, want bad_path *pathjail.Error", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "x.txt")); err == nil {
		t.Error("write escaped the jail through the symlink")
	}
}
