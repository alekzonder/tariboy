// Package agentmode implements the restricted tbox command set that works
// inside an image session. It reads only the unpacked image directory.
package agentmode

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/rundir"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/textsafe"
)

// Environment variables of an image session.
const (
	EnvImageDir = "TBOX_IMAGE_DIR"
	EnvImage    = "TBOX_IMAGE"
	EnvSkillDir = "TBOX_SKILL_DIR"
)

// Host carries the process dependencies, so tests can fake them.
type Host struct {
	Stdout, Stderr io.Writer
	Environ        []string
	Exec           func(argv0 string, argv, env []string) error
}

const maxSkillDoc = 8 << 20

// Run executes one agent-mode command and returns the exit code.
func Run(host Host, imageDir string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(host.Stderr, "usage: tbox skills | skill NAME | skill exec NAME SCRIPT [ARGS...]")
		return 2
	}
	switch args[0] {
	case "skills", "skill":
	default:
		refuse(host, imageDir, args[0])
		return 2
	}
	run, err := rundir.Open(imageDir)
	if err != nil {
		fmt.Fprintf(host.Stderr, "tbox: cannot open the image session directory %s: %v\n", imageDir, err)
		return 1
	}
	if args[0] == "skills" {
		return listSkills(host, run)
	}
	return skillCommand(host, run, args[1:])
}

// available lists the commands of a session, for the refusal of any other.
const available = "available: tbox skills, tbox skill NAME, tbox skill exec NAME SCRIPT [ARGS...], tbox version, tbox help"

// refuse explains that command does not work in this session. It names the
// image when TBOX_IMAGE is set and the image directory otherwise.
func refuse(host Host, imageDir, command string) {
	where := "image directory " + imageDir
	if img := lookup(host.Environ, EnvImage); img != "" {
		where = "image " + img
	}
	fmt.Fprintf(host.Stderr, "tbox: %s is not available inside the session of %s\n%s\n", textsafe.Line(command, 100), textsafe.Line(where, 1000), available)
}

func lookup(environ []string, key string) string {
	for i := len(environ) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(environ[i], key+"="); ok {
			return v
		}
	}
	return ""
}

func listSkills(host Host, run rundir.Run) int {
	for _, s := range run.Skills {
		fmt.Fprintf(host.Stdout, "%s\t%s\n", s.Name, oneLine(s.Description))
	}
	return 0
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func skillCommand(host Host, run rundir.Run, args []string) int {
	if len(args) > 0 && args[0] == "exec" {
		return skillExec(host, run, args[1:])
	}
	if len(args) != 1 {
		fmt.Fprintln(host.Stderr, "usage: tbox skill NAME")
		return 2
	}
	meta, code := find(host, run, args[0])
	if meta == nil {
		return code
	}
	dir := filepath.Join(run.ImageDir, meta.ArchiveRoot)
	doc, err := fsafe.ReadFileNoFollow(filepath.Join(dir, "SKILL.md"), maxSkillDoc)
	if err != nil {
		fmt.Fprintf(host.Stderr, "tbox: read SKILL.md of %s: %v\n", meta.Name, err)
		return 1
	}
	files, err := listFiles(dir)
	if err != nil {
		fmt.Fprintf(host.Stderr, "tbox: list files of %s: %v\n", meta.Name, err)
		return 1
	}
	fmt.Fprintf(host.Stdout, "name: %s\ndescription: %s\nfiles:\n", meta.Name, oneLine(meta.Description))
	for _, f := range files {
		fmt.Fprintf(host.Stdout, "  %s\n", f)
	}
	fmt.Fprintf(host.Stdout, "\n%s", doc)
	return 0
}

func listFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	sort.Strings(files)
	return files, err
}

// find returns the named skill, or nil and the exit code after reporting.
func find(host Host, run rundir.Run, name string) (*skill.Metadata, int) {
	if strings.Contains(name, "/") {
		fmt.Fprintf(host.Stderr, "tbox: skill names inside a session have no store prefix: use %q\n", name[strings.LastIndex(name, "/")+1:])
		return nil, 1
	}
	for i := range run.Skills {
		if run.Skills[i].Name == name {
			return &run.Skills[i], 0
		}
	}
	names := make([]string, 0, len(run.Skills))
	for _, s := range run.Skills {
		names = append(names, s.Name)
	}
	fmt.Fprintf(host.Stderr, "tbox: unknown skill %q; available: %s\n", name, strings.Join(names, ", "))
	return nil, 1
}

func skillExec(host Host, run rundir.Run, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(host.Stderr, "usage: tbox skill exec NAME SCRIPT [ARGS...]")
		return 2
	}
	meta, code := find(host, run, args[0])
	if meta == nil {
		return code
	}
	dir := filepath.Join(run.ImageDir, meta.ArchiveRoot)
	argv0, argv, err := resolveScript(dir, args[1], args[2:])
	if err != nil {
		fmt.Fprintf(host.Stderr, "tbox: %v\n", err)
		return 1
	}
	env := withSkillDir(host.Environ, dir)
	// The image directory belongs to the session user, so the gap between the
	// path check and exec is not a privilege boundary.
	if err := host.Exec(argv0, argv, env); err != nil {
		fmt.Fprintf(host.Stderr, "tbox: run %s: %v\n", args[1], err)
		return 126
	}
	return 0
}

// resolveScript maps a skill-relative script to the program to execute.
func resolveScript(dir, script string, rest []string) (string, []string, error) {
	if filepath.IsAbs(script) {
		return "", nil, fmt.Errorf("script %q must be relative to the skill directory", script)
	}
	path, err := fsafe.Within(dir, filepath.Join(dir, script))
	if err != nil {
		return "", nil, fmt.Errorf("script %q: %w", script, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("script %q: %w", script, err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("script %q is not a regular file", script)
	}
	switch {
	case info.Mode().Perm()&0o111 != 0:
		return path, append([]string{path}, rest...), nil
	case strings.HasSuffix(path, ".sh"):
		return "/bin/sh", append([]string{"/bin/sh", path}, rest...), nil
	}
	return "", nil, errors.New("script " + script + " is not executable and does not end in .sh")
}

func withSkillDir(environ []string, dir string) []string {
	out := make([]string, 0, len(environ)+1)
	for _, e := range environ {
		if !strings.HasPrefix(e, EnvSkillDir+"=") {
			out = append(out, e)
		}
	}
	return append(out, EnvSkillDir+"="+dir)
}
