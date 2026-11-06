package compose

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/version"
)

// composeValidate checks the file without changing anything: schema,
// references, local paths and the tariboy range against the CLI version, and
// with daemon also against the running daemon (one status read). It prints
// every problem, not only the first.
func composeValidate(call Caller, file string, daemon bool, out, errOut io.Writer) int {
	if abs, err := filepath.Abs(file); err == nil {
		file = abs
	}
	problems := validateFile(call, file, daemon)
	if len(problems) == 0 {
		fmt.Fprintf(out, "%s: valid\n", file)
		return 0
	}
	for _, p := range problems {
		fmt.Fprintf(errOut, "- %v\n", p)
	}
	fmt.Fprintf(errOut, "compose validate: %d problem(s) in %s\n", len(problems), file)
	return 1
}

func validateFile(call Caller, file string, daemon bool) []error {
	f, dir, err := loadComposeFile(file)
	if err != nil {
		var typeErr *yaml.TypeError
		if errors.As(err, &typeErr) {
			out := make([]error, 0, len(typeErr.Errors))
			for _, line := range typeErr.Errors {
				out = append(out, errors.New(line))
			}
			return out
		}
		return []error{err}
	}
	problems := f.problems()
	if f.Version != 1 && f.Version != 2 {
		return problems
	}
	problems = append(problems, f.pathProblems(dir)...)
	c, err := f.VersionConstraint()
	if err != nil || c.IsZero() {
		return problems // a bad range is already one of problems
	}
	if cli, err := version.ParseSemVer(version.Version); err != nil {
		problems = append(problems, fmt.Errorf("CLI %w", err))
	} else if err := checkVersion("CLI", cli, c, file); err != nil {
		problems = append(problems, err)
	}
	if !daemon {
		return problems
	}
	v, err := daemonVersion(call)
	switch {
	case client.IsDaemonDown(err):
		problems = append(problems, errors.New("tariboyd is not running; cannot check the daemon version"))
	case err != nil:
		problems = append(problems, err)
	default:
		if err := checkVersion("daemon", v, c, file); err != nil {
			problems = append(problems, err)
		}
	}
	return problems
}

// pathProblems reports each local path of stores, workflows and images that
// does not exist. A relative path is resolved from the file's directory dir.
func (f File) pathProblems(dir string) []error {
	var out []error
	check := func(kind, name, path string) {
		if path == "" {
			return
		}
		full := path
		if !filepath.IsAbs(full) {
			full = filepath.Join(dir, full)
		}
		if _, err := os.Stat(full); err != nil {
			out = append(out, fmt.Errorf("%s %q path %q does not exist", kind, name, path))
		}
	}
	for _, name := range sortedKeys(f.Stores) {
		check("store", name, f.Stores[name].Path)
	}
	for _, name := range sortedKeys(f.Workflows) {
		check("workflow", name, f.Workflows[name].Path)
	}
	for _, name := range sortedKeys(f.Images) {
		check("image", name, f.Images[name].BuildPath())
	}
	return out
}
