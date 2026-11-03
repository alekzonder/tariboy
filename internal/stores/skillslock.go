package stores

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const skillsLockFile = "skills-lock.json"

// ErrLockMismatch reports installed skills that differ from skills-lock.json.
var ErrLockMismatch = errors.New("skills-lock.json mismatch")

// sentinelError carries its own message and matches sentinel with errors.Is.
type sentinelError struct {
	sentinel error
	msg      string
}

func (e *sentinelError) Error() string        { return e.msg }
func (e *sentinelError) Is(target error) bool { return target == e.sentinel }

// SkillHash returns the computedHash the skills installer writes into
// skills-lock.json for the skill directory dir: sha256 over every file, each
// as its slash-separated relative path followed by its content. Files are
// ordered like JavaScript localeCompare; .git and node_modules are skipped.
func SkillHash(dir string) (string, error) {
	type member struct{ rel, path string }
	var files []member
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != dir && (entry.Name() == ".git" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, member{rel: filepath.ToSlash(rel), path: path})
		return nil
	})
	if err != nil {
		return "", err
	}
	order := collate.New(language.Und)
	sort.SliceStable(files, func(i, j int) bool { return order.CompareString(files[i].rel, files[j].rel) < 0 })
	h := sha256.New()
	for _, file := range files {
		body, err := os.ReadFile(file.path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(file.rel))
		h.Write(body)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// lockFile is one skills-lock.json an automatic build installs.
type lockFile struct {
	label string // path from the Store root, slash-separated
	body  []byte
}

type lockedSkill struct {
	hash  string
	label string
}

// LockCheck records which skills-lock.json each installer run used and then
// verifies the installed .agents/skills against them.
type LockCheck struct {
	locks    []lockFile
	next     int
	dirs     []string
	expected map[string]map[string]lockedSkill
	last     map[string]string
}

// NewLockCheck lists the skills-lock.json files an automatic build of
// imageDir installs, in installation order: the Store root's, then each
// extends layer's. root is the Store root.
func NewLockCheck(root, imageDir string) (*LockCheck, error) {
	layers, err := imagefile.Layers(imageDir)
	if err != nil {
		return nil, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	check := &LockCheck{expected: map[string]map[string]lockedSkill{}, last: map[string]string{}}
	for _, dir := range append([]string{canonicalRoot}, layers...) {
		body, err := os.ReadFile(filepath.Join(dir, skillsLockFile))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(canonicalRoot, filepath.Join(dir, skillsLockFile))
		if err != nil {
			return nil, err
		}
		check.locks = append(check.locks, lockFile{label: filepath.ToSlash(rel), body: body})
	}
	return check, nil
}

// Digest returns the lock_digest of the build: sha256 over one line
// "<path>\x00<sha256 hex of the file>\n" per lock in installation order.
func (l *LockCheck) Digest() string {
	h := sha256.New()
	for _, lock := range l.locks {
		sum := sha256.Sum256(lock.body)
		fmt.Fprintf(h, "%s\x00%s\n", lock.label, hex.EncodeToString(sum[:]))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Record notes the skills-lock.json in dir before the installer runs there.
// A directory without a lock is not recorded.
func (l *LockCheck) Record(dir string) error {
	body, err := os.ReadFile(filepath.Join(dir, skillsLockFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if l.next >= len(l.locks) || !bytes.Equal(l.locks[l.next].body, body) {
		return fmt.Errorf("skills-lock.json in %s is not the next lock of the build", dir)
	}
	lock := l.locks[l.next]
	l.next++
	var parsed struct {
		Skills map[string]struct {
			ComputedHash string `json:"computedHash"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("skills-lock.json %s: %w", lock.label, err)
	}
	skills, ok := l.expected[dir]
	if !ok {
		skills = map[string]lockedSkill{}
		l.expected[dir] = skills
		l.dirs = append(l.dirs, dir)
	}
	for name, skill := range parsed.Skills {
		skills[name] = lockedSkill{hash: skill.ComputedHash, label: lock.label}
	}
	l.last[dir] = lock.label
	return nil
}

// Verify checks every recorded directory: each locked skill is installed in
// .agents/skills with its computedHash, and no other skill is there.
func (l *LockCheck) Verify() error {
	var problems []string
	for _, dir := range l.dirs {
		expected := l.expected[dir]
		skillsDir := filepath.Join(dir, ".agents", "skills")
		names := make([]string, 0, len(expected))
		for name := range expected {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			skill := expected[name]
			prefix := fmt.Sprintf("skills-lock.json %s: skill %s: ", skill.label, name)
			skillDir := filepath.Join(skillsDir, name)
			if info, err := os.Lstat(skillDir); err != nil || !info.IsDir() {
				problems = append(problems, prefix+"is not installed")
				continue
			}
			got, err := SkillHash(skillDir)
			if err != nil {
				problems = append(problems, prefix+"read installed content: "+err.Error())
				continue
			}
			if got != skill.hash {
				problems = append(problems, fmt.Sprintf("%sinstalled content sha256:%s does not match computedHash %s", prefix, got, skill.hash))
			}
		}
		entries, err := os.ReadDir(skillsDir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, fmt.Sprintf("read %s: %v", skillsDir, err))
		}
		for _, entry := range entries {
			if _, ok := expected[entry.Name()]; !ok {
				problems = append(problems, fmt.Sprintf("skills-lock.json %s: skill %s: is installed but not in skills-lock.json", l.last[dir], entry.Name()))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &sentinelError{sentinel: ErrLockMismatch, msg: strings.Join(problems, "; ")}
}
