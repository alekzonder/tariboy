package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/build"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/source"
)

func init() {
	commands["build"] = runBuild
}

// tagList is a repeatable --tag flag.
type tagList []string

func (t *tagList) String() string     { return strings.Join(*t, ",") }
func (t *tagList) Set(v string) error { *t = append(*t, v); return nil }

// newResolver builds the production source resolver of a tbox home. Tests of
// this package replace it to reach local repositories.
var newResolver = func(home paths.Home) *source.Resolver {
	return &source.Resolver{Home: home, Git: productionGit(), HTTP: productionHTTP()}
}

// openStore returns the image store of the environment's tbox home.
func openStore(env *Env) (imgstore.Store, error) {
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return imgstore.Store{}, err
	}
	return imgstore.Store{Dir: home.ImagesDir()}, nil
}

func fail(env *Env, err error) int {
	fmt.Fprintf(env.Stderr, "tbox: %v\n", err)
	return 1
}

func newFlags(name string, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(w)
	return fs
}

func runBuild(env *Env, args []string) int {
	fs := newFlags("build", env.Stderr)
	path := fs.String("path", "", "directory of the Tariboyfile")
	name := fs.String("name", "", "image name (default: the directory name)")
	var tags tagList
	update := fs.Bool("update", false, "re-resolve remote entries and rewrite tbox-lock.json")
	fs.Var(&tags, "tag", "tag to publish; repeatable (default: image_version and latest)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(env.Stderr, "tbox build: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return fail(env, err)
	}
	store := imgstore.Store{Dir: home.ImagesDir()}
	dir := *path
	if dir == "" {
		dir = "."
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(env.Cwd, dir)
	}
	res, err := build.Build(build.Options{SourceDir: dir, Name: *name, Tags: tags, Store: store, Now: env.Now, Update: *update, Resolver: newResolver(home)})
	if err != nil {
		return fail(env, err)
	}
	printWarnings(env, res)
	printBuild(env, res)
	return 0
}

// printWarnings writes each build warning to stderr.
func printWarnings(env *Env, res build.Result) {
	for _, w := range res.Warnings {
		fmt.Fprintf(env.Stderr, "warning: %s\n", w)
	}
}

func printBuild(env *Env, res build.Result) {
	for _, u := range res.Updates {
		fmt.Fprintf(env.Stdout, "updated %s %s..%s\n", u.Key, shortCommit(u.Old), shortCommit(u.New))
	}
	if !res.Built {
		fmt.Fprintf(env.Stdout, "up to date %s %s\n", res.Name, res.Digest)
		return
	}
	for _, tag := range res.Tags {
		fmt.Fprintf(env.Stdout, "built %s:%s %s\n", res.Name, tag, res.Digest)
	}
	for _, tag := range res.Moved {
		fmt.Fprintf(env.Stdout, "moved %s to new content\n", tag)
	}
}

func shortCommit(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
