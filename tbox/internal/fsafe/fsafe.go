// Package fsafe holds small filesystem helpers that refuse to follow symlinks
// or escape a root directory.
package fsafe

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ReadFileNoFollow reads a regular file without following a symlink in the
// final path component and fails when the file is larger than max bytes.
func ReadFileNoFollow(path string, max int64) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("open file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > max {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, max)
	}
	body, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, max)
	}
	return body, nil
}

// Within returns the cleaned absolute target with symlinks resolved for its
// longest existing prefix, and fails when the result is outside root after
// symlink evaluation. The target does not have to exist.
func Within(root, target string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	canonRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	resolved, err := resolveExistingPrefix(absTarget)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(canonRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside %s", target, root)
	}
	return resolved, nil
}

// resolveExistingPrefix evaluates symlinks of the longest existing ancestor of
// p and appends the remaining, not yet existing, components.
func resolveExistingPrefix(p string) (string, error) {
	var rest []string
	cur := p
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, rest[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if _, lerr := os.Lstat(cur); lerr == nil {
			return "", fmt.Errorf("%s is a dangling symlink", cur)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}
