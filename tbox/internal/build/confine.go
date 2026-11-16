package build

import (
	"fmt"
	"path/filepath"

	"github.com/alekzonder/tariboy/tbox/internal/skillslock"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

// A Store image is confined to its Store, like tariboy's Store builds: every
// extends layer, local skill directory and local prompt file must lie inside
// the canonical Store root. Remote entries, and skills restored into the tbox
// cache from skills-lock.json, are exempt; local skills-lock.json sources are
// confined by the restorer. A --path build (no StoreRoot) is not confined.

// storeRoot returns the canonical Store root, or "" for a --path build.
func (j *job) storeRoot() (string, error) {
	if j.opts.StoreRoot == "" {
		return "", nil
	}
	root, err := filepath.EvalSymlinks(j.opts.StoreRoot)
	if err != nil {
		return "", fmt.Errorf("store root: %w", err)
	}
	return root, nil
}

func outsideStore(what, value string) error {
	return fmt.Errorf("%s %q points outside the Store", what, value)
}

// confineLayers checks every directory of the extends chain.
func (j *job) confineLayers() error {
	root, err := j.storeRoot()
	if root == "" || err != nil {
		return err
	}
	dirs, err := tfile.LayerDirs(j.dir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if !skillslock.Inside(root, dir) {
			return outsideStore("extends layer", dir)
		}
	}
	return nil
}

// confineEntries checks the local skill and prompt entries as written,
// before the skills-lock.json remap. A path that cannot be located is left
// to the build, which reports it.
func (j *job) confineEntries() error {
	root, err := j.storeRoot()
	if root == "" || err != nil {
		return err
	}
	for _, s := range j.f.Skills {
		if remoteSkill(s) {
			continue
		}
		if abs, err := tfile.EntryPath(s.BaseDir, s.Dir, "skill"); err == nil && !insideStore(root, abs) {
			return outsideStore("skill", s.Dir)
		}
	}
	for _, p := range j.f.Prompts {
		if remotePrompt(p) || p.File == "" {
			continue
		}
		if abs, err := tfile.EntryPath(p.BaseDir, p.File, "prompt"); err == nil && !insideStore(root, abs) {
			return outsideStore("prompt", p.File)
		}
	}
	return nil
}

// insideStore reports whether abs lies inside root, as written or, when it
// exists, after its symlinks are evaluated (the build rejects symlinked
// entry paths separately).
func insideStore(root, abs string) bool {
	if skillslock.Inside(root, abs) {
		return true
	}
	canon, err := filepath.EvalSymlinks(abs)
	return err == nil && skillslock.Inside(root, canon)
}
