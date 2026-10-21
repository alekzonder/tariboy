// Package refstore is the name and tag layer shared by the on-disk image
// stores (image and workflowimage):
//
//	<Dir>/<name>/refs/<id>   content, owned by the caller (a file or a directory)
//	<Dir>/<name>/tags/<tag>  one line holding the hex id the tag points at
//
// Only names, tag pointers and their atomic writes live here. What a ref is,
// how its id is computed and how content is published stay with the caller.
// The store does no locking; callers serialize writes.
package refstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	// ErrNotFound marks a tag that does not exist.
	ErrNotFound = errors.New("tag not found")
	// ErrInvalidName marks a name or tag that could escape the store when
	// joined into a path.
	ErrInvalidName = errors.New("invalid name")
	// ErrInvalidID marks an id that is not 64 lowercase hex digits.
	ErrInvalidID = errors.New("invalid ref id")
	// ErrInvalidPointer marks a tag file that does not hold an id.
	ErrInvalidPointer = errors.New("invalid tag pointer")
)

const tmpPrefix = ".tmp-"

var idPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// IsID reports whether s has the form of a ref id.
func IsID(s string) bool { return idPattern.MatchString(s) }

// PointerError reports a tag file whose content is not an id.
type PointerError struct {
	Name, Tag, Value string
}

func (e *PointerError) Error() string {
	return fmt.Sprintf("tag %s of %s holds %q, not a ref id", e.Tag, e.Name, e.Value)
}

func (e *PointerError) Is(target error) bool { return target == ErrInvalidPointer }

// CheckName rejects a name or tag that could escape the store when joined
// into a path: empty, ".", "..", or containing a slash, backslash or NUL.
// Stores may apply a stricter rule of their own before calling in.
func CheckName(v string) error {
	if v == "" || v == "." || v == ".." || strings.ContainsAny(v, "/\\\x00") {
		return fmt.Errorf("%w: %q", ErrInvalidName, v)
	}
	return nil
}

func checkNames(name, tag string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	return CheckName(tag)
}

// Store is the name and tag layer rooted at Dir.
type Store struct{ Dir string }

// NameDir is <Dir>/<name>. Callers may stage temporary files here.
func (s Store) NameDir(name string) string { return filepath.Join(s.Dir, name) }

// RefsDir is <Dir>/<name>/refs, where the caller keeps its content.
func (s Store) RefsDir(name string) string { return filepath.Join(s.Dir, name, "refs") }

// TagsDir is <Dir>/<name>/tags.
func (s Store) TagsDir(name string) string { return filepath.Join(s.Dir, name, "tags") }

// TagPath is <Dir>/<name>/tags/<tag>.
func (s Store) TagPath(name, tag string) string { return filepath.Join(s.TagsDir(name), tag) }

// Resolve returns the id a tag points at. A missing tag matches ErrNotFound;
// a tag file that holds anything but an id is a *PointerError.
func (s Store) Resolve(name, tag string) (string, error) {
	if err := checkNames(name, tag); err != nil {
		return "", err
	}
	data, err := os.ReadFile(s.TagPath(name, tag))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %s %s", ErrNotFound, name, tag)
	}
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if !IsID(id) {
		return "", &PointerError{Name: name, Tag: tag, Value: id}
	}
	return id, nil
}

// WriteTag points tag at id. The pointer is written to a temporary file,
// synced, renamed over the tag and the tags directory is synced, so a crash
// leaves either the old pointer or the new one.
func (s Store) WriteTag(name, tag, id string) error {
	if err := checkNames(name, tag); err != nil {
		return err
	}
	if !IsID(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	dir := s.TagsDir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, tmpPrefix)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(id + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.TagPath(name, tag)); err != nil {
		return err
	}
	return SyncDir(dir)
}

// RemoveTag deletes a tag and syncs the tags directory. A missing tag
// matches fs.ErrNotExist.
func (s Store) RemoveTag(name, tag string) error {
	if err := checkNames(name, tag); err != nil {
		return err
	}
	if err := os.Remove(s.TagPath(name, tag)); err != nil {
		return err
	}
	return SyncDir(s.TagsDir(name))
}

// Tags returns the tags of name, sorted: the regular files in its tags
// directory whose names do not start with a dot (temporary files do).
func (s Store) Tags(name string) ([]string, error) {
	if err := CheckName(name); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.TagsDir(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// List returns the names in the store, sorted: every directory under Dir
// whose name passes CheckName. A missing Dir is an empty store.
func (s Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && CheckName(e.Name()) == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// PruneEmpty removes the tags, refs and name directories of name when they
// are empty. It is best effort: a directory that still holds anything stays.
func (s Store) PruneEmpty(name string) {
	if CheckName(name) != nil {
		return
	}
	_ = os.Remove(s.TagsDir(name))
	_ = os.Remove(s.RefsDir(name))
	_ = os.Remove(s.NameDir(name))
}

// SyncDir fsyncs a directory so a rename or removal inside it is durable.
func SyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
