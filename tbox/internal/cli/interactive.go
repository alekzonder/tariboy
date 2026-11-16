package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/harness"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
	"github.com/alekzonder/tariboy/tbox/internal/tui"
)

const (
	exitUsage     = 2
	exitCancelled = 130

	newEmptyPromptLabel = "new empty prompt (instructions.md)"
	noContentText       = "no skills or prompts found; add a Store with: tbox store add NAME SOURCE"
	noImagesText        = "no images found; build one with: tbox build --path DIR, or add a Store with: tbox store add NAME SOURCE"
	runNoTerminalHint   = "tbox run: IMAGE and HARNESS are required when stdin or stderr is not a terminal\n" + runUsage
	newNoTerminalHint   = "tbox image new: NAME and one of --skill, --prompt or --new-prompt are required when stdin or stderr is not a terminal\n" + imageNewUsage + "\n"
)

// exitError ends an interactive command with a code and a message that is
// printed after the terminal is restored.
type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string { return e.msg }

// withTerminal opens the terminal, runs fn and restores the terminal exactly
// once on every path, including a panic. Nothing may be printed to stdout or
// stderr inside fn.
func withTerminal(env *Env, fn func(tui.Terminal) error) error {
	if env.OpenTerminal == nil {
		return tui.ErrNoTerminal
	}
	t, err := env.OpenTerminal()
	if err != nil {
		return err
	}
	defer func() { _ = t.Close() }()
	return fn(t)
}

// interactiveExit prints what an interactive failure needs and returns the
// exit code. noTerminal is the hint printed when there is no terminal.
func interactiveExit(env *Env, err error, noTerminal string) int {
	var ee exitError
	switch {
	case errors.Is(err, tui.ErrNoTerminal):
		fmt.Fprint(env.Stderr, noTerminal)
		return exitUsage
	case errors.Is(err, tui.ErrCancelled):
		fmt.Fprintln(env.Stderr, "tbox: cancelled")
		return exitCancelled
	case errors.As(err, &ee):
		fmt.Fprintf(env.Stderr, "tbox: %s\n", safe(ee.msg))
		return ee.code
	}
	return fail(env, err)
}

// quietEnv returns a copy of env whose stderr is buf, so warnings raised
// while the terminal is raw are printed afterwards.
func quietEnv(env *Env, buf *bytes.Buffer) *Env {
	c := *env
	c.Stderr = buf
	return &c
}

type newPick struct {
	name      string
	skills    []stores.Selector
	prompts   []stores.Selector
	newPrompt bool
}

// checkNewName reports why name cannot be the name of a new source: an
// invalid image name, or an existing source when force is not set.
func checkNewName(home paths.Home, name string, force bool) error {
	_, err := newNameProblem(home, name, force)
	return err
}

// newNameProblem returns a short reason for the prompt and the full error,
// both empty when name is acceptable.
func newNameProblem(home paths.Home, name string, force bool) (string, error) {
	if err := imgstore.CheckName(name); err != nil {
		return err.Error(), err
	}
	dir := filepath.Join(home.SourcesDir(), name)
	if _, err := os.Lstat(dir); err == nil && !force {
		return "source " + name + " exists; choose another name", fmt.Errorf("source %s exists; use --force to replace it", dir)
	}
	return "", nil
}

// askName reads the image name until checkNewName accepts it; the prompt of
// each retry carries the reason.
func askName(t tui.Terminal, home paths.Home, force bool) (string, error) {
	prompt := "image name"
	for {
		in, err := tui.Input(t, prompt, "")
		if err != nil {
			return "", err
		}
		name := strings.TrimSpace(in)
		reason, cerr := newNameProblem(home, name, force)
		if cerr == nil {
			return name, nil
		}
		prompt = "image name (" + reason + ")"
	}
}

// pickNew asks for the name (when empty), the skills and the prompts.
func pickNew(env *Env, home paths.Home, name string, force bool) (newPick, error) {
	var buf bytes.Buffer
	pick := newPick{name: name}
	err := withTerminal(env, func(t tui.Terminal) error {
		all, err := readInventories(quietEnv(env, &buf), newRegistry(home, env.Cwd))
		if err != nil {
			return err
		}
		skillItems, skillSels, promptItems, promptSels := libraryItems(all)
		if len(skillItems) == 0 && len(promptItems) == 0 {
			return exitError{1, noContentText}
		}
		if pick.name == "" {
			if pick.name, err = askName(t, home, force); err != nil {
				return err
			}
		}
		return pickItems(t, &pick, skillItems, skillSels, promptItems, promptSels)
	})
	_, _ = env.Stderr.Write(buf.Bytes())
	return pick, err
}

func pickItems(t tui.Terminal, pick *newPick, skillItems []tui.Item, skillSels []stores.Selector, promptItems []tui.Item, promptSels []stores.Selector) error {
	if len(skillItems) > 0 {
		idx, err := tui.MultiSelect(t, "skills (Space toggles)", skillItems, nil)
		if err != nil {
			return err
		}
		for _, i := range idx {
			pick.skills = append(pick.skills, skillSels[i])
		}
	}
	items := append([]tui.Item{{Label: newEmptyPromptLabel}}, promptItems...)
	idx, err := tui.MultiSelect(t, "prompts (Space toggles)", items, nil)
	if err != nil {
		return err
	}
	for _, i := range idx {
		if i == 0 {
			pick.newPrompt = true
			continue
		}
		pick.prompts = append(pick.prompts, promptSels[i-1])
	}
	return nil
}

// libraryItems lists every skill and prompt of every Store without a problem.
func libraryItems(all []storeInventory) (skills []tui.Item, skillSels []stores.Selector, prompts []tui.Item, promptSels []stores.Selector) {
	for _, si := range all {
		for _, s := range si.inv.Skills {
			if s.Err != "" || s.Name == "" {
				continue
			}
			skills = append(skills, tui.Item{Label: si.store.Name + "/" + s.Name, Detail: s.Description})
			skillSels = append(skillSels, stores.Selector{Store: si.store.Name, Name: s.Name})
		}
		for _, p := range si.inv.Prompts {
			if p.Err != "" || p.Name == "" {
				continue
			}
			prompts = append(prompts, tui.Item{Label: si.store.Name + "/" + p.Name})
			promptSels = append(promptSels, stores.Selector{Store: si.store.Name, Name: p.Name})
		}
	}
	return
}

// completeRun fills the missing positional arguments of tbox run from lists.
// pos holds the arguments given; want is 2 for IMAGE HARNESS and 1 for
// --path. The returned note is printed after the terminal is restored.
func completeRun(env *Env, pos []string, want int) (out []string, note string, err error) {
	var buf bytes.Buffer
	out = slices.Clone(pos) // pos aliases the arguments after "--"
	err = withTerminal(env, func(t tui.Terminal) error {
		// The harness check comes before any list is drawn.
		installed := harness.Installed(harnessHost(env))
		if len(installed) == 0 {
			return exitError{1, "no harness is installed (looked for " + strings.Join(harness.Names(), ", ") + " on PATH)"}
		}
		if want == 2 && len(out) == 0 {
			ref, err := pickImage(quietEnv(env, &buf), t)
			if err != nil {
				return err
			}
			out = append(out, ref)
		}
		name, n, err := pickHarness(t, installed)
		if err != nil {
			return err
		}
		out, note = append(out, name), n
		return nil
	})
	_, _ = env.Stderr.Write(buf.Bytes())
	return out, note, err
}

// pickHarness chooses one of the installed harnesses, of which there is at
// least one; a single one is taken without a list.
func pickHarness(t tui.Terminal, names []string) (name, note string, err error) {
	if len(names) == 1 {
		return names[0], "using harness " + names[0], nil
	}
	items := make([]tui.Item, len(names))
	for i, n := range names {
		items[i] = tui.Item{Label: n}
	}
	i, err := tui.Select(t, "harness", items)
	if err != nil {
		return "", "", err
	}
	return names[i], "", nil
}

// pickImage lists the built images, then the images of the Stores.
func pickImage(env *Env, t tui.Terminal) (string, error) {
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return "", err
	}
	built, err := imgstore.Store{Dir: home.ImagesDir()}.List()
	if err != nil {
		return "", err
	}
	var items []tui.Item
	var refs []string
	have := map[[2]string]bool{}
	for _, e := range built {
		have[[2]string{e.Name, e.ImageVersion}] = true
		items = append(items, tui.Item{Label: e.Name + ":" + e.Tag, Detail: strings.TrimSpace(e.ImageVersion + " " + short(e.Digest))})
		refs = append(refs, e.Name+":"+e.Tag)
	}
	if r, rerr := openRegistry(env); rerr == nil {
		if all, ierr := readInventories(env, r); ierr == nil {
			for _, si := range all {
				for _, i := range si.inv.Images {
					if i.Err != "" || i.Name == "" {
						continue
					}
					status := "not built"
					if have[[2]string{i.Name, i.Version}] {
						status = "built"
					}
					items = append(items, tui.Item{Label: si.store.Name + "/" + i.Name, Detail: strings.TrimSpace(i.Version + " " + status)})
					refs = append(refs, si.store.Name+"/"+i.Name)
				}
			}
		}
	}
	if len(items) == 0 {
		return "", exitError{1, noImagesText}
	}
	i, err := tui.Select(t, "image", items)
	if err != nil {
		return "", err
	}
	return refs[i], nil
}
