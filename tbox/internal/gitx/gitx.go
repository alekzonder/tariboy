// Package gitx runs git for tbox remote sources and Stores. Git is always
// invoked with an argument array, a timeout and no terminal prompts.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
)

const (
	defaultTimeout = 120 * time.Second
	maxStderr      = 300
)

// ErrRefNotFound marks a ref that is no branch, tag or commit of the remote.
var ErrRefNotFound = errors.New("git ref not found")

// Git runs git commands. The zero value is usable.
type Git struct {
	// Timeout bounds every git invocation; zero means 120 seconds.
	Timeout time.Duration
	// AllowLocal permits local paths, file:// and http:// remotes (tests).
	AllowLocal bool
	// Env holds extra KEY=VALUE entries appended to the environment.
	Env []string
}

// TimeoutEnv names the environment variable that sets the git timeout in
// whole seconds when Git.Timeout is zero.
const TimeoutEnv = "TBOX_GIT_TIMEOUT"

func (g Git) timeout() time.Duration {
	if g.Timeout > 0 {
		return g.Timeout
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(TimeoutEnv))); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return defaultTimeout
}

func (g Git) environ() []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return append(env, g.Env...)
}

// command builds a git command with the fixed safety options. The user's
// line-ending and attribute settings are overridden so an export does not
// depend on them; .gitattributes inside the exported tree (export-ignore,
// export-subst) still apply to git archive.
func (g Git) command(ctx context.Context, args ...string) *exec.Cmd {
	full := []string{"-c", "core.hooksPath=/dev/null", "-c", "protocol.ext.allow=never",
		"-c", "core.autocrlf=false", "-c", "core.eol=lf", "-c", "core.attributesFile=/dev/null"}
	if !g.AllowLocal {
		full = append(full, "-c", "protocol.file.allow=never")
	}
	cmd := exec.CommandContext(ctx, "git", append(full, args...)...)
	cmd.Env = g.environ()
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// gitError describes a failed git run without the command line or any
// credential: the subcommand, a timeout notice or masked stderr lines.
func gitError(ctx context.Context, op string, err error, stderr []byte) error {
	if ctx.Err() != nil {
		return &gitFail{msg: fmt.Sprintf("git %s: %v", op, ctx.Err()), err: ctx.Err()}
	}
	msg := lastLine(maskCredentials(string(stderr)))
	if msg == "" {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg = fmt.Sprintf("exit status %d", ee.ExitCode())
		} else {
			msg = err.Error()
		}
	}
	return &gitFail{msg: fmt.Sprintf("git %s: %s", op, msg), err: err}
}

// gitFail is a git failure whose text is safe to show and whose cause stays
// reachable through errors.As.
type gitFail struct {
	msg string
	err error
}

func (e *gitFail) Error() string { return e.msg }
func (e *gitFail) Unwrap() error { return e.err }

// lastLine returns the last non-empty line of s, cut to maxStderr bytes on a
// rune boundary.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	if len(line) <= maxStderr {
		return line
	}
	cut := maxStderr
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + "..."
}

// run runs git with args and returns its stdout.
func (g Git) run(ctx context.Context, op string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout())
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := g.command(ctx, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, gitError(ctx, op, err, stderr.Bytes())
	}
	return stdout.Bytes(), nil
}

// stream runs git with args and hands its stdout to use. When use fails the
// process is killed and use's error is returned.
func (g Git) stream(ctx context.Context, op string, use func(io.Reader) error, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, g.timeout())
	defer cancel()
	var stderr bytes.Buffer
	cmd := g.command(ctx, args...)
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git %s: %w", op, err)
	}
	useErr := use(out)
	if useErr != nil {
		cancel()
	}
	_, _ = io.Copy(io.Discard, out)
	waitErr := cmd.Wait()
	if useErr != nil {
		return useErr
	}
	if waitErr != nil {
		return gitError(ctx, op, waitErr, stderr.Bytes())
	}
	return nil
}

func gitDir(dir string) string { return "--git-dir=" + dir }

// mirrorRefspecs are the refs a mirror keeps: branches and tags only, not
// refs/pull/* or other hosting refs.
var mirrorRefspecs = []string{"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"}

// Mirror creates a bare mirror of the branches and tags of url in dir, or
// updates and prunes it when dir exists. A new mirror is fetched into a
// temporary sibling and renamed into place; a dir that is not a git
// repository is removed and fetched again.
func (g Git) Mirror(ctx context.Context, url, dir string) error {
	url, err := NormalizeURL(url, g.AllowLocal)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dir); err == nil {
		isRepo, err := g.isRepository(ctx, dir)
		if err != nil {
			return err
		}
		if isRepo {
			return g.updateMirror(ctx, url, dir)
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return g.newMirror(ctx, url, dir)
}

// isRepository reports whether dir is a bare git repository. It returns an
// error when the check did not run to completion (cancelled, timed out, git
// not started), so the caller never takes dir for a broken mirror then.
func (g Git) isRepository(ctx context.Context, dir string) (bool, error) {
	out, err := g.run(ctx, "rev-parse", gitDir(dir), "rev-parse", "--is-bare-repository")
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func (g Git) newMirror(ctx context.Context, url, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "."+filepath.Base(dir)+".new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if _, err := g.run(ctx, "init", "init", "--quiet", "--bare", "--", tmp); err != nil {
		return err
	}
	if _, err := g.run(ctx, "remote", gitDir(tmp), "remote", "add", "origin", "--", url); err != nil {
		return err
	}
	if err := g.fetchMirror(ctx, tmp); err != nil {
		return err
	}
	if err := g.syncHead(ctx, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		if isRepo, _ := g.isRepository(ctx, dir); !isRepo {
			return err
		}
	}
	return nil
}

func (g Git) updateMirror(ctx context.Context, url, dir string) error {
	if _, err := g.run(ctx, "remote", gitDir(dir), "remote", "set-url", "origin", "--", url); err != nil {
		return err
	}
	if err := g.fetchMirror(ctx, dir); err != nil {
		return err
	}
	return g.syncHead(ctx, dir)
}

func (g Git) fetchMirror(ctx context.Context, dir string) error {
	args := append([]string{gitDir(dir), "fetch", "--quiet", "--prune", "--no-tags", "origin"}, mirrorRefspecs...)
	_, err := g.run(ctx, "fetch", args...)
	return err
}

// HasCommit reports whether the repository in mirrorDir holds commit. A
// missing or broken mirror holds nothing.
func (g Git) HasCommit(ctx context.Context, mirrorDir, commit string) bool {
	if checkCommit(commit) != nil {
		return false
	}
	if _, err := os.Lstat(mirrorDir); err != nil {
		return false
	}
	_, err := g.run(ctx, "cat-file", gitDir(mirrorDir), "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// syncHead points the mirror's HEAD at the branch the remote HEAD names, as
// a fetch does not follow a renamed default branch. A remote without a symbolic HEAD, or one naming a
// branch the mirror lacks, leaves HEAD alone.
func (g Git) syncHead(ctx context.Context, dir string) error {
	out, err := g.run(ctx, "ls-remote", gitDir(dir), "ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return err
	}
	branch := ""
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(line, "ref: "); ok {
			branch, _, _ = strings.Cut(rest, "\t")
			break
		}
	}
	if !strings.HasPrefix(branch, "refs/heads/") || CheckRef(strings.TrimPrefix(branch, "refs/heads/")) != nil {
		return nil
	}
	if _, err := g.run(ctx, "show-ref", gitDir(dir), "show-ref", "--verify", "--quiet", branch); err != nil {
		return nil
	}
	_, err = g.run(ctx, "symbolic-ref", gitDir(dir), "symbolic-ref", "HEAD", branch)
	return err
}

// Resolve returns the full commit id of ref in the mirror in mirrorDir. An
// empty ref is the remote default branch; otherwise a branch, then a tag,
// then a 7-40 hex commit id is tried. ErrRefNotFound is returned when none
// matches.
func (g Git) Resolve(ctx context.Context, mirrorDir, ref string) (string, error) {
	return g.resolve(ctx, gitDir(mirrorDir), "refs/heads/", "HEAD", ref)
}

// resolve resolves ref in the repository selected by gitArg. Branches are
// looked up under branchPrefix and the default branch is defaultRef.
func (g Git) resolve(ctx context.Context, gitArg, branchPrefix, defaultRef, ref string) (string, error) {
	if ref == "" {
		return g.revParse(ctx, gitArg, defaultRef, true)
	}
	if err := CheckRef(ref); err != nil {
		return "", err
	}
	candidates := []string{branchPrefix + ref, "refs/tags/" + ref}
	if isHex(ref, 7, 40) {
		candidates = append(candidates, ref)
	}
	for _, c := range candidates {
		commit, err := g.revParse(ctx, gitArg, c, false)
		if err == nil {
			return commit, nil
		}
		if !errors.Is(err, ErrRefNotFound) {
			return "", err
		}
	}
	return "", fmt.Errorf("%w: %s", ErrRefNotFound, ref)
}

// revParse peels name to a commit. A miss (exit status 1 without output of
// rev-parse --verify --quiet) is ErrRefNotFound; any other failure is
// returned as its own error.
func (g Git) revParse(ctx context.Context, gitArg, name string, isDefault bool) (string, error) {
	out, err := g.run(ctx, "rev-parse", gitArg, "rev-parse", "--verify", "--quiet", name+"^{commit}")
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 && ctx.Err() == nil && strings.HasSuffix(err.Error(), "exit status 1") {
			if isDefault {
				return "", fmt.Errorf("%w: default branch", ErrRefNotFound)
			}
			return "", fmt.Errorf("%w: %s", ErrRefNotFound, name)
		}
		return "", err
	}
	commit := strings.TrimSpace(string(out))
	if !isHex(commit, 40, 40) {
		return "", fmt.Errorf("git rev-parse: unexpected output for %s", name)
	}
	return commit, nil
}

func checkCommit(commit string) error {
	if !isHex(commit, 40, 40) {
		return errors.New("commit must be a full 40-hex id")
	}
	return nil
}

// Export writes the tree of commit, or its subtree at subpath, into the new
// directory dest through git archive and the safe extractor.
func (g Git) Export(ctx context.Context, mirrorDir, commit, subpath, dest string) error {
	if err := checkCommit(commit); err != nil {
		return err
	}
	sub, err := cleanSubpath(subpath)
	if err != nil {
		return err
	}
	treeish := commit
	if sub != "" {
		treeish += ":" + sub
	}
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("destination %s already exists", dest)
	}
	use := func(r io.Reader) error {
		return fetch.ExtractTar(r, dest, fetch.DefaultLimits)
	}
	if err := g.stream(ctx, "archive", use, gitDir(mirrorDir), "archive", "--format=tar", treeish); err != nil {
		// A failed git archive can still end its stream cleanly and leave an
		// empty tree behind.
		_ = os.RemoveAll(dest)
		return err
	}
	return nil
}

// ExportFile returns the content of the file at path in commit, failing when
// it is larger than max bytes.
func (g Git) ExportFile(ctx context.Context, mirrorDir, commit, path string, max int64) ([]byte, error) {
	if err := checkCommit(commit); err != nil {
		return nil, err
	}
	clean, err := cleanSubpath(path)
	if err != nil || clean == "" {
		return nil, fmt.Errorf("invalid file path %q", path)
	}
	var body []byte
	use := func(r io.Reader) error {
		var rerr error
		body, rerr = io.ReadAll(io.LimitReader(r, max+1))
		if rerr == nil && int64(len(body)) > max {
			return fmt.Errorf("%s exceeds %d bytes", clean, max)
		}
		return rerr
	}
	err = g.stream(ctx, "cat-file", use, gitDir(mirrorDir), "cat-file", "blob", commit+":"+clean)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// Clone makes a working clone of url in the new directory dir and checks out
// ref, detached. It returns the checked-out commit.
func (g Git) Clone(ctx context.Context, url, ref, dir string) (string, error) {
	url, err := NormalizeURL(url, g.AllowLocal)
	if err != nil {
		return "", err
	}
	if ref != "" {
		if err := CheckRef(ref); err != nil {
			return "", err
		}
	}
	if _, err := os.Lstat(dir); err == nil {
		return "", fmt.Errorf("clone destination %s already exists", dir)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	commit, err := g.cloneAndCheckout(ctx, url, ref, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return commit, nil
}

func (g Git) cloneAndCheckout(ctx context.Context, url, ref, dir string) (string, error) {
	if _, err := g.run(ctx, "clone", "clone", "--no-checkout", "--", url, dir); err != nil {
		return "", err
	}
	return g.checkoutRef(ctx, dir, ref)
}

// checkoutRef resolves ref against the remote-tracking state of the clone in
// dir and checks the commit out detached.
func (g Git) checkoutRef(ctx context.Context, dir, ref string) (string, error) {
	commit, err := g.resolve(ctx, "--git-dir="+filepath.Join(dir, ".git"), "refs/remotes/origin/", "refs/remotes/origin/HEAD", ref)
	if err != nil {
		return "", err
	}
	if _, err := g.run(ctx, "checkout", "-C", dir, "checkout", "--detach", commit, "--"); err != nil {
		return "", err
	}
	return commit, nil
}

// Pull fetches the clone in dir and checks out ref, detached. A tree with
// local changes to tracked files is an error and is left untouched.
func (g Git) Pull(ctx context.Context, dir, ref string) (string, error) {
	if ref != "" {
		if err := CheckRef(ref); err != nil {
			return "", err
		}
	}
	if fi, err := os.Lstat(filepath.Join(dir, ".git")); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a git checkout", dir)
	}
	out, err := g.run(ctx, "status", "-C", dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return "", fmt.Errorf("checkout %s has local changes", dir)
	}
	if _, err := g.run(ctx, "fetch", "-C", dir, "fetch", "--prune", "--force", "--tags", "origin"); err != nil {
		return "", err
	}
	if ref == "" {
		// Best effort: follow a changed default branch of the remote.
		_, _ = g.run(ctx, "remote set-head", "-C", dir, "remote", "set-head", "origin", "--auto")
	}
	return g.checkoutRef(ctx, dir, ref)
}
