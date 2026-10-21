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

// DefaultFile is the compose file a verb reads when -f is not given.
const DefaultFile = "tariboy-compose.yaml"

// DefaultTail is how many events `compose logs` asks for per member.
const DefaultTail = 200

// Options are the parsed arguments of one `tariboy compose VERB` call.
type Options struct {
	Verb string
	// File is the compose file; empty means DefaultFile.
	File string
	// Args are the verb's positionals: the member or group names of a
	// lifecycle verb, rm and logs, or the member and prompt words of exec.
	Args    []string
	Output  string // archive: the archive file to create
	Archive string // import: the archive file to upload
	Yes     bool   // import: apply without asking
	Volumes bool   // down: also delete agent data and group shared dirs
	NoStart bool   // up: do not start the daemon
	Tail    int    // logs: events per member
	// Stdin is where import reads its answer when Yes is false.
	Stdin io.Reader
}

// Verbs lists every compose verb in the order its usage names them.
var Verbs = []string{"up", "down", "start", "stop", "restart", "kill", "exec", "ps", "status", "logs", "build", "rm", "archive", "import"}

// VerbFirst moves the verb in front of the flags that precede it, so
// `compose -f FILE up` resolves like `compose up -f FILE`. args follow
// "compose"; a value after -f, --file, --output, --archive or --tail is not
// the verb.
func VerbFirst(args []string) []string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-f" || a == "--file" || a == "--output" || a == "--archive" || a == "--tail":
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
// writes or imports a team archive. It returns a process exit code.
func Run(ctx context.Context, call Caller, o Options, out, errOut io.Writer) int {
	file := o.File
	if file == "" {
		file = DefaultFile
	}
	output, archive, yes, volumes, noStart, tail := o.Output, o.Archive, o.Yes, o.Volumes, o.NoStart, o.Tail
	verb := o.Verb
	verbArgs := o.Args
	switch verb {
	case "archive":
		return composeArchive(file, output, out, errOut)
	case "import":
		return composeImport(call, archive, yes, o.Stdin, out, errOut)
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
	if verb == "up" && !noStart && os.Getenv("TARIBOY_NO_AUTOSTART") == "" {
		if err := ensureDaemonUp(ctx, out); err != nil {
			fmt.Fprintf(errOut, "compose up: %v\n", err)
			return 1
		}
	}

	var runErr error
	switch verb {
	case "up":
		runErr = r.Up(f)
	case "down":
		runErr = r.Down(f, volumes)
	case "build":
		runErr = r.Build(f)
	case "ps":
		runErr = r.Ps(f)
	case "status":
		runErr = r.Status(f)
	case "start", "stop", "restart", "kill":
		runErr = r.Lifecycle(f, verb, verbArgs)
	case "rm":
		runErr = r.Rm(f, verbArgs)
	case "exec":
		runErr = r.Exec(f, verbArgs)
	case "logs":
		runErr = r.Logs(f, verbArgs, tail)
	default:
		fmt.Fprintf(errOut, "compose: unknown verb %q\n", verb)
		return 2
	}
	if runErr != nil {
		if client.IsDaemonDown(runErr) {
			fmt.Fprintln(errOut, "tariboyd is not running (start it with: tariboyd)")
			return 2
		}
		fmt.Fprintf(errOut, "compose %s: %v\n", verb, runErr)
		return 1
	}
	return 0
}
