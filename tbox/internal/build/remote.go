package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"

	"github.com/alekzonder/tariboy/tbox/internal/lock"
	srcpkg "github.com/alekzonder/tariboy/tbox/internal/source"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

// errUnknown means the fingerprint cannot be computed without the network:
// a remote entry has no lock record. Such a source always builds.
var errUnknown = errors.New("fingerprint unknown: a remote entry has no lock record")

// Update reports one remote entry whose commit or content hash changed
// during a build. Old and New are commits for git entries, hashes for url
// entries.
type Update struct {
	Key string
	Old string
	New string
}

func skillKey(e tfile.SkillEntry) string {
	if e.Git != "" {
		return lock.Key("git", e.Git, e.Path, e.Ref)
	}
	return lock.Key("url", e.URL, "", "")
}

func promptKey(e tfile.PromptEntry) string {
	if e.Git != "" {
		return lock.Key("git", e.Git, e.Path, e.Ref)
	}
	return lock.Key("url", e.URL, "", "")
}

func remoteSkill(e tfile.SkillEntry) bool   { return e.Git != "" || e.URL != "" }
func remotePrompt(e tfile.PromptEntry) bool { return e.Git != "" || e.URL != "" }

// firstRemote returns a description of the first remote entry of f, or "".
func firstRemote(f *tfile.File) string {
	for _, s := range f.Skills {
		if remoteSkill(s) {
			return "skill " + skillKey(s)
		}
	}
	for _, p := range f.Prompts {
		if remotePrompt(p) {
			return "prompt " + promptKey(p)
		}
	}
	return ""
}

// resolution is the result of resolving every remote entry of a source.
type resolution struct {
	skills  map[string]string
	prompts map[string]string
	lock    lock.File
	updates []Update
}

func changeValue(e lock.Entry) string {
	if e.Kind == "git" {
		return e.Commit
	}
	return e.SHA256
}

func (rs *resolution) record(key string, old lock.File, got lock.Entry) {
	rs.lock.Entries[key] = got
	if prev, ok := old.Entries[key]; ok && (prev.Commit != got.Commit || prev.SHA256 != got.SHA256) {
		rs.updates = append(rs.updates, Update{Key: key, Old: changeValue(prev), New: changeValue(got)})
	}
}

// resolve fetches every remote entry of f. Entries pinned in old are fetched
// at their pinned state unless update is set.
func resolve(ctx context.Context, r *srcpkg.Resolver, f *tfile.File, old lock.File, update bool) (*resolution, error) {
	rs := &resolution{
		skills:  map[string]string{},
		prompts: map[string]string{},
		lock:    lock.File{Version: 1, Entries: map[string]lock.Entry{}},
	}
	pin := func(key string) *lock.Entry {
		if e, ok := old.Entries[key]; ok && !update {
			return &e
		}
		return nil
	}
	for _, s := range f.Skills {
		if !remoteSkill(s) {
			continue
		}
		key := skillKey(s)
		if _, done := rs.skills[key]; done {
			continue
		}
		dir, got, err := r.SkillDir(ctx, s, pin(key))
		if err != nil {
			return nil, err
		}
		rs.skills[key] = dir
		rs.record(key, old, got)
	}
	for _, p := range f.Prompts {
		if !remotePrompt(p) {
			continue
		}
		key := promptKey(p)
		if _, done := rs.prompts[key]; done {
			continue
		}
		file, got, err := r.PromptFile(ctx, p, pin(key))
		if err != nil {
			return nil, err
		}
		rs.prompts[key] = file
		rs.record(key, old, got)
	}
	return rs, nil
}

// skillDir resolves a skill entry to its directory, source text and category.
func (rs *resolution) skillDir(e tfile.SkillEntry, remap tfile.Remap) (string, string, string, error) {
	if !remoteSkill(e) {
		return tfile.ResolveSkillDirWith(e, remap)
	}
	key := skillKey(e)
	return rs.skills[key], srcpkg.Describe(rs.lock.Entries[key]), "source", nil
}

// promptFile resolves a prompt entry to its file, source text and category.
func (rs *resolution) promptFile(e tfile.PromptEntry, remap tfile.Remap, skillDirs []string) (string, string, string, error) {
	if !remotePrompt(e) {
		return tfile.ResolvePromptFileWith(e, remap, skillDirs...)
	}
	key := promptKey(e)
	return rs.prompts[key], srcpkg.Describe(rs.lock.Entries[key]), "source", nil
}

func lockChanged(old, now lock.File) bool {
	return !reflect.DeepEqual(old.Entries, now.Entries)
}

func noResolverError(f *tfile.File) error {
	return fmt.Errorf("%s: remote entries need a source resolver", firstRemote(f))
}

func skillLabel(e tfile.SkillEntry) string {
	switch {
	case e.Git != "":
		return "git:" + e.Git + "#" + e.Path
	case e.URL != "":
		return "url:" + e.URL
	}
	return e.Dir
}

func promptLabel(e tfile.PromptEntry) string {
	switch {
	case e.Git != "":
		return "git:" + e.Git + "#" + e.Path
	case e.URL != "":
		return "url:" + e.URL
	}
	return e.File
}

// verifySkill compares the tree hash of a prepared remote skill to its lock
// record. A local entry always passes.
func (rs *resolution) verifySkill(e tfile.SkillEntry, tree string) error {
	if !remoteSkill(e) {
		return nil
	}
	if want := rs.lock.Entries[skillKey(e)].SHA256; tree != want {
		return fmt.Errorf("content does not match the lock (tree %s, locked %s); run tbox build --update", tree, want)
	}
	return nil
}

// verifyPrompt compares the body hash of a remote prompt to its lock record.
func (rs *resolution) verifyPrompt(e tfile.PromptEntry, body []byte) error {
	if !remotePrompt(e) {
		return nil
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if want := rs.lock.Entries[promptKey(e)].SHA256; got != want {
		return fmt.Errorf("content does not match the lock (sha256 %s, locked %s); run tbox build --update", got, want)
	}
	return nil
}
