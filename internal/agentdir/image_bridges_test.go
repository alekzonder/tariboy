package agentdir

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestSyncBridgeTreeIgnoresUnsupportedDirectorySyncOnly(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := syncBridgeTreeWith(root, func(name string) error {
		if name == root {
			return os.ErrInvalid
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unsupported directory sync was fatal: %v", err)
	}

	err = syncBridgeTreeWith(root, func(name string) error {
		if name == file {
			return os.ErrInvalid
		}
		return nil
	})
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("regular file sync error = %v, want os.ErrInvalid", err)
	}
}

func bridgeFixture(t *testing.T) (string, []image.ManifestSkill) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "review")
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: Review code safely.\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	prepared, err := agentskills.Prepare(imagefile.ResolvedDirectory{Source: "./skills/review", Path: dir, Category: "source"})
	if err != nil {
		t.Fatal(err)
	}
	meta := prepared.Metadata
	return root, []image.ManifestSkill{{
		Name: meta.Name, Description: meta.Description, Source: meta.Source, Category: meta.Category,
		ArchiveRoot: meta.ArchiveRoot, FileCount: meta.FileCount, Size: meta.Size, TreeSHA256: meta.TreeSHA256,
	}}
}

func TestImageBridgeDirUsesDigestContractAndHarness(t *testing.T) {
	l := New(t.TempDir(), "worker")
	digest := strings.Repeat("a", 64)
	got, err := l.ImageBridgeDir(digest, "1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(l.Root, "image-bridges", digest, "1", "claude")
	if got != want || l.ImageBridgesDir() != filepath.Join(l.Root, "image-bridges") {
		t.Fatalf("bridge paths = %q / %q, want %q", got, l.ImageBridgesDir(), want)
	}
}

func TestImageBridgeDirRejectsUnsafeSegments(t *testing.T) {
	l := New(t.TempDir(), "worker")
	digest := strings.Repeat("a", 64)
	for _, tc := range []struct{ digest, contract, harness string }{
		{"", "1", "claude"}, {"sha256:" + digest, "1", "claude"}, {strings.Repeat("A", 64), "1", "claude"},
		{digest, "", "claude"}, {digest, "../1", "claude"}, {digest, "1", ""}, {digest, "1", "../claude"},
	} {
		if _, err := l.ImageBridgeDir(tc.digest, tc.contract, tc.harness); err == nil {
			t.Fatalf("accepted unsafe bridge key %#v", tc)
		}
	}
}

func TestPrepareImageBridgeCopiesSkillsAndGeneratedFilesOwnerOnly(t *testing.T) {
	source, expected := bridgeFixture(t)
	l := New(t.TempDir(), "worker")
	finalDir, err := l.ImageBridgeDir(strings.Repeat("b", 64), "1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	plan := BridgePlan{
		SkillDestination: "skills",
		Files:            []BridgeFile{{Path: ".claude-plugin/plugin.json", Body: []byte("{\"name\":\"fixture\"}\n"), Mode: 0o600}},
	}
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	for path, wantMode := range map[string]os.FileMode{
		"skills/review/SKILL.md":         0o600,
		"skills/review/scripts/check.sh": 0o700,
		".claude-plugin/plugin.json":     0o600,
		"bridge-manifest.json":           0o600,
	} {
		info, err := os.Stat(filepath.Join(finalDir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if info.Mode().Perm() != wantMode {
			t.Fatalf("%s mode = %#o, want %#o", path, info.Mode().Perm(), wantMode)
		}
	}
	info, err := os.Stat(finalDir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("bridge dir mode = %#o, %v", info.Mode().Perm(), err)
	}
}

func TestPrepareImageBridgeReusesValidPublishedBridgeWithoutWrites(t *testing.T) {
	source, expected := bridgeFixture(t)
	l := New(t.TempDir(), "worker")
	finalDir, _ := l.ImageBridgeDir(strings.Repeat("c", 64), "1", "opencode")
	plan := BridgePlan{SkillDestination: "skills", Files: []BridgeFile{{Path: "opencode.json", Body: []byte("{}\n"), Mode: 0o600}}}
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(finalDir, "bridge-manifest.json")
	before, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("valid bridge was rewritten: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestPrepareImageBridgeRebuildsInvalidPublishedBridge(t *testing.T) {
	source, expected := bridgeFixture(t)
	l := New(t.TempDir(), "worker")
	finalDir, _ := l.ImageBridgeDir(strings.Repeat("d", 64), "1", "claude")
	plan := BridgePlan{SkillDestination: "skills"}
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(finalDir, "skills", "review", "SKILL.md"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(finalDir, "skills", "review", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "name: review") {
		t.Fatalf("bridge was not rebuilt: %q, %v", body, err)
	}
}

func TestPrepareImageBridgeRejectsManifestedUnexpectedGeneratedFile(t *testing.T) {
	source, expected := bridgeFixture(t)
	finalDir, _ := New(t.TempDir(), "worker").ImageBridgeDir(strings.Repeat("2", 64), "1", "claude")
	plan := BridgePlan{SkillDestination: "skills"}
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	extraBody := []byte("unexpected")
	if err := os.WriteFile(filepath.Join(finalDir, "extra.json"), extraBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(finalDir, "bridge-manifest.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest BridgeManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Files["extra.json"] = fileRecord(extraBody, 0o600)
	body, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(finalDir, "extra.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected generated file survived validation: %v", err)
	}
}

func TestPrepareImageBridgeRejectsUnsafeSourceAndPlanPaths(t *testing.T) {
	t.Run("source symlink", func(t *testing.T) {
		source, expected := bridgeFixture(t)
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(source, "review", "leak")); err != nil {
			t.Fatal(err)
		}
		finalDir, _ := New(t.TempDir(), "worker").ImageBridgeDir(strings.Repeat("e", 64), "1", "claude")
		if err := PrepareImageBridge(source, finalDir, expected, BridgePlan{SkillDestination: "skills"}); err == nil {
			t.Fatal("accepted source symlink")
		}
	})
	t.Run("generated escape", func(t *testing.T) {
		source, expected := bridgeFixture(t)
		root := t.TempDir()
		finalDir, _ := New(root, "worker").ImageBridgeDir(strings.Repeat("f", 64), "1", "claude")
		plan := BridgePlan{SkillDestination: "skills", Files: []BridgeFile{{Path: "../escape", Body: []byte("x"), Mode: 0o600}}}
		if err := PrepareImageBridge(source, finalDir, expected, plan); err == nil {
			t.Fatal("accepted generated path escape")
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(finalDir), "escape")); !os.IsNotExist(err) {
			t.Fatalf("escape path was written: %v", err)
		}
	})
}

func TestPrepareImageBridgeConcurrentIdenticalPublishSucceeds(t *testing.T) {
	source, expected := bridgeFixture(t)
	finalDir, _ := New(t.TempDir(), "worker").ImageBridgeDir(strings.Repeat("1", 64), "1", "codex")
	plan := BridgePlan{SkillDestination: "plugin/skills", Files: []BridgeFile{{Path: "plugin/plugin.json", Body: []byte("{}\n"), Mode: 0o600}}}
	start := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- PrepareImageBridge(source, finalDir, expected, plan)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(finalDir, "bridge-manifest.json")); err != nil {
		t.Fatal(err)
	}
}
