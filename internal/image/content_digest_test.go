package image

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// skillSource writes a one-skill schema-v2 source that declares version.
func skillSource(t *testing.T, version string) (*imagefile.V2, string) {
	t.Helper()
	source := t.TempDir()
	skillDir := writeTestSkill(t, filepath.Join(source, "skills"), "digest-skill")
	return &imagefile.V2{SchemaVersion: 2, ImageVersion: version, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/digest-skill"}}}, skillDir
}

// legacyArchive rewrites the manifest id the way older releases wrote it:
// RefID(name, image_version) when the image declares a version, else no id.
func legacyArchive(t *testing.T, archive []byte) []byte {
	t.Helper()
	return rewriteArchiveMember(t, archive, "manifest.json", func(body []byte) []byte {
		var fields map[string]any
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		version, _ := fields["image_version"].(string)
		if id := RefID(fields["name"].(string), version); id != "" {
			fields["id"] = id
		} else {
			delete(fields, "id")
		}
		out, err := json.MarshalIndent(fields, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return out
	})
}

func clockAt(sec int64) func() time.Time { return func() time.Time { return time.Unix(sec, 0) } }

func TestBuildV2VersionedSkillBytesChangeDigest(t *testing.T) {
	src, skillDir := skillSource(t, "1.0.0")
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, &Store{Dir: t.TempDir()}, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := BuildV2(src, imagefile.ResolveRoots{}, ref, &Store{Dir: t.TempDir()}, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatalf("skill bytes changed but digest stayed %s", first.Digest)
	}
}

func TestBuildV2VersionedSkillDescriptionChangesDigest(t *testing.T) {
	src, skillDir := skillSource(t, "1.0.0")
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, &Store{Dir: t.TempDir()}, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	body := "---\nname: digest-skill\ndescription: Use this skill for other image tests.\n---\n# Skill\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := BuildV2(src, imagefile.ResolveRoots{}, ref, &Store{Dir: t.TempDir()}, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatalf("skill description changed but digest stayed %s", first.Digest)
	}
}

func TestBuildV2SameContentOtherBuiltAtKeepsDigestAndArchive(t *testing.T) {
	src, _ := skillSource(t, "1.0.0")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	path := store.contentPath(ref.Name, first.Digest)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("same content got digests %s and %s", first.Digest, second.Digest)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Sys().(*syscall.Stat_t).Ino != after.Sys().(*syscall.Stat_t).Ino || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("archive of the same content was rewritten")
	}
	inspected, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.BuiltAt != first.BuiltAt {
		t.Fatalf("stored built_at = %s, want the first build %s", inspected.BuiltAt, first.BuiltAt)
	}
}

func TestBuildV2WritesContentDigestAsID(t *testing.T) {
	src, _ := skillSource(t, "1.0.0")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	built, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if built.Digest == RefID(ref.Name, "1.0.0") {
		t.Fatal("digest is still derived from name and version")
	}
	body, err := readFileFromTar(store.contentPath(ref.Name, built.Digest), "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.ID != built.Digest {
		t.Fatalf("manifest id = %q, want %q", stored.ID, built.Digest)
	}
}

// storeRaw writes archive bytes under id and points ref at them, the way an
// older release left them on disk.
func storeRaw(t *testing.T, store *Store, ref Ref, id string, archive []byte) {
	t.Helper()
	if err := os.MkdirAll(store.refs().RefsDir(ref.Name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.contentPath(ref.Name, id), archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTag(ref, id); err != nil {
		t.Fatal(err)
	}
}

func builtArchive(t *testing.T, src *imagefile.V2, ref Ref) []byte {
	t.Helper()
	staging := &Store{Dir: t.TempDir()}
	if _, err := BuildV2(src, imagefile.ResolveRoots{}, ref, staging, clockAt(1), nil); err != nil {
		t.Fatal(err)
	}
	archive, err := staging.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestInspectRejectsSubstitutedMemberUnderContentID(t *testing.T) {
	src, _ := skillSource(t, "1.0.0")
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	store := &Store{Dir: t.TempDir()}
	built, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	tampered := rewriteArchiveMember(t, archive, "skills/digest-skill/scripts/check.sh", func([]byte) []byte { return []byte("#!/bin/sh\nexit 7\n") })
	if err := os.WriteFile(store.contentPath(ref.Name, built.Digest), tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect(ref); err == nil || !strings.Contains(err.Error(), "does not match content digest") {
		t.Fatalf("Inspect err = %v, want content digest mismatch", err)
	}
	if _, err := store.InspectPinned(ref, built.Digest); err == nil {
		t.Fatal("InspectPinned accepted a substituted member")
	}
}

func TestValidatePortableArchiveRejectsChangedManifestUnderContentID(t *testing.T) {
	src, _ := skillSource(t, "1.0.0")
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	tampered := rewriteArchiveMember(t, builtArchive(t, src, ref), "manifest.json", func(body []byte) []byte {
		var fields map[string]any
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		fields["plugins"] = []any{map[string]any{"name": "extra"}}
		out, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return out
	})
	if _, err := ValidatePortableArchive(tampered, ref); err == nil || !strings.Contains(err.Error(), "does not match content digest") {
		t.Fatalf("ValidatePortableArchive err = %v, want content digest mismatch", err)
	}
}

func TestLegacyVersionIDArchiveReadsByItsID(t *testing.T) {
	src, _ := skillSource(t, "1.0.0")
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	legacy := legacyArchive(t, builtArchive(t, src, ref))
	legacyID := RefID(ref.Name, "1.0.0")
	store := &Store{Dir: t.TempDir()}
	storeRaw(t, store, ref, legacyID, legacy)
	for name, read := range map[string]func() (Manifest, error){
		"Inspect":                  func() (Manifest, error) { return store.Inspect(ref) },
		"InspectPinned":            func() (Manifest, error) { return store.InspectPinned(ref, legacyID) },
		"ValidatePortableArchive":  func() (Manifest, error) { return ValidatePortableArchive(legacy, ref) },
		"ValidateArchive (staged)": func() (Manifest, error) { return ValidateArchive(store.contentPath(ref.Name, legacyID), ref) },
	} {
		got, err := read()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Digest != legacyID {
			t.Fatalf("%s digest = %s, want legacy id %s", name, got.Digest, legacyID)
		}
	}
}

func TestArchiveWithoutIDReadsByFileDigest(t *testing.T) {
	src, _ := skillSource(t, "")
	ref := Ref{Name: "digest", Tag: "latest"}
	legacy := legacyArchive(t, builtArchive(t, src, ref))
	want := sha256hex(legacy)
	store := &Store{Dir: t.TempDir()}
	storeRaw(t, store, ref, want, legacy)
	got, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := ValidatePortableArchive(legacy, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != want || validated.Digest != want {
		t.Fatalf("digests = %s / %s, want file digest %s", got.Digest, validated.Digest, want)
	}
}
