package imageportable

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestArtifactExportImportRunsWithoutSources(t *testing.T) {
	origin := t.TempDir()
	source := filepath.Join(origin, "source")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte("portable"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "demo", Tag: "v1"}
	store := &image.Store{Dir: filepath.Join(origin, "images")}
	_, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}, imagefile.ResolveRoots{}, ref, store, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{BaseDir: origin, StagingRoot: filepath.Join(origin, "imports")}
	var archive bytes.Buffer
	if err := service.Export(context.Background(), ref.String(), &archive); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(archive.Bytes(), []byte(source)) {
		t.Fatal("export leaked source cwd")
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	importer := Service{BaseDir: target, StagingRoot: filepath.Join(target, "imports")}
	preview, err := importer.Preview(context.Background(), bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Apply(context.Background(), preview.ImportID, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Ref != ref.String() || result.Digest != preview.Digest || result.Reused {
		t.Fatalf("result=%#v preview=%#v", result, preview)
	}
	if prompt, err := (&image.Store{Dir: filepath.Join(target, "images")}).RenderPrompt(ref); err != nil || prompt != "portable" {
		t.Fatalf("prompt=%q err=%v", prompt, err)
	}
}

func TestArtifactExportImportRetainsPackagedSkillsWithoutSources(t *testing.T) {
	origin := t.TempDir()
	source := filepath.Join(origin, "source")
	skill := filepath.Join(source, "skills", "portable-skill")
	if err := os.MkdirAll(filepath.Join(skill, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: portable-skill\ndescription: Use for portable image tests.\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "skill-image", Tag: "v1"}
	store := &image.Store{Dir: filepath.Join(origin, "images")}
	manifest, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/portable-skill"}}}, imagefile.ResolveRoots{}, ref, store, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("skills = %#v", manifest.Skills)
	}
	service := Service{BaseDir: origin, StagingRoot: filepath.Join(origin, "imports")}
	var archive bytes.Buffer
	if err := service.Export(context.Background(), ref.String(), &archive); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	importer := Service{BaseDir: target, StagingRoot: filepath.Join(target, "imports")}
	preview, err := importer.Preview(context.Background(), bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Apply(context.Background(), preview.ImportID, ""); err != nil {
		t.Fatal(err)
	}
	installed := &image.Store{Dir: filepath.Join(target, "images")}
	got, err := installed.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Skills) != 1 || got.Skills[0].TreeSHA256 != manifest.Skills[0].TreeSHA256 {
		t.Fatalf("imported skills = %#v", got.Skills)
	}
	unpacked := t.TempDir()
	if err := installed.Unpack(ref, unpacked); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(unpacked, "skills", "portable-skill", "scripts", "check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("imported script mode = %#o", info.Mode().Perm())
	}
}

func TestArtifactImportIsIdempotentAndConflictsByDigest(t *testing.T) {
	base := t.TempDir()
	source := t.TempDir()
	_ = os.WriteFile(filepath.Join(source, "p.md"), []byte("x"), 0o600)
	ref := image.Ref{Name: "same", Tag: "latest"}
	store := &image.Store{Dir: filepath.Join(base, "images")}
	_, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./p.md"}}}, imagefile.ResolveRoots{}, ref, store, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{BaseDir: base, StagingRoot: filepath.Join(base, "imports")}
	var buf bytes.Buffer
	if err := service.Export(context.Background(), ref.String(), &buf); err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), preview.ImportID, "")
	if err != nil || !result.Reused {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestArtifactImportSupportsExplicitRetag(t *testing.T) {
	origin := t.TempDir()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "p.md"), []byte("retag me"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := image.Ref{Name: "original", Tag: "v1"}
	store := &image.Store{Dir: filepath.Join(origin, "images")}
	if _, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./p.md"}}}, imagefile.ResolveRoots{}, original, store, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	exporter := Service{BaseDir: origin, StagingRoot: filepath.Join(origin, "imports")}
	var archive bytes.Buffer
	if err := exporter.Export(context.Background(), original.String(), &archive); err != nil {
		t.Fatal(err)
	}

	targetBase := t.TempDir()
	importer := Service{BaseDir: targetBase, StagingRoot: filepath.Join(targetBase, "imports")}
	preview, err := importer.Preview(context.Background(), bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	target := image.Ref{Name: "copy", Tag: "latest"}
	result, err := importer.Apply(context.Background(), preview.ImportID, target.String())
	if err != nil {
		t.Fatal(err)
	}
	if result.Ref != target.String() || result.Digest == "" || result.Digest == preview.Digest {
		t.Fatalf("unexpected retag result: %#v", result)
	}
	installed := &image.Store{Dir: filepath.Join(targetBase, "images")}
	if prompt, err := installed.RenderPrompt(target); err != nil || prompt != "retag me" {
		t.Fatalf("prompt=%q err=%v", prompt, err)
	}
	preview, err = importer.Preview(context.Background(), bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	reused, err := importer.Apply(context.Background(), preview.ImportID, target.String())
	if err != nil || !reused.Reused || reused.Digest != result.Digest {
		t.Fatalf("repeated retag = %#v, %v", reused, err)
	}
}
