// Package source fetches the remote skill and prompt entries of a
// Tariboyfile into the tbox cache. An entry that is pinned in the lock file
// and present in the cache needs no network.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/flock"
	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

// maxPromptBytes is the size limit of a remote prompt file, like a local one.
const maxPromptBytes = 4 << 20

const updateHint = "run tbox build --update to accept the new content"

// Resolver fetches remote entries. It is safe for concurrent processes: each
// source address has a file lock and every cache directory is staged and
// renamed.
type Resolver struct {
	Home paths.Home
	Git  gitx.Git
	HTTP fetch.Client
	// RewriteURL, when set, maps every git URL before it is fetched; lock keys
	// and metadata keep the original URL. Tests use it; production leaves nil.
	RewriteURL func(string) string
}

// Describe returns the skill or prompt source text of a resolved entry:
// git:<url>#<path>@<commit> or url:<url>.
func Describe(e lock.Entry) string {
	if e.Kind == "git" {
		return fmt.Sprintf("git:%s#%s@%s", e.URL, e.Path, e.Commit)
	}
	return "url:" + e.URL
}

// SkillDir returns the cached directory of a remote skill entry, named after
// the skill, and its lock entry. With pinned set, the pinned commit or hash is
// fetched; without it the entry is resolved afresh.
func (r Resolver) SkillDir(ctx context.Context, e tfile.SkillEntry, pinned *lock.Entry) (string, lock.Entry, error) {
	var dir string
	var got lock.Entry
	err := checkPinned(pinned)
	switch {
	case err != nil:
	case e.Git != "":
		dir, got, err = r.gitSkill(ctx, e, pinned)
	case e.URL != "":
		dir, got, err = r.urlSkill(ctx, e, pinned)
	default:
		return "", lock.Entry{}, errors.New("skill entry is not remote")
	}
	if err != nil {
		return "", lock.Entry{}, fmt.Errorf("skill %s: %w", address(e.Git, e.URL, e.Path), err)
	}
	return dir, got, nil
}

// PromptFile returns the cached path of a remote prompt file and its lock
// entry, with the same pinning rules as SkillDir.
func (r Resolver) PromptFile(ctx context.Context, e tfile.PromptEntry, pinned *lock.Entry) (string, lock.Entry, error) {
	var file string
	var got lock.Entry
	err := checkPinned(pinned)
	switch {
	case err != nil:
	case e.Git != "":
		file, got, err = r.gitPrompt(ctx, e, pinned)
	case e.URL != "":
		file, got, err = r.urlPrompt(ctx, e, pinned)
	default:
		return "", lock.Entry{}, errors.New("prompt entry is not remote")
	}
	if err != nil {
		return "", lock.Entry{}, fmt.Errorf("prompt %s: %w", address(e.Git, e.URL, e.Path), err)
	}
	return file, got, nil
}

func address(git, url, p string) string {
	if git != "" {
		return git + "#" + p
	}
	return url
}

func hexSum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (r Resolver) sourcesDir() string { return filepath.Join(r.Home.CacheDir(), "sources") }

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// lockSource serializes the work on one source address between processes.
func (r Resolver) lockSource(addr string) (func(), error) {
	return flock.Lock(filepath.Join(r.Home.CacheDir(), "locks", hexSum([]byte(addr))+".lock"))
}

// publish moves the staged directory to final unless another process
// published it first.
func publish(stage, final string) error {
	if err := os.Chmod(stage, 0o755); err != nil {
		return err
	}
	if err := os.Rename(stage, final); err != nil && !exists(final) {
		return err
	}
	return nil
}

// staleStage is the age after which a staging directory is a leftover of a
// crashed process.
const staleStage = time.Hour

// newStage makes a staging directory beside the cache entries and removes
// stale ones. Callers hold the lock of their source.
func (r Resolver) newStage() (string, error) {
	if err := os.MkdirAll(r.sourcesDir(), 0o755); err != nil {
		return "", err
	}
	old, _ := filepath.Glob(filepath.Join(r.sourcesDir(), ".stage-*"))
	for _, dir := range old {
		if info, err := os.Lstat(dir); err == nil && time.Since(info.ModTime()) > staleStage {
			_ = os.RemoveAll(dir)
		}
	}
	return os.MkdirTemp(r.sourcesDir(), ".stage-")
}

// checkPinned rejects a lock record that cannot verify the cache.
func checkPinned(pinned *lock.Entry) error {
	if pinned == nil {
		return nil
	}
	if pinned.SHA256 == "" || (pinned.Kind == "git" && pinned.Commit == "") {
		return errors.New("lock entry is malformed: it has no commit or sha256; " + updateHint)
	}
	return nil
}

// withPin returns base with the commit and hash of the lock record. Only
// those two fields are taken from the lock file.
func withPin(base lock.Entry, pinned *lock.Entry) lock.Entry {
	base.Commit, base.SHA256 = pinned.Commit, pinned.SHA256
	return base
}

func pinnedHash(pinned *lock.Entry) string {
	if pinned == nil {
		return ""
	}
	return pinned.SHA256
}

// fileMatches reports whether the file at path has the body hash sha.
func fileMatches(path, sha string) bool {
	body, err := fsafe.ReadFileNoFollow(path, maxPromptBytes)
	return err == nil && hexSum(body) == sha
}

// treeMeta sits next to a cached skill so its directory is found offline.
type treeMeta struct {
	Name       string `json:"name"`
	TreeSHA256 string `json:"tree_sha256"`
}

const metaName = "meta.json"

func writeMeta(stage, name, sha string) error {
	meta, _ := json.Marshal(treeMeta{Name: name, TreeSHA256: sha})
	return os.WriteFile(filepath.Join(stage, metaName), meta, 0o644)
}

// cachedTree verifies the cache entry dir offline: its meta.json must name a
// skill directory whose recomputed tree hash equals the recorded one.
func cachedTree(dir string) (name, sha string, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, metaName))
	var m treeMeta
	if err != nil || json.Unmarshal(data, &m) != nil || m.Name == "" || !filepath.IsLocal(m.Name) {
		return "", "", false
	}
	prepared, err := skill.Prepare(filepath.Join(dir, m.Name), "cache", "source")
	if err != nil || prepared.Metadata.TreeSHA256 != m.TreeSHA256 {
		return "", "", false
	}
	return m.Name, m.TreeSHA256, true
}

// leafName validates a path inside a repository and returns its last element.
func leafName(p string) (string, error) {
	if p == "" || strings.ContainsAny(p, "\\\x00") || path.IsAbs(p) {
		return "", fmt.Errorf("invalid path %q", p)
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("path %q escapes the repository", p)
	}
	return path.Base(c), nil
}

// treeSHA validates the skill at dir and returns its tree hash. It fails with
// the update hint when pinnedSHA is set and differs.
func treeSHA(dir, src, pinnedSHA string) (string, error) {
	prepared, err := skill.Prepare(dir, src, "source")
	if err != nil {
		return "", err
	}
	got := prepared.Metadata.TreeSHA256
	if pinnedSHA != "" && got != pinnedSHA {
		return "", fmt.Errorf("remote content changed since it was locked; %s", updateHint)
	}
	return got, nil
}
