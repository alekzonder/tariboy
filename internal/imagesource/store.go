package imagesource

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/safefs"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tidy"
	"gopkg.in/yaml.v3"
)

type Store struct {
	Root  string
	Clock func() time.Time

	rename  func(oldPath, newPath string) error
	syncDir func(path string) error
}

type sourceFile struct {
	SchemaVersion int                     `yaml:"schema_version"`
	Prompts       []imagefile.PromptEntry `yaml:"prompts"`
}

func (s *Store) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func (s *Store) ensureRoot() error {
	if s.Root == "" {
		return fmt.Errorf("%w: empty store root", ErrInvalidPath)
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return fmt.Errorf("create image source root: %w", err)
	}
	if err := os.Chmod(s.Root, 0o700); err != nil {
		return fmt.Errorf("secure image source root: %w", err)
	}
	return nil
}

func validateName(name string) error {
	if name == "" || name == image.BareRef.Name {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	ref, err := image.ParseRef(name + ":latest")
	if err != nil || ref.Name != name {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

func (s *Store) sourceDir(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	dir := filepath.Join(s.Root, name)
	st, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return "", err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return "", fmt.Errorf("%w: source root %s is not a real directory", ErrUnsafeFile, name)
	}
	return dir, nil
}

func (s *Store) Create(req CreateRequest) (Source, error) {
	if err := validateName(req.Name); err != nil {
		return Source{}, err
	}
	if err := s.ensureRoot(); err != nil {
		return Source{}, err
	}
	target := filepath.Join(s.Root, req.Name)
	if _, err := os.Lstat(target); err == nil {
		return Source{}, fmt.Errorf("%w: %s", ErrExists, req.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Source{}, err
	}

	stage, err := os.MkdirTemp(s.Root, ".staging-"+req.Name+"-")
	if err != nil {
		return Source{}, fmt.Errorf("create source staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0o700); err != nil {
		return Source{}, err
	}

	config, err := yaml.Marshal(sourceFile{
		SchemaVersion: 2,
		Prompts:       []imagefile.PromptEntry{{File: "./PROMPT.md"}},
	})
	if err != nil {
		return Source{}, fmt.Errorf("marshal Tariboyfile: %w", err)
	}

	created := store.FormatTime(s.now())
	src := Source{
		SchemaVersion: 2,
		Name:          req.Name,
		CreatedAt:     created,
		UpdatedAt:     created,
		Provenance:    req.Provenance,
	}
	if err := s.writeAtomic(filepath.Join(stage, "Tariboyfile.yaml"), config, 0o600); err != nil {
		return Source{}, err
	}
	if err := s.writeAtomic(filepath.Join(stage, "PROMPT.md"), []byte(req.Prompt), 0o600); err != nil {
		return Source{}, err
	}
	if err := s.writeMetadataAt(stage, src); err != nil {
		return Source{}, err
	}
	if _, err := imagefile.ParseV2(stage); err != nil {
		return Source{}, err
	}
	if err := os.Rename(stage, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Source{}, fmt.Errorf("%w: %s", ErrExists, req.Name)
		}
		return Source{}, fmt.Errorf("publish image source: %w", err)
	}
	if err := syncDir(s.Root); err != nil {
		return Source{}, err
	}
	return src, nil
}

func (s *Store) List() ([]Source, error) {
	if err := s.ensureRoot(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := make([]Source, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".staging-") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return nil, fmt.Errorf("%w: unexpected entry %s", ErrUnsafeFile, entry.Name())
		}
		src, err := s.Get(entry.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, nil
}

func (s *Store) Get(name string) (Source, error) {
	if err := s.ensureRoot(); err != nil {
		return Source{}, err
	}
	dir, err := s.sourceDir(name)
	if err != nil {
		return Source{}, err
	}
	path := filepath.Join(dir, MetadataFilename)
	data, err := readRegularUTF8(path)
	if errors.Is(err, os.ErrNotExist) {
		return Source{}, fmt.Errorf("%w: metadata for %s", ErrNotFound, name)
	}
	if err != nil {
		return Source{}, err
	}
	var src Source
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&src); err != nil {
		return Source{}, fmt.Errorf("decode %s metadata: %w", name, err)
	}
	if (src.SchemaVersion != 1 && src.SchemaVersion != 2) || src.Name != name {
		return Source{}, fmt.Errorf("%w: inconsistent metadata for %s", ErrUnsafeFile, name)
	}
	return src, nil
}

func (s *Store) Delete(name string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	dir, err := s.sourceDir(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete image source %s: %w", name, err)
	}
	return syncDir(s.Root)
}

// ImportTree validates and atomically publishes a portable source tree.
func (s *Store) ImportTree(name, incoming string) (Source, error) {
	if err := validateName(name); err != nil {
		return Source{}, err
	}
	if err := s.ensureRoot(); err != nil {
		return Source{}, err
	}
	target := filepath.Join(s.Root, name)
	if _, err := os.Lstat(target); err == nil {
		return Source{}, fmt.Errorf("%w: %s", ErrExists, name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Source{}, err
	}
	info, err := os.Lstat(incoming)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Source{}, fmt.Errorf("%w: import root", ErrUnsafeFile)
	}
	stage, err := os.MkdirTemp(s.Root, ".staging-"+name+"-")
	if err != nil {
		return Source{}, err
	}
	defer os.RemoveAll(stage)
	err = safefs.WalkRegular(incoming, safefs.WalkOptions{Dirs: true, MaxFileSize: MaxFileSize}, func(rel string, _ fs.DirEntry, info fs.FileInfo) error {
		if rel == MetadataFilename {
			return fmt.Errorf("%w: %s", ErrInvalidPath, rel)
		}
		destination := filepath.Join(stage, filepath.FromSlash(rel))
		if info.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		data, err := readSourceFile(incoming, rel)
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("%w: %s", ErrInvalidUTF8, rel)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o600)
	})
	if err != nil {
		return Source{}, walkError(err)
	}
	parsed, err := imagefile.ParseV2(stage)
	if err != nil {
		return Source{}, err
	}
	now := store.FormatTime(s.now())
	source := Source{SchemaVersion: parsed.SchemaVersion, Name: name, CreatedAt: now, UpdatedAt: now}
	if err := s.writeMetadataAt(stage, source); err != nil {
		return Source{}, err
	}
	if err := os.Rename(stage, target); err != nil {
		return Source{}, err
	}
	tidy.Log(nil, "sync image source root", syncDir(s.Root), "path", s.Root)
	return source, nil
}

// sourceTarget applies safefs.CleanRel plus the reserved metadata file rule
// and returns the OS-form relative path and its absolute target below root.
func sourceTarget(root, path string) (string, string, error) {
	clean, err := safefs.CleanRel(path)
	if err != nil || clean == MetadataFilename || strings.HasPrefix(clean, MetadataFilename+"/") {
		return "", "", fmt.Errorf("%w: %q", ErrInvalidPath, path)
	}
	rel := filepath.FromSlash(clean)
	return rel, filepath.Join(root, rel), nil
}

// readSourceFile reads the walked regular file root/rel with
// safefs.ReadRegular, so an entry swapped for a symlink or FIFO, or grown,
// after the walk is refused instead of followed, blocked on or read whole.
func readSourceFile(root, rel string) ([]byte, error) {
	data, _, err := safefs.ReadRegular(filepath.Join(root, filepath.FromSlash(rel)), MaxFileSize)
	switch {
	case errors.Is(err, safefs.ErrFileTooLarge):
		return nil, fmt.Errorf("%w: %s", ErrFileTooLarge, rel)
	case errors.Is(err, safefs.ErrUnsafeFile):
		return nil, fmt.Errorf("%w: non-regular file %s", ErrUnsafeFile, rel)
	case errors.Is(err, os.ErrNotExist):
		return nil, err
	case err != nil:
		return nil, fmt.Errorf("%w: %s %v", ErrUnsafeFile, rel, err)
	}
	return data, nil
}

// walkError maps a safefs refusal onto this package's errors and texts.
func walkError(err error) error {
	var pathErr *safefs.PathError
	if !errors.As(err, &pathErr) {
		return err
	}
	switch {
	case errors.Is(err, safefs.ErrUnsafeFile):
		return fmt.Errorf("%w: %s", ErrUnsafeFile, pathErr.Error())
	case errors.Is(err, safefs.ErrFileTooLarge):
		return fmt.Errorf("%w: %s", ErrFileTooLarge, pathErr.Rel)
	}
	return err
}

func (s *Store) ListFiles(name string) ([]FileEntry, error) {
	if err := s.ensureRoot(); err != nil {
		return nil, err
	}
	dir, err := s.sourceDir(name)
	if err != nil {
		return nil, err
	}
	var out []FileEntry
	skipMetadata := func(rel string) bool { return rel == MetadataFilename }
	err = safefs.WalkRegular(dir, safefs.WalkOptions{Skip: skipMetadata, MaxFileSize: MaxFileSize}, func(rel string, _ fs.DirEntry, info fs.FileInfo) error {
		data, err := readSourceFile(dir, rel)
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("%w: %s", ErrInvalidUTF8, rel)
		}
		out = append(out, FileEntry{Path: rel, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, walkError(err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *Store) ReadFile(name, path string) ([]byte, error) {
	if err := s.ensureRoot(); err != nil {
		return nil, err
	}
	dir, err := s.sourceDir(name)
	if err != nil {
		return nil, err
	}
	clean, target, err := sourceTarget(dir, path)
	if err != nil {
		return nil, err
	}
	if err := checkParents(dir, filepath.Dir(clean), false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, name, path)
		}
		return nil, err
	}
	data, err := readRegularUTF8(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, name, path)
	}
	return data, err
}

func (s *Store) WriteFile(name, path string, content []byte) error {
	if len(content) > MaxFileSize {
		return fmt.Errorf("%w: %s is %d bytes", ErrFileTooLarge, path, len(content))
	}
	if !utf8.Valid(content) {
		return fmt.Errorf("%w: %s", ErrInvalidUTF8, path)
	}
	if err := s.ensureRoot(); err != nil {
		return err
	}
	dir, err := s.sourceDir(name)
	if err != nil {
		return err
	}
	clean, target, err := sourceTarget(dir, path)
	if err != nil {
		return err
	}
	if err := checkParents(dir, filepath.Dir(clean), true); err != nil {
		return err
	}
	if st, err := os.Lstat(target); err == nil {
		if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrUnsafeFile, path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.writeAtomic(target, content, 0o600)
}

func (s *Store) RecordBuild(
	name string,
	build func(dir string) (BuildRecord, error),
	rollback func() error,
) (BuildRecord, error) {
	if build == nil {
		return BuildRecord{}, errors.New("image source build callback is nil")
	}
	src, err := s.Get(name)
	if err != nil {
		return BuildRecord{}, err
	}
	dir := filepath.Join(s.Root, name)
	record, err := build(dir)
	if err != nil {
		return BuildRecord{}, err
	}
	if record.Ref == "" || record.Digest == "" || record.BuiltAt == "" {
		return BuildRecord{}, errors.New("image source build returned an incomplete record")
	}
	src.UpdatedAt = store.FormatTime(s.now())
	src.LastBuild = &record
	if err := s.writeMetadataAt(dir, src); err != nil {
		if rollback != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				return BuildRecord{}, errors.Join(
					err,
					fmt.Errorf("roll back built image: %w", rollbackErr),
				)
			}
		}
		return BuildRecord{}, err
	}
	return record, nil
}

func (s *Store) writeMetadataAt(dir string, src Source) error {
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return s.writeAtomic(filepath.Join(dir, MetadataFilename), data, 0o600)
}

func (s *Store) writeAtomic(path string, content []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return fmt.Errorf("create temporary image source file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tidy.Close(nil, tmp)
		tidy.Remove(nil, tmpName)
	}()
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	rename := s.rename
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(tmpName, path); err != nil {
		return fmt.Errorf("publish image source file: %w", err)
	}
	// The rename is the transaction commit point: callers can already observe
	// the new file. A directory fsync only strengthens crash durability; turning
	// its failure into an operation failure would invite callers to roll back a
	// related artifact while the committed metadata remains visible.
	syncDirectory := s.syncDir
	if syncDirectory == nil {
		syncDirectory = syncDir
	}
	tidy.Log(nil, "sync image source directory", syncDirectory(dir), "path", dir)
	return nil
}

func readRegularUTF8(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrUnsafeFile, path)
	}
	if st.Size() > MaxFileSize {
		return nil, fmt.Errorf("%w: %s", ErrFileTooLarge, path)
	}
	data, _, err := safefs.ReadRegular(path, MaxFileSize)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, err
	case errors.Is(err, safefs.ErrFileTooLarge):
		return nil, fmt.Errorf("%w: %s", ErrFileTooLarge, path)
	case err != nil:
		return nil, fmt.Errorf("%w: %s", ErrUnsafeFile, path)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidUTF8, path)
	}
	return data, nil
}

func checkParents(root, rel string, create bool) error {
	if rel == "." || rel == "" {
		return nil
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		st, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && create {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			st, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return fmt.Errorf("%w: parent %s is not a real directory", ErrUnsafeFile, rel)
		}
	}
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return err
	}
	return nil
}
