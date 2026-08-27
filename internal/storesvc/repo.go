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
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
)

// ErrDigestMismatch is returned by Put when the uploaded bytes do not hash to
// the claimed digest (content-addressing integrity, spec §13).
var ErrDigestMismatch = errors.New("digest mismatch")
var ErrInvalidArchive = errors.New("invalid image archive")

// Repo is the content-addressed image repository on disk. Its layout is
// byte-identical to internal/image.Store (<Dir>/<name>/<tag>.tar.gz + .digest)
// so an image.Store rooted at the same Dir reads pushed blobs directly.
type Repo struct {
	Dir string
	img *image.Store
}

func NewRepo(dir string) *Repo { return &Repo{Dir: dir, img: &image.Store{Dir: dir}} }

func (r *Repo) refDir(ref image.Ref) string { return filepath.Join(r.Dir, ref.Name) }
func (r *Repo) tarPath(ref image.Ref) string {
	return filepath.Join(r.Dir, ref.Name, ref.Tag+".tar.gz")
}
func (r *Repo) digestPath(ref image.Ref) string {
	return filepath.Join(r.Dir, ref.Name, ref.Tag+".digest")
}

// Head reports whether the blob exists and its stored digest (from the sidecar).
func (r *Repo) Head(ref image.Ref) (string, bool) {
	if !r.img.Exists(ref) {
		return "", false
	}
	b, err := os.ReadFile(r.digestPath(ref))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// Put streams body to a temp file, computes sha256(body), rejects a mismatch
// against claimed, then atomically renames into place and writes the .digest
// sidecar. ref MUST already be image.ParseRef-validated by the caller (the
// path-traversal guard). Mirrors internal/image.Store.writeArchive's
// temp+rename+sidecar discipline.
func (r *Repo) Put(ref image.Ref, body io.Reader, claimed string) (string, error) {
	if claimed == "" {
		return "", errors.New("missing claimed digest")
	}
	if err := os.MkdirAll(r.refDir(ref), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(r.refDir(ref), ref.Tag+".*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
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
	if err := os.Rename(tmpName, r.tarPath(ref)); err != nil {
		return "", err
	}
	if err := os.WriteFile(r.digestPath(ref), []byte(got+"\n"), 0o600); err != nil {
		return "", err
	}
	return got, nil
}

// Open returns a reader over the blob and its stored digest, for GET streaming.
func (r *Repo) Open(ref image.Ref) (io.ReadCloser, string, error) {
	digest, ok := r.Head(ref)
	if !ok {
		return nil, "", fmt.Errorf("image %s not found", ref.String())
	}
	f, err := os.Open(r.tarPath(ref))
	if err != nil {
		return nil, "", err
	}
	return f, digest, nil
}

func (r *Repo) Inspect(ref image.Ref) (image.Manifest, error) { return r.img.Inspect(ref) }
func (r *Repo) List() ([]image.Manifest, error)               { return r.img.List() }
