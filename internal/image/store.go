package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Store is an on-disk image store rooted at Dir (paths.ImagesDir()).
type Store struct{ Dir string }

// ponytail: global lock, use per-ref locks if mutable image publication becomes a bottleneck.
var mutablePublishMu sync.Mutex

// ponytail: global lock, use per-ref locks if controlled image publication becomes a bottleneck.
var publicationGate sync.Mutex

// WithPublicationGate prevents ordinary mutable authoring from racing a
// controlled release's immutable archive and release record.
func WithPublicationGate(fn func() error) error {
	publicationGate.Lock()
	defer publicationGate.Unlock()
	return fn()
}

func (s *Store) refDir(ref Ref) string     { return filepath.Join(s.Dir, ref.Name) }
func (s *Store) tarPath(ref Ref) string    { return filepath.Join(s.Dir, ref.Name, ref.Tag+".tar.gz") }
func (s *Store) digestPath(ref Ref) string { return filepath.Join(s.Dir, ref.Name, ref.Tag+".digest") }
func (s *Store) mutablePath(ref Ref) string {
	return filepath.Join(s.Dir, ref.Name, ref.Tag+".mutable")
}
func (s *Store) pinnedManagedPath(ref Ref, digest string) string {
	return filepath.Join(s.Dir, ".managed", ref.Name, ref.Tag, digest+".tar.gz")
}
func (s *Store) pinnedMutablePath(ref Ref, digest string) string {
	return filepath.Join(s.Dir, ".mutable", ref.Name, ref.Tag, digest+".tar.gz")
}

func (s *Store) Exists(ref Ref) bool {
	_, err := os.Stat(s.tarPath(ref))
	return err == nil
}

// IsMutable reports whether ref was published through the mutable authoring path.
func (s *Store) IsMutable(ref Ref) bool {
	info, err := os.Stat(s.mutablePath(ref))
	return err == nil && info.Mode().IsRegular()
}

func (s *Store) ArchiveBytes(ref Ref) ([]byte, error) {
	if !s.Exists(ref) {
		return nil, fmt.Errorf("image %s not found", ref.String())
	}
	return os.ReadFile(s.tarPath(ref))
}

// InstallPortableArchive publishes a validated runnable archive without
// reconstructing it from source files.
func (s *Store) InstallPortableArchive(ref Ref, archive []byte) error {
	if IsReserved(ref) {
		return fmt.Errorf("portable install cannot replace reserved ref: %s", ref.String())
	}
	manifest, err := validatePortableArchive(archive, ref)
	if err != nil {
		return fmt.Errorf("validate imported image: %w", err)
	}
	if err := os.MkdirAll(s.refDir(ref), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.refDir(ref), ref.Tag+".import-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(archive); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(name, s.tarPath(ref)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrExists, ref.String())
		}
		return err
	}
	if err := writeDigestCache(s.digestPath(ref), manifest.Digest); err != nil {
		_ = os.Remove(s.digestPath(ref))
	}
	return nil
}

// RetagPortableArchive installs a runnable archive under a different immutable
// ref by rewriting only manifest identity. Prompt/plugin payload bytes and their
// declared order are preserved; the resulting archive has its own digest.
func (s *Store) RetagPortableArchive(source, target Ref, archive []byte) (string, error) {
	if IsReserved(target) {
		return "", fmt.Errorf("portable install cannot replace reserved ref: %s", target.String())
	}
	if _, err := validatePortableArchive(archive, source); err != nil {
		return "", fmt.Errorf("validate imported image: %w", err)
	}
	if err := os.MkdirAll(s.refDir(target), 0o700); err != nil {
		return "", err
	}
	sourceFile, err := os.CreateTemp(s.refDir(target), target.Tag+".source-*.tmp")
	if err != nil {
		return "", err
	}
	sourceName := sourceFile.Name()
	defer os.Remove(sourceName)
	defer sourceFile.Close()
	if _, err := sourceFile.Write(archive); err != nil {
		sourceFile.Close()
		return "", err
	}
	if err := sourceFile.Close(); err != nil {
		return "", err
	}
	in, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(s.refDir(target), target.Tag+".retag-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()
	hasher := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(tmp, hasher))
	tw := tar.NewWriter(gz)
	tr := tar.NewReader(in)
	seenManifest := false
	for {
		header, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return "", nextErr
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return "", fmt.Errorf("invalid image archive member %q", header.Name)
		}
		body, readErr := io.ReadAll(tr)
		if readErr != nil {
			return "", readErr
		}
		if header.Name == "manifest.json" {
			if seenManifest {
				return "", errors.New("duplicate image manifest")
			}
			seenManifest = true
			var fields map[string]any
			if err := json.Unmarshal(body, &fields); err != nil {
				return "", err
			}
			fields["name"] = target.Name
			fields["tag"] = target.Tag
			delete(fields, "digest")
			body, err = json.MarshalIndent(fields, "", "  ")
			if err != nil {
				return "", err
			}
		}
		if err := writeTarFileMode(tw, header.Name, body, header.Mode&0o7777); err != nil {
			return "", err
		}
	}
	if !seenManifest {
		return "", errors.New("image manifest is missing")
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	retagged, err := os.ReadFile(tmpName)
	if err != nil {
		return "", err
	}
	if _, err := validatePortableArchive(retagged, target); err != nil {
		return "", fmt.Errorf("validate retagged image: %w", err)
	}
	if err := os.Link(tmpName, s.tarPath(target)); err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, inspectErr := s.Inspect(target)
			if inspectErr == nil && existing.Digest == digest {
				return digest, nil
			}
			return "", fmt.Errorf("%w: %s", ErrExists, target.String())
		}
		return "", err
	}
	if err := writeDigestCache(s.digestPath(target), digest); err != nil {
		_ = os.Remove(s.digestPath(target))
	}
	return digest, nil
}

// Inspect reads manifest.json from the archive and derives Digest from the
// immutable archive itself. The sidecar is only an optimization/diagnostic
// artifact and is never trusted as the source of truth.
func (s *Store) Inspect(ref Ref) (Manifest, error) {
	if !s.Exists(ref) {
		return Manifest{}, fmt.Errorf("image %s not found", ref.String())
	}
	return inspectArchive(s.tarPath(ref), ref)
}

// InspectPinned resolves the immutable archive identity assigned to an agent.
// Ordinary refs are immutable and therefore must still match the current
// archive. Daemon-managed refs may advance during an upgrade, so their prior
// validated bytes are retained by digest and remain available to existing
// agents until those agents are explicitly assigned another image.
func (s *Store) InspectPinned(ref Ref, digest string) (Manifest, error) {
	if len(digest) != sha256.Size*2 {
		return Manifest{}, errors.New("invalid pinned image digest")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return Manifest{}, errors.New("invalid pinned image digest")
	}
	if current, err := s.Inspect(ref); err == nil && current.Digest == digest {
		return current, nil
	}
	if !IsReserved(ref) && !s.IsMutable(ref) {
		return Manifest{}, fmt.Errorf("image %s digest does not match pinned identity", ref.String())
	}
	historyPath := s.pinnedManagedPath(ref, digest)
	kind := "managed"
	if !IsReserved(ref) {
		historyPath = s.pinnedMutablePath(ref, digest)
		kind = "mutable"
	}
	pinned, err := inspectArchive(historyPath, ref)
	if err != nil {
		return Manifest{}, fmt.Errorf("pinned %s image %s@%s is unavailable: %w", kind, ref.String(), digest, err)
	}
	if pinned.Digest != digest {
		return Manifest{}, fmt.Errorf("%s image %s history digest mismatch", kind, ref.String())
	}
	return pinned, nil
}

func inspectArchive(archivePath string, ref Ref) (Manifest, error) {
	data, err := readFileFromTar(archivePath, "manifest.json")
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest of %s: %w", ref.String(), err)
	}
	if m.SchemaVersion > ManifestSchemaVersion {
		return Manifest{}, fmt.Errorf("image %s manifest schema_version %d is newer than supported %d", ref.String(), m.SchemaVersion, ManifestSchemaVersion)
	}
	if m.Name != ref.Name || m.Tag != ref.Tag {
		return Manifest{}, fmt.Errorf("archive ref %s:%s does not match %s", m.Name, m.Tag, ref.String())
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return Manifest{}, err
	}
	defer archive.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, archive); err != nil {
		return Manifest{}, fmt.Errorf("digest image %s: %w", ref.String(), err)
	}
	m.Digest = hex.EncodeToString(hasher.Sum(nil))
	return m, nil
}

// ValidateArchive verifies a staged archive before callers publish it at ref.
func ValidateArchive(archivePath string, ref Ref) (Manifest, error) {
	return inspectArchive(archivePath, ref)
}

// InstallManagedArchive atomically replaces one daemon-managed ref with a
// validated complete archive. Public authoring does not call this trusted path.
func (s *Store) InstallManagedArchive(ref Ref, archive []byte) error {
	if !IsReserved(ref) {
		return fmt.Errorf("managed install requires reserved ref: %s", ref.String())
	}
	if err := os.MkdirAll(s.refDir(ref), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.refDir(ref), ref.Tag+".managed-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(archive); err != nil {
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
	manifest, err := inspectArchive(tmpName, ref)
	if err != nil {
		return fmt.Errorf("validate managed image %s: %w", ref.String(), err)
	}
	if s.Exists(ref) {
		current, err := s.Inspect(ref)
		if err != nil {
			return fmt.Errorf("inspect current managed image %s: %w", ref.String(), err)
		}
		historyPath := s.pinnedManagedPath(ref, current.Digest)
		if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
			return err
		}
		if err := os.Link(s.tarPath(ref), historyPath); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("preserve managed image %s@%s: %w", ref.String(), current.Digest, err)
		}
	}
	if err := os.Rename(tmpName, s.tarPath(ref)); err != nil {
		return fmt.Errorf("publish managed image %s: %w", ref.String(), err)
	}
	if err := writeDigestCache(s.digestPath(ref), manifest.Digest); err != nil {
		_ = os.Remove(s.digestPath(ref))
	}
	return nil
}

func (s *Store) ReadBody(ref Ref) (string, error) {
	b, err := readFileFromTar(s.tarPath(ref), "BODY.md")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func readFileFromTar(archive, want string) ([]byte, error) {
	f, err := os.Open(archive)
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
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Name == want {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%s not found in %s", want, archive)
}

// writeArchive builds the tar.gz in a temporary file. publishArchive keeps the
// immutable no-clobber default and optionally advances a marked mutable ref.
func (s *Store) writeArchive(ref Ref, man Manifest, prompt, tail, body string, skillDirs []string, mutable bool) (string, error) {
	if err := os.MkdirAll(s.refDir(ref), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.refDir(ref), ref.Tag+".*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)

	manJSON, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		tmp.Close()
		return "", err
	}
	members := []struct {
		name string
		data []byte
	}{
		{"manifest.json", manJSON},
		{"PROMPT.md", []byte(prompt)},
		{"PROMPT_TAIL.md", []byte(tail)},
		{"BODY.md", []byte(body)},
	}
	for _, m := range members {
		if err := writeTarFile(tw, m.name, m.data); err != nil {
			tmp.Close()
			return "", err
		}
	}
	for _, sd := range skillDirs {
		base := filepath.Base(sd)
		err := filepath.Walk(sd, func(path string, info os.FileInfo, werr error) error {
			if werr != nil {
				return werr
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(sd, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return writeTarFile(tw, filepath.ToSlash(filepath.Join("skills", base, rel)), data)
		})
		if err != nil {
			tmp.Close()
			return "", err
		}
	}
	if err := tw.Close(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return s.publishArchive(ref, tmpName, mutable)
}

func (s *Store) publishArchive(ref Ref, tmpName string, mutable bool) (string, error) {
	if mutable && IsReserved(ref) {
		return "", fmt.Errorf("%w: %s", ErrReserved, ref.String())
	}
	manifest, err := ValidateArchive(tmpName, ref)
	if err != nil {
		return "", fmt.Errorf("validate image %s: %w", ref.String(), err)
	}
	if !mutable {
		if err := os.Link(tmpName, s.tarPath(ref)); err != nil {
			if errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("%w: %s", ErrExists, ref.String())
			}
			return "", fmt.Errorf("publish image %s: %w", ref.String(), err)
		}
	} else {
		mutablePublishMu.Lock()
		defer mutablePublishMu.Unlock()
		markerCreated := false
		if !s.IsMutable(ref) {
			if err := writeMutableMarker(s.mutablePath(ref)); err != nil {
				return "", fmt.Errorf("mark mutable image %s: %w", ref.String(), err)
			}
			markerCreated = true
		}
		defer func() {
			if markerCreated {
				_ = os.Remove(s.mutablePath(ref))
			}
		}()
		if s.Exists(ref) {
			current, err := s.Inspect(ref)
			if err != nil {
				return "", fmt.Errorf("inspect current mutable image %s: %w", ref.String(), err)
			}
			historyPath := s.pinnedMutablePath(ref, current.Digest)
			if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
				return "", err
			}
			if err := os.Link(s.tarPath(ref), historyPath); err != nil && !errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("preserve mutable image %s@%s: %w", ref.String(), current.Digest, err)
			}
		}
		if err := os.Rename(tmpName, s.tarPath(ref)); err != nil {
			return "", fmt.Errorf("publish mutable image %s: %w", ref.String(), err)
		}
		markerCreated = false
	}
	if err := writeDigestCache(s.digestPath(ref), manifest.Digest); err != nil {
		_ = os.Remove(s.digestPath(ref))
	}
	return manifest.Digest, nil
}

func writeMutableMarker(path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mutable-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, "mutable\n"); err != nil {
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
	return os.Rename(tmpName, path)
}

func writeDigestCache(path, digest string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".digest-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, digest+"\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func writeTarFile(tw *tar.Writer, name string, data []byte) error {
	return writeTarFileMode(tw, name, data, 0o600)
}

func writeTarFileMode(tw *tar.Writer, name string, data []byte, mode int64) error {
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg,
	}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
