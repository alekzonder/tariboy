// Package rundir unpacks an image into the per-run directory a harness
// session reads from, and opens an unpacked image again.
package rundir

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
)

const (
	manifestName = "manifest.json"
	promptName   = "PROMPT.md"
	maxManifest  = 8 << 20
)

// Run describes an unpacked image. Dir is the run directory, which holds the
// image directory and PROMPT.md.
type Run struct {
	Dir          string
	ImageDir     string
	SkillsDir    string
	PromptPath   string
	Name         string
	Tag          string
	Digest       string
	ImageVersion string
	Skills       []skill.Metadata
}

func newRun(dir string, man archive.Manifest) Run {
	image := filepath.Join(dir, "image")
	return Run{
		Dir:          dir,
		ImageDir:     image,
		SkillsDir:    filepath.Join(image, "skills"),
		PromptPath:   filepath.Join(dir, promptName),
		Name:         man.Name,
		Digest:       man.Digest,
		ImageVersion: man.ImageVersion,
		Skills:       man.Skills,
	}
}

// Prepare unpacks img for harness under home. The unpacking is staged in a
// temporary sibling and renamed, and an already complete run directory is
// reused untouched.
func Prepare(home paths.Home, img archive.Image, ref imgstore.Ref, harness string) (Run, error) {
	digest := img.Manifest.Digest
	if digest == "" || harness == "" {
		return Run{}, errors.New("image digest and harness are required")
	}
	dir := home.RunDir(digest, harness)
	run := newRun(dir, img.Manifest)
	run.Tag = ref.Tag
	if complete(run) {
		return run, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Run{}, err
	}
	stage, err := os.MkdirTemp(dir, ".stage-")
	if err != nil {
		return Run{}, err
	}
	defer os.RemoveAll(stage)
	if err := unpack(filepath.Join(stage, "image"), img.Members); err != nil {
		return Run{}, err
	}
	prompt, err := promptText(img)
	if err != nil {
		return Run{}, err
	}
	if err := os.WriteFile(filepath.Join(stage, promptName), prompt, 0o600); err != nil {
		return Run{}, err
	}
	// The image directory is renamed last: its presence marks a complete run.
	for _, name := range []string{promptName, "image"} {
		target := filepath.Join(dir, name)
		if err := os.RemoveAll(target); err != nil {
			return Run{}, err
		}
		if err := os.Rename(filepath.Join(stage, name), target); err != nil {
			return Run{}, err
		}
	}
	return run, nil
}

func complete(run Run) bool {
	for _, p := range []string{filepath.Join(run.ImageDir, manifestName), run.PromptPath} {
		if info, err := os.Lstat(p); err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func unpack(root string, members []archive.Member) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	for _, m := range members {
		if !filepath.IsLocal(m.Name) {
			return fmt.Errorf("unsafe member name %q", m.Name)
		}
		path := filepath.Join(root, m.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, werr := f.Write(m.Body)
		cerr := f.Close()
		if err := errors.Join(werr, cerr, os.Chmod(path, os.FileMode(m.Mode).Perm())); err != nil {
			return err
		}
	}
	return nil
}

// promptText joins the bodies of the template's file entries in order, each
// followed by a blank line. Runtime entries add nothing.
func promptText(img archive.Image) ([]byte, error) {
	byName := make(map[string][]byte, len(img.Members))
	for _, m := range img.Members {
		byName[m.Name] = m.Body
	}
	var out bytes.Buffer
	for _, e := range img.Template.Entries {
		if e.Kind != "file" {
			continue
		}
		body, ok := byName[e.ArchivePath]
		if !ok {
			return nil, fmt.Errorf("prompt layer %s is missing", e.ArchivePath)
		}
		out.Write(body)
		if !bytes.HasSuffix(body, []byte("\n")) {
			out.WriteByte('\n')
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// Open reads the manifest of an unpacked image directory.
func Open(imageDir string) (Run, error) {
	body, err := fsafe.ReadFileNoFollow(filepath.Join(imageDir, manifestName), maxManifest)
	if err != nil {
		return Run{}, fmt.Errorf("read image manifest: %w", err)
	}
	var man archive.Manifest
	if err := json.Unmarshal(body, &man); err != nil {
		return Run{}, fmt.Errorf("parse image manifest: %w", err)
	}
	if man.Name == "" {
		return Run{}, errors.New("image manifest has no name")
	}
	for _, s := range man.Skills {
		if s.Name == "" || !filepath.IsLocal(s.ArchiveRoot) {
			return Run{}, fmt.Errorf("image manifest has an invalid skill %q", s.Name)
		}
		if info, err := os.Stat(filepath.Join(imageDir, s.ArchiveRoot)); err != nil || !info.IsDir() {
			return Run{}, fmt.Errorf("skill %s is missing from the image directory", s.Name)
		}
	}
	man.Digest = man.ID
	abs, err := filepath.Abs(imageDir)
	if err != nil {
		return Run{}, err
	}
	run := newRun(filepath.Dir(abs), man)
	run.ImageDir = abs
	run.SkillsDir = filepath.Join(abs, "skills")
	return run, nil
}
