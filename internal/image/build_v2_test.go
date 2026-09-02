package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func appendArchiveMember(t *testing.T, archive []byte, name, body string) []byte {
	t.Helper()
	in, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	tr := tar.NewReader(in)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		copyHeader := *h
		if err := tw.WriteHeader(&copyHeader); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeTarFile(tw, name, []byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func rewriteArchiveMember(t *testing.T, archive []byte, target string, rewrite func([]byte) []byte) []byte {
	t.Helper()
	in, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	tr := tar.NewReader(in)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == target {
			data = rewrite(data)
		}
		copyHeader := *h
		copyHeader.Size = int64(len(data))
		if err := tw.WriteHeader(&copyHeader); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func rewriteArchive(t *testing.T, archive []byte, rewrite func(*tar.Header, []byte) (*tar.Header, []byte)) []byte {
	t.Helper()
	in, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	tr := tar.NewReader(in)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		copyHeader := *header
		nextHeader, nextBody := rewrite(&copyHeader, body)
		if nextHeader == nil {
			continue
		}
		nextHeader.Size = int64(len(nextBody))
		if err := tw.WriteHeader(nextHeader); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(nextBody); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func archiveHeaders(t *testing.T, archive []byte) map[string]tar.Header {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := map[string]tar.Header{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out[header.Name] = *header
	}
}

func writeTestSkill(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: Use this skill for image tests.\n---\n# Skill\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBuildV2PackagesCanonicalSkillsAndModes(t *testing.T) {
	source := t.TempDir()
	skillDir := writeTestSkill(t, filepath.Join(source, "skills"), "code-review")
	src := &imagefile.V2{
		SchemaVersion: 2,
		Dir:           source,
		Skills:        []imagefile.SkillEntry{{Dir: "./skills/code-review"}},
	}
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "reviewer", Tag: "v1"}
	manifest, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, func() time.Time { return time.Unix(1, 0) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("skills = %#v", manifest.Skills)
	}
	got := manifest.Skills[0]
	if got.Name != "code-review" || got.Description != "Use this skill for image tests." || got.Source != "./skills/code-review" || got.Category != "source" || got.ArchiveRoot != "skills/code-review" || got.FileCount != 2 || got.Size <= 0 || len(got.TreeSHA256) != 64 {
		t.Fatalf("skill metadata = %#v", got)
	}
	archive, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	headers := archiveHeaders(t, archive)
	if headers["skills/code-review/SKILL.md"].Mode != 0o600 {
		t.Fatalf("SKILL.md mode = %#o", headers["skills/code-review/SKILL.md"].Mode)
	}
	if headers["skills/code-review/scripts/check.sh"].Mode != 0o700 {
		t.Fatalf("check.sh mode = %#o", headers["skills/code-review/scripts/check.sh"].Mode)
	}
	if err := os.RemoveAll(skillDir); err != nil {
		t.Fatal(err)
	}
	if body, err := store.ReadFile(ref, "skills/code-review/scripts/check.sh"); err != nil || string(body) != "#!/bin/sh\nexit 0\n" {
		t.Fatalf("packed script = %q, %v", body, err)
	}
}

func TestBuildV2PreservesCurrentStoreSkillClientVersion(t *testing.T) {
	currentStore := t.TempDir()
	writeTestSkill(t, filepath.Join(currentStore, "skills"), "whoami")
	src := &imagefile.V2{
		SchemaVersion: 2,
		Dir:           t.TempDir(),
		Skills:        []imagefile.SkillEntry{{Dir: "$CURRENT_VERSION_STORE/skills/whoami"}},
	}
	manifest, err := BuildV2(src, imagefile.ResolveRoots{
		CurrentVersionStore: currentStore,
		CurrentStoreVersion: "0.45.2",
	}, Ref{Name: "versioned", Tag: "latest"}, &Store{Dir: t.TempDir()}, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Skills[0].ClientVersion; got != "0.45.2" {
		t.Fatalf("client version = %q, want producing Store version", got)
	}
}

func TestBuildV2SkillBytesChangeTreeAndImageDigests(t *testing.T) {
	source := t.TempDir()
	skillDir := writeTestSkill(t, filepath.Join(source, "skills"), "digest-skill")
	src := &imagefile.V2{SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/digest-skill"}}}
	ref := Ref{Name: "digest", Tag: "v1"}
	clock := func() time.Time { return time.Unix(1, 0) }
	first, err := BuildV2(src, imagefile.ResolveRoots{}, ref, &Store{Dir: t.TempDir()}, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := BuildV2(src, imagefile.ResolveRoots{}, ref, &Store{Dir: t.TempDir()}, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest || first.Skills[0].TreeSHA256 == second.Skills[0].TreeSHA256 {
		t.Fatalf("digests did not change: image %s/%s tree %s/%s", first.Digest, second.Digest, first.Skills[0].TreeSHA256, second.Skills[0].TreeSHA256)
	}
}

func TestUnpackV2PreservesNormalizedSkillExecutableMode(t *testing.T) {
	source := t.TempDir()
	writeTestSkill(t, filepath.Join(source, "skills"), "mode-skill")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "mode", Tag: "v1"}
	if _, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/mode-skill"}}}, imagefile.ResolveRoots{}, ref, store, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := store.Unpack(ref, dest); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		"skills/mode-skill/SKILL.md":         0o600,
		"skills/mode-skill/scripts/check.sh": 0o700,
	} {
		info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s mode = %#o, want %#o", path, info.Mode().Perm(), want)
		}
	}
}

func TestPortableV2ArchiveRejectsTamperedSkillMembers(t *testing.T) {
	source := t.TempDir()
	writeTestSkill(t, filepath.Join(source, "skills"), "portable-skill")
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "portable-skill", Tag: "v1"}
	if _, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/portable-skill"}}}, imagefile.ResolveRoots{}, ref, store, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	original, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*tar.Header, []byte) (*tar.Header, []byte){
		"changed bytes": func(header *tar.Header, body []byte) (*tar.Header, []byte) {
			if header.Name == "skills/portable-skill/scripts/check.sh" {
				body = []byte("#!/bin/sh\nexit 9\n")
			}
			return header, body
		},
		"changed mode": func(header *tar.Header, body []byte) (*tar.Header, []byte) {
			if header.Name == "skills/portable-skill/scripts/check.sh" {
				header.Mode = 0o600
			}
			return header, body
		},
		"executable manifest": func(header *tar.Header, body []byte) (*tar.Header, []byte) {
			if header.Name == "manifest.json" {
				header.Mode = 0o700
			}
			return header, body
		},
		"missing member": func(header *tar.Header, body []byte) (*tar.Header, []byte) {
			if header.Name == "skills/portable-skill/scripts/check.sh" {
				return nil, nil
			}
			return header, body
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			archive := rewriteArchive(t, original, mutate)
			if _, err := ValidatePortableArchive(archive, ref); err == nil {
				t.Fatal("accepted tampered skill archive")
			}
		})
	}
}

func TestRetagPortableV2ArchivePreservesSkillModes(t *testing.T) {
	source := t.TempDir()
	writeTestSkill(t, filepath.Join(source, "skills"), "retag-skill")
	store := &Store{Dir: t.TempDir()}
	original := Ref{Name: "original-skill", Tag: "v1"}
	if _, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/retag-skill"}}}, imagefile.ResolveRoots{}, original, store, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	archive, err := store.ArchiveBytes(original)
	if err != nil {
		t.Fatal(err)
	}
	target := Ref{Name: "retagged-skill", Tag: "v2"}
	if _, err := store.RetagPortableArchive(original, target, archive); err != nil {
		t.Fatal(err)
	}
	retagged, err := store.ArchiveBytes(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := archiveHeaders(t, retagged)["skills/retag-skill/scripts/check.sh"].Mode; got != 0o700 {
		t.Fatalf("retagged executable mode = %#o", got)
	}
}

func TestBuildV2EmbedsStaticLayersInExactOrder(t *testing.T) {
	source := t.TempDir()
	for name, body := range map[string]string{"a.md": "A", "b.md": "B"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	src := &imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./a.md"}, {Runtime: "context"}, {File: "./b.md"}}}
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "ordered", Tag: "latest"}
	manifest, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, func() time.Time { return time.Unix(1, 0) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 2 || manifest.PromptTemplateSHA256 == "" {
		t.Fatalf("manifest = %#v", manifest)
	}
	template, err := store.ReadTemplate(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(template.Entries) != 3 || template.Entries[0].ArchivePath != "prompt/layers/000-a.md" || template.Entries[1].Runtime != "context" || template.Entries[2].ArchivePath != "prompt/layers/002-b.md" {
		t.Fatalf("entries = %#v", template.Entries)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"prompt/layers/000-a.md": "A", "prompt/layers/002-b.md": "B"} {
		got, err := store.ReadFile(ref, path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", path, got, err)
		}
	}
}

func TestBuildV2AddsNoImplicitContent(t *testing.T) {
	for _, plugins := range [][]imagefile.V2Plugin{nil, {{Name: "context"}}} {
		store := &Store{Dir: t.TempDir()}
		ref := Ref{Name: "empty", Tag: "latest"}
		manifest, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: t.TempDir(), Plugins: plugins}, imagefile.ResolveRoots{}, ref, store, time.Now, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(manifest.Plugins) != len(plugins) {
			t.Fatalf("plugins = %#v", manifest.Plugins)
		}
		template, err := store.ReadTemplate(ref)
		if err != nil || len(template.Entries) != 0 {
			t.Fatalf("template = %#v, %v", template, err)
		}
		preview, err := store.RenderPrompt(ref)
		if err != nil || strings.Contains(preview, "Context") || preview != "" {
			t.Fatalf("preview = %q, %v", preview, err)
		}
	}
}

func TestBuildV2MutableRetainsPinnedGeneration(t *testing.T) {
	source := t.TempDir()
	prompt := filepath.Join(source, "prompt.md")
	if err := os.WriteFile(prompt, []byte("first generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "reviewer", Tag: "latest"}
	src := &imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}

	first, err := BuildV2Mutable(src, imagefile.ResolveRoots{}, ref, store, fixedClock(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsMutable(ref) {
		t.Fatal("mutable build did not mark ref")
	}
	if err := os.WriteFile(prompt, []byte("second generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := BuildV2Mutable(src, imagefile.ResolveRoots{}, ref, store, fixedClock(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatal("mutable rebuild did not change digest")
	}
	if pinned, err := store.InspectPinned(ref, first.Digest); err != nil || pinned.Digest != first.Digest {
		t.Fatalf("inspect pinned generation = %#v, %v", pinned, err)
	}
	dest := t.TempDir()
	if err := store.UnpackPinned(ref, first.Digest, dest); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(dest, "prompt", "layers", "000-prompt.md")); err != nil || string(body) != "first generation" {
		t.Fatalf("unpacked pinned layer = %q, %v", body, err)
	}
}

func TestBuildV2MutableArchiveReturnsPublishedBytes(t *testing.T) {
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "reviewer", Tag: "latest"}

	manifest, archive, err := BuildV2MutableArchive(&imagefile.V2{SchemaVersion: 2, Dir: t.TempDir()}, imagefile.ResolveRoots{}, ref, store, fixedClock(), nil)
	if err != nil {
		t.Fatal(err)
	}
	published, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(archive, published) || manifest.Digest != fmt.Sprintf("%x", sha256.Sum256(archive)) {
		t.Fatalf("returned archive does not match published %s", manifest.Digest)
	}
}

func TestPortableV2ArchiveRejectsUnmanifestedHarnessSkills(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "p.md"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	origin := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "safe", Tag: "latest"}
	if _, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./p.md"}}}, imagefile.ResolveRoots{}, ref, origin, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	archive, err := origin.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	archive = appendArchiveMember(t, archive, "skills/evil/SKILL.md", "do not run")
	target := &Store{Dir: t.TempDir()}
	if err := target.InstallPortableArchive(ref, archive); err == nil || !strings.Contains(err.Error(), "unexpected schema-v2 image member") {
		t.Fatalf("InstallPortableArchive error = %v", err)
	}
}

func TestPortableV2ArchiveRejectsTrailingTemplateJSON(t *testing.T) {
	origin := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "strict", Tag: "latest"}
	if _, err := BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: t.TempDir()}, imagefile.ResolveRoots{}, ref, origin, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	archive, err := origin.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	archive = rewriteArchiveMember(t, archive, "prompt/template.json", func(body []byte) []byte {
		return append(body, []byte("\n{}")...)
	})
	if err := (&Store{Dir: t.TempDir()}).InstallPortableArchive(ref, archive); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("InstallPortableArchive error = %v", err)
	}
}

func TestValidateV2RejectsAggregatePromptLayersAboveLimit(t *testing.T) {
	source := t.TempDir()
	body := bytes.Repeat([]byte{'x'}, 4<<20)
	prompts := make([]imagefile.PromptEntry, 0, 9)
	for i := 0; i < 9; i++ {
		name := filepath.Join(source, fmt.Sprintf("layer-%d.md", i))
		if err := os.WriteFile(name, body, 0o600); err != nil {
			t.Fatal(err)
		}
		prompts = append(prompts, imagefile.PromptEntry{File: "./" + filepath.Base(name)})
	}
	_, err := ValidateV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: prompts}, imagefile.ResolveRoots{}, nil)
	if err == nil || !strings.Contains(err.Error(), "aggregate bytes") {
		t.Fatalf("ValidateV2 error = %v", err)
	}
}
