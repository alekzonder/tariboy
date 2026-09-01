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
		if !e.IsDir() {
			continue
		}
		tags, err := os.ReadDir(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, tf := range tags {
			n := tf.Name()
			if !strings.HasSuffix(n, ".tar.gz") {
				continue
			}
			m, err := s.Inspect(Ref{Name: e.Name(), Tag: strings.TrimSuffix(n, ".tar.gz")})
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
	if !s.Exists(ref) {
		return "", fmt.Errorf("image %s not found", ref.String())
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
	prompt, err := readFileFromTar(s.tarPath(ref), "PROMPT.md")
	if err != nil {
		return "", err
	}
	tail, err := readFileFromTar(s.tarPath(ref), "PROMPT_TAIL.md")
	if err != nil {
		return "", err
	}
	return string(prompt) + "\n" + string(tail), nil
}

// ListFiles enumerates every member of the image's tar.gz (paths like
// manifest.json, PROMPT.md, skills/<name>/...), modelled on the Unpack reader.
func (s *Store) ListFiles(ref Ref) ([]FileEntry, error) {
	if !s.Exists(ref) {
		return nil, fmt.Errorf("image %s not found", ref.String())
	}
	f, err := os.Open(s.tarPath(ref))
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
	if !s.Exists(ref) {
		return nil, fmt.Errorf("image %s not found", ref.String())
	}
	clean, err := cleanTarPath(name)
	if err != nil {
		return nil, err
	}
	return readFileFromTar(s.tarPath(ref), clean)
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

func (s *Store) Remove(ref Ref) error {
	if IsReserved(ref) {
		return fmt.Errorf("%w: %s", ErrReserved, ref.String())
	}
	if !s.Exists(ref) {
		return fmt.Errorf("image %s not found", ref.String())
	}
	if err := os.Remove(s.tarPath(ref)); err != nil {
		return err
	}
	_ = os.Remove(s.digestPath(ref))
	_ = os.Remove(s.mutablePath(ref))
	_ = os.RemoveAll(filepath.Dir(s.pinnedMutablePath(ref, "")))
	_ = os.Remove(s.refDir(ref)) // best-effort: drops the dir when it becomes empty
	return syncDirectory(s.Dir)
}

// RestoreMutable returns a moved ref to one of its retained digests and prior
// mutable state without removing retained mutable generations.
func (s *Store) RestoreMutable(ref Ref, digest string, wasMutable bool) error {
	mutablePublishMu.Lock()
	defer mutablePublishMu.Unlock()
	current, err := s.Inspect(ref)
	if err != nil {
		return err
	}
	if current.Digest != digest {
		history := s.pinnedMutablePath(ref, digest)
		if _, err := ValidateArchive(history, ref); err != nil {
			return fmt.Errorf("restore mutable image %s@%s: %w", ref.String(), digest, err)
		}
		tmp, err := os.CreateTemp(s.refDir(ref), ref.Tag+".restore-*.tmp")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		if err := tmp.Close(); err != nil {
			return err
		}
		defer os.Remove(tmpName)
		if err := os.Remove(tmpName); err != nil {
			return err
		}
		if err := os.Link(history, tmpName); err != nil {
			return err
		}
		if err := os.Rename(tmpName, s.tarPath(ref)); err != nil {
			return err
		}
		if err := syncDirectory(s.refDir(ref)); err != nil {
			return err
		}
		if err := writeDigestCache(s.digestPath(ref), digest); err != nil {
			_ = os.Remove(s.digestPath(ref))
		}
	}
	if !wasMutable {
		_ = os.Remove(s.mutablePath(ref))
		_ = os.RemoveAll(filepath.Dir(s.pinnedMutablePath(ref, "")))
	}
	return nil
}

func (s *Store) Unpack(ref Ref, destDir string) error {
	if !s.Exists(ref) {
		return fmt.Errorf("image %s not found", ref.String())
	}
	return unpackArchive(s.tarPath(ref), ref, destDir)
}

// UnpackPinned materializes the exact immutable digest assigned to an agent.
// For daemon-managed refs this may be a retained pre-upgrade archive rather
// than the current bytes published under the moving reserved ref.
func (s *Store) UnpackPinned(ref Ref, digest, destDir string) error {
	manifest, err := s.InspectPinned(ref, digest)
	if err != nil {
		return err
	}
	archivePath := s.tarPath(ref)
	if current, currentErr := s.Inspect(ref); currentErr != nil || current.Digest != manifest.Digest {
		if IsReserved(ref) {
			archivePath = s.pinnedManagedPath(ref, digest)
		} else {
			archivePath = s.pinnedMutablePath(ref, digest)
		}
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
	return os.WriteFile(filepath.Join(destDir, ".image-digest"), []byte(manifest.Digest+"\n"), 0o600)
}
