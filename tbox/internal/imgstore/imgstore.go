// Package imgstore keeps built images under $TBOX_HOME/images:
//
//	<Dir>/<name>/refs/<digest>.tar.gz   one archive per content digest
//	<Dir>/<name>/tags/<tag>             the digest a tag points at
//	<Dir>/<name>/source.json            fingerprint of the last build
//
// Writes are atomic (temporary file, fsync, rename). The store does no
// locking.
package imgstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
)

const (
	tmpPrefix     = ".tmp-"
	refSuffix     = ".tar.gz"
	sourceFile    = "source.json"
	statCacheFile = "stat-cache.json"
)

// ErrNotFound marks a tag or ref that does not exist.
var ErrNotFound = errors.New("image not found")

// Store is the image store rooted at Dir.
type Store struct{ Dir string }

// Entry is one tag of the listing.
type Entry struct {
	Name, Tag, Digest, ImageVersion, BuiltAt string
}

func (s Store) nameDir(name string) string { return filepath.Join(s.Dir, name) }
func (s Store) refsDir(name string) string { return filepath.Join(s.Dir, name, "refs") }
func (s Store) tagsDir(name string) string { return filepath.Join(s.Dir, name, "tags") }

func (s Store) refPath(name, digest string) string {
	return filepath.Join(s.refsDir(name), digest+refSuffix)
}

// StatCachePath is the file the build keeps its per-file hash cache in.
func (s Store) StatCachePath(name string) string {
	return filepath.Join(s.nameDir(name), statCacheFile)
}

// writeAtomic writes data to dir/file through a synced temporary file.
func writeAtomic(dir, file string, write func(io.Writer) error) error {
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
	if err := write(tmp); err != nil {
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
	return os.Rename(tmpName, filepath.Join(dir, file))
}

// WriteFileAtomic writes data to path through a synced temporary file in the
// same directory.
func WriteFileAtomic(path string, data []byte) error {
	return writeAtomic(filepath.Dir(path), filepath.Base(path), func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

func (s Store) readTag(name, tag string) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.tagsDir(name), tag))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %s:%s", ErrNotFound, name, tag)
	}
	if err != nil {
		return "", err
	}
	digest := strings.TrimSpace(string(data))
	if !digestPattern.MatchString(digest) {
		return "", fmt.Errorf("tag %s of %s holds %q, not an image digest", tag, name, digest)
	}
	return digest, nil
}

func (s Store) tags(name string) ([]string, error) {
	entries, err := os.ReadDir(s.tagsDir(name))
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

func checkPublish(name string, tags []string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	for _, tag := range tags {
		if err := CheckTag(tag); err != nil {
			return err
		}
	}
	return nil
}

// Publish writes the archive of img when its ref is absent and points every
// tag at it. It returns the tags that pointed at another digest before.
func (s Store) Publish(name string, img archive.Image, tags []string) ([]string, error) {
	if err := checkPublish(name, tags); err != nil {
		return nil, err
	}
	digest := img.Manifest.Digest
	if !digestPattern.MatchString(digest) {
		return nil, fmt.Errorf("image has no content digest")
	}
	if _, err := os.Stat(s.refPath(name, digest)); errors.Is(err, fs.ErrNotExist) {
		if err := writeAtomic(s.refsDir(name), digest+refSuffix, img.Write); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	var moved []string
	for _, tag := range tags {
		old, err := s.readTag(name, tag)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if err == nil && old == digest {
			continue
		}
		if err == nil {
			moved = append(moved, tag)
		}
		err = writeAtomic(s.tagsDir(name), tag, func(w io.Writer) error {
			_, err := io.WriteString(w, digest+"\n")
			return err
		})
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// Resolve returns the digest a ref points at and the path of its archive.
func (s Store) Resolve(ref Ref) (string, string, error) {
	if err := CheckName(ref.Name); err != nil {
		return "", "", err
	}
	if err := CheckTag(ref.Tag); err != nil {
		return "", "", err
	}
	digest, err := s.readTag(ref.Name, ref.Tag)
	if err != nil {
		return "", "", err
	}
	path := s.refPath(ref.Name, digest)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", fmt.Errorf("%w: archive of %s is missing", ErrNotFound, ref)
		}
		return "", "", err
	}
	return digest, path, nil
}

// Open reads and validates the archive a ref points at.
func (s Store) Open(ref Ref) (archive.Image, error) {
	_, path, err := s.Resolve(ref)
	if err != nil {
		return archive.Image{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return archive.Image{}, err
	}
	defer f.Close()
	return archive.Read(f)
}

// List returns one entry per tag, sorted by name and tag. A tag whose archive
// cannot be read is skipped.
func (s Store) List() ([]Entry, error) {
	dirs, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, d := range dirs {
		if !d.IsDir() || CheckName(d.Name()) != nil {
			continue
		}
		tags, err := s.tags(d.Name())
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			img, err := s.Open(Ref{Name: d.Name(), Tag: tag})
			if err != nil {
				continue
			}
			out = append(out, Entry{Name: d.Name(), Tag: tag, Digest: img.Manifest.Digest, ImageVersion: img.Manifest.ImageVersion, BuiltAt: img.Manifest.BuiltAt})
		}
	}
	return out, nil
}

// Remove drops a tag, and the archive it pointed at when no other tag of the
// image points there. When nothing is left of the image its directory goes.
func (s Store) Remove(ref Ref) error {
	digest, err := s.readTag(ref.Name, ref.Tag)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.tagsDir(ref.Name), ref.Tag)); err != nil {
		return err
	}
	tags, err := s.tags(ref.Name)
	if err != nil {
		return err
	}
	for _, tag := range tags {
		if other, err := s.readTag(ref.Name, tag); err == nil && other == digest {
			return nil
		}
	}
	if err := os.Remove(s.refPath(ref.Name, digest)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	s.prune(ref.Name)
	return nil
}

// prune removes the directories of name when no tag and no archive is left.
func (s Store) prune(name string) {
	if tags, _ := s.tags(name); len(tags) > 0 {
		return
	}
	if refs, _ := os.ReadDir(s.refsDir(name)); len(refs) > 0 {
		return
	}
	_ = os.RemoveAll(s.nameDir(name))
}

// ImportArchive validates an archive from r and publishes it under name with
// tags. The image keeps its content digest.
func (s Store) ImportArchive(name string, r io.Reader, tags []string) (archive.Image, error) {
	if err := checkPublish(name, tags); err != nil {
		return archive.Image{}, err
	}
	img, err := archive.Read(r)
	if err != nil {
		return archive.Image{}, err
	}
	if _, err := s.Publish(name, img, tags); err != nil {
		return archive.Image{}, err
	}
	return img, nil
}

type sourceRecord struct {
	Fingerprint string `json:"fingerprint"`
	Digest      string `json:"digest"`
}

// Fingerprint returns the source fingerprint and digest of the last build.
func (s Store) Fingerprint(name string) (fingerprint, digest string, ok bool) {
	if CheckName(name) != nil {
		return "", "", false
	}
	data, err := os.ReadFile(filepath.Join(s.nameDir(name), sourceFile))
	if err != nil {
		return "", "", false
	}
	var rec sourceRecord
	if json.Unmarshal(data, &rec) != nil || rec.Fingerprint == "" {
		return "", "", false
	}
	return rec.Fingerprint, rec.Digest, true
}

// SetFingerprint records the fingerprint of the last build of name.
func (s Store) SetFingerprint(name, fingerprint, digest string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	data, err := json.Marshal(sourceRecord{Fingerprint: fingerprint, Digest: digest})
	if err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(s.nameDir(name), sourceFile), append(data, '\n'))
}
