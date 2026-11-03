package image

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func changeSkillScript(t *testing.T, skillDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func refsIn(t *testing.T, store *Store, name string) []string {
	t.Helper()
	entries, err := os.ReadDir(store.refs().RefsDir(name))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

func TestRebuildSameVersionOtherContentConflicts(t *testing.T) {
	src, skillDir := skillSource(t, "1.0.0")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	changeSkillScript(t, skillDir)
	other, err := BuildV2(src, imagefile.ResolveRoots{}, ref, &Store{Dir: t.TempDir()}, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(2), nil)
	var conflict *VersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want VersionConflictError", err)
	}
	if conflict.Ref != "digest:1.0.0" || conflict.Version != "1.0.0" || conflict.Current != first.Digest || conflict.New != other.Digest {
		t.Fatalf("conflict = %+v, want ref digest:1.0.0 current %s new %s", conflict, first.Digest, other.Digest)
	}
	for _, want := range []string{"digest:1.0.0", first.Digest, other.Digest, "image_version", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
	after, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("conflicting build changed the tagged archive")
	}
	if got := refsIn(t, store, ref.Name); len(got) != 1 {
		t.Fatalf("refs = %v, want only the first archive", got)
	}
}

func TestForcedRebuildMovesTagAndKeepsOldArchive(t *testing.T) {
	src, skillDir := skillSource(t, "1.0.0")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	changeSkillScript(t, skillDir)
	forced, replaced, err := BuildV2Publish(src, imagefile.ResolveRoots{}, ref, store, clockAt(2), nil, PublishOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if forced.Digest == first.Digest || len(replaced) != 1 || replaced[0] != first.Digest {
		t.Fatalf("forced = %s replaced = %v, want a new digest replacing %s", forced.Digest, replaced, first.Digest)
	}
	tagged, err := store.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if tagged.Digest != forced.Digest {
		t.Fatalf("tag = %s, want %s", tagged.Digest, forced.Digest)
	}
	if _, err := store.InspectPinned(ref, first.Digest); err != nil {
		t.Fatalf("old archive lost: %v", err)
	}
}

func TestRebuildOverLegacyArchive(t *testing.T) {
	src, skillDir := skillSource(t, "1.0.0")
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	legacyID := RefID(ref.Name, "1.0.0")
	store := &Store{Dir: t.TempDir()}
	storeRaw(t, store, ref, legacyID, legacyArchive(t, builtArchive(t, src, ref)))

	same, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(5), nil)
	if err != nil {
		t.Fatalf("same content over a legacy archive: %v", err)
	}
	if same.Digest != legacyID {
		t.Fatalf("digest = %s, want the legacy id %s", same.Digest, legacyID)
	}
	if got := refsIn(t, store, ref.Name); len(got) != 1 {
		t.Fatalf("refs = %v, want only the legacy archive", got)
	}

	changeSkillScript(t, skillDir)
	_, err = BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(6), nil)
	var conflict *VersionConflictError
	if !errors.As(err, &conflict) || conflict.Current != legacyID {
		t.Fatalf("err = %v, want a conflict with the legacy id", err)
	}
}

func TestRebuildWithoutVersionNeverConflicts(t *testing.T) {
	src, skillDir := skillSource(t, "")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "digest", Tag: "latest"}
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	changeSkillScript(t, skillDir)
	second, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(2), nil)
	if err != nil {
		t.Fatalf("unversioned rebuild: %v", err)
	}
	if second.Digest == first.Digest {
		t.Fatal("changed content kept its digest")
	}
}

func TestInstallArchiveRefusesConflictingVersion(t *testing.T) {
	src, skillDir := skillSource(t, "1.0.0")
	ref := Ref{Name: "digest", Tag: "1.0.0"}
	store := &Store{Dir: t.TempDir()}
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, clockAt(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	changeSkillScript(t, skillDir)
	var conflict *VersionConflictError
	if _, err := store.InstallArchive(ref, builtArchive(t, src, ref)); !errors.As(err, &conflict) {
		t.Fatalf("InstallArchive err = %v, want VersionConflictError", err)
	}
	path := filepath.Join(t.TempDir(), "pulled.tar.gz")
	if err := os.WriteFile(path, builtArchive(t, src, ref), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishArchiveFile(ref, path, PublishOptions{}); !errors.As(err, &conflict) {
		t.Fatalf("PublishArchiveFile err = %v, want VersionConflictError", err)
	}
	if tagged, err := store.Inspect(ref); err != nil || tagged.Digest != first.Digest {
		t.Fatalf("tag = %+v err = %v, want %s", tagged.Digest, err, first.Digest)
	}
}

func TestRetagArchiveBytesWritesContentID(t *testing.T) {
	src, _ := skillSource(t, "1.0.0")
	source := Ref{Name: "digest", Tag: "1.0.0"}
	target := Ref{Name: "copy", Tag: "1.0.0"}
	for name, archive := range map[string][]byte{
		"new":    builtArchive(t, src, source),
		"legacy": legacyArchive(t, builtArchive(t, src, source)),
	} {
		_, manifest, err := RetagArchiveBytes(source, target, archive)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if manifest.Digest == RefID(target.Name, "1.0.0") || manifest.Digest != manifest.ID {
			t.Fatalf("%s: retagged digest = %s id = %s, want a content id", name, manifest.Digest, manifest.ID)
		}
	}
}
