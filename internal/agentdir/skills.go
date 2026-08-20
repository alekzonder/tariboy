package agentdir

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// safeSkillName matches a packed skill directory name. It forbids "/", "..",
// and leading dots so a skill name can never escape the destination directory
// when joined onto it (M15 path-traversal confinement). Mirrors image.ParseRef's
// name discipline.
var safeSkillName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// MaterializeSkills copies each packed skill tree from <imageDir>/skills/<name>/**
// into <baseDir>/<subDir>/<name>/** so a harness that reads a native skills
// directory (e.g. claude's .claude/skills) sees an image's skills at run time.
// An absent <imageDir>/skills is a no-op. Each skill name is validated to
// prevent path traversal out of the destination. Files are written 0600, dirs
// 0700, confined to the agent's own tree.
//
// baseDir is the confinement root (the agent cwd / workdir). It is trusted;
// everything below it is not. subDir is the harness-native skills subpath
// relative to baseDir (e.g. ".claude/skills"). Because the agent workdir is
// PERSISTENT and agent-controlled (created once at Provision, reused every
// iteration), an agent can, on one run, plant a destination ANCESTOR — subDir
// itself or an intermediate like ".claude" — as a symlink pointing OUTSIDE the
// workdir, so that the next run's MkdirAll would follow it and write skill
// content outside the agent cwd. To close that, we walk subDir component by
// component from baseDir down, Lstat each, refuse any existing symlink, and
// create missing components with a non-following os.Mkdir (never MkdirAll
// through an unchecked parent). Only a fully verified, symlink-free chain is
// written into.
func MaterializeSkills(imageDir, baseDir, subDir string) error {
	src := filepath.Join(imageDir, "skills")
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	destDir, err := ensureConfinedDir(baseDir, subDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !safeSkillName.MatchString(name) {
			return fmt.Errorf("unsafe skill name %q", name)
		}
		if err := copyTree(filepath.Join(src, name), filepath.Join(destDir, name)); err != nil {
			return err
		}
	}
	return nil
}

// ensureConfinedDir verifies and creates the directory chain from baseDir down
// to baseDir/subDir, refusing to traverse or create through any symlinked
// component. baseDir is the trusted confinement root; every component of subDir
// is Lstat'd first — an existing symlink (or non-directory) is refused, a
// missing component is created with a non-following os.Mkdir. This is what
// keeps a pre-planted symlinked destDir ancestor from redirecting the skill
// write outside the agent workdir. It returns the verified destination path.
func ensureConfinedDir(baseDir, subDir string) (string, error) {
	clean := filepath.Clean(subDir)
	cur := baseDir
	if clean == "." || clean == "" {
		return cur, nil
	}
	for _, comp := range strings.Split(clean, string(filepath.Separator)) {
		if comp == "" || comp == "." {
			continue
		}
		if comp == ".." {
			return "", fmt.Errorf("skills subdir escapes base: %q", subDir)
		}
		cur = filepath.Join(cur, comp)
		fi, lerr := os.Lstat(cur)
		switch {
		case lerr == nil:
			if fi.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("refusing symlinked skill dest ancestor: %s", cur)
			}
			if !fi.IsDir() {
				return "", fmt.Errorf("skill dest ancestor is not a directory: %s", cur)
			}
		case os.IsNotExist(lerr):
			if err := os.Mkdir(cur, 0o700); err != nil {
				return "", err
			}
		default:
			return "", lerr
		}
	}
	return cur, nil
}

// copyTree copies the tree rooted at src into dst. It is a LOCAL confinement
// point for the M15 skill copy: the source is a semi-untrusted image, and the
// running agent may write to its own cwd (which contains dst) between
// iterations. Two symlink vectors are closed here rather than relying on any
// upstream invariant (e.g. Store.Unpack stripping symlinks):
//
//  1. Source deref: filepath.Walk uses Lstat, so a symlink in the source tree
//     is reported (info.Mode()&os.ModeSymlink != 0) rather than followed. We
//     refuse it, so its target's content is never read or copied.
//  2. Destination TOCTOU: a pre-planted symlinked dest component would let a
//     MkdirAll/WriteFile write escape outside dst. Because Walk visits every
//     source dir level (and thus every dest component maps to a callback), we
//     Lstat each target first and refuse to write through an existing symlink.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		// (1) Refuse to dereference a symlink in the SOURCE tree. Walk uses
		// Lstat, so a symlink is reported rather than followed; we skip it
		// (never read or copy its target) and keep copying the rest.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		// (2) Refuse to write through an existing symlinked DEST component.
		if fi, lerr := os.Lstat(target); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlinked skill dest: %s", target)
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}
