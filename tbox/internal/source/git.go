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
	if pinned != nil {
		if dir := filepath.Join(r.treeDir(key, pinned.Commit), name); exists(dir) {
			return dir, *pinned, nil
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
	got := lock.Entry{Kind: "git", URL: e.Git, Path: e.Path, Ref: e.Ref, Commit: commit}
	pinnedSHA := ""
	if pinned != nil {
		pinnedSHA = pinned.SHA256
	}
	final := r.treeDir(key, commit)
	if !exists(filepath.Join(final, name)) {
		if err := r.exportTree(ctx, e, commit, name, final, pinnedSHA); err != nil {
			return "", lock.Entry{}, err
		}
	}
	dir := filepath.Join(final, name)
	if got.SHA256, err = treeSHA(dir, Describe(got), pinnedSHA); err != nil {
		return "", lock.Entry{}, err
	}
	return dir, got, nil
}

// exportTree exports the skill subtree of commit into <final>/<name> through
// a staging directory.
func (r Resolver) exportTree(ctx context.Context, e tfile.SkillEntry, commit, name, final, pinnedSHA string) error {
	stage, err := r.newStage()
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	dir := filepath.Join(stage, name)
	if err := r.Git.Export(ctx, r.mirrorDir(e.Git), commit, e.Path, dir); err != nil {
		return err
	}
	if _, err := treeSHA(dir, "git:"+e.Git, pinnedSHA); err != nil {
		return err
	}
	return publish(stage, final)
}

func (r Resolver) gitPrompt(ctx context.Context, e tfile.PromptEntry, pinned *lock.Entry) (string, lock.Entry, error) {
	name, err := leafName(e.Path)
	if err != nil {
		return "", lock.Entry{}, err
	}
	key := lock.Key("git", e.Git, e.Path, e.Ref)
	if pinned != nil {
		if file := filepath.Join(r.treeDir(key, pinned.Commit), name); exists(file) {
			return file, *pinned, nil
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
	got := lock.Entry{Kind: "git", URL: e.Git, Path: e.Path, Ref: e.Ref, Commit: commit, SHA256: hexSum(body)}
	if pinned != nil && pinned.SHA256 != "" && pinned.SHA256 != got.SHA256 {
		return "", lock.Entry{}, fmt.Errorf("remote content changed since it was locked; %s", updateHint)
	}
	file, err := r.storeFile(r.treeDir(key, commit), name, body)
	return file, got, err
}

// storeFile publishes body as <final>/<name> through a staging directory.
func (r Resolver) storeFile(final, name string, body []byte) (string, error) {
	file := filepath.Join(final, name)
	if exists(file) {
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
	return file, publish(stage, final)
}
