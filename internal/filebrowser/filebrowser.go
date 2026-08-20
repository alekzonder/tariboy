// Package filebrowser implements a CWD-jailed view+CRUD file API for an agent's
// working directory. Every operation resolves the caller-supplied relative path
// under a fixed root and refuses to touch anything outside it — rejecting both
// lexical traversal (.., absolute paths) and symlinks whose real target escapes
// the root. The package is UI-agnostic: it returns plain structs and typed
// *Error values, leaving HTTP/JSON shaping to the api layer.
package filebrowser

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"
)

// MaxReadBytes caps how large a file Read will return inline; anything bigger is
// reported as kind="too_large" instead of streaming megabytes into a JSON body.
const MaxReadBytes = 2 << 20 // 2 MiB

// Error is a typed failure carrying a stable code the api layer maps to its JSON
// error envelope (bad_path / not_found / exists / is_dir / ...).
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

func badPath(msg string) *Error  { return &Error{Code: "bad_path", Msg: msg} }
func notFound(msg string) *Error { return &Error{Code: "not_found", Msg: msg} }

// Entry is one item in a directory listing.
type Entry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"` // unix seconds
}

// Content is the result of Read. Kind is one of "text", "binary" or "too_large".
// Data holds the file bytes only when Kind == "text".
type Content struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Data string `json:"content,omitempty"`
	Size int64  `json:"size"`
}

// Resolve joins rel under root and returns the cleaned absolute target, refusing
// any path that escapes root either lexically or via a symlink whose real target
// lands outside root. The target need not exist yet (Create/Write): the deepest
// existing ancestor is resolved and the not-yet-existing tail re-appended, so a
// symlink anywhere along the existing portion is still followed and checked. rel
// is treated as relative to root regardless of a leading "/", so an absolute-
// looking input cannot escape.
func Resolve(root, rel string) (string, error) {
	root = filepath.Clean(root)
	// Reject relative traversal outright. An absolute-looking rel ("/etc/passwd")
	// is intentionally re-based onto root rather than rejected, so we strip a
	// leading separator before the check; what remains must not climb above root.
	clean := strings.TrimPrefix(filepath.Clean(rel), string(os.PathSeparator))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", badPath("path escapes the agent working directory")
	}
	target := filepath.Join(root, filepath.Clean("/"+rel))
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return "", badPath("path escapes the agent working directory")
	}
	realRoot, err := resolveExisting(root)
	if err != nil {
		return "", badPath("cannot resolve agent working directory")
	}
	realTarget, err := resolveExisting(target)
	if err != nil {
		return "", badPath("cannot resolve path")
	}
	if realTarget != realRoot && !strings.HasPrefix(realTarget, realRoot+string(os.PathSeparator)) {
		return "", badPath("path escapes the agent working directory (symlink)")
	}
	return target, nil
}

// resolveExisting evaluates symlinks on the deepest existing ancestor of p and
// re-appends the trailing components that do not exist yet, so it also works for
// a path about to be created. Any symlink along the existing portion (including p
// itself) is followed to its real location.
func resolveExisting(p string) (string, error) {
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

// List returns the entries of the directory at rel (empty rel => root). Entries
// are sorted directories-first, then by name.
func List(root, rel string) ([]Entry, error) {
	target, err := Resolve(root, rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFound("directory not found")
		}
		if isNotDir(err) {
			return nil, &Error{Code: "not_dir", Msg: "path is not a directory"}
		}
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue // entry vanished between ReadDir and Info; skip it
		}
		out = append(out, Entry{
			Name:  de.Name(),
			IsDir: de.IsDir(),
			Size:  info.Size(),
			Mtime: info.ModTime().Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir // dirs first
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Read returns the file at rel. Files larger than MaxReadBytes are reported as
// kind="too_large" without being read; non-UTF-8 / NUL-bearing content is
// reported as kind="binary"; otherwise the text is returned inline.
func Read(root, rel string) (Content, error) {
	target, err := Resolve(root, rel)
	if err != nil {
		return Content{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return Content{}, notFound("file not found")
		}
		return Content{}, err
	}
	if info.IsDir() {
		return Content{}, &Error{Code: "is_dir", Msg: "path is a directory"}
	}
	if info.Size() > MaxReadBytes {
		return Content{Path: rel, Kind: "too_large", Size: info.Size()}, nil
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return Content{}, err
	}
	if !utf8.Valid(raw) || containsNUL(raw) {
		return Content{Path: rel, Kind: "binary", Size: info.Size()}, nil
	}
	return Content{Path: rel, Kind: "text", Data: string(raw), Size: info.Size()}, nil
}

// Write overwrites (or creates) the file at rel with content, creating any
// missing parent directories. rel must not resolve to the root itself.
func Write(root, rel string, content []byte) error {
	target, err := Resolve(root, rel)
	if err != nil {
		return err
	}
	if err := notRoot(root, target); err != nil {
		return err
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return &Error{Code: "is_dir", Msg: "path is a directory"}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	return os.WriteFile(target, content, 0o600)
}

// Create makes a new file or directory at rel. kind is "file" or "dir". It fails
// with code "exists" if something is already there.
func Create(root, rel, kind string) error {
	target, err := Resolve(root, rel)
	if err != nil {
		return err
	}
	if err := notRoot(root, target); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return &Error{Code: "exists", Msg: "path already exists"}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	switch kind {
	case "dir":
		return os.Mkdir(target, 0o700)
	case "file", "":
		f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		return f.Close()
	default:
		return &Error{Code: "bad_type", Msg: "type must be file or dir"}
	}
}

// Rename moves from -> to, both confined to root. The destination's parent is
// created if missing; from must not be the root.
func Rename(root, from, to string) error {
	src, err := Resolve(root, from)
	if err != nil {
		return err
	}
	if err := notRoot(root, src); err != nil {
		return err
	}
	dst, err := Resolve(root, to)
	if err != nil {
		return err
	}
	if err := notRoot(root, dst); err != nil {
		return err
	}
	if _, err := os.Lstat(src); err != nil {
		if os.IsNotExist(err) {
			return notFound("source not found")
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// Delete removes the file or directory (recursively) at rel. It refuses to
// delete the root itself.
func Delete(root, rel string) error {
	target, err := Resolve(root, rel)
	if err != nil {
		return err
	}
	if err := notRoot(root, target); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err != nil {
		if os.IsNotExist(err) {
			return notFound("path not found")
		}
		return err
	}
	return os.RemoveAll(target)
}

// notRoot rejects an operation whose resolved target is the jail root itself, so
// write/create/rename/delete can never clobber the working directory as a whole.
func notRoot(root, target string) error {
	if filepath.Clean(target) == filepath.Clean(root) {
		return badPath("operation not allowed on the root directory")
	}
	return nil
}

func containsNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func isNotDir(err error) bool {
	return errors.Is(err, syscall.ENOTDIR)
}
