// Package lock reads and writes tbox-lock.json, which pins every remote
// skill and prompt entry of an image source to a commit or a content hash.
package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/paths"
)

// Filename is the lock file next to a Tariboyfile.
const Filename = "tbox-lock.json"

const currentVersion = 1

// Entry pins one remote entry. Kind is "git" or "url". Commit is set for git.
// SHA256 is the tree_sha256 of a skill directory or the body hash of a file.
type Entry struct {
	Kind   string `json:"kind"`
	URL    string `json:"url"`
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
	SHA256 string `json:"sha256"`
}

// File is the content of a lock file.
type File struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

// Key identifies a remote entry by its declared address.
func Key(kind, url, path, ref string) string {
	return kind + "|" + url + "|" + path + "|" + ref
}

// Read loads the lock at path. A missing file is an empty lock.
func Read(path string) (File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return File{Version: currentVersion, Entries: map[string]Entry{}}, nil
	}
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Version != currentVersion {
		return File{}, fmt.Errorf("%s: unsupported lock version %d", path, f.Version)
	}
	if f.Entries == nil {
		f.Entries = map[string]Entry{}
	}
	return f, nil
}

// Write stores the lock at path atomically with sorted keys and a trailing
// newline.
func (f File) Write(path string) error {
	f.Version = currentVersion
	if f.Entries == nil {
		f.Entries = map[string]Entry{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lock-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// PathFor returns where the lock of an image source lives: next to the
// Tariboyfile when sourceDir is writable and not inside the Stores
// directory, otherwise under <home>/locks keyed by the absolute source path.
func PathFor(sourceDir string, home paths.Home) string {
	abs, err := filepath.Abs(sourceDir)
	if err != nil {
		abs = sourceDir
	}
	if !underStores(abs, home) && writable(abs) {
		return filepath.Join(abs, Filename)
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(string(home), "locks", hex.EncodeToString(sum[:])+".json")
}

func canonical(p string) string {
	if c, err := filepath.EvalSymlinks(p); err == nil {
		return c
	}
	return filepath.Clean(p)
}

func underStores(abs string, home paths.Home) bool {
	rel, err := filepath.Rel(canonical(home.StoresDir()), canonical(abs))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".tbox-probe-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}
