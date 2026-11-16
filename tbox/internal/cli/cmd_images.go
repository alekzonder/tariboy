package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"text/tabwriter"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
)

func init() {
	commands["images"] = runImages
	commands["image"] = runImage
}

const digestPrefix = 12

func short(digest string) string {
	if len(digest) > digestPrefix {
		return digest[:digestPrefix]
	}
	return digest
}

// imageSubcommands are the subcommands of image that parse their own
// arguments; show and rm take a REF and are handled by runImage.
var imageSubcommands = map[string]Command{
	"export": runExport,
	"import": runImport,
}

func runImages(env *Env, args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(env.Stderr, "usage: tbox images")
		return 2
	}
	store, err := openStore(env)
	if err != nil {
		return fail(env, err)
	}
	entries, err := store.List()
	if err != nil {
		return fail(env, err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(env.Stdout, "no images")
		return 0
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTAG\tVERSION\tDIGEST\tBUILT")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", safe(e.Name), safe(e.Tag), safe(e.ImageVersion), safe(short(e.Digest)), safe(e.BuiltAt))
	}
	if err := tw.Flush(); err != nil {
		return fail(env, err)
	}
	return 0
}

func runImage(env *Env, args []string) int {
	if len(args) > 0 {
		if sub, ok := imageSubcommands[args[0]]; ok {
			return sub(env, args[1:])
		}
	}
	if len(args) != 2 || (args[0] != "show" && args[0] != "rm") {
		fmt.Fprintln(env.Stderr, "usage: tbox image show REF | rm REF | export REF [-o FILE] [--force] | import FILE")
		return 2
	}
	ref, err := imgstore.ParseRef(args[1])
	if err != nil {
		return fail(env, err)
	}
	store, err := openStore(env)
	if err != nil {
		return fail(env, err)
	}
	if args[0] == "rm" {
		return removeImage(env, store, ref)
	}
	return showImage(env, store, ref)
}

func removeImage(env *Env, store imgstore.Store, ref imgstore.Ref) int {
	if err := store.Remove(ref); err != nil {
		if errors.Is(err, imgstore.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return fail(env, fmt.Errorf("image %s not found", ref))
		}
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "removed %s\n", ref)
	return 0
}

func showImage(env *Env, store imgstore.Store, ref imgstore.Ref) int {
	img, err := store.Open(ref)
	if err != nil {
		return fail(env, err)
	}
	printImage(env, ref, img)
	return 0
}

func printImage(env *Env, ref imgstore.Ref, img archive.Image) {
	m := img.Manifest
	w := env.Stdout
	fmt.Fprintf(w, "ref: %s\nname: %s\ndigest: %s\n", ref, safe(m.Name), safe(m.Digest))
	if m.ImageVersion != "" {
		fmt.Fprintf(w, "version: %s\n", safe(m.ImageVersion))
	}
	fmt.Fprintf(w, "built: %s\nschema: %d\n", safe(m.BuiltAt), m.SchemaVersion)
	if m.Role != nil {
		fmt.Fprintf(w, "role: %s\n", safe(m.Role.Text))
	}
	fmt.Fprintf(w, "skills (%d):\n", len(m.Skills))
	for _, s := range m.Skills {
		fmt.Fprintf(w, "  %s: %s\n", safe(s.Name), safe(s.Description))
	}
	fmt.Fprintf(w, "prompt layers (%d):\n", len(img.Template.Entries))
	for _, e := range img.Template.Entries {
		src := e.Source
		if e.Kind == "runtime" {
			src = "runtime " + e.Runtime
		}
		fmt.Fprintf(w, "  %s (%s)\n", safe(src), safe(e.Category))
	}
}
