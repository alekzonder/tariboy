package portable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
)

// legacyInner rewrites the manifest of img with edit and returns the inner
// archive bytes and the image read back from them.
func legacyInner(t *testing.T, img archive.Image, edit func(map[string]any)) ([]byte, archive.Image) {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(img.Members[0].Body, &fields); err != nil {
		t.Fatal(err)
	}
	edit(fields)
	body, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	members := append([]archive.Member{}, img.Members...)
	members[0].Body = body
	var buf bytes.Buffer
	if err := (archive.Image{Members: members}).Write(&buf); err != nil {
		t.Fatal(err)
	}
	read, err := archive.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), read
}

// handEnvelope packs inner the way imageportable.Service.Export does, with
// digest as the metadata digest.
func handEnvelope(t *testing.T, inner []byte, ref, digest string) []byte {
	t.Helper()
	sum := sha256.Sum256(inner)
	meta, _ := json.Marshal(metadata{Ref: ref, Digest: digest, ProducerVersion: "0.9.0", Plugins: []string{}})
	manifest, _ := json.Marshal(envelope{Format: "tariboy-portable", Version: 1, Kind: envelopeKind,
		Files: []file{{Path: imageFile, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(inner))}}, Metadata: meta})
	return envelopeBytes(t, []member{{name: manifestFile, body: manifest}, {name: imageFile, body: inner}})
}

// TestImportLegacyIDs imports envelopes of images built before ids were
// content digests. tariboy's export records the digest archiveDigest gives
// them: the sha256 of the archive file without an id, the RefID with a
// legacy id. The imported image keeps its content digest as its tbox
// identity, and a tbox export of it records the tariboy digest again.
func TestImportLegacyIDs(t *testing.T) {
	img := markedImage(t)
	refID := sha256.Sum256([]byte("compat-marked\n0.3.0"))
	cases := map[string]struct {
		edit   func(map[string]any)
		digest func(inner []byte) string
	}{
		"no id": {func(f map[string]any) { delete(f, "id") }, func(inner []byte) string {
			sum := sha256.Sum256(inner)
			return hex.EncodeToString(sum[:])
		}},
		"legacy id": {func(f map[string]any) { f["id"] = hex.EncodeToString(refID[:]) }, func([]byte) string {
			return hex.EncodeToString(refID[:])
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			inner, legacy := legacyInner(t, img, c.edit)
			want := c.digest(inner)
			got, ref, err := Import(bytes.NewReader(handEnvelope(t, inner, "compat-marked:old", want)))
			if err != nil {
				t.Fatal(err)
			}
			if got.Manifest.Digest != legacy.Manifest.Digest || ref.Tag != "old" {
				t.Fatalf("digest %s ref %v", got.Manifest.Digest, ref)
			}
			// The content digest is not what tariboy recorded.
			if _, _, err := Import(bytes.NewReader(handEnvelope(t, inner, "compat-marked:old", legacy.Manifest.Digest))); err == nil {
				t.Fatal("content digest accepted for a legacy archive")
			}
			out := export(t, got, imgstore.Ref{Name: "compat-marked", Tag: "old"})
			var m struct{ Metadata metadata }
			if err := json.Unmarshal(unpack(t, out)[manifestFile], &m); err != nil {
				t.Fatal(err)
			}
			if want := c.digest(unpack(t, out)[imageFile]); m.Metadata.Digest != want {
				t.Fatalf("export digest %s, want %s", m.Metadata.Digest, want)
			}
			if _, _, err := Import(bytes.NewReader(out)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
