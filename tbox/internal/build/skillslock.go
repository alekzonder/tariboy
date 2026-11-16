package build

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/skillslock"
	srcpkg "github.com/alekzonder/tariboy/tbox/internal/source"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

const pluginsWarning = "image declares plugins; skills that need the tariboy daemon will not work here"

// lockSet is one skills-lock.json and the directory that holds it.
type lockSet struct {
	dir    string
	skills map[string]skillslock.Skill
}

// restorer restores the skills of a schema v2 source from its
// skills-lock.json files on demand: only a skill that the Tariboyfile reaches
// through <root>/.agents/skills/<name> is restored, at most once.
type restorer struct {
	ctx   context.Context
	r     *srcpkg.Resolver
	roots []string
	// sets are searched in order: the image's own lock first.
	sets []lockSet
	pin  skillslock.Pin
	// within confines local lock sources to the Store root; empty for none.
	within   string
	dirs     map[string]string
	entries  map[string]lock.Entry
	keys     map[string]string
	warnings []string
	err      error
}

// readSkillLocks reads skills-lock.json in the image directory and in the
// Store root. It returns the locks found, image first, and the canonical
// directories whose .agents/skills paths they remap.
func readSkillLocks(dir, storeRoot string) ([]lockSet, []string, error) {
	var roots []string
	if storeRoot != "" {
		roots = append(roots, storeRoot)
	}
	roots = append(roots, dir)
	var sets []lockSet
	for i := len(roots) - 1; i >= 0; i-- {
		canonical, err := filepath.EvalSymlinks(roots[i])
		if err != nil {
			return nil, nil, err
		}
		roots[i] = canonical
		skills, err := skillslock.Read(filepath.Join(canonical, skillslock.Filename))
		if err != nil {
			return nil, nil, err
		}
		if skills != nil {
			sets = append(sets, lockSet{dir: canonical, skills: skills})
		}
	}
	return sets, roots, nil
}

// remap returns the path remap of the restorer; nil without one.
func (rs *restorer) remap() tfile.Remap {
	if rs == nil {
		return nil
	}
	return skillslock.Remapper(rs.roots, rs.lookup)
}

// lookup returns the restored directory of skill name. The first failure is
// kept in rs.err and stops further restores.
func (rs *restorer) lookup(name string) (string, bool) {
	if dir, ok := rs.dirs[name]; ok {
		return dir, true
	}
	if rs.err != nil {
		return "", false
	}
	for _, set := range rs.sets {
		s, ok := set.skills[name]
		if !ok {
			continue
		}
		got, err := skillslock.RestoreWithin(rs.ctx, rs.r, set.dir, rs.within, map[string]skillslock.Skill{name: s}, rs.pin)
		if err != nil {
			rs.err = fmt.Errorf("%s: %w", filepath.Join(set.dir, skillslock.Filename), err)
			return "", false
		}
		for key, e := range got.Entries {
			rs.entries[key] = e
		}
		for dir, key := range got.Keys {
			rs.keys[dir] = key
		}
		rs.warnings = append(rs.warnings, got.Warnings...)
		rs.dirs[name] = got.Dirs[name]
		return got.Dirs[name], true
	}
	return "", false
}

// prime restores every skill entry of f now, so the job knows which
// restored skills need lock records before it computes a fingerprint. A
// resolution error other than a restore failure is left to the build.
func (rs *restorer) prime(f *tfile.File) error {
	if rs == nil {
		return nil
	}
	for _, s := range f.Skills {
		if !remoteSkill(s) {
			_, _, _, _ = tfile.ResolveSkillDirWith(s, f.Remap)
		}
	}
	return rs.failure()
}

func (rs *restorer) failure() error {
	if rs == nil {
		return nil
	}
	return rs.err
}

// restoredKeys maps restored github skill directories to their lock keys.
func (rs *restorer) restoredKeys() map[string]string {
	if rs == nil {
		return nil
	}
	return rs.keys
}

func (rs *restorer) lockEntries() map[string]lock.Entry {
	if rs == nil {
		return nil
	}
	return rs.entries
}

// prepareRestore sets up the restorer of a schema v2 source that has a
// skills-lock.json in its directory or in the Store root. With a resolver it
// also opens tbox-lock.json, whose records pin the github skills.
func (j *job) prepareRestore() error {
	hdr, err := tfile.Parse(j.dir)
	if err != nil || hdr.SchemaVersion != 2 {
		// Load reports a parse error with its context.
		return nil
	}
	sets, roots, err := readSkillLocks(j.dir, j.opts.StoreRoot)
	if err != nil || len(sets) == 0 {
		return err
	}
	if j.opts.Resolver != nil {
		if err := j.openLock(); err != nil {
			return err
		}
	}
	j.restore = &restorer{
		ctx: context.Background(), r: j.opts.Resolver, roots: roots, sets: sets, pin: j.pin, within: j.opts.StoreRoot,
		dirs: map[string]string{}, entries: map[string]lock.Entry{}, keys: map[string]string{},
	}
	return nil
}

// pin returns the lock record of key unless the build updates.
func (j *job) pin(key string) *lock.Entry {
	if e, ok := j.lock.Entries[key]; ok && !j.opts.Update {
		return &e
	}
	return nil
}

// withRestored returns lk with the records of the restored github skills.
func (j *job) withRestored(lk lock.File) lock.File {
	entries := j.restore.lockEntries()
	if len(entries) == 0 {
		return lk
	}
	out := lock.File{Version: lk.Version, Entries: map[string]lock.Entry{}}
	for key, e := range lk.Entries {
		out.Entries[key] = e
	}
	for key, e := range entries {
		out.Entries[key] = e
	}
	return out
}

// recordRestored adds the restored github skills to a resolution and
// reports the ones whose commit or hash changed.
func (j *job) recordRestored(rs *resolution) {
	entries := j.restore.lockEntries()
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rs.record(key, j.lock, entries[key])
	}
}

// sourceWarnings returns the warnings of a loaded source: content that
// differs from skills-lock.json, and plugins of a schema v2 image, which is
// not marked up for tbox.
func (j *job) sourceWarnings(loaded *tfile.File) []string {
	var out []string
	if j.restore != nil {
		out = append(out, j.restore.warnings...)
	}
	if loaded.SchemaVersion == 2 && len(loaded.Plugins) > 0 {
		out = append(out, pluginsWarning)
	}
	return out
}
