package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/daemonctl"
)

// ensureDaemonUp is the auto-start seam (overridden in tests). It brings the
// starts the daemon detached before `compose up` applies the file.
var ensureDaemonUp = func(ctx context.Context, out io.Writer) error {
	cfg, err := daemonctl.ResolveConfig(os.Getenv)
	if err != nil {
		return err
	}
	_, err = daemonctl.EnsureUp(ctx, cfg, out)
	return err
}

// Load reads and parses (but does not validate) a compose file, returning it and
// the absolute directory it lives in (used to resolve relative image contexts,
// the $CWD token, and the default agent cwd).
func Load(path string) (File, string, error) {
	return loadComposeFile(path)
}

func loadComposeFile(path string) (File, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, "", fmt.Errorf("read %s: %w", path, err)
	}
	f, err := Parse(b)
	if err != nil {
		return File{}, "", err
	}
	dir := filepath.Dir(path)
	// Absolutize so $CWD and the default cwd (compose dir) resolve to a path the
	// daemon accepts (it requires an absolute, existing dir), regardless of
	// whether -f was given as a relative path.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return f, dir, nil
}

// DefaultTail is how many events `compose logs` asks for per member.
const DefaultTail = 200

// Options are the parsed arguments of one `tariboy compose VERB` call.
type Options struct {
	Verb string
	// File is the compose file; empty means FindFile in the working directory.
	File string
	// Args are the verb's positionals: the member or group names of a
	// lifecycle verb, rm and logs, or the member and prompt words of exec.
	Args    []string
	Output  string // pack: the archive file to create
	Archive string // unpack (import): the archive file; validate: check this archive
	Yes     bool   // unpack (import): apply without asking
	NoBuild bool   // pack: take the images and workflows already built
	Volumes bool   // down: also delete agent data and group shared dirs
	NoStart bool   // up: do not start the daemon
	DryRun  bool   // up, unpack: print the plan by phase; change nothing
	Tail    int    // logs: events per member
	Daemon  bool   // validate: also check the daemon version
	// Stdin is where unpack reads its answer when Yes is false.
	Stdin io.Reader
}

// Verbs lists every compose verb in the order its usage names them.
var Verbs = []string{"up", "down", "start", "stop", "restart", "kill", "exec", "ps", "status", "logs", "build", "rm", "validate", "pack", "unpack", "archive", "import"}

// VerbFirst moves the verb in front of the flags that precede it, so
// `compose -f FILE up` resolves like `compose up -f FILE`. args follow
// "compose"; a value after -f, --file, -o, --output, --archive or --tail is
// not the verb.
func VerbFirst(args []string) []string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-f" || a == "--file" || a == "-o" || a == "--output" || a == "--archive" || a == "--tail":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			out := append([]string{a}, args[:i]...)
			return append(out, args[i+1:]...)
		}
	}
	return args
}

// Run executes one compose verb: it loads the file and drives the Runner, or
// packs, checks or unpacks a compose archive. It returns a process exit code.
func Run(ctx context.Context, call Caller, o Options, out, errOut io.Writer) int {
	if code, ok := runArchiveVerb(call, o, out, errOut); ok {
		return code
	}
	noStart, verb := o.NoStart, o.Verb
	file, err := resolveFile(o.File)
	if err != nil {
		fmt.Fprintf(errOut, "compose: %v\n", err)
		return 1
	}
	if verb == "validate" {
		return composeValidate(call, file, o.Daemon, out, errOut)
	}

	f, workdir, err := loadComposeFile(file)
	if err != nil {
		fmt.Fprintf(errOut, "compose: %v\n", err)
		return 1
	}
	if err := f.Validate(); err != nil {
		fmt.Fprintf(errOut, "compose: %v\n", err)
		return 1
	}
	r := NewRunner(call, workdir, out)

	// Auto-start the daemon before applying the file (default on). --no-start or
	// TARIBOY_NO_AUTOSTART restores the plain "not running" error path below.
	// A dry run changes nothing, starting the daemon included.
	if verb == "up" && !noStart && !o.DryRun && os.Getenv("TARIBOY_NO_AUTOSTART") == "" {
		if err := ensureDaemonUp(ctx, out); err != nil {
			fmt.Fprintf(errOut, "compose up: %v\n", err)
			return 1
		}
	}

	// The daemon must be inside the file's tariboy range before any change.
	runErr := checkDaemonVersion(call, f, file)
	if runErr != nil {
		return reportRunError(verb, runErr, errOut)
	}
	known, runErr := r.runVerb(f, o)
	if !known {
		fmt.Fprintf(errOut, "compose: unknown verb %q\n", verb)
		return 2
	}
	return reportRunError(verb, runErr, errOut)
}

// runVerb runs a verb that reads the file and calls the daemon; known is
// false for any other verb.
func (r *Runner) runVerb(f File, o Options) (known bool, err error) {
	switch o.Verb {
	case "up":
		if o.DryRun {
			return true, r.DryRun(f)
		}
		return true, r.Up(f)
	case "down":
		return true, r.Down(f, o.Volumes)
	case "build":
		return true, r.Build(f)
	case "ps":
		return true, r.Ps(f)
	case "status":
		return true, r.Status(f)
	case "start", "stop", "restart", "kill":
		return true, r.Lifecycle(f, o.Verb, o.Args)
	case "rm":
		return true, r.Rm(f, o.Args)
	case "exec":
		return true, r.Exec(f, o.Args)
	case "logs":
		return true, r.Logs(f, o.Args, o.Tail)
	}
	return false, nil
}

// reportRunError prints the error of a verb and returns its exit code.
func reportRunError(verb string, err error, errOut io.Writer) int {
	if err == nil {
		return 0
	}
	if client.IsDaemonDown(err) {
		fmt.Fprintln(errOut, "tariboyd is not running (start it with: tariboyd)")
		return 2
	}
	fmt.Fprintf(errOut, "compose %s: %v\n", verb, err)
	return 1
}
