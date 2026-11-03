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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/internal/refstore"
	"github.com/alekzonder/tariboy/internal/rolefile"
	storetime "github.com/alekzonder/tariboy/internal/store"
)

// Store is an on-disk image store rooted at Dir (paths.ImagesDir()).
//
// Layout:
//
//	<dir>/<name>/refs/<id>.tar.gz   the image content, addressed by ref id
//	<dir>/<name>/tags/<tag>         one line naming the id that tag points at
//
// A tag is only a pointer. Building writes the content under its content
// digest (see contentDigest) and moves the requested tags onto it. Content
// stored under an id is never rewritten. Rebuilding an image_version that
// already exists with other content is refused with VersionConflictError
// unless the caller forces it. Agents and the operator CLI share this one
// mechanism.
type Store struct{ Dir string }

// RefID is the id older releases derived from an image name and its
// image_version. New builds no longer write it; it only recognises those
// legacy archives, whose digest stays this id.
func RefID(name, imageVersion string) string {
	if imageVersion == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(name + "\n" + imageVersion))
	return hex.EncodeToString(sum[:])
}

// refs is the shared name and tag layer under Dir. The content kept at
// refs/<id>.tar.gz is this package's own.
func (s *Store) refs() refstore.Store { return refstore.Store{Dir: s.Dir} }

// checkRef refuses a ref whose name or tag cannot be a path component, before
// anything is staged or published under it.
func checkRef(ref Ref) error {
	if refstore.CheckName(ref.Name) != nil || refstore.CheckName(ref.Tag) != nil {
		return fmt.Errorf("invalid image ref %q", ref.String())
	}
	return nil
}

func (s *Store) contentPath(name, id string) string {
	return filepath.Join(s.refs().RefsDir(name), id+".tar.gz")
}

// Resolve reports the ref id a tag currently points at. A name or tag that
// cannot be a path component cannot exist here and reads as not found.
func (s *Store) Resolve(ref Ref) (string, error) {
	id, err := s.refs().Resolve(ref.Name, ref.Tag)
	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, refstore.ErrNotFound), errors.Is(err, refstore.ErrInvalidName):
		return "", fmt.Errorf("image %s not found", ref.String())
	case errors.Is(err, refstore.ErrInvalidPointer):
		return "", fmt.Errorf("image %s has an invalid tag pointer", ref.String())
	}
	return "", err
}

// archiveFor resolves the tag and returns the path of its content.
func (s *Store) archiveFor(ref Ref) (string, error) {
	id, err := s.Resolve(ref)
	if err != nil {
		return "", err
	}
	path := s.contentPath(ref.Name, id)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("image %s content %s is unavailable", ref.String(), id)
	}
	return path, nil
}

// TagExists reports whether the tag pointer exists, which distinguishes a ref
// that was never built here from one whose content cannot be read. A stat
// failure other than "not exist" is returned rather than reported as absent.
func (s *Store) TagExists(ref Ref) (bool, error) {
	_, err := s.refs().Resolve(ref.Name, ref.Tag)
	switch {
	case err == nil, errors.Is(err, refstore.ErrInvalidPointer):
		return true, nil
	case errors.Is(err, refstore.ErrNotFound), errors.Is(err, refstore.ErrInvalidName):
		return false, nil
	}
	return false, err
}

func (s *Store) Exists(ref Ref) bool {
	_, err := s.archiveFor(ref)
	return err == nil
}

// ArchivePath returns the on-disk file a ref currently points at.
func (s *Store) ArchivePath(ref Ref) (string, error) { return s.archiveFor(ref) }

func (s *Store) ArchiveBytes(ref Ref) ([]byte, error) {
	path, err := s.archiveFor(ref)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// SetTag points one tag at content that is already stored. This is how a build
// publishes several tags — a versioned tag and latest — without building twice.
func (s *Store) SetTag(ref Ref, id string) error {
	if !refstore.IsID(id) {
		return fmt.Errorf("invalid image ref id %q", id)
	}
	if _, err := os.Stat(s.contentPath(ref.Name, id)); err != nil {
		return fmt.Errorf("image %s content %s is unavailable", ref.String(), id)
	}
	return s.refs().WriteTag(ref.Name, ref.Tag, id)
}

// InstallArchive publishes validated archive bytes as the content of their ref
// and moves ref onto it under the image_version rule of publish, unforced.
func (s *Store) InstallArchive(ref Ref, archive []byte) (Manifest, error) {
	manifest, _, err := validatePortableArchive(archive, ref)
	if err != nil {
		return Manifest{}, fmt.Errorf("validate image %s: %w", ref.String(), err)
	}
	return s.installValidated(ref, archive, manifest)
}

// installValidated publishes archive bytes that already passed
// validatePortableArchive for ref, with manifest as its result.
func (s *Store) installValidated(ref Ref, archive []byte, manifest Manifest) (Manifest, error) {
	tmpName, err := s.stageBytes(ref, "install", archive)
	if err != nil {
		return Manifest{}, err
	}
	defer os.Remove(tmpName)
	id, _, err := s.publish(ref, manifest, tmpName, PublishOptions{})
	if err != nil {
		return Manifest{}, err
	}
	manifest.Digest = id
	return manifest, nil
}

// PublishArchiveFile publishes an already staged archive file, checking only
// its manifest identity, and consumes the file. Callers holding untrusted
// bytes use InstallArchive, which applies the full portable contract.
func (s *Store) PublishArchiveFile(ref Ref, path string, opts PublishOptions) (string, error) {
	id, _, err := s.publishArchive(ref, path, nil, opts)
	return id, err
}

// PublishOptions changes how publish treats an image_version that already
// exists with other content.
type PublishOptions struct {
	// Force publishes the new content anyway and moves the tags onto it.
	// Archives of the same version stay on disk under their own digests.
	Force bool
}

// VersionConflictError refuses to publish content under an image_version that
// already exists here with other content.
type VersionConflictError struct {
	Ref     string // name:image_version
	Version string
	Current string // digest of the content this version has now
	New     string // digest of the refused content
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("image %s already exists with digest %s; this build has digest %s. Bump image_version in Tariboyfile.yaml or rebuild with --force.", e.Ref, e.Current, e.New)
}

// publish stores the staged archive tmpName, whose validated manifest is
// manifest, and moves ref onto it. It returns the id ref now points at and,
// for a forced publish, the ids of the same image_version it replaced.
//
// An image_version that already has an archive with the same content reuses
// that archive, legacy ones included, and writes nothing. One with other
// content is a VersionConflictError unless opts.Force is set. An image
// without image_version never conflicts.
func (s *Store) publish(ref Ref, manifest Manifest, tmpName string, opts PublishOptions) (string, []string, error) {
	if err := checkRef(ref); err != nil {
		return "", nil, err
	}
	id := manifest.Digest
	var replaced []string
	if manifest.ImageVersion != "" {
		existing, err := s.versionArchives(ref.Name, manifest.ImageVersion)
		if err != nil {
			return "", nil, err
		}
		content, err := fileContentDigest(tmpName)
		if err != nil {
			return "", nil, fmt.Errorf("digest image %s: %w", ref.String(), err)
		}
		for _, archive := range existing {
			if archive.content == content {
				return archive.id, nil, s.SetTag(ref, archive.id)
			}
		}
		if len(existing) > 0 && !opts.Force {
			return "", nil, &VersionConflictError{Ref: ref.Name + ":" + manifest.ImageVersion, Version: manifest.ImageVersion, Current: s.currentVersionID(ref.Name, manifest.ImageVersion, existing), New: id}
		}
		for _, archive := range existing {
			replaced = append(replaced, archive.id)
		}
	}
	// Only a forced legacy archive can reuse the id of other content: legacy
	// ids name a version, not content. It replaces that file as before.
	overwrite := opts.Force && slices.Contains(replaced, id)
	return id, replaced, s.install(ref, id, tmpName, overwrite)
}

// versionArchive is one stored archive of an image_version: its id and the
// digest of its content, empty when the archive cannot be read.
type versionArchive struct {
	id, content, builtAt string
}

// versionArchives lists the stored archives of name that declare version.
func (s *Store) versionArchives(name, version string) ([]versionArchive, error) {
	entries, err := os.ReadDir(s.refs().RefsDir(name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []versionArchive
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".tar.gz")
		if !ok || !refstore.IsID(id) {
			continue
		}
		path := s.contentPath(name, id)
		var m Manifest
		if body, err := readFileFromTar(path, "manifest.json"); err != nil || json.Unmarshal(body, &m) != nil || m.ImageVersion != version {
			continue
		}
		// An unreadable archive keeps an empty content digest: it matches
		// nothing, so it conflicts rather than being replaced silently.
		content, _ := fileContentDigest(path)
		out = append(out, versionArchive{id: id, content: content, builtAt: m.BuiltAt})
	}
	return out, nil
}

// currentVersionID names the archive a conflict reports as current: the one
// the tag named after the version points at, else the latest built.
func (s *Store) currentVersionID(name, version string, archives []versionArchive) string {
	if id, err := s.refs().Resolve(name, version); err == nil {
		for _, archive := range archives {
			if archive.id == id {
				return id
			}
		}
	}
	current := archives[0]
	for _, archive := range archives[1:] {
		later, laterErr := storetime.ParseInputTime(archive.builtAt)
		earlier, earlierErr := storetime.ParseInputTime(current.builtAt)
		if laterErr == nil && (earlierErr != nil || later.After(earlier)) {
			current = archive
		}
	}
	return current.id
}

func fileContentDigest(path string) (string, error) {
	archive, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	members, _, err := readImageMembers(archive)
	if err != nil {
		return "", err
	}
	return contentDigest(members)
}

// RetagArchive publishes a runnable archive under another image name by
// rewriting only its manifest identity. Payload bytes and their declared order
// are preserved. The archive is read twice: once to parse the source and once
// to verify the rewritten result, which is then published as verified.
func (s *Store) RetagArchive(source, target Ref, archive []byte) (Manifest, error) {
	retagged, manifest, err := RetagArchiveBytes(source, target, archive)
	if err != nil {
		return Manifest{}, err
	}
	return s.installValidated(target, retagged, manifest)
}

func (s *Store) stageBytes(ref Ref, kind string, archive []byte) (string, error) {
	if err := checkRef(ref); err != nil {
		return "", err
	}
	stageDir := s.refs().NameDir(ref.Name)
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(stageDir, "."+kind+"-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if _, err := tmp.Write(archive); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return tmpName, nil
}

// install moves staged content into place and advances ref onto it. Content
// already stored under id stays unless overwrite is set.
func (s *Store) install(ref Ref, id, tmpName string, overwrite bool) error {
	if err := checkRef(ref); err != nil {
		return err
	}
	if !refstore.IsID(id) {
		return fmt.Errorf("invalid image ref id for %s", ref.String())
	}
	contentDir := s.refs().RefsDir(ref.Name)
	if err := os.MkdirAll(contentDir, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(s.contentPath(ref.Name, id)); err == nil && !overwrite {
		return s.SetTag(ref, id)
	}
	if err := os.Rename(tmpName, s.contentPath(ref.Name, id)); err != nil {
		return fmt.Errorf("publish image %s: %w", ref.String(), err)
	}
	if err := refstore.SyncDir(contentDir); err != nil {
		return err
	}
	return s.SetTag(ref, id)
}

// RetagArchiveBytes rewrites manifest identity without touching a store. It
// writes the result from the members parsed during validation and reads the
// archive bytes only to validate the source and the result.
func RetagArchiveBytes(source, target Ref, archive []byte) ([]byte, Manifest, error) {
	_, parsed, err := validatePortableArchive(archive, source)
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("validate imported image: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(parsed.members["manifest.json"].body, &fields); err != nil {
		return nil, Manifest{}, err
	}
	fields["name"] = target.Name
	delete(fields, "tag")
	delete(fields, "digest")
	delete(fields, "id")
	// The new name changes the content, so the archive gets the content id
	// of its new form, whatever id class the source had.
	manifestMember := parsed.members["manifest.json"]
	if manifestMember.body, err = json.MarshalIndent(fields, "", "  "); err != nil {
		return nil, Manifest{}, err
	}
	members := maps.Clone(parsed.members)
	members["manifest.json"] = manifestMember
	if fields["id"], err = contentDigest(members); err != nil {
		return nil, Manifest{}, err
	}
	if manifestMember.body, err = json.MarshalIndent(fields, "", "  "); err != nil {
		return nil, Manifest{}, err
	}
	members["manifest.json"] = manifestMember
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, name := range parsed.order {
		member := members[name]
		if err := writeTarFileMode(tw, name, member.body, member.mode); err != nil {
			return nil, Manifest{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, Manifest{}, err
	}
	if err := gz.Close(); err != nil {
		return nil, Manifest{}, err
	}
	retagged := out.Bytes()
	manifest, _, err := validatePortableArchive(retagged, target)
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("validate retagged image: %w", err)
	}
	return retagged, manifest, nil
}

// Inspect reads manifest.json from the content the tag points at.
func (s *Store) Inspect(ref Ref) (Manifest, error) {
	path, err := s.archiveFor(ref)
	if err != nil {
		return Manifest{}, err
	}
	return inspectArchive(path, ref)
}

// InspectPinned resolves the exact generation assigned to an agent. Content is
// kept by id, so a moved tag never invalidates a pinned assignment.
func (s *Store) InspectPinned(ref Ref, id string) (Manifest, error) {
	manifest, _, err := s.inspectPinnedArchive(ref, id)
	return manifest, err
}

// PinnedManifestBytes returns manifest.json of the archive stored under id as
// it is in the archive. It does not verify the archive; InspectPinned does.
func (s *Store) PinnedManifestBytes(ref Ref, id string) ([]byte, error) {
	if !refstore.IsID(id) {
		return nil, errors.New("invalid pinned image id")
	}
	body, err := readFileFromTar(s.contentPath(ref.Name, id), "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("pinned image %s@%s is unavailable: %w", ref.String(), id, err)
	}
	return body, nil
}

func (s *Store) inspectPinnedArchive(ref Ref, id string) (Manifest, string, error) {
	if !refstore.IsID(id) {
		return Manifest{}, "", errors.New("invalid pinned image id")
	}
	path := s.contentPath(ref.Name, id)
	manifest, err := inspectArchive(path, ref)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("pinned image %s@%s is unavailable: %w", ref.String(), id, err)
	}
	if manifest.Digest != id {
		return Manifest{}, "", fmt.Errorf("image %s id mismatch for %s", ref.String(), id)
	}
	return manifest, path, nil
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
	if m.Name != ref.Name {
		return Manifest{}, fmt.Errorf("archive image %s does not match %s", m.Name, ref.String())
	}
	m.Tag = ref.Tag
	digest, err := archiveDigest(m, func() (map[string]portableImageMember, error) {
		archive, err := os.Open(archivePath)
		if err != nil {
			return nil, err
		}
		defer archive.Close()
		members, _, err := readImageMembers(archive)
		return members, err
	}, func() (string, error) { return fileDigest(archivePath) })
	if err != nil {
		return Manifest{}, fmt.Errorf("digest image %s: %w", ref.String(), err)
	}
	m.Digest = digest
	return m, nil
}

func fileDigest(path string) (string, error) {
	archive, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, archive); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ValidateArchive verifies a staged archive before callers publish it at ref.
// The role is checked first, so a bad role reports itself rather than the
// digest mismatch it also causes.
func ValidateArchive(archivePath string, ref Ref) (Manifest, error) {
	data, err := readFileFromTar(archivePath, "manifest.json")
	if err != nil {
		return Manifest{}, err
	}
	var roleOnly struct {
		Role *rolefile.Role `json:"role"`
	}
	if json.Unmarshal(data, &roleOnly) == nil {
		if err := validatePortableRole(roleOnly.Role, data); err != nil {
			return Manifest{}, err
		}
	}
	return inspectArchive(archivePath, ref)
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

// publishArchive validates staged content and publishes it through publish:
// content is stored under its id, never rewritten, and an image_version that
// exists with other content conflicts unless opts.Force is set. archiveOut
// receives the bytes ref now points at.
func (s *Store) publishArchive(ref Ref, tmpName string, archiveOut *[]byte, opts PublishOptions) (string, []string, error) {
	manifest, err := ValidateArchive(tmpName, ref)
	if err != nil {
		return "", nil, fmt.Errorf("validate image %s: %w", ref.String(), err)
	}
	id, replaced, err := s.publish(ref, manifest, tmpName, opts)
	if err != nil {
		return "", nil, err
	}
	if archiveOut != nil {
		if *archiveOut, err = os.ReadFile(s.contentPath(ref.Name, id)); err != nil {
			return "", nil, err
		}
	}
	return id, replaced, nil
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
