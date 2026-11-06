package workflowimage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/internal/portablearchive"
	"github.com/alekzonder/tariboy/internal/safefs"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

const archiveKind = "workflow-image"

// archiveMetadata travels in the portable archive manifest. The archive
// format does not keep permission bits, so the executable paths are listed
// here; the digest lets the importer prove it rebuilt the same content.
type archiveMetadata struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Digest     string   `json:"digest"`
	Executable []string `json:"executable"`
}

// Export writes the stored files of the image a tag or digest names as a
// portable archive of kind "workflow-image".
func (s *Store) Export(w io.Writer, name, tagOrDigest string) (Manifest, error) {
	m, err := s.Inspect(name, tagOrDigest)
	if err != nil {
		return Manifest{}, err
	}
	meta := archiveMetadata{Name: m.Name, Version: m.Version, Digest: m.Digest, Executable: []string{}}
	for _, f := range m.Files {
		if f.Executable {
			meta.Executable = append(meta.Executable, f.Path)
		}
	}
	return m, writeArchive(w, s.ContentDir(m.Name, m.Digest), m.Files, meta)
}

func writeArchive(w io.Writer, root string, files []FileEntry, meta archiveMetadata) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return writePortable(w, root, files, archiveKind, raw)
}

func writePortable(w io.Writer, root string, files []FileEntry, kind string, meta json.RawMessage) error {
	list := make([]portablearchive.File, len(files))
	paths := make([]string, len(files))
	for i, f := range files {
		list[i] = portablearchive.File{Path: f.Path, SHA256: f.SHA256, Size: f.Size}
		paths[i] = f.Path
	}
	return portablearchive.Write(w, root, portablearchive.Manifest{
		Format: "tariboy-portable", Version: 1, Kind: kind, Files: list, Metadata: meta,
	}, paths)
}

// Import stages an archive written by Export under stagingRoot and publishes
// it like a build: the version tag and "latest" point at it. Content whose
// digest differs from the one the archive names is refused before anything is
// written. The staged copy is always removed.
func (r *Registry) Import(archive io.Reader, size int64, stagingRoot string, now time.Time) (Manifest, bool, error) {
	src, meta, cleanup, err := stageArchive(archive, size, stagingRoot, "import-")
	if err != nil {
		return Manifest{}, false, err
	}
	defer cleanup()
	return r.published(r.publish(src, meta.Digest, now))
}

// ArchiveInfo is what VerifyArchive proves about a workflow image archive.
type ArchiveInfo struct {
	Name    string
	Version string
	Digest  string
}

// VerifyArchive stages an archive written by Export under stagingRoot,
// recomputes the digest of its content and compares it with the one the
// archive names, as Import does, but writes nothing to a store. The staged
// copy is always removed.
func VerifyArchive(archive io.Reader, size int64, stagingRoot string) (ArchiveInfo, error) {
	src, meta, cleanup, err := stageArchive(archive, size, stagingRoot, "verify-")
	if err != nil {
		return ArchiveInfo{}, err
	}
	defer cleanup()
	_, entries, _, err := prepareSource(src)
	if err != nil {
		return ArchiveInfo{}, err
	}
	if digest := computeDigest(entries); digest != meta.Digest {
		return ArchiveInfo{}, fmt.Errorf("%w: archive names %s, content is %s", ErrDigestMismatch, meta.Digest, digest)
	}
	return ArchiveInfo{Name: src.Name, Version: src.WorkflowVersion, Digest: meta.Digest}, nil
}

// stageArchive unpacks a workflow image archive into a new directory under
// stagingRoot, restores the executable bits it lists and parses its
// Workflowfile. cleanup removes the directory; it is nil on error.
func stageArchive(archive io.Reader, size int64, stagingRoot, prefix string) (*workflowfile.File, archiveMetadata, func(), error) {
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return nil, archiveMetadata{}, nil, err
	}
	tmp, err := os.MkdirTemp(stagingRoot, prefix)
	if err != nil {
		return nil, archiveMetadata{}, nil, err
	}
	src, meta, err := stageInto(archive, size, filepath.Join(tmp, "source"))
	if err != nil {
		os.RemoveAll(tmp)
		return nil, archiveMetadata{}, nil, err
	}
	return src, meta, func() { os.RemoveAll(tmp) }, nil
}

func stageInto(archive io.Reader, size int64, dir string) (*workflowfile.File, archiveMetadata, error) {
	pm, err := portablearchive.Stage(archive, size, dir, portablearchive.DefaultLimits())
	if err != nil {
		return nil, archiveMetadata{}, fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	if pm.Kind != archiveKind {
		return nil, archiveMetadata{}, fmt.Errorf("%w: archive kind is %q", ErrBadArchive, pm.Kind)
	}
	var meta archiveMetadata
	if err := safefs.DecodeStrictJSON(bytes.NewReader(pm.Metadata), &meta); err != nil || !IsDigest(meta.Digest) {
		return nil, archiveMetadata{}, fmt.Errorf("%w: invalid metadata", ErrBadArchive)
	}
	listed := make(map[string]bool, len(pm.Files))
	for _, f := range pm.Files {
		listed[f.Path] = true
	}
	for _, p := range meta.Executable {
		// Stage checked every listed path, so a listed one is safe to join.
		if !listed[p] {
			return nil, archiveMetadata{}, fmt.Errorf("%w: executable %q is not in the archive", ErrBadArchive, p)
		}
		if err := os.Chmod(filepath.Join(dir, filepath.FromSlash(p)), 0o700); err != nil {
			return nil, archiveMetadata{}, err
		}
	}
	src, err := workflowfile.Parse(dir)
	if err != nil {
		return nil, archiveMetadata{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return src, meta, nil
}
