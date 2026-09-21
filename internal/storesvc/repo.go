// Package storesvc is the tariboy-store backend: a content-addressed image
// repository, a tiny SQLite catalog/token DB, and a registry HTTP API served
// over mandatory TLS with bearer auth. It reuses the v2 image artifact verbatim
// (internal/image) and the daemon's envelope/auth technique (internal/api).
package storesvc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alekzonder/tariboy/internal/image"
)

// ErrDigestMismatch is returned by Put when the uploaded bytes do not hash to
// the claimed digest (content-addressing integrity, spec §13).
var ErrDigestMismatch = errors.New("digest mismatch")
var ErrInvalidArchive = errors.New("invalid image archive")

// Repo is the image repository on disk. It delegates to internal/image.Store,
// so its layout is exactly the daemon's: content under <Dir>/<name>/refs/<id>
// with a tag pointer per ref. The digest a client claims and receives is the
// sha256 of the transferred archive, which is what detects a tampered or
// truncated transfer; the ref identity is carried inside the archive.
type Repo struct {
	Dir string
	img *image.Store
}

func NewRepo(dir string) *Repo { return &Repo{Dir: dir, img: &image.Store{Dir: dir}} }

// Head reports whether the ref exists and the sha256 of its stored archive.
func (r *Repo) Head(ref image.Ref) (string, bool) {
	path, err := r.img.ArchivePath(ref)
	if err != nil {
		return "", false
	}
	digest, err := fileDigest(path)
	if err != nil {
		return "", false
	}
	return digest, true
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// Put streams body to a temp file, rejects an identity that differs from
// claimed, then publishes it through the image store. ref MUST already be
// image.ParseRef-validated by the caller (the path-traversal guard). A failed
// upload never replaces the stored ref.
func (r *Repo) Put(ref image.Ref, body io.Reader, claimed string) (string, error) {
	if claimed == "" {
		return "", errors.New("missing claimed digest")
	}
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(r.Dir, "."+ref.Name+"-push-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once published
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), body); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if got != claimed {
		return "", fmt.Errorf("%w: computed %s, claimed %s", ErrDigestMismatch, got, claimed)
	}
	if _, err := image.ValidateArchive(tmpName, ref); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	if _, err := r.img.PublishArchiveFile(ref, tmpName); err != nil {
		return "", err
	}
	return got, nil
}

// Open returns a reader over the stored content and its archive digest.
func (r *Repo) Open(ref image.Ref) (io.ReadCloser, string, error) {
	digest, ok := r.Head(ref)
	if !ok {
		return nil, "", fmt.Errorf("image %s not found", ref.String())
	}
	path, err := r.img.ArchivePath(ref)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	return f, digest, nil
}

func (r *Repo) Inspect(ref image.Ref) (image.Manifest, error) { return r.img.Inspect(ref) }
func (r *Repo) List() ([]image.Manifest, error)               { return r.img.List() }
