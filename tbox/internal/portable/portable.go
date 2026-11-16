// Package portable reads and writes the tariboy-portable envelope that
// carries one image archive between tbox and tariboy (internal/portablearchive
// and internal/imageportable). The envelope is a gzip tar holding
// manifest.json and the single file image.tar.gz.
package portable

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
	"path"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
)

// Limits of portablearchive.DefaultLimits.
const (
	maxCompressed = int64(64 << 20)
	maxExpanded   = int64(256 << 20)
	maxMembers    = 4096
	maxPathBytes  = 512

	imageFile    = "image.tar.gz"
	manifestFile = "manifest.json"
	envelopeKind = "image-artifact"
)

type file struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type envelope struct {
	Format   string          `json:"format"`
	Version  int             `json:"version"`
	Kind     string          `json:"kind"`
	Files    []file          `json:"files"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type metadata struct {
	Ref             string   `json:"ref"`
	Digest          string   `json:"digest"`
	ProducerVersion string   `json:"producer_version"`
	Plugins         []string `json:"plugins"`
}

// Export writes img as an image-artifact envelope the way
// imageportable.Service.Export does. The digest in the metadata is the one
// tariboy records for the archive (archive.TariboyDigest): the content
// digest, except for images built before ids were content digests.
func Export(w io.Writer, img archive.Image, ref imgstore.Ref, producerVersion string) error {
	var body bytes.Buffer
	if err := img.Write(&body); err != nil {
		return err
	}
	sum := sha256.Sum256(body.Bytes())
	plugins := make([]string, 0, len(img.Manifest.Plugins))
	for _, p := range img.Manifest.Plugins {
		plugins = append(plugins, p.Name)
	}
	meta, err := json.Marshal(metadata{Ref: ref.String(), Digest: archive.TariboyDigest(img, body.Bytes()), ProducerVersion: producerVersion, Plugins: plugins})
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(envelope{
		Format: "tariboy-portable", Version: 1, Kind: envelopeKind,
		Files:    []file{{Path: imageFile, SHA256: hex.EncodeToString(sum[:]), Size: int64(body.Len())}},
		Metadata: meta,
	})
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(w)
	gz.Header.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(gz)
	for _, m := range []struct {
		name string
		body []byte
	}{{manifestFile, manifest}, {imageFile, body.Bytes()}} {
		hdr := &tar.Header{Name: m.name, Mode: 0o600, Size: int64(len(m.body)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(m.body); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// Import reads an image-artifact envelope with the limits and checks of
// portablearchive.Stage and imageportable.Service.Preview, and returns the
// inner image with the ref the producer recorded. The metadata digest is
// compared under tariboy's rule (archive.TariboyDigest); the returned image
// keeps its content digest. It touches no file.
func Import(r io.Reader) (archive.Image, imgstore.Ref, error) {
	manifest, body, err := readEnvelope(r)
	if err != nil {
		return archive.Image{}, imgstore.Ref{}, err
	}
	meta, err := checkEnvelope(manifest)
	if err != nil {
		return archive.Image{}, imgstore.Ref{}, err
	}
	ref, err := imgstore.ParseRef(meta.Ref)
	if err != nil || (ref.Name == "bare" && ref.Tag == "latest") {
		return archive.Image{}, imgstore.Ref{}, errors.New("image portable: invalid image ref")
	}
	img, err := archive.Read(bytes.NewReader(body))
	if err != nil {
		return archive.Image{}, imgstore.Ref{}, fmt.Errorf("image portable: invalid runnable artifact: %w", err)
	}
	if img.Manifest.Name != ref.Name {
		return archive.Image{}, imgstore.Ref{}, errors.New("image portable: invalid runnable artifact: name mismatch")
	}
	if archive.TariboyDigest(img, body) != meta.Digest {
		return archive.Image{}, imgstore.Ref{}, errors.New("image portable: image digest mismatch")
	}
	if !samePlugins(img.Manifest.Plugins, meta.Plugins) {
		return archive.Image{}, imgstore.Ref{}, errors.New("image portable: plugin metadata mismatch")
	}
	return img, ref, nil
}

func samePlugins(have []archive.Plugin, want []string) bool {
	names := make([]string, 0, len(have))
	for _, p := range have {
		names = append(names, p.Name)
	}
	return slices.Equal(names, want)
}

// checkEnvelope validates the envelope manifest and returns its metadata.
func checkEnvelope(m envelope) (metadata, error) {
	if m.Kind != envelopeKind || len(m.Files) != 1 || m.Files[0].Path != imageFile {
		return metadata{}, errors.New("image portable: archive is not a runnable image artifact")
	}
	var meta metadata
	err := decodeStrict(m.Metadata, &meta)
	if errors.Is(err, errTrailing) {
		return metadata{}, errors.New("image portable: invalid metadata")
	}
	if err != nil || meta.Ref == "" || meta.Digest == "" || meta.ProducerVersion == "" || meta.Plugins == nil {
		return metadata{}, errors.New("image portable: incomplete metadata")
	}
	return meta, nil
}

var errTrailing = errors.New("trailing data")

// decodeStrict decodes exactly one JSON value and rejects unknown fields.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errTrailing
	}
	return nil
}

// cleanRel is internal/safefs CleanRel as a predicate.
func cleanRel(name string) bool {
	if name == "" || strings.IndexByte(name, 0) >= 0 || strings.ContainsRune(name, '\\') || path.IsAbs(name) {
		return false
	}
	if path.Clean(name) != name || name == "." {
		return false
	}
	return !slices.Contains(strings.Split(name, "/"), "..")
}

// admit applies the member rules of portablearchive Stage to h: a clean
// unique path, a regular file or directory, the member count and the
// expanded size.
func admit(h *tar.Header, seen map[string]bool, expanded *int64) error {
	if len(h.Name) > maxPathBytes || !cleanRel(h.Name) {
		return fmt.Errorf("portable archive: invalid path %q", h.Name)
	}
	if seen[h.Name] {
		return fmt.Errorf("portable archive: duplicate path %s", h.Name)
	}
	seen[h.Name] = true
	if len(seen) > maxMembers {
		return errors.New("portable archive: too many entries")
	}
	if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
		return fmt.Errorf("portable archive: unsupported entry type for %s", h.Name)
	}
	if h.Size < 0 || *expanded > maxExpanded-h.Size {
		return errors.New("portable archive: expanded size exceeded")
	}
	*expanded += h.Size
	return nil
}

// readEnvelope reads the members of an envelope. It returns the manifest and
// the body of image.tar.gz, and checks the manifest file list against every
// regular file read.
func readEnvelope(r io.Reader) (envelope, []byte, error) {
	limited := &io.LimitedReader{R: r, N: maxCompressed + 1}
	gz, err := gzip.NewReader(limited)
	if err != nil {
		return envelope{}, nil, fmt.Errorf("portable archive: gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	written := map[string]file{}
	var manifest envelope
	var image []byte
	manifestSeen := false
	var expanded int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return envelope{}, nil, fmt.Errorf("portable archive: tar: %w", err)
		}
		if err := admit(h, seen, &expanded); err != nil {
			return envelope{}, nil, err
		}
		switch {
		case h.Name == manifestFile:
			if h.Typeflag == tar.TypeDir {
				return envelope{}, nil, errors.New("portable archive: manifest is not a file")
			}
			if manifest, err = readManifest(tr, h); err != nil {
				return envelope{}, nil, err
			}
			manifestSeen = true
		case h.Typeflag == tar.TypeReg:
			f, body, err := readFile(tr, h)
			if err != nil {
				return envelope{}, nil, err
			}
			written[h.Name] = f
			if h.Name == imageFile {
				image = body
			}
		}
	}
	if limited.N <= 0 {
		return envelope{}, nil, errors.New("portable archive: limit exceeded")
	}
	if !manifestSeen {
		return envelope{}, nil, errors.New("portable archive: manifest missing")
	}
	if err := verifyFiles(manifest, written); err != nil {
		return envelope{}, nil, err
	}
	return manifest, image, nil
}

func readManifest(r io.Reader, h *tar.Header) (envelope, error) {
	data, err := io.ReadAll(io.LimitReader(r, h.Size+1))
	if err != nil || int64(len(data)) != h.Size {
		return envelope{}, errors.New("portable archive: invalid manifest body")
	}
	var m envelope
	if err := decodeStrict(data, &m); err != nil {
		return envelope{}, fmt.Errorf("portable archive: manifest: %w", err)
	}
	if m.Format != "tariboy-portable" || m.Version != 1 || m.Kind == "" {
		return envelope{}, errors.New("portable archive: unsupported manifest")
	}
	return m, nil
}

// readFile hashes the body of h and returns it only for image.tar.gz, so a
// large unexpected file costs no memory.
func readFile(r io.Reader, h *tar.Header) (file, []byte, error) {
	hash := sha256.New()
	var keep bytes.Buffer
	var dst io.Writer = hash
	if h.Name == imageFile {
		dst = io.MultiWriter(hash, &keep)
	}
	n, err := io.CopyN(dst, r, h.Size)
	if err != nil || n != h.Size {
		return file{}, nil, errors.New("portable archive: truncated file")
	}
	return file{Path: h.Name, Size: h.Size, SHA256: hex.EncodeToString(hash.Sum(nil))}, keep.Bytes(), nil
}

// verifyFiles requires the manifest's file list to describe exactly the
// files read.
func verifyFiles(m envelope, written map[string]file) error {
	want := map[string]file{}
	for _, f := range m.Files {
		if !cleanRel(f.Path) || len(f.Path) > maxPathBytes || f.Path == manifestFile || want[f.Path].Path != "" {
			return errors.New("portable archive: invalid manifest file list")
		}
		want[f.Path] = f
	}
	if len(want) != len(written) {
		return errors.New("portable archive: manifest does not match files")
	}
	for name, got := range written {
		if exp, ok := want[name]; !ok || got.Size != exp.Size || got.SHA256 != exp.SHA256 {
			return fmt.Errorf("portable archive: file mismatch %s", name)
		}
	}
	return nil
}
