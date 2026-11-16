package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
)

// The commands below read Store content outside a session. Inside a session
// the agent-mode gate of Main answers skills and skill, and rejects prompts
// and prompt.
func init() {
	commands["skills"] = runSkills
	commands["skill"] = runSkill
	commands["prompts"] = runPrompts
	commands["prompt"] = runPrompt
}

const (
	maxPromptBytes = 4 << 20
	// maxWalkEntries bounds the directory entries visited for one skill.
	maxWalkEntries = 4 * skill.MaxFilesPerSkill
)

// storeInventory is a registered Store with its inventory.
type storeInventory struct {
	store stores.Store
	inv   stores.Inventory
}

func openRegistry(env *Env) (stores.Registry, error) {
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return stores.Registry{}, err
	}
	return newRegistry(home, env.Cwd), nil
}

// readInventories reads every registered Store. A Store whose inventory
// cannot be read is reported on stderr and left out.
func readInventories(env *Env, r stores.Registry) ([]storeInventory, error) {
	list, err := r.List()
	if err != nil {
		return nil, err
	}
	var out []storeInventory
	for _, s := range list {
		inv, err := r.Inventory(s)
		if err != nil {
			fmt.Fprintf(env.Stderr, "warning: %s\n", safe(err.Error()))
			continue
		}
		out = append(out, storeInventory{s, inv})
	}
	return out, nil
}

func warnRow(env *Env, store, name, msg string) {
	label := safe(store)
	if name != "" {
		label += "/" + safe(name)
	}
	fmt.Fprintf(env.Stderr, "warning: %s: %s\n", label, safe(msg))
}

func runSkills(env *Env, args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(env.Stderr, "usage: tbox skills")
		return 2
	}
	r, err := openRegistry(env)
	if err != nil {
		return fail(env, err)
	}
	all, err := readInventories(env, r)
	if err != nil {
		return fail(env, err)
	}
	for _, si := range all {
		for _, s := range si.inv.Skills {
			if s.Err != "" {
				warnRow(env, si.store.Name, s.Name, s.Err)
				continue
			}
			fmt.Fprintf(env.Stdout, "%s/%s\t%s\n", safe(si.store.Name), safe(s.Name), safe(s.Description))
		}
	}
	return 0
}

func runPrompts(env *Env, args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(env.Stderr, "usage: tbox prompts")
		return 2
	}
	r, err := openRegistry(env)
	if err != nil {
		return fail(env, err)
	}
	all, err := readInventories(env, r)
	if err != nil {
		return fail(env, err)
	}
	for _, si := range all {
		for _, p := range si.inv.Prompts {
			if p.Err != "" {
				warnRow(env, si.store.Name, p.Name, p.Err)
				continue
			}
			fmt.Fprintf(env.Stdout, "%s/%s\n", safe(si.store.Name), safe(p.Name))
		}
	}
	return 0
}

// selectorArg parses the single STORE/NAME argument of skill and prompt. A
// bare name is an error that lists the STORE/NAME candidates.
func selectorArg(env *Env, r stores.Registry, kind string, args []string, names func(stores.Inventory) []string) (stores.Selector, error) {
	if len(args) != 1 {
		return stores.Selector{}, usageError{fmt.Sprintf("usage: tbox %s STORE/%s", kind, strings.ToUpper(kind))}
	}
	if sel, ok := stores.ParseSelector(args[0]); ok {
		return sel, nil
	}
	if strings.Contains(args[0], "/") {
		return stores.Selector{}, fmt.Errorf("%s %q is not STORE/%s", kind, safe(args[0]), strings.ToUpper(kind))
	}
	all, err := readInventories(env, r)
	if err != nil {
		return stores.Selector{}, err
	}
	var cands []string
	for _, si := range all {
		for _, n := range names(si.inv) {
			if n == args[0] {
				cands = append(cands, safe(si.store.Name)+"/"+safe(n))
			}
		}
	}
	if len(cands) == 0 {
		return stores.Selector{}, fmt.Errorf("%s %q needs a store prefix, and no store has it", kind, safe(args[0]))
	}
	return stores.Selector{}, fmt.Errorf("%s %q needs a store prefix: %s", kind, safe(args[0]), strings.Join(cands, ", "))
}

// usageError is an error that makes a command exit with status 2.
type usageError struct{ text string }

func (e usageError) Error() string { return e.text }

func failSelector(env *Env, err error) int {
	var u usageError
	if errors.As(err, &u) {
		fmt.Fprintln(env.Stderr, u.text)
		return 2
	}
	return fail(env, err)
}

func skillNames(inv stores.Inventory) []string {
	var out []string
	for _, s := range inv.Skills {
		if s.Err == "" {
			out = append(out, s.Name)
		}
	}
	return out
}

func promptNames(inv stores.Inventory) []string {
	var out []string
	for _, p := range inv.Prompts {
		if p.Err == "" {
			out = append(out, p.Name)
		}
	}
	return out
}

// storeInventoryOf reads the inventory of the Store a selector names.
func storeInventoryOf(r stores.Registry, sel stores.Selector) (stores.Inventory, error) {
	s, err := r.Get(sel.Store)
	if err != nil {
		return stores.Inventory{}, err
	}
	return r.Inventory(s)
}

func runSkill(env *Env, args []string) int {
	r, err := openRegistry(env)
	if err != nil {
		return fail(env, err)
	}
	sel, err := selectorArg(env, r, "skill", args, skillNames)
	if err != nil {
		return failSelector(env, err)
	}
	inv, err := storeInventoryOf(r, sel)
	if err != nil {
		return fail(env, err)
	}
	for _, s := range inv.Skills {
		if s.Name != sel.Name {
			continue
		}
		if s.Err != "" {
			return fail(env, fmt.Errorf("skill %s/%s: %s", sel.Store, sel.Name, s.Err))
		}
		return showSkill(env, s)
	}
	return fail(env, fmt.Errorf("store %s has no skill %q", safe(sel.Store), safe(sel.Name)))
}

func showSkill(env *Env, s stores.Skill) int {
	doc, err := fsafe.ReadFileNoFollow(filepath.Join(s.Dir, "SKILL.md"), skill.MaxFileBytes)
	if err != nil {
		return fail(env, fmt.Errorf("read SKILL.md of %s: %w", s.Name, err))
	}
	files, more := listSkillFiles(s.Dir)
	fmt.Fprintf(env.Stdout, "name: %s\ndescription: %s\nfiles:\n", safe(s.Name), safe(s.Description))
	for _, f := range files {
		fmt.Fprintf(env.Stdout, "  %s\n", safe(f))
	}
	if more {
		fmt.Fprintln(env.Stdout, "  … more files not shown")
	}
	fmt.Fprintf(env.Stdout, "\n%s", doc)
	return 0
}

// listSkillFiles returns the regular files below dir as sorted relative
// paths. It follows no symlink, skips .git, and stops at the skill file
// limit; the second result tells that files were left out.
func listSkillFiles(dir string) ([]string, bool) {
	var files []string
	var visited int
	more := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		if visited++; visited > maxWalkEntries {
			more = true
			return fs.SkipAll
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if len(files) >= skill.MaxFilesPerSkill {
			more = true
			return fs.SkipAll
		}
		if rel, rerr := filepath.Rel(dir, p); rerr == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files, more
}

func runPrompt(env *Env, args []string) int {
	r, err := openRegistry(env)
	if err != nil {
		return fail(env, err)
	}
	sel, err := selectorArg(env, r, "prompt", args, promptNames)
	if err != nil {
		return failSelector(env, err)
	}
	inv, err := storeInventoryOf(r, sel)
	if err != nil {
		return fail(env, err)
	}
	for _, p := range inv.Prompts {
		if p.Name != sel.Name {
			continue
		}
		if p.Err != "" {
			return fail(env, fmt.Errorf("prompt %s/%s: %s", sel.Store, sel.Name, p.Err))
		}
		body, err := fsafe.ReadFileNoFollow(p.Path, maxPromptBytes)
		if err != nil {
			return fail(env, fmt.Errorf("read prompt %s/%s: %w", sel.Store, sel.Name, err))
		}
		_, err = env.Stdout.Write(body)
		if err != nil {
			return fail(env, err)
		}
		return 0
	}
	return fail(env, fmt.Errorf("store %s has no prompt %q", safe(sel.Store), safe(sel.Name)))
}
