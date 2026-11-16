// Package harness prepares the files and the command line that start a
// harness session (Claude Code, Codex, OpenCode) with an unpacked image.
package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/alekzonder/tariboy/tbox/internal/agentmode"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

// Host carries the process dependencies, so tests can fake them. Output runs
// a command with the current environment plus env, extra KEY=VALUE entries
// that may be nil, and returns its standard output. Note receives a line for
// the user about something clean left in place; nil drops it.
type Host struct {
	Getenv   func(string) string
	LookPath func(string) (string, error)
	Output   func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	Note     func(string)
}

func (h Host) note(format string, args ...any) {
	if h.Note != nil {
		h.Note(fmt.Sprintf(format, args...))
	}
}

// Launch is the prepared harness start. Env holds additions to the current
// environment as KEY=VALUE. Notes are lines the CLI prints to stderr before
// the launch.
type Launch struct {
	Path  string
	Argv  []string
	Env   []string
	Notes []string
}

// Adapter prepares one harness.
type Adapter interface {
	Name() string
	Prepare(ctx context.Context, host Host, run rundir.Run, extra []string) (Launch, error)
	// Clean removes what the adapter wrote outside $TBOX_HOME for image.
	Clean(host Host, image string) error
}

// ErrUnsupportedVersion reports a harness too old for tbox images.
type ErrUnsupportedVersion struct {
	Harness, Found, Minimum string
}

func (e ErrUnsupportedVersion) Error() string {
	return fmt.Sprintf("%s %s is not supported: version %s or newer is required", e.Harness, e.Found, e.Minimum)
}

var names = []string{"claude", "codex", "opencode"}

var adapters = map[string]Adapter{
	"claude":   claude{},
	"codex":    codex{},
	"opencode": openCode{},
}

// Names returns every harness tbox knows, in display order.
func Names() []string { return append([]string(nil), names...) }

// Get returns the adapter of a harness.
func Get(name string) (Adapter, error) {
	if a, ok := adapters[name]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("unknown harness %q (known: claude, codex, opencode)", name)
}

// Installed returns the known harnesses whose binary is on PATH.
func Installed(host Host) []string {
	var out []string
	for _, n := range names {
		if _, err := host.LookPath(n); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// CleanAll removes what tbox prepared for harness sessions: every adapter's
// files outside $TBOX_HOME and the run directories. With an image name it
// limits both to that image; its run directories are found by the image name
// in their unpacked manifest, so run directories of removed tags go too.
func CleanAll(host Host, home paths.Home, image string) error {
	var errs []error
	for _, n := range names {
		if a, ok := adapters[n]; ok {
			if err := a.Clean(host, image); err != nil {
				errs = append(errs, fmt.Errorf("clean %s: %w", n, err))
			}
		}
	}
	if image == "" {
		errs = append(errs, os.RemoveAll(home.RunRoot()))
		return errors.Join(errs...)
	}
	digests, err := os.ReadDir(home.RunRoot())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	for _, d := range digests {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(home.RunRoot(), d.Name())
		mine, readable := runOfImage(dir, image)
		switch {
		case mine:
			errs = append(errs, os.RemoveAll(dir))
		case !readable:
			host.note("tbox: kept run directory %s: its image cannot be read; tbox clean without an image removes it", dir)
		}
	}
	return errors.Join(errs...)
}

// runOfImage reports whether any harness run under the digest directory dir
// holds an unpacked image named image, and whether every run there could be
// read.
func runOfImage(dir, image string) (mine, readable bool) {
	runs, _ := os.ReadDir(dir)
	readable = true
	for _, r := range runs {
		if !r.IsDir() {
			continue
		}
		run, err := rundir.Open(filepath.Join(dir, r.Name(), "image"))
		if err != nil {
			readable = false
			continue
		}
		if run.Name == image {
			return true, true
		}
	}
	return false, readable
}

// lookPath resolves a harness binary with an error that names it.
func lookPath(host Host, name string) (string, error) {
	path, err := host.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is not installed or not on PATH: %w", name, err)
	}
	return path, nil
}

// imageEnv is the environment every image session carries.
func imageEnv(run rundir.Run) []string {
	ref := run.Name
	if run.Tag != "" {
		ref += ":" + run.Tag
	}
	return []string{agentmode.EnvImageDir + "=" + run.ImageDir, agentmode.EnvImage + "=" + ref}
}

var nonKebab = regexp.MustCompile(`[^a-z0-9-]`)

// pluginName maps an image name to the kebab-case name of its harness
// plugin: every character outside [a-z0-9-] becomes "-". Distinct image names
// can map to the same plugin name (a.b and a-b).
func pluginName(image string) string {
	return nonKebab.ReplaceAllString(image, "-")
}

// stagePrefix names the staging and retired directories of a plugin swap.
const stagePrefix = ".plugin-"

// sweepStaging removes staging directories left in dir by crashed launches.
// The caller holds the lock of dir, so none of them is in use.
func sweepStaging(dir string) {
	stages, _ := filepath.Glob(filepath.Join(dir, stagePrefix+"*"))
	for _, s := range stages {
		_ = os.RemoveAll(s)
	}
}

// swapDir puts the staged directory stage at target, a sibling of stage. An
// existing target is first renamed aside and removed after the swap, so
// target never holds a partial tree. The caller holds the lock of the parent.
func swapDir(stage, target string) error {
	if _, err := os.Lstat(target); err == nil {
		old, err := os.MkdirTemp(filepath.Dir(target), stagePrefix+"old-")
		if err != nil {
			return err
		}
		// os.Rename refuses an existing directory as the destination, so the
		// old tree moves inside the retired directory.
		if err := os.Rename(target, filepath.Join(old, "tree")); err != nil {
			os.Remove(old)
			return err
		}
		defer os.RemoveAll(old)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(stage, target)
}

var semver = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// parseVersion finds the first semantic version in s.
func parseVersion(s string) ([3]int, string, bool) {
	m := semver.FindStringSubmatch(s)
	if m == nil {
		return [3]int{}, "", false
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, "", false
		}
		v[i] = n
	}
	return v, m[0], true
}

// atLeast reports whether version text found is minimum or newer.
func atLeast(found, minimum string) bool {
	f, _, ok := parseVersion(found)
	m, _, _ := parseVersion(minimum)
	if !ok {
		return false
	}
	for i := range f {
		if f[i] != m[i] {
			return f[i] > m[i]
		}
	}
	return true
}

// promptEmpty reports whether the run prompt has no text.
func promptEmpty(run rundir.Run) (bool, error) {
	body, err := os.ReadFile(run.PromptPath)
	if err != nil {
		return false, fmt.Errorf("read prompt: %w", err)
	}
	return len(bytes.TrimSpace(body)) == 0, nil
}

// sameFile reports whether path is a regular file, not a symlink, holding
// exactly body.
func sameFile(path string, body []byte) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	old, err := os.ReadFile(path)
	return err == nil && bytes.Equal(old, body)
}

// writeIfChanged writes body to path through a temporary sibling and a rename,
// and leaves an identical file untouched.
func writeIfChanged(path string, body []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, body) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	_, werr := f.Write(body)
	if err := errors.Join(werr, f.Close()); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// copyTree copies the directories and regular files under src to dst, keeping
// permission bits. Any other entry, a symlink included, is an error, so the
// copy never contains a link.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("skill file %s is not a regular file", path)
		}
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, body, 0o600); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}
