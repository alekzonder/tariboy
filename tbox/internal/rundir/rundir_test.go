package rundir

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
)

func fixtureImage(t *testing.T, withRuntime bool) archive.Image {
	t.Helper()
	prep, err := skill.Prepare(filepath.Join("..", "..", "testdata", "compat", "skills", "hello-skill"), "dir", "")
	if err != nil {
		t.Fatal(err)
	}
	prompts := []archive.PromptLayer{
		{Source: "a", Base: "a.md", Body: []byte("first\n")},
		{Source: "b", Base: "b.md", Body: []byte("second\n")},
	}
	img, err := archive.Assemble("demo", "1.0.0", time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), []skill.Prepared{prep}, prompts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if withRuntime {
		e := img.Template.Entries
		img.Template.Entries = []archive.TemplateEntry{e[0], {Kind: "runtime", Runtime: "codex"}, e[1]}
	}
	return img
}

func TestPrepareUnpacksWithModes(t *testing.T) {
	home := paths.Home(t.TempDir())
	img := fixtureImage(t, false)
	run, err := Prepare(home, img, imgstore.Ref{Name: "demo", Tag: "latest"}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if run.Dir != home.RunDir(img.Manifest.Digest, "codex") || run.SkillsDir != filepath.Join(run.ImageDir, "skills") || run.Tag != "latest" || run.Name != "demo" {
		t.Fatalf("run %+v", run)
	}
	info, err := os.Stat(filepath.Join(run.SkillsDir, "hello-skill", "scripts", "run.sh"))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("run.sh: %v %v", info, err)
	}
	got, err := os.ReadFile(run.PromptPath)
	if err != nil || string(got) != "first\n\nsecond\n\n" {
		t.Fatalf("prompt %q %v", got, err)
	}
	opened, err := Open(run.ImageDir)
	if err != nil || opened.Name != "demo" || opened.Digest != img.Manifest.Digest || len(opened.Skills) != 1 {
		t.Fatalf("open %+v %v", opened, err)
	}
}

func TestPromptSkipsRuntimeEntries(t *testing.T) {
	home := paths.Home(t.TempDir())
	run, err := Prepare(home, fixtureImage(t, true), imgstore.Ref{Name: "demo", Tag: "latest"}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(run.PromptPath)
	if string(got) != "first\n\nsecond\n\n" {
		t.Fatalf("prompt %q", got)
	}
}

func TestPrepareReusesCompleteRun(t *testing.T) {
	home := paths.Home(t.TempDir())
	img := fixtureImage(t, false)
	ref := imgstore.Ref{Name: "demo", Tag: "latest"}
	run, err := Prepare(home, img, ref, "codex")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	files := []string{run.PromptPath, filepath.Join(run.ImageDir, "manifest.json")}
	for _, f := range files {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Prepare(home, img, ref, "codex"); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		info, _ := os.Stat(f)
		if !info.ModTime().Equal(old.Truncate(time.Nanosecond)) && info.ModTime().Sub(old).Abs() > time.Second {
			t.Errorf("%s rewritten", f)
		}
	}
}

func TestOpenRejectsBrokenDirectories(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("invalid manifest accepted")
	}
}

func TestPrepareConcurrent(t *testing.T) {
	home := paths.Home(t.TempDir())
	img := fixtureImage(t, false)
	ref := imgstore.Ref{Name: "demo", Tag: "latest"}
	const n = 8
	errs := make(chan error, n)
	runs := make(chan Run, n)
	for i := 0; i < n; i++ {
		go func() {
			run, err := Prepare(home, img, ref, "codex")
			errs <- err
			runs <- run
		}()
	}
	var last Run
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		last = <-runs
	}
	if !complete(last) {
		t.Fatal("run is not complete")
	}
	first, err := os.Stat(filepath.Join(last.ImageDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(home, img, ref, "codex"); err != nil {
		t.Fatal(err)
	}
	again, _ := os.Stat(filepath.Join(last.ImageDir, "manifest.json"))
	if !os.SameFile(first, again) {
		t.Fatal("complete run was replaced")
	}
}

func TestPrepareSweepsStaleStages(t *testing.T) {
	home := paths.Home(t.TempDir())
	img := fixtureImage(t, false)
	dir := home.RunDir(img.Manifest.Digest, "codex")
	stale := filepath.Join(dir, ".stage-old")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(home, img, imgstore.Ref{Name: "demo", Tag: "latest"}, "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale stage kept: %v", err)
	}
}

func TestPromptNewlineRule(t *testing.T) {
	img := fixtureImage(t, false)
	for i := range img.Members {
		switch img.Members[i].Name {
		case img.Template.Entries[0].ArchivePath:
			img.Members[i].Body = []byte("no newline")
		case img.Template.Entries[1].ArchivePath:
			img.Members[i].Body = []byte("with newline\n")
		}
	}
	got, err := promptText(img)
	if err != nil || string(got) != "no newline\n\nwith newline\n\n" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestOpenRefusesSymlinkedSkillDir(t *testing.T) {
	run, err := Prepare(paths.Home(t.TempDir()), fixtureImage(t, false), imgstore.Ref{Name: "demo", Tag: "latest"}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(run.ImageDir, "skills", "hello-skill")
	moved := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Rename(skillDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, skillDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(run.ImageDir); err == nil {
		t.Fatal("symlinked skill directory accepted")
	}
}
