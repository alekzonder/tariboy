// Package build turns a Tariboyfile source directory into a stored image and
// skips the work when the source did not change.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

const fingerprintHeader = "tbox-source/1\n"

// racyWindow is how close to the moment of hashing a file's mtime or change
// time may be before the cached hash is not trusted: a write in the same
// clock tick would leave the stat unchanged. Tests lower it, since they
// cannot set an old change time.
var racyWindow = 2 * time.Second

// hashHook, when set, is called with the path of every file body the
// fingerprint reads. Tests count calls to prove a rebuild check does not
// reread unchanged files.
var hashHook func(path string)

// cacheEntry is the stat of a file when it was hashed. An entry of the older
// format has no inode and change time and never matches.
type cacheEntry struct {
	Size     int64  `json:"size"`
	MtimeNS  int64  `json:"mtime_ns"`
	CtimeNS  int64  `json:"ctime_ns"`
	Inode    uint64 `json:"inode"`
	SHA256   string `json:"sha256"`
	HashedAt int64  `json:"hashed_at_ns"`
}

// statCache remembers the content hash of a file by absolute path, size,
// mtime, change time and inode: a same-size edit that restores the mtime
// (cp -p, rsync -t, tar extraction) still changes the change time or the
// inode. A nil cache hashes every file.
type statCache struct {
	old  map[string]cacheEntry
	used map[string]cacheEntry
}

func loadCache(path string) *statCache {
	c := &statCache{old: map[string]cacheEntry{}, used: map[string]cacheEntry{}}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &c.old) != nil {
			c.old = map[string]cacheEntry{}
		}
	}
	return c
}

// statOf returns the cache key fields of info; ok is false when the
// platform gives no inode and change time.
func statOf(info fs.FileInfo) (cacheEntry, bool) {
	ino, ctime, ok := statID(info)
	return cacheEntry{Size: info.Size(), MtimeNS: info.ModTime().UnixNano(), CtimeNS: ctime, Inode: ino}, ok && ino != 0 && ctime != 0
}

// trusted reports whether e describes the file whose stat is now and was
// hashed long enough after its last change.
func (e cacheEntry) trusted(now cacheEntry) bool {
	if e.Size != now.Size || e.MtimeNS != now.MtimeNS || e.CtimeNS != now.CtimeNS || e.Inode != now.Inode {
		return false
	}
	return e.HashedAt-max(now.MtimeNS, now.CtimeNS) >= int64(racyWindow)
}

// hash returns the SHA-256 of the file body at abs described by info.
func (c *statCache) hash(abs string, info fs.FileInfo) (string, error) {
	now, cacheable := statOf(info)
	if c != nil && cacheable {
		if e, ok := c.old[abs]; ok && e.trusted(now) {
			c.used[abs] = e
			return e.SHA256, nil
		}
	}
	if hashHook != nil {
		hashHook(abs)
	}
	body, err := fsafe.ReadFileNoFollow(abs, skill.MaxFileBytes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	hexSum := hex.EncodeToString(sum[:])
	if c != nil && cacheable {
		now.SHA256, now.HashedAt = hexSum, time.Now().UnixNano()
		c.used[abs] = now
	}
	return hexSum, nil
}

// save writes the entries used by the last fingerprint. It writes nothing
// when they are the entries it loaded.
func (c *statCache) save(path string) error {
	if c == nil || maps.Equal(c.used, c.old) {
		return nil
	}
	data, err := json.Marshal(c.used)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// Fingerprint is the SHA-256 of the canonical text of a source: the
// resolved, tbox-visible Tariboyfile (schema, image_version, and the skill
// and prompt entries of the whole extends chain, which Load has flattened),
// then for every packaged local skill file and prompt file its entry value,
// relative path, mode bit and content hash, then ROLE.md. Paths of the
// source directory do not enter it, and neither do plugins, runtime prompts
// and only: tariboy entries, which f.ForTbox() has dropped.
func Fingerprint(f *tfile.File) (string, error) {
	return fingerprint(f, nil, nil, nil)
}

// fpInput is what the text of one entry depends on besides the entry.
type fpInput struct {
	cache *statCache
	lk    *lock.File
	remap tfile.Remap
	// restored maps the directory of a skill restored from github through
	// skills-lock.json to its lock key.
	restored map[string]string
}

// fingerprint hashes the source. A remote entry, and a skill restored from
// github through skills-lock.json, is represented by its lock key, commit
// and hash from lk; one without a record gives errUnknown. Local entries are
// read through f.Remap.
func fingerprint(f *tfile.File, cache *statCache, lk *lock.File, restored map[string]string) (string, error) {
	in := fpInput{cache: cache, lk: lk, remap: f.Remap, restored: restored}
	var text strings.Builder
	text.WriteString(fingerprintHeader)
	fmt.Fprintf(&text, "schema %d\nimage_version %s\n", f.SchemaVersion, f.ImageVersion)
	for _, s := range f.Skills {
		if err := in.skillText(&text, s); err != nil {
			return "", err
		}
	}
	skillDirs := f.SkillDirs()
	for _, p := range f.Prompts {
		if err := in.promptText(&text, p, skillDirs); err != nil {
			return "", err
		}
	}
	role, err := tfile.RoleRead(f.Dir)
	if err != nil {
		return "", err
	}
	if role == nil {
		text.WriteString("role none\n")
	} else {
		sum := sha256.Sum256([]byte(role.Text))
		fmt.Fprintf(&text, "role %s\n", hex.EncodeToString(sum[:]))
	}
	sum := sha256.Sum256([]byte(text.String()))
	return hex.EncodeToString(sum[:]), nil
}

func modeText(info fs.FileInfo) string {
	if info.Mode().Perm()&0o100 != 0 {
		return "0700"
	}
	return "0600"
}

func remoteText(text *strings.Builder, kind, key string, lk *lock.File) error {
	if lk == nil {
		return errUnknown
	}
	e, ok := lk.Entries[key]
	if !ok {
		return errUnknown
	}
	fmt.Fprintf(text, "%s remote %q %s %s\n", kind, key, e.Commit, e.SHA256)
	return nil
}

func (in fpInput) skillText(text *strings.Builder, s tfile.SkillEntry) error {
	if remoteSkill(s) {
		return remoteText(text, "skill", skillKey(s), in.lk)
	}
	abs, source, category, err := tfile.ResolveSkillDirWith(s, in.remap)
	if err != nil {
		return err
	}
	if key, ok := in.restored[abs]; ok {
		fmt.Fprintf(text, "skill %q %s restored\n", source, category)
		return remoteText(text, "skill", key, in.lk)
	}
	fmt.Fprintf(text, "skill %q %s\n", source, category)
	return filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "evals" && d.IsDir() {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			// skill.Prepare rejects it with a precise error during the build.
			fmt.Fprintf(text, "  special %q\n", rel)
			return nil
		}
		sum, err := in.cache.hash(path, info)
		if err != nil {
			return err
		}
		fmt.Fprintf(text, "  file %q %s %s\n", rel, modeText(info), sum)
		return nil
	})
}

func (in fpInput) promptText(text *strings.Builder, p tfile.PromptEntry, skillDirs []string) error {
	if remotePrompt(p) {
		return remoteText(text, "prompt", promptKey(p), in.lk)
	}
	abs, source, category, err := tfile.ResolvePromptFileWith(p, in.remap, skillDirs...)
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	sum, err := in.cache.hash(abs, info)
	if err != nil {
		return err
	}
	fmt.Fprintf(text, "prompt %q %s %s %s\n", source, category, modeText(info), sum)
	return nil
}
