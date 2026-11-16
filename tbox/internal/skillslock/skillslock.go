// Package skillslock reads the skills-lock.json that the skills installer
// (npx skills) writes for a schema v2 image and restores its skills without
// Node: github skills through the tbox source cache, local skills in place.
// Nothing is written next to the lock file.
package skillslock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/source"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

// Filename is the lock file of the skills installer.
const Filename = "skills-lock.json"

const (
	lockVersion  = 1
	maxLockBytes = 4 << 20
	skillFile    = "SKILL.md"
)

// Skill is one entry of skills-lock.json. A github entry names the
// repository as owner/repo in Source and the SKILL.md inside it in
// SkillPath. A local entry's Source is relative to the lock's directory.
type Skill struct {
	Source       string `json:"source"`
	SourceType   string `json:"sourceType"`
	SkillPath    string `json:"skillPath,omitempty"`
	ComputedHash string `json:"computedHash"`
}

// Read loads the skills of the lock file at path, keyed by skill name. A
// missing file returns nil and no error. The file is not followed through a
// symlink.
func Read(path string) (map[string]Skill, error) {
	data, err := fsafe.ReadFileNoFollow(path, maxLockBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f struct {
		Version int              `json:"version"`
		Skills  map[string]Skill `json:"skills"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Version != lockVersion {
		return nil, fmt.Errorf("%s: unsupported skills lock version %d", path, f.Version)
	}
	for name := range f.Skills {
		if !validName(name) {
			return nil, fmt.Errorf("%s: invalid skill name %q", path, name)
		}
	}
	if f.Skills == nil {
		f.Skills = map[string]Skill{}
	}
	return f.Skills, nil
}

// validName accepts a name that is one plain path element.
func validName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}

// Pin returns the tbox-lock.json record of a lock key, or nil to resolve the
// key afresh.
type Pin func(key string) *lock.Entry

// Restored is the result of Restore.
type Restored struct {
	// Dirs maps each skill name to the directory that holds it.
	Dirs map[string]string
	// Entries are the tbox-lock.json records of the github skills.
	Entries map[string]lock.Entry
	// Keys maps the directory of each github skill to its lock key.
	Keys map[string]string
	// Warnings name the skills whose content differs from computedHash.
	Warnings []string

	within string
}

var githubSource = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Restore finds the directory of every skill. A github skill is the
// directory of SkillPath in https://github.com/<source>.git at the remote
// default branch, or at the commit pin returns, fetched through r and cached
// in a directory named after the skill; SkillPath "SKILL.md" makes the whole
// repository the skill. A local
// skill is Source relative to lockDir, used in place. Any other sourceType is
// an error. A skill whose content hash differs from computedHash gets a
// warning, not an error. pin may be nil.
func Restore(ctx context.Context, r *source.Resolver, lockDir string, skills map[string]Skill, pin Pin) (*Restored, error) {
	return RestoreWithin(ctx, r, lockDir, "", skills, pin)
}

// RestoreWithin is Restore with every local source confined to the
// directory within (a Store root): a local source that lies outside it is an
// error. An empty within confines nothing. Github skills live in the tbox
// cache and are not confined.
func RestoreWithin(ctx context.Context, r *source.Resolver, lockDir, within string, skills map[string]Skill, pin Pin) (*Restored, error) {
	base, err := filepath.EvalSymlinks(lockDir)
	if err != nil {
		return nil, err
	}
	if within != "" {
		if within, err = filepath.EvalSymlinks(within); err != nil {
			return nil, err
		}
	}
	out := &Restored{Dirs: map[string]string{}, Entries: map[string]lock.Entry{}, Keys: map[string]string{}, within: within}
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := out.restore(ctx, r, base, name, skills[name], pin); err != nil {
			return nil, fmt.Errorf("skill %s: %w", name, err)
		}
	}
	return out, nil
}

func (out *Restored) restore(ctx context.Context, r *source.Resolver, base, name string, s Skill, pin Pin) error {
	if !validName(name) {
		return errors.New("invalid skill name")
	}
	var dir string
	var err error
	switch s.SourceType {
	case "github":
		dir, err = out.github(ctx, r, name, s, pin)
	case "local":
		dir, err = local(base, name, s.Source)
		if err == nil && out.within != "" && !Inside(out.within, dir) {
			err = fmt.Errorf("local source %q points outside the Store", s.Source)
		}
	default:
		return fmt.Errorf("unsupported sourceType %q", s.SourceType)
	}
	if err != nil {
		return err
	}
	got, err := Hash(dir)
	if err != nil {
		return err
	}
	if got != s.ComputedHash {
		out.Warnings = append(out.Warnings, fmt.Sprintf("skill %s: content differs from skills-lock.json", name))
	}
	out.Dirs[name] = dir
	return nil
}

func (out *Restored) github(ctx context.Context, r *source.Resolver, name string, s Skill, pin Pin) (string, error) {
	if !githubSource.MatchString(s.Source) || strings.Contains(s.Source, "..") {
		return "", fmt.Errorf("invalid github source %q", s.Source)
	}
	// The directory that holds SKILL.md is the skill, "." for the repository
	// root. It is cached under the skill name, as the installer copies it to
	// .agents/skills/<name>; skill.Prepare then requires the frontmatter name
	// to equal it.
	sub := path.Dir(s.SkillPath)
	if path.Base(s.SkillPath) != skillFile {
		return "", fmt.Errorf("skillPath %q must end with %s", s.SkillPath, skillFile)
	}
	if r == nil {
		return "", errors.New("github skills need a source resolver")
	}
	entry := tfile.SkillEntry{Git: "https://github.com/" + s.Source + ".git", Path: sub}
	key := lock.Key("git", entry.Git, entry.Path, "")
	var pinned *lock.Entry
	if pin != nil {
		pinned = pin(key)
	}
	dir, got, err := r.GitSkillAs(ctx, entry, name, pinned)
	if err != nil {
		return "", err
	}
	// The cache may sit below a symlink; the build rejects symlinked paths.
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return "", err
	}
	out.Entries[key] = got
	out.Keys[dir] = key
	return dir, nil
}

// local returns the directory of a local skill. It must be a directory named
// after the skill, as the installer copies it to .agents/skills/<name>.
func local(base, name, src string) (string, error) {
	if src == "" || strings.ContainsRune(src, 0) {
		return "", fmt.Errorf("invalid local source %q", src)
	}
	dir := filepath.FromSlash(src)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(base, dir)
	}
	dir = filepath.Clean(dir)
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("local source %q: %w", src, err)
	}
	if !info.IsDir() || filepath.Base(dir) != name {
		return "", fmt.Errorf("local source %q must be directory named %s", src, name)
	}
	return dir, nil
}

// Inside reports whether path is root or lies below it. Both are compared
// as cleaned paths; root must be canonical.
func Inside(root, path string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Remapper returns a tfile.Remap that maps <root>/.agents/skills/<name>[/...]
// for every root to the directory lookup returns for name, keeping the rest
// of the path. A name lookup does not know, and every other path, is kept.
// roots must be canonical absolute paths.
func Remapper(roots []string, lookup func(name string) (string, bool)) tfile.Remap {
	return func(abs string) string {
		for _, root := range roots {
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				continue
			}
			parts := strings.SplitN(filepath.ToSlash(rel), "/", 4)
			if len(parts) < 3 || parts[0] != ".agents" || parts[1] != "skills" {
				continue
			}
			dir, ok := lookup(parts[2])
			if !ok {
				return abs
			}
			if len(parts) == 4 {
				return filepath.Join(dir, filepath.FromSlash(parts[3]))
			}
			return dir
		}
		return abs
	}
}
