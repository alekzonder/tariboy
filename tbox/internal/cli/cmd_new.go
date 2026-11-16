package cli

import (
	"flag"
	"fmt"

	"github.com/alekzonder/tariboy/tbox/internal/build"
	"github.com/alekzonder/tariboy/tbox/internal/construct"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
)

const imageNewUsage = "usage: tbox image new NAME [--skill STORE/SKILL]... [--prompt STORE/PROMPT]... [--new-prompt] [--version V] [--force] [--no-build]"

// selectorList is a repeatable flag of STORE/NAME selectors.
type selectorList struct {
	list []stores.Selector
}

func (s *selectorList) String() string { return fmt.Sprint(len(s.list)) }

func (s *selectorList) Set(v string) error {
	sel, ok := stores.ParseSelector(v)
	if !ok {
		return fmt.Errorf("%q is not STORE/NAME", v)
	}
	s.list = append(s.list, sel)
	return nil
}

func runImageNew(env *Env, args []string) int {
	var skills, prompts selectorList
	var version string
	var newPrompt, force, noBuild bool
	pos, err := parseInterleaved(env, "image new", args, func(fs *flag.FlagSet) {
		fs.Var(&skills, "skill", "skill to add as STORE/SKILL; repeatable")
		fs.Var(&prompts, "prompt", "prompt to add as STORE/PROMPT; repeatable")
		fs.BoolVar(&newPrompt, "new-prompt", false, "add an empty instructions.md after the prompts")
		fs.StringVar(&version, "version", "", "image_version (default "+construct.DefaultVersion+")")
		fs.BoolVar(&force, "force", false, "replace an existing source")
		fs.BoolVar(&noBuild, "no-build", false, "create the source without building it")
	})
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(env.Stderr, imageNewUsage)
		return 2
	}
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return fail(env, err)
	}
	dir, err := construct.Create(newRegistry(home, env.Cwd), home, construct.Spec{
		Name: pos[0], Version: version, Skills: skills.list, Prompts: prompts.list,
		NewPrompt: newPrompt, Force: force,
	})
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "created %s\n", safe(dir))
	if noBuild {
		return 0
	}
	res, err := buildImage(env, home, build.Options{SourceDir: dir, Name: pos[0]}, "")
	if err != nil {
		return fail(env, err)
	}
	printWarnings(env, res.Result)
	printBuild(env, res.Result)
	if res.note != "" {
		fmt.Fprintln(env.Stdout, res.note)
	}
	return 0
}
