package agentdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstantHooksDir is the harness plugin with the instant mail hooks of the
// agent. It lives in the agent directory, never in the customer repository
// or the user's harness configuration.
func (l Layout) InstantHooksDir(harness string) string {
	return filepath.Join(l.Root, "harness-hooks", harness)
}

// WritePluginFiles writes files under dir, each through a temporary file and a
// rename, so a harness never reads a half-written file. Directories are 0700.
func WritePluginFiles(dir string, files []BridgeFile) error {
	for _, f := range files {
		rel := filepath.FromSlash(f.Path)
		if f.Path == "" || filepath.IsAbs(rel) || !filepath.IsLocal(rel) || strings.Contains(f.Path, "\\") {
			return fmt.Errorf("invalid plugin file path %q", f.Path)
		}
		path := filepath.Join(dir, rel)
		if err := mkdirAll0700(dir, filepath.Dir(path)); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
		if err != nil {
			return err
		}
		_, werr := tmp.Write(f.Body)
		cerr := tmp.Close()
		if err := firstErr(werr, cerr, os.Chmod(tmp.Name(), f.Mode.Perm())); err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
	}
	return nil
}

// mkdirAll0700 creates path with 0700 directories and sets 0700 on root and
// every directory between root and path, also on ones that already exist.
func mkdirAll0700(root, path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	for p := path; ; p = filepath.Dir(p) {
		if err := os.Chmod(p, 0o700); err != nil {
			return err
		}
		if p == root || p == filepath.Dir(p) {
			return nil
		}
	}
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
