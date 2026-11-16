package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/harness"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/textsafe"
)

func init() { commands["clean"] = runClean }

// probeTimeout bounds each harness probe (version, config), so a hanging
// harness binary cannot block tbox run.
var probeTimeout = 10 * time.Second

// harnessHost adapts the command environment to the harness package.
func harnessHost(env *Env) harness.Host {
	lookPath := env.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return harness.Host{
		Getenv:   env.Getenv,
		LookPath: lookPath,
		Note:     func(n string) { fmt.Fprintln(env.Stderr, textsafe.Line(n, 2000)) },
		Output: func(ctx context.Context, extra []string, name string, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, name, args...)
			// A child that keeps stdout open must not hold Output past the
			// timeout.
			cmd.WaitDelay = time.Second
			base := os.Environ()
			if env.Environ != nil {
				base = env.Environ()
			}
			cmd.Env = append(base, extra...)
			return cmd.Output()
		},
	}
}

// runClean removes the run directories, the harness files outside
// $TBOX_HOME and the image archives no tag points at, of one image or of
// all.
func runClean(env *Env, args []string) int {
	pos, err := parseInterleaved(env, "clean", args, func(*flag.FlagSet) {})
	if err != nil || len(pos) > 1 {
		fmt.Fprint(env.Stderr, cleanUsage)
		return 2
	}
	image := ""
	if len(pos) == 1 {
		// An image named before the name rule was tightened can still be
		// cleaned.
		image = pos[0]
		if err := imgstore.CheckExistingName(image); err != nil {
			return fail(env, err)
		}
	}
	home, err := paths.Resolve(env.Getenv)
	if err != nil {
		return fail(env, err)
	}
	if err := harness.CleanAll(harnessHost(env), home, image); err != nil {
		return fail(env, err)
	}
	if image == "" {
		fmt.Fprintln(env.Stdout, "removed all run directories and harness files")
	} else {
		fmt.Fprintf(env.Stdout, "removed run directories and harness files of %s\n", image)
	}
	n, err := pruneArchives(imgstore.Store{Dir: home.ImagesDir()}, image)
	if n > 0 {
		fmt.Fprintf(env.Stdout, "removed %d untagged image archive(s)\n", n)
	}
	if err != nil {
		return fail(env, err)
	}
	return 0
}

// pruneArchives removes the untagged archives of image, or of every image
// when image is empty, and returns how many it removed.
func pruneArchives(store imgstore.Store, image string) (int, error) {
	if image == "" {
		removed, err := store.PruneAllUntagged()
		return len(removed), err
	}
	removed, err := store.PruneUntagged(image)
	return len(removed), err
}
