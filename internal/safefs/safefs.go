// Package safefs holds the one rule set for untrusted relative paths, source
// trees and strict JSON documents that the image, workflow and portable
// archive packages share. Each caller keeps its own package-specific rules
// (reserved names, prefixes) and error texts on top of these primitives.
package safefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrInvalidPath reports a relative path that CleanRel rejects.
var ErrInvalidPath = errors.New("invalid relative path")

// CleanRel validates a slash-separated relative path of an archive member or
// a tree entry and returns it unchanged. It rejects an empty path, ".", an
// absolute path, any ".." segment (a bare ".." included), a backslash, a NUL
// byte and any non-canonical form (path.Clean(name) != name), so "./a",
// "a//b" and "a/" are rejected rather than normalised. A name that merely
// starts with two dots, such as "..foo", is valid.
func CleanRel(name string) (string, error) {
	if name == "" || strings.IndexByte(name, 0) >= 0 || strings.ContainsRune(name, '\\') || path.IsAbs(name) {
		return "", fmt.Errorf("%w %q", ErrInvalidPath, name)
	}
	if path.Clean(name) != name || name == "." {
		return "", fmt.Errorf("%w %q", ErrInvalidPath, name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w %q", ErrInvalidPath, name)
		}
	}
	return name, nil
}

// unsafeKind is an ErrUnsafeFile reason. It matches ErrUnsafeFile with
// errors.Is while keeping its own short text for callers' messages.
type unsafeKind string

func (k unsafeKind) Error() string        { return string(k) }
func (k unsafeKind) Is(target error) bool { return target == ErrUnsafeFile }

var (
	// ErrUnsafeFile matches every entry WalkRegular refuses by type.
	ErrUnsafeFile = errors.New("unsafe file")
	// ErrSymlink is a symbolic link anywhere in the tree, the root included.
	ErrSymlink error = unsafeKind("symlink")
	// ErrNonRegular is a FIFO, socket, device or other non-regular file.
	ErrNonRegular error = unsafeKind("non-regular file")

	ErrFileTooLarge  = errors.New("file too large")
	ErrTooManyFiles  = errors.New("too many files")
	ErrTotalTooLarge = errors.New("total size too large")
)

// PathError names the tree entry (slash-separated, relative to the walk root,
// "." for the root) that WalkRegular refused and why. Its text is
// "<reason> <rel>", for example "symlink skills/a/link".
type PathError struct {
	Rel string
	Err error
}

func (e *PathError) Error() string { return e.Err.Error() + " " + e.Rel }
func (e *PathError) Unwrap() error { return e.Err }

// WalkOptions configures WalkRegular. A zero limit means no limit.
type WalkOptions struct {
	// Dirs passes directories to the callback as well as regular files.
	Dirs bool
	// Skip leaves an entry out before any check: a skipped directory is not
	// descended into. It receives the slash-separated relative path.
	Skip func(rel string) bool
	// MaxFileSize bounds each regular file's size in bytes.
	MaxFileSize int64
	// MaxFiles bounds the number of regular files.
	MaxFiles int
	// MaxTotal bounds the summed size of regular files in bytes.
	MaxTotal int64
}

// WalkRegular walks the directory tree at root in lexical order and calls fn
// for each regular file, and for each directory when opts.Dirs is set, with
// the slash-separated path relative to root. The root itself is not passed to
// fn and must be a real directory. A symbolic link or any other non-regular
// entry stops the walk with a *PathError matching ErrUnsafeFile; the limits in
// opts stop it with a *PathError matching the limit's error. fn may return
// fs.SkipDir for a directory to leave it out; any other error stops the walk
// and is returned as is.
func WalkRegular(root string, opts WalkOptions, fn func(rel string, d fs.DirEntry, info fs.FileInfo) error) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	switch {
	case rootInfo.Mode()&fs.ModeSymlink != 0:
		return &PathError{Rel: ".", Err: ErrSymlink}
	case !rootInfo.IsDir():
		return fmt.Errorf("walk %s: not a directory", root)
	}
	var files int
	var total int64
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == root {
			return nil
		}
		relOS, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)
		if opts.Skip != nil && opts.Skip(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode&fs.ModeSymlink != 0:
			return &PathError{Rel: rel, Err: ErrSymlink}
		case mode.IsDir():
			if !opts.Dirs {
				return nil
			}
			return fn(rel, d, info)
		case !mode.IsRegular():
			return &PathError{Rel: rel, Err: ErrNonRegular}
		}
		files++
		if opts.MaxFiles > 0 && files > opts.MaxFiles {
			return &PathError{Rel: rel, Err: ErrTooManyFiles}
		}
		if opts.MaxFileSize > 0 && info.Size() > opts.MaxFileSize {
			return &PathError{Rel: rel, Err: ErrFileTooLarge}
		}
		total += info.Size()
		if opts.MaxTotal > 0 && total > opts.MaxTotal {
			return &PathError{Rel: rel, Err: ErrTotalTooLarge}
		}
		return fn(rel, d, info)
	})
}

// readError keeps ReadRegular's short text while matching its kind (and so
// ErrUnsafeFile for ErrNonRegular) with errors.Is.
type readError struct {
	msg  string
	kind error
}

func (e *readError) Error() string { return e.msg }
func (e *readError) Unwrap() error { return e.kind }

// ReadRegular reads the regular file at path without following a symbolic
// link and without blocking on a FIFO: it opens with O_NOFOLLOW|O_NONBLOCK,
// takes type, size and mode from the opened descriptor, and stops reading one
// byte past limit. A limit of zero or less means no limit. It closes the gap
// between a walk's Lstat and the read, when the entry may have been swapped
// or grown. Errors: "cannot be opened as a regular file: <cause>" (wrapping
// the open error), "is not a regular file" (matches ErrNonRegular), and
// "is larger than <limit> bytes" (matches ErrFileTooLarge).
func ReadRegular(path string, limit int64) ([]byte, fs.FileInfo, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot be opened as a regular file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, &readError{msg: "is not a regular file", kind: ErrNonRegular}
	}
	tooLarge := &readError{msg: fmt.Sprintf("is larger than %d bytes", limit), kind: ErrFileTooLarge}
	var r io.Reader = f
	if limit > 0 {
		if info.Size() > limit {
			return nil, nil, tooLarge
		}
		r = io.LimitReader(f, limit+1)
	}
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	if limit > 0 && int64(len(content)) > limit {
		return nil, nil, tooLarge
	}
	return content, info, nil
}

// ErrTrailingData reports data after the single JSON value DecodeStrictJSON
// reads.
var ErrTrailingData = errors.New("data after the JSON value")

// DecodeStrictJSON decodes exactly one JSON value from r into v. Unknown
// object fields are rejected, and anything but white space after the value is
// an error matching ErrTrailingData. A decode error is returned unwrapped.
func DecodeStrictJSON(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return ErrTrailingData
		}
		return fmt.Errorf("%w: %v", ErrTrailingData, err)
	}
	return nil
}
