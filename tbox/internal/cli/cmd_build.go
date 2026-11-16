package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/build"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/source"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
	"github.com/alekzonder/tariboy/tbox/internal/textsafe"
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
	fmt.Fprintf(env.Stderr, "tbox: %s\n", textsafe.Line(err.Error(), 2000))
	return 1
}

func newFlags(name string, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(w)
	return fs
}

func runBuild(env *Env, args []string) int {
	var path, name string
	var tags tagList
	var update bool
	pos, err := parseInterleaved(env, "build", args, func(fs *flag.FlagSet) {
		fs.StringVar(&path, "path", "", "directory of the Tariboyfile")
		fs.StringVar(&name, "name", "", "image name (default: the directory name)")
		fs.BoolVar(&update, "update", false, "re-resolve remote entries and rewrite tbox-lock.json")
		fs.Var(&tags, "tag", "tag to publish; repeatable (default: image_version and latest)")
	})
	if err != nil {
		return 2
	}
	if len(pos) > 1 || (len(pos) == 1 && path != "") {
		fmt.Fprint(env.Stderr, buildUsage)
		return 2
	}
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return fail(env, err)
	}
	opts := build.Options{Name: name, Tags: tags, Update: update}
	var label string
	if len(pos) == 1 {
		opts.SourceDir, opts.StoreRoot, err = storeImageDir(env, home, pos[0], name)
		label = pos[0]
	} else {
		opts.SourceDir, label = absDir(env, path), ""
	}
	if err != nil {
		return fail(env, err)
	}
	res, err := buildImage(env, home, opts, label)
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

// absDir makes dir absolute against the working directory; empty means ".".
func absDir(env *Env, dir string) string {
	if dir == "" {
		dir = "."
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(env.Cwd, dir)
	}
	return dir
}

// storeImageDir returns the source directory of the image STORE/IMAGE and the
// root of its Store. The directory is never reached through a symlink. IMAGE
// is the directory name as the Store lists it; when it is not a valid image
// name, name (the --name flag) must give one.
func storeImageDir(env *Env, home paths.Home, arg, name string) (dir, root string, err error) {
	sel, ok := stores.ParseSelector(arg)
	if !ok || sel.Name == "." || sel.Name == ".." {
		return "", "", fmt.Errorf("%q is not STORE/IMAGE", safe(arg))
	}
	reg := newRegistry(home, env.Cwd)
	s, err := reg.Get(sel.Store)
	if err != nil {
		return "", "", err
	}
	root = reg.Root(s)
	dir = filepath.Join(root, "images", sel.Name)
	if fi, lerr := os.Lstat(filepath.Join(root, "images")); lerr != nil || !fi.IsDir() {
		return "", "", fmt.Errorf("store %s has no image %q", sel.Store, sel.Name)
	}
	if fi, lerr := os.Lstat(dir); lerr != nil || !fi.IsDir() {
		return "", "", fmt.Errorf("store %s has no image %q", sel.Store, sel.Name)
	}
	if name == "" && imgstore.CheckName(sel.Name) != nil {
		return "", "", fmt.Errorf("store image %q is not a valid image name (first character a-z or 0-9, then a-z 0-9 . _ -); build it with tbox build %s --name NAME", safe(sel.Name), safe(arg))
	}
	return dir, root, nil
}

// builtResult is a build result with the note about a moved image name.
type builtResult struct {
	build.Result
	note string
}

// buildImage builds opts, resolving remote entries with the production
// resolver, and records the source directory beside the fingerprint. The note
// is set when the previous build of the same image name came from another
// directory; label names the new source in it, or is empty for the directory.
func buildImage(env *Env, home paths.Home, opts build.Options, label string) (builtResult, error) {
	opts.Store = imgstore.Store{Dir: home.ImagesDir()}
	opts.Now = env.Now
	opts.Resolver = newResolver(home)
	res, err := build.Build(opts)
	if err != nil {
		return builtResult{}, err
	}
	out := builtResult{Result: res}
	dir, rerr := filepath.Abs(opts.SourceDir)
	if rerr != nil {
		return out, nil
	}
	if prev, ok := opts.Store.SourceDir(res.Name); ok && prev != dir {
		if label == "" {
			label = dir
		}
		out.note = fmt.Sprintf("note: image %s now comes from %s", safe(res.Name), safe(label))
	}
	_ = opts.Store.SetSourceDir(res.Name, dir)
	return out, nil
}

// printWarnings writes each build warning to stderr.
func printWarnings(env *Env, res build.Result) {
	for _, w := range res.Warnings {
		fmt.Fprintf(env.Stderr, "warning: %s\n", textsafe.Line(w, 2000))
	}
}

func printBuild(env *Env, res build.Result) {
	for _, u := range res.Updates {
		fmt.Fprintf(env.Stdout, "updated %s %s..%s\n", safe(u.Key), safe(shortCommit(u.Old)), safe(shortCommit(u.New)))
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
