// Package paths resolves the tbox state directory layout.
package paths

import (
	"errors"
	"path/filepath"
)

// Home is the tbox root directory, `$TBOX_HOME` or `~/.tbox`.
type Home string

// Resolve returns the tbox root from `$TBOX_HOME`, else `$HOME/.tbox`.
func Resolve(getenv func(string) string) (Home, error) {
	if v := getenv("TBOX_HOME"); v != "" {
		return Home(v), nil
	}
	if h := getenv("HOME"); h != "" {
		return Home(filepath.Join(h, ".tbox")), nil
	}
	return "", errors.New("neither TBOX_HOME nor HOME is set")
}

func (h Home) join(parts ...string) string {
	return filepath.Join(append([]string{string(h)}, parts...)...)
}

// ImagesDir is the directory of installed images.
func (h Home) ImagesDir() string { return h.join("images") }

// RunRoot is the parent of all per-run directories.
func (h Home) RunRoot() string { return h.join("run") }

// RunDir is the run directory of one image digest and harness.
func (h Home) RunDir(digest, harness string) string { return h.join("run", digest, harness) }

// StoresDir is the directory of configured stores.
func (h Home) StoresDir() string { return h.join("stores") }

// CacheDir is the download and build cache.
func (h Home) CacheDir() string { return h.join("cache") }

// SourcesDir is the directory of image sources.
func (h Home) SourcesDir() string { return h.join("sources") }
