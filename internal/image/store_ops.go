package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// FileEntry describes one member of an image's tar.gz, enough to render a tree.
type FileEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func (s *Store) List() ([]Manifest, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		tags, err := os.ReadDir(s.tagsDir(e.Name()))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, tf := range tags {
			if tf.IsDir() || strings.HasPrefix(tf.Name(), ".") {
				continue
			}
			m, err := s.Inspect(Ref{Name: e.Name(), Tag: tf.Name()})
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
// The path is normalised and traversal ('..' / absolute) is rejected before the
// lookup, so a caller can never escape the archive namespace.
func (s *Store) ReadFile(ref Ref, name string) ([]byte, error) {
	archivePath, err := s.archiveFor(ref)
	if err != nil {
		return nil, err
	}
	clean, err := cleanTarPath(name)
	if err != nil {
		return nil, err
	}
	return readFileFromTar(archivePath, clean)
}

// cleanTarPath normalises a slash-separated in-archive path and rejects any
// attempt to traverse out of the archive root (absolute paths or any '..'
// segment), so a caller can never escape the archive namespace.
func cleanTarPath(name string) (string, error) {
	if path.IsAbs(name) {
		return "", fmt.Errorf("invalid file path %q: must be relative", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid file path %q: '..' not allowed", name)
		}
	}
	clean := path.Clean(name)
	if clean == "" || clean == "." {
		return "", fmt.Errorf("invalid file path %q", name)
	}
	return clean, nil
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
	if err := os.Remove(s.tagPath(ref)); err != nil {
		return err
	}
	if err := syncDirectory(s.tagsDir(ref.Name)); err != nil {
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
	// best-effort: drop the directories once they become empty
	_ = os.Remove(s.tagsDir(ref.Name))
	_ = os.Remove(s.refsDir(ref.Name))
	_ = os.Remove(s.nameDir(ref.Name))
	return syncDirectory(s.Dir)
}

// tagReferences reports whether any tag of name still points at id.
func (s *Store) tagReferences(name, id string) (bool, error) {
	entries, err := os.ReadDir(s.tagsDir(name))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		other, err := s.Resolve(Ref{Name: name, Tag: entry.Name()})
		if err != nil {
			continue
		}
		if other == id {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) Unpack(ref Ref, destDir string) error {
	archivePath, err := s.archiveFor(ref)
	if err != nil {
		return err
	}
	return unpackArchive(archivePath, ref, destDir)
}

// UnpackPinned materializes the exact generation assigned to an agent, which
// may be an earlier ref id than the one the tag points at today.
func (s *Store) UnpackPinned(ref Ref, id, destDir string) error {
	_, archivePath, err := s.inspectPinnedArchive(ref, id)
	if err != nil {
		return err
	}
	return unpackArchive(archivePath, ref, destDir)
}

func unpackArchive(archivePath string, ref Ref, destDir string) error {
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	manifest, err := validatePortableArchive(archive, ref)
	if err != nil {
		return fmt.Errorf("validate image before unpack: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(h.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("unsafe path in archive: %s", h.Name)
		}
		target := filepath.Join(destDir, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if strings.HasPrefix(filepath.ToSlash(clean), "skills/") && h.Mode == 0o700 {
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
		if _, err := io.Copy(out, tr); err != nil {
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
