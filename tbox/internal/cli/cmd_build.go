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
)

func init() {
	commands["build"] = runBuild
}

// tagList is a repeatable --tag flag.
type tagList []string

func (t *tagList) String() string     { return strings.Join(*t, ",") }
func (t *tagList) Set(v string) error { *t = append(*t, v); return nil }

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
	fs.Var(&tags, "tag", "tag to publish; repeatable (default: image_version and latest)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(env.Stderr, "tbox build: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	store, err := openStore(env)
	if err != nil {
		return fail(env, err)
	}
	dir := *path
	if dir == "" {
		dir = "."
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(env.Cwd, dir)
	}
	res, err := build.Build(build.Options{SourceDir: dir, Name: *name, Tags: tags, Store: store, Now: env.Now})
	if err != nil {
		return fail(env, err)
	}
	printBuild(env, res)
	return 0
}

func printBuild(env *Env, res build.Result) {
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
