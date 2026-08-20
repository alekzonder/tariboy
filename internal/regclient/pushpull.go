package regclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
)

func tarPath(dir string, ref image.Ref) string {
	return filepath.Join(dir, ref.Name, ref.Tag+".tar.gz")
}
func digestPath(dir string, ref image.Ref) string {
	return filepath.Join(dir, ref.Name, ref.Tag+".digest")
}

// Push uploads the local archive for ref, skipping the transfer when the store
// already HEADs the same digest.
func Push(imagesDir string, ref image.Ref, c *Client) (map[string]any, error) {
	local := &image.Store{Dir: imagesDir}
	if !local.Exists(ref) {
		return nil, fmt.Errorf("image %s not found locally (build it first)", ref.String())
	}
	db, err := os.ReadFile(digestPath(imagesDir, ref))
	if err != nil {
		return nil, fmt.Errorf("read local digest: %w", err)
	}
	digest := strings.TrimSpace(string(db))
	if have, exists, err := c.Head(ref); err != nil {
		return nil, err
	} else if exists && have == digest {
		return map[string]any{"name": ref.Name, "tag": ref.Tag, "digest": digest, "skipped": true}, nil
	}
	if err := c.Put(ref, tarPath(imagesDir, ref), digest); err != nil {
		return nil, err
	}
	return map[string]any{"name": ref.Name, "tag": ref.Tag, "digest": digest, "pushed": true}, nil
}

// Pull downloads ref, RE-VERIFIES the digest against the server header, then
// installs atomically (temp+rename+sidecar) into the local image store and
// re-Inspects the manifest (schema-version re-check via image.Store.Inspect). A
// tampered/corrupt download can never install (spec §13).
func Pull(imagesDir string, ref image.Ref, c *Client) (map[string]any, error) {
	refDir := filepath.Join(imagesDir, ref.Name)
	if err := os.MkdirAll(refDir, 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(refDir, ref.Tag+".*.tmp")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	hasher := sha256.New()
	header, err := c.Get(ref, io.MultiWriter(tmp, hasher))
	if err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if header == "" {
		return nil, fmt.Errorf("store advertised no digest for %s; refusing to install unverified", ref.String())
	}
	if header != got {
		return nil, fmt.Errorf("digest mismatch on pull: server=%s downloaded=%s (refusing to install)", header, got)
	}
	if err := os.Rename(tmpName, tarPath(imagesDir, ref)); err != nil {
		return nil, err
	}
	if err := os.WriteFile(digestPath(imagesDir, ref), []byte(got+"\n"), 0o600); err != nil {
		return nil, err
	}
	local := &image.Store{Dir: imagesDir}
	m, err := local.Inspect(ref)
	if err != nil {
		return nil, fmt.Errorf("pulled archive failed inspect (corrupt or unsupported schema): %w", err)
	}
	return map[string]any{"name": m.Name, "tag": m.Tag, "digest": got, "pulled": true}, nil
}
