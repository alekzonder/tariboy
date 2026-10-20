package regclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/alekzonder/tariboy/internal/image"
)

// Push uploads the local archive for ref, skipping the transfer when the store
// already HEADs the same digest.
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

// PushResult is the push output; JSON keys and order match the former map.
type PushResult struct {
	Digest  string `json:"digest"`
	Name    string `json:"name"`
	Pushed  bool   `json:"pushed,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
	Tag     string `json:"tag"`
}

// PullResult is the pull output; JSON keys and order match the former map.
type PullResult struct {
	Digest string `json:"digest"`
	Name   string `json:"name"`
	Pulled bool   `json:"pulled"`
	Tag    string `json:"tag"`
}

func Push(ctx context.Context, imagesDir string, ref image.Ref, c *Client) (*PushResult, error) {
	local := &image.Store{Dir: imagesDir}
	archive, err := local.ArchivePath(ref)
	if err != nil {
		return nil, fmt.Errorf("image %s not found locally (build it first)", ref.String())
	}
	digest, err := fileDigest(archive)
	if err != nil {
		return nil, err
	}
	if have, exists, err := c.Head(ctx, ref); err != nil {
		return nil, err
	} else if exists && have == digest {
		return &PushResult{Name: ref.Name, Tag: ref.Tag, Digest: digest, Skipped: true}, nil
	}
	if err := c.Put(ctx, ref, archive, digest); err != nil {
		return nil, err
	}
	return &PushResult{Name: ref.Name, Tag: ref.Tag, Digest: digest, Pushed: true}, nil
}

// Pull downloads ref, RE-VERIFIES the digest against the server header, then
// installs atomically (temp+rename+sidecar) into the local image store and
// re-Inspects the manifest (schema-version re-check via image.Store.Inspect). A
// tampered/corrupt download can never install (spec §13).
func Pull(ctx context.Context, imagesDir string, ref image.Ref, c *Client) (*PullResult, error) {
	if err := os.MkdirAll(imagesDir, 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(imagesDir, "."+ref.Name+"-pull-*.tmp")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	hasher := sha256.New()
	header, err := c.Get(ctx, ref, io.MultiWriter(tmp, hasher))
	if err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if header == "" {
		return nil, fmt.Errorf("store advertised no digest for %s; refusing to install unverified", ref.String())
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if header != got {
		return nil, fmt.Errorf("digest mismatch on pull: server=%s downloaded=%s (refusing to install)", header, got)
	}
	m, err := image.ValidateArchive(tmpName, ref)
	if err != nil {
		return nil, fmt.Errorf("pulled archive failed inspect (corrupt or unsupported schema): %w", err)
	}
	if _, err := (&image.Store{Dir: imagesDir}).PublishArchiveFile(ref, tmpName); err != nil {
		return nil, err
	}
	return &PullResult{Name: m.Name, Tag: m.Tag, Digest: got, Pulled: true}, nil
}
