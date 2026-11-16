package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/portable"
	"github.com/alekzonder/tariboy/tbox/internal/version"
)

func init() {
	// The image command is registered by cmd_images.go, which sorts first;
	// export and import join it here and the rest stays with it.
	rest := commands["image"]
	commands["image"] = func(env *Env, args []string) int {
		if len(args) > 0 {
			switch args[0] {
			case "export":
				return runExport(env, args[1:])
			case "import":
				return runImport(env, args[1:])
			}
		}
		return rest(env, args)
	}
}

// inCwd resolves a user path against the working directory of env.
func inCwd(env *Env, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(env.Cwd, p)
}

func runExport(env *Env, args []string) int {
	fs := newFlags("image export", env.Stderr)
	out := fs.String("o", "", "output file (default: NAME-TAG.tariboy-image.tar.gz in the current directory)")
	var refText string
	for {
		if err := fs.Parse(args); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		if refText != "" {
			fmt.Fprintln(env.Stderr, "usage: tbox image export REF [-o FILE]")
			return 2
		}
		refText, args = fs.Arg(0), fs.Args()[1:]
	}
	if refText == "" {
		fmt.Fprintln(env.Stderr, "usage: tbox image export REF [-o FILE]")
		return 2
	}
	ref, err := imgstore.ParseRef(refText)
	if err != nil {
		return fail(env, err)
	}
	store, err := openStore(env)
	if err != nil {
		return fail(env, err)
	}
	img, err := store.Open(ref)
	if err != nil {
		return fail(env, err)
	}
	var buf bytes.Buffer
	if err := portable.Export(&buf, img, ref, "tbox-"+version.Version); err != nil {
		return fail(env, err)
	}
	name := *out
	if name == "" {
		name = fmt.Sprintf("%s-%s.tariboy-image.tar.gz", ref.Name, ref.Tag)
	}
	target := inCwd(env, name)
	if err := os.WriteFile(target, buf.Bytes(), 0o644); err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "exported %s %s to %s\n", ref, img.Manifest.Digest, target)
	return 0
}

func runImport(env *Env, args []string) int {
	fs := newFlags("image import", env.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(env.Stderr, "usage: tbox image import FILE")
		return 2
	}
	store, err := openStore(env)
	if err != nil {
		return fail(env, err)
	}
	f, err := os.Open(inCwd(env, fs.Arg(0)))
	if err != nil {
		return fail(env, err)
	}
	defer f.Close()
	img, ref, err := portable.Import(f)
	if err != nil {
		return fail(env, err)
	}
	moved, err := store.Publish(ref.Name, img, []string{ref.Tag})
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "imported %s %s\n", ref, img.Manifest.Digest)
	for _, tag := range moved {
		fmt.Fprintf(env.Stdout, "moved %s to new content\n", tag)
	}
	return 0
}
