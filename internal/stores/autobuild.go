package stores

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// ErrNotGit reports a Store whose root is not the root of a Git repository.
var ErrNotGit = errors.New("Store is not a Git repository")

// AutoExport is an automatic build's copy of one Store commit.
type AutoExport struct {
	PreparedBuild
	// Root is the exported commit; PreparedBuild.Path is the image in it.
	Root string
	// Commit is the full SHA of the exported commit.
	Commit string
	// SourcePath is the image directory in the Store working copy.
	SourcePath string
}

// PrepareAutoBuild exports the HEAD commit of the selected Store into a new
// owner-only directory under BaseDir/image-build and returns the image in it.
// Ignored and untracked files of the working copy stay out, and the working
// copy is not changed. The catalog lock is held until release, which also
// removes the export.
func (c *Catalog) PrepareAutoBuild(ctx context.Context, selector string) (AutoExport, func(), error) {
	prepared, unlock, err := c.PrepareBuild(selector)
	if err != nil {
		return AutoExport{}, nil, err
	}
	storeRoot := filepath.Dir(filepath.Dir(prepared.Path))
	export := AutoExport{SourcePath: prepared.Path}
	release := unlock
	fail := func(cause error) (AutoExport, func(), error) {
		release()
		return AutoExport{}, nil, cause
	}
	if err := gitRoot(ctx, storeRoot); err != nil {
		return fail(err)
	}
	if export.Commit, err = output(ctx, storeRoot, "git", "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		return fail(err)
	}
	buildRoot := filepath.Join(c.BaseDir, "image-build")
	if err := os.MkdirAll(buildRoot, 0o700); err != nil {
		return fail(err)
	}
	if export.Root, err = os.MkdirTemp(buildRoot, "store-auto-"); err != nil {
		return fail(err)
	}
	release = func() {
		os.RemoveAll(export.Root)
		unlock()
	}
	if err := exportCommit(ctx, storeRoot, export.Commit, export.Root); err != nil {
		return fail(err)
	}
	rel, err := filepath.Rel(storeRoot, prepared.Path)
	if err != nil {
		return fail(err)
	}
	export.PreparedBuild = PreparedBuild{Name: prepared.Name, Path: filepath.Join(export.Root, rel)}
	if err := regularFile(filepath.Join(export.Path, imagefile.DefaultFilename)); err != nil {
		return fail(&sentinelError{sentinel: ErrInvalid, msg: fmt.Sprintf("%s is not committed", filepath.ToSlash(filepath.Join(rel, imagefile.DefaultFilename)))})
	}
	if err := checkCommitPaths(export.Root, export.Path); err != nil {
		return fail(err)
	}
	return export, release, nil
}

// gitRoot fails unless dir holds .git and Git reports dir as the top level of
// its work tree.
func gitRoot(ctx context.Context, dir string) error {
	notGit := &sentinelError{sentinel: ErrNotGit, msg: fmt.Sprintf("Store root %s is not the root of a Git repository", dir)}
	if _, err := os.Lstat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		return notGit
	} else if err != nil {
		return err
	}
	top, err := output(ctx, dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return notGit
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if got, err := filepath.EvalSymlinks(top); err != nil || got != want {
		return notGit
	}
	return nil
}

// output runs a command in dir and returns its trimmed stdout.
func output(ctx context.Context, dir, name string, args ...string) (string, error) {
	var stdout bytes.Buffer
	if err := runTo(ctx, dir, &stdout, name, args...); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// exportCommit writes the tracked files of commit into dest, like
// git archive | tar -x, but refuses a symbolic link that leaves dest.
func exportCommit(ctx context.Context, storeRoot, commit, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "archive", "--format=tar", commit)
	cmd.Dir = storeRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	extractErr := extractTar(stdout, dest)
	if extractErr != nil {
		// Drain so git can exit before Wait.
		_, _ = io.Copy(io.Discard, stdout)
	}
	waitErr := cmd.Wait()
	if extractErr != nil {
		return extractErr
	}
	if waitErr != nil {
		return fmt.Errorf("git archive failed: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func extractTar(r io.Reader, dest string) error {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(header.Name)
		if !local(name) {
			return unsafeError("archive member %s points outside the Store commit", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeXGlobalHeader:
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeMember(root, name, header, tr); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if path.IsAbs(header.Linkname) || !local(path.Join(path.Dir(name), header.Linkname)) {
				return unsafeError("symlink %s points outside the Store commit", name)
			}
			if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
				return err
			}
			if err := root.Symlink(header.Linkname, name); err != nil {
				return err
			}
		default:
			return unsafeError("archive member %s has unsupported type %q", name, header.Typeflag)
		}
	}
}

func writeMember(root *os.Root, name string, header *tar.Header, body io.Reader) error {
	if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if header.Mode&0o111 != 0 {
		mode = 0o755
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, body); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func unsafeError(format string, args ...any) error {
	return &sentinelError{sentinel: ErrUnsafe, msg: fmt.Sprintf(format, args...)}
}

// local reports whether the slash-separated relative path stays inside its
// root.
func local(name string) bool {
	return name != ".." && !strings.HasPrefix(name, "../") && !path.IsAbs(name)
}

// checkCommitPaths fails when an extends layer, a skill directory or a prompt
// file of the image in root names a path outside root.
func checkCommitPaths(root, imageDir string) error {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	inside := func(target string) bool {
		rel, err := filepath.Rel(canonicalRoot, target)
		return err == nil && local(filepath.ToSlash(rel))
	}
	layers, err := imagefile.Layers(imageDir)
	if err != nil {
		return &sentinelError{sentinel: ErrInvalid, msg: err.Error()}
	}
	for _, layer := range layers {
		if !inside(layer) {
			return unsafeError("extends layer %s points outside the Store commit", layer)
		}
		src, err := imagefile.ParseV2(layer)
		if err != nil {
			return &sentinelError{sentinel: ErrInvalid, msg: err.Error()}
		}
		refs := make([]string, 0, len(src.Skills)+len(src.Prompts))
		for _, skill := range src.Skills {
			refs = append(refs, skill.Dir)
		}
		for _, prompt := range src.Prompts {
			refs = append(refs, prompt.File)
		}
		for _, ref := range refs {
			if !filepath.IsAbs(ref) && !strings.HasPrefix(ref, "../") {
				continue
			}
			target := ref
			if !filepath.IsAbs(ref) {
				target = filepath.Join(layer, filepath.FromSlash(ref))
			}
			if !inside(target) {
				rel, _ := filepath.Rel(canonicalRoot, layer)
				return unsafeError("%s in %s points outside the Store commit", ref, filepath.ToSlash(rel))
			}
		}
	}
	return nil
}
