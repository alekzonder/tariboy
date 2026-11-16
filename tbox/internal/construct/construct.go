// Package construct creates an image source under $TBOX_HOME/sources from
// skills and prompts that live in registered Stores. The source never names a
// Store: items are addressed by git, by absolute directory or file, or by a
// copy inside the source directory. Nothing is written into a Store.
package construct

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/flock"
	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

const staleAge = time.Hour

const (
	// DefaultVersion is the image_version of a source with no --version.
	DefaultVersion = "0.1.0"
	// InstructionsFile is the prompt the constructor writes for the author.
	InstructionsFile = "instructions.md"

	maxPromptBytes = 4 << 20
)

// Spec describes the source to create.
type Spec struct {
	Name, Version string
	Skills        []stores.Selector
	Prompts       []stores.Selector
	// NewPrompt adds an empty instructions.md after the selected prompts. It
	// is added anyway when no prompt was selected.
	NewPrompt bool
	// Force replaces an existing source directory as a whole.
	Force bool
	// Warn receives non-fatal problems, such as an old directory that could
	// not be removed after a successful replace. Nil discards them.
	Warn func(msg string)
}

type skillPlan struct {
	sel      stores.Selector
	name     string
	entry    tfile.SkillEntry
	prepared skill.Prepared
	copy     bool
	// remoteRoot marks a skill at the root of a git Store; it is validated
	// when the image is built.
	remoteRoot bool
}

type promptPlan struct {
	sel   stores.Selector
	name  string
	entry tfile.PromptEntry
	from  string
	copy  bool
}

// Create writes the source for spec and returns its directory.
func Create(reg stores.Registry, home paths.Home, spec Spec) (string, error) {
	if err := checkName(spec.Name); err != nil {
		return "", err
	}
	if spec.Version == "" {
		spec.Version = DefaultVersion
	}
	if err := tfile.ValidImageVersion(spec.Version); err != nil {
		return "", fmt.Errorf("image version: %w", err)
	}
	dir := filepath.Join(home.SourcesDir(), spec.Name)
	unlock, err := flock.Lock(filepath.Join(home.SourcesDir(), ".create-"+spec.Name+".lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	sweepStale(home.SourcesDir(), time.Now().Add(-staleAge))
	if _, err := os.Lstat(dir); err == nil && !spec.Force {
		return "", fmt.Errorf("source %s exists; use --force to replace it", dir)
	}
	pl := &planner{reg: reg, inv: map[string]loaded{}}
	skills, err := pl.skills(spec.Skills)
	if err != nil {
		return "", err
	}
	prompts, err := pl.prompts(spec.Prompts)
	if err != nil {
		return "", err
	}
	if err := writeSource(dir, spec, skills, prompts); err != nil {
		return "", err
	}
	return dir, nil
}

// checkName accepts an image name that also starts with a lower-case letter
// or digit: the sources directory keeps its own .new-* and .old-* entries,
// and a leading - reads as an option.
func checkName(name string) error {
	if err := imgstore.CheckName(name); err != nil {
		return err
	}
	if c := name[0]; (c < 'a' || c > 'z') && (c < '0' || c > '9') {
		return fmt.Errorf("invalid image name %q: it must start with a lower-case letter or digit", name)
	}
	return nil
}

type loaded struct {
	store stores.Store
	inv   stores.Inventory
}

type planner struct {
	reg stores.Registry
	inv map[string]loaded
}

func (p *planner) load(name string) (loaded, error) {
	if l, ok := p.inv[name]; ok {
		return l, nil
	}
	s, err := p.reg.Get(name)
	if err != nil {
		return loaded{}, err
	}
	if s.Kind == stores.KindDir {
		// A registry written before Add stored canonical paths may name
		// the directory through a symlink, which the build rejects.
		if canon, err := filepath.EvalSymlinks(s.Source); err == nil {
			s.Source = canon
		}
	}
	inv, err := p.reg.Inventory(s)
	if err != nil {
		return loaded{}, err
	}
	l := loaded{s, inv}
	p.inv[name] = l
	return l, nil
}

func label(s stores.Selector) string { return s.Store + "/" + s.Name }

func (p *planner) skills(sels []stores.Selector) ([]skillPlan, error) {
	var out []skillPlan
	seen := map[string]stores.Selector{}
	for _, sel := range sels {
		sp, err := p.skill(sel)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[sp.name]; dup {
			return nil, fmt.Errorf("skills %s and %s have the same name %q", label(prev), label(sel), sp.name)
		}
		seen[sp.name] = sel
		out = append(out, sp)
	}
	return out, nil
}

func (p *planner) skill(sel stores.Selector) (skillPlan, error) {
	l, err := p.load(sel.Store)
	if err != nil {
		return skillPlan{}, err
	}
	var row *stores.Skill
	var names []string
	for i, s := range l.inv.Skills {
		if s.Name == "" {
			continue
		}
		names = append(names, s.Name)
		if s.Name == sel.Name && row == nil {
			// The first row of a name is the one used; later ones are
			// duplicates.
			row = &l.inv.Skills[i]
		}
	}
	if row == nil {
		return skillPlan{}, fmt.Errorf("store %s has no skill %q (has: %s)", sel.Store, sel.Name, listNames(names))
	}
	if row.Err != "" {
		return skillPlan{}, fmt.Errorf("skill %s is not usable: %s", label(sel), row.Err)
	}
	if row.Dir == p.reg.Root(l.store) && holdsStoreContent(row.Dir) {
		return skillPlan{}, fmt.Errorf("skill %s sits at the root of a Store that also holds images or skills; that root holds Store content, not a single skill", label(sel))
	}
	sp := skillPlan{sel: sel, name: row.Name}
	if err := p.address(&sp, l.store, row); err != nil {
		return skillPlan{}, fmt.Errorf("skill %s: %w", label(sel), err)
	}
	if sp.remoteRoot {
		return sp, nil
	}
	if sp.prepared, err = p.prepare(row, sp.copy && row.Dir == p.reg.Root(l.store)); err != nil {
		return skillPlan{}, fmt.Errorf("skill %s is not usable: %w", label(sel), err)
	}
	return sp, nil
}

// address fills the entry of sp by the kind of the Store that holds it.
func (p *planner) address(sp *skillPlan, s stores.Store, row *stores.Skill) error {
	switch s.Kind {
	case stores.KindGit:
		rel, err := p.relative(s, row.Dir)
		if err != nil {
			return err
		}
		sp.remoteRoot = rel == "."
		sp.entry = tfile.SkillEntry{Git: s.Source, Path: rel, Ref: s.Commit}
	case stores.KindDir:
		if row.Dir == p.reg.Root(s) {
			sp.copy = true
			sp.entry = tfile.SkillEntry{Dir: "./skills/" + row.Name}
			return nil
		}
		sp.entry = tfile.SkillEntry{Dir: row.Dir}
	default:
		sp.copy = true
		sp.entry = tfile.SkillEntry{Dir: "./skills/" + row.Name}
	}
	return nil
}

// relative returns the slash path of abs inside the Store root.
func (p *planner) relative(s stores.Store, abs string) (string, error) {
	rel, err := filepath.Rel(p.reg.Root(s), abs)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%s is outside the store", abs)
	}
	return filepath.ToSlash(rel), nil
}

func (p *planner) prompts(sels []stores.Selector) ([]promptPlan, error) {
	var out []promptPlan
	copied := map[string]stores.Selector{}
	for _, sel := range sels {
		pp, err := p.prompt(sel)
		if err != nil {
			return nil, err
		}
		if pp.copy {
			if prev, dup := copied[pp.name]; dup {
				return nil, fmt.Errorf("prompts %s and %s would both be copied to prompts/%s.md", label(prev), label(sel), pp.name)
			}
			copied[pp.name] = sel
		}
		out = append(out, pp)
	}
	return out, nil
}

func (p *planner) prompt(sel stores.Selector) (promptPlan, error) {
	l, err := p.load(sel.Store)
	if err != nil {
		return promptPlan{}, err
	}
	var row *stores.Prompt
	var names []string
	for i, pr := range l.inv.Prompts {
		if pr.Name == "" {
			continue
		}
		names = append(names, pr.Name)
		if pr.Name == sel.Name {
			row = &l.inv.Prompts[i]
		}
	}
	if row == nil {
		return promptPlan{}, fmt.Errorf("store %s has no prompt %q (has: %s)", sel.Store, sel.Name, listNames(names))
	}
	if row.Err != "" {
		return promptPlan{}, fmt.Errorf("prompt %s is not usable: %s", label(sel), row.Err)
	}
	pp := promptPlan{sel: sel, name: row.Name, from: row.Path}
	switch l.store.Kind {
	case stores.KindGit:
		rel, err := p.relative(l.store, row.Path)
		if err != nil {
			return promptPlan{}, fmt.Errorf("prompt %s: %w", label(sel), err)
		}
		pp.entry = tfile.PromptEntry{Git: l.store.Source, Path: rel, Ref: l.store.Commit}
	case stores.KindDir:
		pp.entry = tfile.PromptEntry{File: row.Path}
	default:
		pp.copy = true
		pp.entry = tfile.PromptEntry{File: "./prompts/" + row.Name + ".md"}
	}
	return pp, nil
}

func listNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// writeSource stages the source in a sibling directory, checks that it
// parses, and renames it into place.
func writeSource(dir string, spec Spec, skills []skillPlan, prompts []promptPlan) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".new-"+spec.Name+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	file := &tfile.File{SchemaVersion: 3, ImageVersion: spec.Version}
	for _, s := range skills {
		file.Skills = append(file.Skills, s.entry)
		if s.copy {
			if err := copySkill(filepath.Join(stage, "skills", s.name), s.prepared); err != nil {
				return err
			}
		}
	}
	for _, pr := range prompts {
		file.Prompts = append(file.Prompts, pr.entry)
		if pr.copy {
			if err := copyPrompt(filepath.Join(stage, "prompts", pr.name+".md"), pr.from); err != nil {
				return fmt.Errorf("prompt %s: %w", label(pr.sel), err)
			}
		}
	}
	if spec.NewPrompt || len(prompts) == 0 {
		file.Prompts = append(file.Prompts, tfile.PromptEntry{File: "./" + InstructionsFile})
		body := "# " + spec.Name + "\n\nDescribe the process of this agent here.\n"
		if err := os.WriteFile(filepath.Join(stage, InstructionsFile), []byte(body), 0o644); err != nil {
			return err
		}
	}
	data, err := tfile.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, tfile.Filename), data, 0o644); err != nil {
		return err
	}
	if _, err := tfile.Parse(stage); err != nil {
		return fmt.Errorf("constructed source is invalid: %w", err)
	}
	return replace(stage, dir, spec.Force, spec.Warn)
}

// replace renames stage to dir. With force an existing dir is moved aside and
// removed only after the rename succeeds.
func replace(stage, dir string, force bool, warn func(string)) error {
	if _, err := os.Lstat(dir); err == nil {
		if !force {
			return fmt.Errorf("source %s exists; use --force to replace it", dir)
		}
		old, err := os.MkdirTemp(filepath.Dir(dir), ".old-")
		if err != nil {
			return err
		}
		aside := filepath.Join(old, "src")
		if err := os.Rename(dir, aside); err != nil {
			_ = os.Remove(old)
			return err
		}
		if err := os.Rename(stage, dir); err != nil {
			_ = os.Rename(aside, dir)
			_ = os.Remove(old)
			return err
		}
		if err := os.RemoveAll(old); err != nil && warn != nil {
			warn(fmt.Sprintf("could not remove the replaced source at %s: %v", old, err))
		}
		return nil
	}
	if err := os.Rename(stage, dir); err != nil {
		if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("source %s exists; use --force to replace it", dir)
		}
		return err
	}
	return nil
}

func copySkill(dest string, p skill.Prepared) error {
	for _, f := range p.Files {
		path := filepath.Join(dest, filepath.FromSlash(f.RelativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(path, f.Body, mode); err != nil {
			return err
		}
	}
	return nil
}

func copyPrompt(dest, from string) error {
	body, err := fsafe.ReadFileNoFollow(from, maxPromptBytes)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
}

// holdsStoreContent reports whether root, which has a SKILL.md, is a Store
// with content rather than a single-skill repository: it has an images/
// directory or a skills/<name>/SKILL.md.
func holdsStoreContent(root string) bool {
	if fi, err := os.Lstat(filepath.Join(root, "images")); err == nil && fi.IsDir() {
		return true
	}
	found, _ := filepath.Glob(filepath.Join(root, "skills", "*", "SKILL.md"))
	return len(found) > 0
}

// prepare validates the skill of row. A skill at a Store root is the whole
// root except its dot-directories and evals/, as for every Store kind; it is
// first copied into a directory named after the skill.
func (p *planner) prepare(row *stores.Skill, atRoot bool) (skill.Prepared, error) {
	if !atRoot {
		return skill.Prepare(row.Dir, "constructor", "source")
	}
	tmp, err := os.MkdirTemp("", "tbox-rootskill-")
	if err != nil {
		return skill.Prepared{}, err
	}
	defer os.RemoveAll(tmp)
	dest := filepath.Join(tmp, row.Name)
	if err := copyRootSkill(row.Dir, dest); err != nil {
		return skill.Prepared{}, err
	}
	return skill.Prepare(dest, "constructor", "source")
}

// copyRootSkill copies the Store root src to dest without its top-level
// dot-directories and evals/.
// Only regular files and directories are copied; anything else is an error.
func copyRootSkill(src, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries := 0
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." {
			return nil
		}
		if entries++; entries > 4*skill.MaxFilesPerSkill {
			return errors.New("skill has too many entries")
		}
		if d.IsDir() {
			if !strings.Contains(filepath.ToSlash(rel), "/") && (strings.HasPrefix(rel, ".") || rel == "evals") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dest, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", rel)
		}
		body, err := fsafe.ReadFileNoFollow(path, skill.MaxFileBytes)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, ierr := d.Info(); ierr == nil && info.Mode()&0o100 != 0 {
			mode = 0o755
		}
		return os.WriteFile(filepath.Join(dest, rel), body, mode)
	})
}

// sweepStale removes staging and moved-aside directories older than cutoff.
func sweepStale(parent string, cutoff time.Time) {
	for _, pat := range []string{".new-*", ".old-*"} {
		matches, _ := filepath.Glob(filepath.Join(parent, pat))
		for _, m := range matches {
			if fi, err := os.Lstat(m); err == nil && fi.IsDir() && fi.ModTime().Before(cutoff) {
				_ = os.RemoveAll(m)
			}
		}
	}
}
