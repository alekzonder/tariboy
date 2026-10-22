package image

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/safefs"
	"github.com/alekzonder/tariboy/internal/tidy"

	"github.com/alekzonder/tariboy/internal/refstore"
)

// FileEntry describes one member of an image's tar.gz, enough to render a tree.
type FileEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func (s *Store) List() ([]Manifest, error) {
	names, err := s.refs().List()
	if err != nil {
		return nil, err
	}
	var out []Manifest
	for _, name := range names {
		// Dot directories hold legacy history and staging, not images.
		if strings.HasPrefix(name, ".") {
			continue
		}
		tags, err := s.refs().Tags(name)
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			m, err := s.Inspect(Ref{Name: name, Tag: tag})
			if err != nil {
				return nil, err
			}
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

func (s *Store) RenderPrompt(ref Ref) (string, error) {
	archivePath, err := s.archiveFor(ref)
	if err != nil {
		return "", err
	}
	manifest, err := s.Inspect(ref)
	if err != nil {
		return "", err
	}
	if manifest.SchemaVersion == 2 {
		template, err := s.ReadTemplate(ref)
		if err != nil {
			return "", err
		}
		var parts []string
		for _, entry := range template.Entries {
			if entry.Kind == "runtime" {
				parts = append(parts, "[runtime: "+entry.Runtime+"]")
				continue
			}
			body, err := s.ReadFile(ref, entry.ArchivePath)
			if err != nil {
				return "", err
			}
			parts = append(parts, string(body))
		}
		return strings.Join(parts, "\n\n"), nil
	}
	prompt, err := readFileFromTar(archivePath, "PROMPT.md")
	if err != nil {
		return "", err
	}
	tail, err := readFileFromTar(archivePath, "PROMPT_TAIL.md")
	if err != nil {
		return "", err
	}
	return string(prompt) + "\n" + string(tail), nil
}

// ListFiles enumerates every member of the image's tar.gz (paths like
// manifest.json, PROMPT.md, skills/<name>/...), modelled on the Unpack reader.
func (s *Store) ListFiles(ref Ref) ([]FileEntry, error) {
	archivePath, err := s.archiveFor(ref)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var out []FileEntry
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, FileEntry{
			Path:  path.Clean(h.Name),
			IsDir: h.Typeflag == tar.TypeDir,
			Size:  h.Size,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ReadFile returns the raw content of a single member of the image's tar.gz.
// The path must pass safefs.CleanRel (relative, canonical, no '..' segment)
// before the lookup, so a caller can never escape the archive namespace.
func (s *Store) ReadFile(ref Ref, name string) ([]byte, error) {
	archivePath, err := s.archiveFor(ref)
	if err != nil {
		return nil, err
	}
	clean, err := safefs.CleanRel(name)
	if err != nil {
		return nil, fmt.Errorf("invalid file path %q", name)
	}
	return readFileFromTar(archivePath, clean)
}

// Remove drops one tag. The content it pointed at is deleted once no other tag
// of the same image names it.
func (s *Store) Remove(ref Ref) error {
	if IsReserved(ref) {
		return fmt.Errorf("%w: %s", ErrReserved, ref.String())
	}
	id, err := s.Resolve(ref)
	if err != nil {
		return err
	}
	if err := s.refs().RemoveTag(ref.Name, ref.Tag); err != nil {
		return err
	}
	used, err := s.tagReferences(ref.Name, id)
	if err != nil {
		return err
	}
	if !used {
		if err := os.Remove(s.contentPath(ref.Name, id)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	s.refs().PruneEmpty(ref.Name)
	return refstore.SyncDir(s.Dir)
}

// tagReferences reports whether any tag of name still points at id.
func (s *Store) tagReferences(name, id string) (bool, error) {
	tags, err := s.refs().Tags(name)
	if err != nil {
		return false, err
	}
	for _, tag := range tags {
		other, err := s.Resolve(Ref{Name: name, Tag: tag})
		if err != nil {
			continue
		}
		if other == id {
			return true, nil
		}
	}
	return false, nil
}

// UnpackStagingPrefix starts the names of the sibling directories Unpack
// stages into and parks a replaced destination under. A crash can leave them
// behind next to the destination; owners of the parent remove them.
const UnpackStagingPrefix = ".unpack-"

// Unpack materializes ref into destDir atomically. The image is written to a
// staging directory next to destDir, synced, and renamed into place; an
// existing destDir is moved to a sibling backup first and removed once the new
// tree is in place. On any error the staging directory is removed and destDir
// is left as it was, or absent.
func (s *Store) Unpack(ref Ref, destDir string) error {
	archivePath, err := s.archiveFor(ref)
	if err != nil {
		return err
	}
	return unpackReplacing(archivePath, ref, destDir)
}

func unpackReplacing(archivePath string, ref Ref, destDir string) (err error) {
	parent := filepath.Dir(destDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, UnpackStagingPrefix)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tidy.RemoveAll(stage)
		}
	}()
	if err := unpackArchive(archivePath, ref, stage); err != nil {
		return err
	}
	if err := syncTree(stage); err != nil {
		return err
	}
	backup := filepath.Join(parent, UnpackStagingPrefix+"backup-"+filepath.Base(destDir))
	commit, _, err := safefs.SwapDir(destDir, backup, stage)
	if err != nil {
		return err
	}
	commit()
	return refstore.SyncDir(parent)
}

// syncTree flushes every file and directory under root to disk.
func syncTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		syncErr := f.Sync()
		closeErr := f.Close()
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	})
}

// UnpackPinned materializes the exact generation assigned to an agent, which
// may be an earlier ref id than the one the tag points at today. destDir is
// the caller's own staging directory and is written in place.
func (s *Store) UnpackPinned(ref Ref, id, destDir string) error {
	_, archivePath, err := s.inspectPinnedArchive(ref, id)
	if err != nil {
		return err
	}
	return unpackArchive(archivePath, ref, destDir)
}

// unpackFailAfter, when a test in this package sets it above zero, makes an
// unpack fail with errUnpackInjected once that many members are written.
var unpackFailAfter int

var errUnpackInjected = errors.New("injected unpack failure")

func unpackArchive(archivePath string, ref Ref, destDir string) error {
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	manifest, parsed, err := validatePortableArchive(archive, ref)
	if err != nil {
		return fmt.Errorf("validate image before unpack: %w", err)
	}
	for written, name := range parsed.order {
		if unpackFailAfter > 0 && written >= unpackFailAfter {
			return errUnpackInjected
		}
		member := parsed.members[name]
		if member.typ != tar.TypeReg {
			return fmt.Errorf("invalid image archive member %q: regular files only", name)
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if strings.HasPrefix(name, "skills/") && member.headerMode == 0o700 {
			mode = 0o700
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		if err := out.Chmod(mode); err != nil {
			out.Close()
			return err
		}
		if _, err := out.Write(member.body); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	// The archive carries no tag, so the unpacked tree records the ref it was
	// materialized for alongside its id.
	if err := os.WriteFile(filepath.Join(destDir, ".image-ref"), []byte(ref.String()+"\n"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destDir, ".image-digest"), []byte(manifest.Digest+"\n"), 0o600)
}
