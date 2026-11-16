package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/build"
	"github.com/alekzonder/tariboy/tbox/internal/harness"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

func init() { commands["run"] = runRun }

const runUsage = "usage: tbox run IMAGE HARNESS [-- ARGS...]\n       tbox run --path DIR HARNESS [-- ARGS...]\n"

// runRun starts a harness session with an image. On success the process is
// replaced by the harness, so tbox does not stay in the process tree.
func runRun(env *Env, args []string) int {
	own, extra := splitDashDash(args)
	fs := newFlags("run", env.Stderr)
	path := fs.String("path", "", "build-if-changed from this source directory and run its image")
	if err := fs.Parse(own); err != nil {
		return 2
	}
	pos := fs.Args()
	want := 2
	if *path != "" {
		want = 1
	}
	if len(pos) != want {
		// Interactive selection of the image and the harness arrives in
		// stage 3; until then both arguments are required.
		fmt.Fprint(env.Stderr, "tbox run: image and harness are required (interactive selection arrives in stage 3)\n"+runUsage)
		return 2
	}
	harnessName := pos[want-1]
	adapter, err := harness.Get(harnessName)
	if err != nil {
		fmt.Fprintf(env.Stderr, "tbox: unknown harness %q (known: %s)\n", harnessName, strings.Join(harness.Names(), ", "))
		return 2
	}
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return fail(env, err)
	}
	store := imgstore.Store{Dir: home.ImagesDir()}
	ref, code := resolveRunRef(env, home, store, *path, pos)
	if code != 0 {
		return code
	}
	img, err := store.Open(ref)
	if err != nil {
		return failMissing(env, store, ref, err)
	}
	run, err := rundir.Prepare(home, img, ref, harnessName)
	if err != nil {
		return fail(env, err)
	}
	launch, err := adapter.Prepare(context.Background(), harnessHost(env), run, extra)
	if err != nil {
		return fail(env, err)
	}
	for _, note := range launch.Notes {
		fmt.Fprintln(env.Stderr, note)
	}
	err = env.Exec(launch.Path, launch.Argv, mergeEnv(sessionEnviron(env), launch.Env))
	return fail(env, fmt.Errorf("start %s: %w", harnessName, err))
}

// splitDashDash splits args at the first "--": tbox's own arguments, then the
// arguments passed to the harness unchanged.
func splitDashDash(args []string) (own, extra []string) {
	if i := slices.Index(args, "--"); i >= 0 {
		return args[:i], args[i+1:]
	}
	return args, nil
}

// resolveRunRef returns the image to run, building it first for --path.
func resolveRunRef(env *Env, home paths.Home, store imgstore.Store, path string, pos []string) (imgstore.Ref, int) {
	if path != "" {
		dir := path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(env.Cwd, dir)
		}
		res, err := build.Build(build.Options{SourceDir: dir, Tags: []string{"latest"}, Store: store, Now: env.Now, Resolver: newResolver(home)})
		if err != nil {
			return imgstore.Ref{}, fail(env, err)
		}
		printWarnings(env, res)
		if res.Built {
			fmt.Fprintf(env.Stderr, "built %s:latest %s\n", res.Name, res.Digest)
		} else {
			fmt.Fprintf(env.Stderr, "up to date %s %s\n", res.Name, res.Digest)
		}
		return imgstore.Ref{Name: res.Name, Tag: "latest"}, 0
	}
	if strings.Contains(pos[0], "/") {
		return imgstore.Ref{}, fail(env, fmt.Errorf("%q looks like a Store selector; Stores arrive in stage 2, until then run a built image by name[:tag]", pos[0]))
	}
	ref, err := imgstore.ParseRef(pos[0])
	if err != nil {
		return imgstore.Ref{}, fail(env, err)
	}
	return ref, 0
}

// failMissing reports a missing image together with the built images.
func failMissing(env *Env, store imgstore.Store, ref imgstore.Ref, err error) int {
	if !errors.Is(err, imgstore.ErrNotFound) {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stderr, "tbox: image %s not found\n", ref)
	entries, lerr := store.List()
	if lerr != nil || len(entries) == 0 {
		fmt.Fprintln(env.Stderr, "no images are built; run tbox build --path DIR")
		return 1
	}
	fmt.Fprintln(env.Stderr, "built images:")
	for _, e := range entries {
		fmt.Fprintf(env.Stderr, "  %s:%s\n", e.Name, e.Tag)
	}
	return 1
}

func sessionEnviron(env *Env) []string {
	if env.Environ != nil {
		return env.Environ()
	}
	return os.Environ()
}

// mergeEnv returns base with the KEY=VALUE entries of add replacing
// same-named variables, in order of appearance.
func mergeEnv(base, add []string) []string {
	replaced := make(map[string]bool, len(add))
	for _, kv := range add {
		replaced[envKey(kv)] = true
	}
	out := make([]string, 0, len(base)+len(add))
	for _, kv := range base {
		if !replaced[envKey(kv)] {
			out = append(out, kv)
		}
	}
	return append(out, add...)
}

func envKey(kv string) string {
	k, _, _ := strings.Cut(kv, "=")
	return k
}
