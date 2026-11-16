package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

var updateCompat = flag.Bool("update-compat", false, "regenerate testdata/compat/v3-marked.tar.gz")

var compatDir = filepath.Join("..", "..", "testdata", "compat")

var buildTime = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// buildDir runs the tbox build pipeline on a Tariboyfile directory: load,
// drop what tbox ignores, resolve, prepare and assemble.
func buildDir(t *testing.T, dir, name string, builtAt time.Time) Image {
	t.Helper()
	loaded, err := tfile.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := loaded.ForTbox()
	var skills []skill.Prepared
	for _, entry := range f.Skills {
		abs, source, category, err := tfile.ResolveSkillDir(entry)
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := skill.Prepare(abs, source, category)
		if err != nil {
			t.Fatal(err)
		}
		skills = append(skills, prepared)
	}
	var prompts []PromptLayer
	for _, entry := range f.Prompts {
		abs, source, category, err := tfile.ResolvePromptFile(entry, f.SkillDirs()...)
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		prompts = append(prompts, PromptLayer{Source: source, Category: category, Base: filepath.Base(abs), Body: body})
	}
	role, err := tfile.RoleRead(dir)
	if err != nil {
		t.Fatal(err)
	}
	img, err := Assemble(name, f.ImageVersion, builtAt, skills, prompts, role)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func archiveBytes(t *testing.T, img Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := img.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(compatDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(body))
}

func TestAssembleRoundTrip(t *testing.T) {
	img := buildDir(t, filepath.Join(compatDir, "v2-basic"), "compat-basic", buildTime)
	got, err := Read(bytes.NewReader(archiveBytes(t, img)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Manifest, img.Manifest) {
		t.Fatalf("manifest = %+v, want %+v", got.Manifest, img.Manifest)
	}
	if !reflect.DeepEqual(got.Template, img.Template) {
		t.Fatalf("template = %+v, want %+v", got.Template, img.Template)
	}
	if !reflect.DeepEqual(got.Members, img.Members) {
		t.Fatalf("members differ")
	}
	var names []string
	for _, m := range img.Members {
		names = append(names, m.Name)
	}
	want := []string{"manifest.json", "prompt/template.json", "prompt/layers/000-instructions.md",
		"skills/hello-skill/SKILL.md", "skills/hello-skill/reference.md", "skills/hello-skill/scripts/run.sh"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("members = %v, want %v", names, want)
	}
	if img.Members[5].Mode != 0o700 || img.Members[4].Mode != 0o600 {
		t.Fatalf("modes = %o %o", img.Members[5].Mode, img.Members[4].Mode)
	}
	if img.Manifest.BuiltAt != "2026-11-16T12:00:00.000000000Z" || img.Manifest.Digest != img.Manifest.ID {
		t.Fatalf("manifest = %+v", img.Manifest)
	}
	if img.Manifest.Role == nil || !strings.HasPrefix(img.Manifest.Role.Text, "# Greeter") {
		t.Fatalf("role = %+v", img.Manifest.Role)
	}
	meta, files, ok := got.Skill("hello-skill")
	if !ok || meta.Name != "hello-skill" || len(files) != 3 || files[0].Name != "SKILL.md" {
		t.Fatalf("Skill = %+v %+v %v", meta, files, ok)
	}
	if _, _, ok := got.Skill("missing"); ok {
		t.Fatal("Skill(missing) found")
	}
}

func TestDigestIgnoresBuiltAt(t *testing.T) {
	dir := filepath.Join(compatDir, "v2-basic")
	a := buildDir(t, dir, "compat-basic", buildTime)
	b := buildDir(t, dir, "compat-basic", buildTime.Add(time.Hour))
	if a.Manifest.BuiltAt == b.Manifest.BuiltAt {
		t.Fatal("build times are equal")
	}
	if a.Manifest.ID != b.Manifest.ID {
		t.Fatalf("ids differ: %s %s", a.Manifest.ID, b.Manifest.ID)
	}
}

func TestCompatDigest(t *testing.T) {
	img := buildDir(t, filepath.Join(compatDir, "v2-basic"), "compat-basic", buildTime)
	if want := readFixture(t, "v2-basic.digest"); img.Manifest.ID != want {
		t.Fatalf("id = %s, v2-basic.digest = %s", img.Manifest.ID, want)
	}
}

// TestCompatMarkedArchive keeps the committed v3-marked archive equal to a
// fresh build; -update-compat rewrites it.
func TestCompatMarkedArchive(t *testing.T) {
	img := buildDir(t, filepath.Join(compatDir, "v3-marked"), "compat-marked", buildTime)
	path := filepath.Join(compatDir, "v3-marked.tar.gz")
	if *updateCompat {
		if err := os.WriteFile(path, archiveBytes(t, img), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := Read(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if committed.Manifest.ID != img.Manifest.ID {
		t.Fatalf("committed id = %s, fresh build = %s; run go test ./internal/archive -update-compat", committed.Manifest.ID, img.Manifest.ID)
	}
	if len(img.Manifest.Plugins) != 0 || len(img.Manifest.Skills) != 1 {
		t.Fatalf("manifest = %+v", img.Manifest)
	}
	var layers []string
	for _, entry := range img.Template.Entries {
		if entry.Kind != "file" {
			t.Fatalf("template entry %+v is not a file", entry)
		}
		layers = append(layers, entry.ArchivePath)
	}
	if want := []string{"prompt/layers/000-intro.md", "prompt/layers/001-closing.md"}; !reflect.DeepEqual(layers, want) {
		t.Fatalf("layers = %v, want %v", layers, want)
	}
	for _, m := range img.Members {
		if strings.Contains(m.Name, "tariboy-only") {
			t.Fatalf("member %s is marked only: tariboy", m.Name)
		}
	}
}

// rewrite returns the archive of img with change applied to its members.
func rewrite(t *testing.T, img Image, change func([]Member) []Member) []byte {
	t.Helper()
	members := change(append([]Member(nil), img.Members...))
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		if err := tw.WriteHeader(&tar.Header{Name: m.Name, Mode: m.Mode, Size: int64(len(m.Body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(m.Body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// editManifest decodes manifest.json, applies edit and encodes it again.
func editManifest(t *testing.T, edit func(map[string]any)) func([]Member) []Member {
	return func(members []Member) []Member {
		var fields map[string]any
		if err := json.Unmarshal(members[0].Body, &fields); err != nil {
			t.Fatal(err)
		}
		edit(fields)
		body, err := json.MarshalIndent(fields, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		members[0].Body = body
		return members
	}
}

func TestReadRejects(t *testing.T) {
	img := buildDir(t, filepath.Join(compatDir, "v2-basic"), "compat-basic", buildTime)
	cases := map[string]struct {
		change func([]Member) []Member
		want   string
	}{
		"unknown manifest field": {editManifest(t, func(f map[string]any) { f["extra"] = true }), "unknown field"},
		"extra member": {func(m []Member) []Member {
			return append(m, Member{Name: "x.txt", Mode: 0o600, Body: []byte("x")})
		}, `unexpected schema-v2 image member "x.txt"`},
		"tampered layer": {func(m []Member) []Member {
			m[2].Body = []byte("tampered")
			return m
		}, "layer integrity mismatch"},
		"wrong id":   {editManifest(t, func(f map[string]any) { f["id"] = strings.Repeat("0", 64) }), "id does not match content digest"},
		"missing id": {editManifest(t, func(f map[string]any) { delete(f, "id") }), "id"},
		"bad mode": {func(m []Member) []Member {
			m[1].Mode = 0o644
			return m
		}, "mode"},
		"tampered skill": {func(m []Member) []Member {
			m[4].Body = []byte("tampered")
			return m
		}, "skill"},
		"duplicate member": {func(m []Member) []Member {
			return append(m, m[4])
		}, "duplicate"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Read(bytes.NewReader(rewrite(t, img, tc.change)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestReadAcceptsTariboyExtras reads an archive the way tariboy exports it:
// with plugins and runtime template entries.
func TestReadAcceptsTariboyExtras(t *testing.T) {
	img := buildDir(t, filepath.Join(compatDir, "v2-basic"), "compat-basic", buildTime)
	entries := append([]TemplateEntry{{Kind: "runtime", Runtime: "user-prompt"}}, img.Template.Entries...)
	hash, err := templateHash(entries)
	if err != nil {
		t.Fatal(err)
	}
	template := Template{SchemaVersion: 2, Entries: entries, SHA256: hash}
	templateJSON, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	man := img.Manifest
	man.Plugins = []Plugin{{Name: "whoami"}}
	man.PromptTemplateSHA256 = hash
	man.ID = ""
	members := append([]Member(nil), img.Members...)
	members[1].Body = templateJSON
	if members[0].Body, err = json.MarshalIndent(man, "", "  "); err != nil {
		t.Fatal(err)
	}
	if man.ID, err = ContentDigest(members); err != nil {
		t.Fatal(err)
	}
	if members[0].Body, err = json.MarshalIndent(man, "", "  "); err != nil {
		t.Fatal(err)
	}
	got, err := Read(bytes.NewReader(rewrite(t, Image{Members: members}, func(m []Member) []Member { return m })))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Manifest.Plugins) != 1 || got.Template.Entries[0].Kind != "runtime" {
		t.Fatalf("image = %+v", got)
	}
}
