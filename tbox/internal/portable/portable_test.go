package portable

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
)

var updateCompat = flag.Bool("update-compat", false, "regenerate testdata/compat/v3-marked.tariboy-image.tar.gz")

var compatDir = filepath.Join("..", "..", "testdata", "compat")

func markedImage(t *testing.T) archive.Image {
	t.Helper()
	f, err := os.Open(filepath.Join(compatDir, "v3-marked.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := archive.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

type member struct {
	name string
	body []byte
	typ  byte
	size int64 // declared size when typ is regular and size != 0
}

// envelopeBytes writes a gzip tar of the given members.
func envelopeBytes(t *testing.T, members []member) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		typ := m.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		size := int64(len(m.body))
		if m.size != 0 {
			size = m.size
		}
		if err := tw.WriteHeader(&tar.Header{Name: m.name, Mode: 0o600, Size: size, Typeflag: typ}); err != nil {
			t.Fatal(err)
		}
		if size == int64(len(m.body)) {
			if _, err := tw.Write(m.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if m := members; len(m) > 0 && m[len(m)-1].size != 0 {
		_ = tw.Flush()
	} else if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// unpack returns the members of an exported envelope by name.
func unpack(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err != nil {
			return out
		}
		var b bytes.Buffer
		_, _ = b.ReadFrom(tr)
		out[h.Name] = b.Bytes()
	}
}

func export(t *testing.T, img archive.Image, ref imgstore.Ref) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Export(&buf, img, ref, "tbox-0.0.0"); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// mutate re-packs an export with the manifest changed by edit.
func mutate(t *testing.T, data []byte, edit func(m map[string]any), inner ...[]byte) []byte {
	t.Helper()
	parts := unpack(t, data)
	var m map[string]any
	if err := json.Unmarshal(parts[manifestFile], &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	manifest, _ := json.Marshal(m)
	image := parts[imageFile]
	if len(inner) > 0 {
		image = inner[0]
	}
	return envelopeBytes(t, []member{{name: manifestFile, body: manifest}, {name: imageFile, body: image}})
}

func TestExportImportRoundTrip(t *testing.T) {
	img := markedImage(t)
	ref := imgstore.Ref{Name: "compat-marked", Tag: "latest"}
	data := export(t, img, ref)
	got, gotRef, err := Import(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.Digest != img.Manifest.Digest || gotRef != ref {
		t.Fatalf("digest %s ref %v, want %s %v", got.Manifest.Digest, gotRef, img.Manifest.Digest, ref)
	}
}

func TestExportEnvelopeShape(t *testing.T) {
	img := markedImage(t)
	parts := unpack(t, export(t, img, imgstore.Ref{Name: "a", Tag: "b"}))
	if len(parts) != 2 {
		t.Fatalf("members = %d", len(parts))
	}
	var m struct {
		Format, Kind string
		Version      int
		Files        []struct{ Path string }
		Metadata     map[string]any
	}
	if err := json.Unmarshal(parts[manifestFile], &m); err != nil {
		t.Fatal(err)
	}
	if m.Format != "tariboy-portable" || m.Version != 1 || m.Kind != "image-artifact" || len(m.Files) != 1 || m.Files[0].Path != "image.tar.gz" {
		t.Fatalf("manifest %+v", m)
	}
	if m.Metadata["ref"] != "a:b" || m.Metadata["digest"] != img.Manifest.Digest || m.Metadata["producer_version"] != "tbox-0.0.0" {
		t.Fatalf("metadata %v", m.Metadata)
	}
	if p, ok := m.Metadata["plugins"].([]any); !ok || len(p) != 0 {
		t.Fatalf("plugins %#v, want empty array", m.Metadata["plugins"])
	}
}

func TestImportRejects(t *testing.T) {
	img := markedImage(t)
	data := export(t, img, imgstore.Ref{Name: "compat-marked", Tag: "latest"})
	parts := unpack(t, data)
	setMeta := func(key string, v any) func(map[string]any) {
		return func(m map[string]any) { m["metadata"].(map[string]any)[key] = v }
	}
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"wrong format", mutate(t, data, func(m map[string]any) { m["format"] = "other" }), "unsupported manifest"},
		{"wrong kind", mutate(t, data, func(m map[string]any) { m["kind"] = "workflow" }), "not a runnable image artifact"},
		{"digest differs", mutate(t, data, setMeta("digest", strings.Repeat("0", 64))), "digest mismatch"},
		{"plugins differ", mutate(t, data, setMeta("plugins", []string{"x"})), "plugin metadata mismatch"},
		{"plugins missing", mutate(t, data, setMeta("plugins", nil)), "incomplete metadata"},
		{"ref name differs from archive name", mutate(t, data, setMeta("ref", "victim:latest")), "name mismatch"},
		{"inner archive invalid", garbageInner(t, data), "invalid runnable artifact"},
		{"bad ref", mutate(t, data, setMeta("ref", "Bad Ref")), "invalid image ref"},
		{"reserved ref", mutate(t, data, setMeta("ref", "bare:latest")), "invalid image ref"},
		{"file hash differs", mutate(t, data, func(m map[string]any) {
			m["files"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
		}), "file mismatch"},
		{"second file", envelopeBytes(t, []member{{name: manifestFile, body: parts[manifestFile]}, {name: imageFile, body: parts[imageFile]}, {name: "extra.txt", body: []byte("x")}}), "manifest does not match files"},
		{"no manifest", envelopeBytes(t, []member{{name: imageFile, body: parts[imageFile]}}), "manifest missing"},
		{"traversal", envelopeBytes(t, []member{{name: "../evil", body: []byte("x")}}), "invalid path"},
		{"absolute", envelopeBytes(t, []member{{name: "/evil", body: []byte("x")}}), "invalid path"},
		{"duplicate", envelopeBytes(t, []member{{name: imageFile, body: []byte("a")}, {name: imageFile, body: []byte("b")}}), "duplicate path"},
		{"symlink", envelopeBytes(t, []member{{name: "link", typ: tar.TypeSymlink}}), "unsupported entry type"},
		{"hardlink", envelopeBytes(t, []member{{name: "link", typ: tar.TypeLink}}), "unsupported entry type"},
		{"device", envelopeBytes(t, []member{{name: "dev", typ: tar.TypeChar}}), "unsupported entry type"},
		{"manifest directory", envelopeBytes(t, []member{{name: manifestFile, typ: tar.TypeDir}}), "manifest is not a file"},
		{"inner dot-dot", envelopeBytes(t, []member{{name: "a/../../evil", body: []byte("x")}}), "invalid path"},
		{"long path", envelopeBytes(t, []member{{name: strings.Repeat("a", 513), body: []byte("x")}}), "invalid path"},
		{"oversized declared size", envelopeBytes(t, []member{{name: imageFile, body: []byte("x"), size: maxExpanded + 1}}), "expanded size exceeded"},
		{"not gzip", []byte("plain"), "gzip"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := Import(bytes.NewReader(c.data))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

// garbageInner re-packs an export with a valid envelope whose image.tar.gz is
// not an image archive, with the file list describing it correctly.
func garbageInner(t *testing.T, data []byte) []byte {
	t.Helper()
	body := []byte("not an image archive")
	sum := sha256.Sum256(body)
	return mutate(t, data, func(m map[string]any) {
		f := m["files"].([]any)[0].(map[string]any)
		f["sha256"] = hex.EncodeToString(sum[:])
		f["size"] = len(body)
	}, body)
}

func TestImportRejectsTooManyMembers(t *testing.T) {
	members := make([]member, 0, maxMembers+1)
	for i := 0; i <= maxMembers; i++ {
		members = append(members, member{name: "d" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "/" + itoa(i), body: nil})
	}
	_, _, err := Import(bytes.NewReader(envelopeBytes(t, members)))
	if err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("err = %v", err)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// TestCompatPortableArchive keeps the committed envelope importable and equal
// in content to the v3-marked archive; -update-compat rewrites it.
func TestCompatPortableArchive(t *testing.T) {
	img := markedImage(t)
	path := filepath.Join(compatDir, "v3-marked.tariboy-image.tar.gz")
	if *updateCompat {
		data := export(t, img, imgstore.Ref{Name: "compat-marked", Tag: "latest"})
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, ref, err := Import(f)
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.Digest != img.Manifest.Digest || ref.String() != "compat-marked:latest" {
		t.Fatalf("digest %s ref %s; run go test ./internal/portable -run TestCompatPortableArchive -update-compat", got.Manifest.Digest, ref)
	}
}
