package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

func (r Resolver) mirrorDir(url string) string {
	return filepath.Join(r.Home.CacheDir(), "git", hexSum([]byte(url)))
}

func (r Resolver) treeDir(key, commit string) string {
	return filepath.Join(r.sourcesDir(), hexSum([]byte(key+commit)))
}

// gitCommit makes the mirror of url current and returns the commit to use:
// the pinned one, or the resolution of ref.
func (r Resolver) gitCommit(ctx context.Context, url, ref string, pinned *lock.Entry) (string, error) {
	fetchURL := url
	if r.RewriteURL != nil {
		fetchURL = r.RewriteURL(url)
	}
	mirror := r.mirrorDir(url)
	if err := r.Git.Mirror(ctx, fetchURL, mirror); err != nil {
		return "", err
	}
	if pinned != nil {
		return pinned.Commit, nil
	}
	return r.Git.Resolve(ctx, mirror, ref)
}

func (r Resolver) gitSkill(ctx context.Context, e tfile.SkillEntry, pinned *lock.Entry) (string, lock.Entry, error) {
	name, err := leafName(e.Path)
	if err != nil {
		return "", lock.Entry{}, err
	}
	key := lock.Key("git", e.Git, e.Path, e.Ref)
	base := lock.Entry{Kind: "git", URL: e.Git, Path: e.Path, Ref: e.Ref}
	if pinned != nil {
		final := r.treeDir(key, pinned.Commit)
		if n, sha, ok := cachedTree(final); ok && n == name && sha == pinned.SHA256 {
			return filepath.Join(final, name), withPin(base, pinned), nil
		}
	}
	unlock, err := r.lockSource(e.Git)
	if err != nil {
		return "", lock.Entry{}, err
	}
	defer unlock()
	commit, err := r.gitCommit(ctx, e.Git, e.Ref, pinned)
	if err != nil {
		return "", lock.Entry{}, err
	}
	base.Commit = commit
	final := r.treeDir(key, commit)
	n, sha, ok := cachedTree(final)
	if !ok || n != name || (pinned != nil && sha != pinned.SHA256) {
		if sha, err = r.exportTree(ctx, e, commit, name, final, pinnedHash(pinned)); err != nil {
			return "", lock.Entry{}, err
		}
	}
	base.SHA256 = sha
	return filepath.Join(final, name), base, nil
}

// exportTree replaces the cache entry final with the skill subtree of commit,
// exported through a staging directory, and returns its tree hash.
func (r Resolver) exportTree(ctx context.Context, e tfile.SkillEntry, commit, name, final, pinnedSHA string) (string, error) {
	stage, err := r.newStage()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	dir := filepath.Join(stage, name)
	if err := r.Git.Export(ctx, r.mirrorDir(e.Git), commit, e.Path, dir); err != nil {
		return "", err
	}
	sha, err := treeSHA(dir, "git:"+e.Git, pinnedSHA)
	if err != nil {
		return "", err
	}
	if err := writeMeta(stage, name, sha); err != nil {
		return "", err
	}
	if err := os.RemoveAll(final); err != nil {
		return "", err
	}
	return sha, publish(stage, final)
}

func (r Resolver) gitPrompt(ctx context.Context, e tfile.PromptEntry, pinned *lock.Entry) (string, lock.Entry, error) {
	name, err := leafName(e.Path)
	if err != nil {
		return "", lock.Entry{}, err
	}
	key := lock.Key("git", e.Git, e.Path, e.Ref)
	base := lock.Entry{Kind: "git", URL: e.Git, Path: e.Path, Ref: e.Ref}
	if pinned != nil {
		if file := filepath.Join(r.treeDir(key, pinned.Commit), name); fileMatches(file, pinned.SHA256) {
			return file, withPin(base, pinned), nil
		}
	}
	unlock, err := r.lockSource(e.Git)
	if err != nil {
		return "", lock.Entry{}, err
	}
	defer unlock()
	commit, err := r.gitCommit(ctx, e.Git, e.Ref, pinned)
	if err != nil {
		return "", lock.Entry{}, err
	}
	body, err := r.Git.ExportFile(ctx, r.mirrorDir(e.Git), commit, e.Path, maxPromptBytes)
	if err != nil {
		return "", lock.Entry{}, err
	}
	base.Commit, base.SHA256 = commit, hexSum(body)
	if pinned != nil && pinned.SHA256 != base.SHA256 {
		return "", lock.Entry{}, fmt.Errorf("remote content changed since it was locked; %s", updateHint)
	}
	file, err := r.storeFile(r.treeDir(key, commit), name, body)
	return file, base, err
}

// storeFile publishes body as <final>/<name> through a staging directory. An
// existing entry with the same body is kept; any other is replaced.
func (r Resolver) storeFile(final, name string, body []byte) (string, error) {
	file := filepath.Join(final, name)
	if fileMatches(file, hexSum(body)) {
		return file, nil
	}
	stage, err := r.newStage()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := os.WriteFile(filepath.Join(stage, name), body, 0o644); err != nil {
		return "", err
	}
	if err := os.RemoveAll(final); err != nil {
		return "", err
	}
	return file, publish(stage, final)
}
