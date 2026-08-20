// Package storeassets installs immutable, versioned assets bundled with Tariboy.
package storeassets

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/paths"
)

// bundled is the canonical source for prompts shipped with this Tariboy version.
//
//go:embed skills/**
var bundled embed.FS

// ReadBundled returns one canonical Store asset from the embedded distribution.
// The files under store/skills remain the source of truth; this fallback keeps
// schema-v1 builders usable in tests and offline tooling before Ensure runs.
func ReadBundled(name string) ([]byte, error) {
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return nil, fmt.Errorf("invalid bundled Store path %q", name)
	}
	return bundled.ReadFile(clean)
}

// Ensure atomically installs the bundled skills for productVersion. An existing
// installation is accepted only when every bundled file has identical bytes.
func Ensure(p paths.Paths, productVersion string) error {
	if productVersion == "" || productVersion == "." || productVersion == ".." || strings.ContainsAny(productVersion, `/\\`) {
		return fmt.Errorf("invalid product version %q", productVersion)
	}
	dst := p.CurrentVersionStoreDir(productVersion)
	if _, err := os.Stat(dst); err == nil {
		return verify(dst)
	} else if !os.IsNotExist(err) {
		return err
	}
	versions := filepath.Dir(dst)
	if err := os.MkdirAll(versions, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(versions, productVersion+".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := fs.WalkDir(bundled, "skills", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(tmp, filepath.FromSlash(name))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		body, err := bundled.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o600)
	}); err != nil {
		return fmt.Errorf("install bundled Store: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			return verify(dst)
		}
		return err
	}
	return nil
}

func verify(dst string) error {
	seen := map[string]bool{}
	err := fs.WalkDir(bundled, "skills", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel := filepath.FromSlash(name)
		seen[filepath.Clean(rel)] = true
		installed := filepath.Join(dst, rel)
		info, err := os.Lstat(installed)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Store asset %s is not a regular file", rel)
		}
		want, err := bundled.ReadFile(name)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(installed)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("conflicting Store asset %s", rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return filepath.WalkDir(dst, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected non-regular Store asset %s", name)
		}
		rel, err := filepath.Rel(dst, name)
		if err != nil {
			return err
		}
		if !seen[filepath.Clean(rel)] {
			return fmt.Errorf("unexpected Store asset %s", rel)
		}
		return nil
	})
}
