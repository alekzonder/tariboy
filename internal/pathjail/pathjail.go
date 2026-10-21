// Package pathjail holds the one path-confinement boundary shared by
// internal/filebrowser and internal/fsbrowser: symlink-aware resolution of a
// path that may not exist yet, a root-aware containment test, and the typed
// Error both packages return. Callers keep their own lexical rules (how a
// caller path maps to a target under the root) and call into this package for
// the symlink and containment checks.
package pathjail

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Error is a typed failure carrying a stable code the command layer maps to
// the api JSON error envelope (bad_path / not_found / not_dir / ...).
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// BadPath returns a bad_path error.
func BadPath(msg string) *Error { return &Error{Code: "bad_path", Msg: msg} }

// NotFound returns a not_found error.
func NotFound(msg string) *Error { return &Error{Code: "not_found", Msg: msg} }

// NotDir returns a not_dir error.
func NotDir(msg string) *Error { return &Error{Code: "not_dir", Msg: msg} }

// Within reports whether target is root itself or a descendant of it. The
// descendant test compares against root plus a single trailing separator; when
// root is the filesystem root ("/") it already ends in a separator, so the
// prefix stays "/" instead of becoming "//" (which would reject every child).
func Within(root, target string) bool {
	if target == root {
		return true
	}
	sep := string(os.PathSeparator)
	prefix := root
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	return strings.HasPrefix(target, prefix)
}

// ResolveExisting evaluates symlinks on the deepest existing ancestor of p and
// re-appends the trailing components that do not exist yet, so it also works for
// a path about to be created. Any symlink along the existing portion (including p
// itself) is followed to its real location.
func ResolveExisting(p string) (string, error) {
	p = filepath.Clean(p)
	rest := ""
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			if rest == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, rest), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p { // reached filesystem root without an existing ancestor
			return "", err
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// IsNotDir reports whether err is ENOTDIR.
func IsNotDir(err error) bool {
	return errors.Is(err, syscall.ENOTDIR)
}
