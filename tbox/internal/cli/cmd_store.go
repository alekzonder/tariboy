package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
	"github.com/alekzonder/tariboy/tbox/internal/textsafe"
)

func init() { commands["store"] = runStore }

// productionGit and productionHTTP are the git and https clients of the
// production resolver and Store registry. Both are zero values: remote
// addresses only, no local paths, default timeouts.
func productionGit() gitx.Git      { return gitx.Git{} }
func productionHTTP() fetch.Client { return fetch.Client{} }

// newRegistry builds the production Store registry of a tbox home. Tests of
// this package replace it to reach local repositories.
var newRegistry = func(home paths.Home, cwd string) stores.Registry {
	return stores.Registry{Home: home, Cwd: cwd, Git: productionGit(), HTTP: productionHTTP()}
}

const storeUsage = `usage: tbox store add NAME SOURCE [--ref REF] | list | show NAME | refresh NAME | ref NAME REF | remove NAME
`

var storeSubcommands = map[string]func(*Env, stores.Registry, []string) int{
	"add":     storeAdd,
	"list":    storeList,
	"show":    storeShow,
	"refresh": storeRefresh,
	"ref":     storeRef,
	"remove":  storeRemove,
}

func runStore(env *Env, args []string) int {
	var sub func(*Env, stores.Registry, []string) int
	if len(args) > 0 {
		sub = storeSubcommands[args[0]]
	}
	if sub == nil {
		io.WriteString(env.Stderr, storeUsage)
		return 2
	}
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return fail(env, err)
	}
	return sub(env, newRegistry(home, env.Cwd), args[1:])
}

func storeUsageError(env *Env, line string) int {
	fmt.Fprintf(env.Stderr, "usage: tbox store %s\n", line)
	return 2
}

// parseInterleaved parses flags that may follow positional arguments.
func parseInterleaved(env *Env, name string, args []string, define func(*flag.FlagSet)) ([]string, error) {
	fs := newFlags(name, env.Stderr)
	define(fs)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func storeAdd(env *Env, r stores.Registry, args []string) int {
	var ref string
	pos, err := parseInterleaved(env, "store add", args, func(fs *flag.FlagSet) {
		fs.StringVar(&ref, "ref", "", "git branch, tag or commit")
	})
	if err != nil {
		return 2
	}
	if len(pos) != 2 {
		return storeUsageError(env, "add NAME SOURCE [--ref REF]")
	}
	s, err := r.Add(context.Background(), pos[0], pos[1], ref)
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "added %s %s\n", s.Name, s.Kind)
	return 0
}

func storeList(env *Env, r stores.Registry, args []string) int {
	if len(args) > 0 {
		return storeUsageError(env, "list")
	}
	list, err := r.List()
	if err != nil {
		return fail(env, err)
	}
	if len(list) == 0 {
		fmt.Fprintln(env.Stdout, "no stores")
		return 0
	}
	for _, s := range list {
		fmt.Fprintf(env.Stdout, "%s\t%s\t%s\t%s\t%s\n", safe(s.Name), s.Kind, safe(s.Source), safe(s.Ref), shortCommit12(safe(s.Commit)))
	}
	return 0
}

// safe makes untrusted text printable on one line.
func safe(s string) string { return textsafe.Line(s, 300) }

func shortCommit12(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

func storeShow(env *Env, r stores.Registry, args []string) int {
	if len(args) != 1 {
		return storeUsageError(env, "show NAME")
	}
	s, err := r.Get(args[0])
	if err != nil {
		return fail(env, err)
	}
	inv, err := r.Inventory(s)
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "name: %s\nkind: %s\nsource: %s\nref: %s\ncommit: %s\n", safe(s.Name), s.Kind, safe(s.Source), safe(s.Ref), safe(s.Commit))
	printInventory(env.Stdout, inv)
	return 0
}

func printInventory(w io.Writer, inv stores.Inventory) {
	fmt.Fprintln(w, "images:")
	for _, i := range inv.Images {
		if i.Err != "" {
			fmt.Fprintf(w, "  %s\terror: %s\n", safe(i.Name), safe(i.Err))
			continue
		}
		fmt.Fprintf(w, "  %s\t%s\tschema %d\n", safe(i.Name), safe(i.Version), i.Schema)
	}
	fmt.Fprintln(w, "skills:")
	for _, s := range inv.Skills {
		if s.Err != "" {
			fmt.Fprintf(w, "  %s\terror: %s\n", safe(s.Name), safe(s.Err))
			continue
		}
		fmt.Fprintf(w, "  %s\t%s\n", safe(s.Name), safe(s.Description))
	}
	fmt.Fprintln(w, "prompts:")
	for _, p := range inv.Prompts {
		if p.Err != "" {
			fmt.Fprintf(w, "  error: %s\n", safe(p.Err))
			continue
		}
		fmt.Fprintf(w, "  %s\n", safe(p.Name))
	}
}

func storeRefresh(env *Env, r stores.Registry, args []string) int {
	if len(args) != 1 {
		return storeUsageError(env, "refresh NAME")
	}
	s, err := r.Refresh(context.Background(), args[0])
	if err != nil {
		return fail(env, err)
	}
	if s.Commit != "" {
		fmt.Fprintf(env.Stdout, "refreshed %s %s\n", safe(s.Name), shortCommit12(s.Commit))
	} else {
		fmt.Fprintf(env.Stdout, "refreshed %s\n", safe(s.Name))
	}
	return 0
}

func storeRef(env *Env, r stores.Registry, args []string) int {
	if len(args) != 2 {
		return storeUsageError(env, "ref NAME REF")
	}
	s, err := r.SetRef(context.Background(), args[0], args[1])
	if err != nil {
		return fail(env, err)
	}
	ref := s.Ref
	if ref == "" {
		ref = "default branch"
	}
	fmt.Fprintf(env.Stdout, "ref %s %s %s\n", safe(s.Name), safe(ref), shortCommit12(s.Commit))
	return 0
}

func storeRemove(env *Env, r stores.Registry, args []string) int {
	if len(args) != 1 {
		return storeUsageError(env, "remove NAME")
	}
	if err := r.Remove(args[0]); err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "removed %s\n", args[0])
	return 0
}
