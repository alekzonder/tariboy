// Package fsbrowser implements a read-only, directory-only listing surface
// rooted at a fixed filesystem root (TARIBOY_FS_ROOT, default the daemon
// user's $HOME). It powers the UI cwd path-autocomplete: every request resolves
// a caller-supplied path under the root and refuses anything that escapes it —
// lexical traversal (.., an absolute path outside the root) and symlinks whose
// real target lands outside the root are all rejected.
//
// It mirrors the path-safety model of internal/filebrowser (Resolve /
// pathjail.ResolveExisting / symlink-escape checks) but differs in two ways that matter
// for a cwd picker: the root is the daemon user's $HOME rather than a single
// agent's workdir, and an absolute path is honored as-is (and refused if it
// falls outside the root) instead of being re-based onto the root. The package
// is UI-agnostic: it returns plain structs and typed *Error values, leaving
// HTTP/JSON shaping to the api/commands layers.
package fsbrowser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/pathjail"
)

// Error is a typed failure carrying a stable code the command layer maps to the
// api JSON error envelope and an HTTP status: bad_path (403), not_found (404),
// not_dir (400).
type Error = pathjail.Error

// within keeps the package-local name; the logic lives in pathjail.
func within(root, target string) bool { return pathjail.Within(root, target) }

// Entry is one directory in a listing. Only directories are ever listed, so Dir
// is always true; it is emitted explicitly so the field is stable for the UI.
type Entry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

// Listing is the result of List: the resolved absolute directory, its parent
// (absolute, or "" when the directory is the root itself), and the child
// directories.
type Listing struct {
	Path    string  `json:"path"`
	Parent  string  `json:"parent"`
	Entries []Entry `json:"entries"`
}

// Root returns the configured jail root: TARIBOY_FS_ROOT if set, otherwise
// the daemon user's home directory. It errors only when neither is available.
func Root() (string, error) {
	if r := strings.TrimSpace(os.Getenv("TARIBOY_FS_ROOT")); r != "" {
		return filepath.Clean(r), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", pathjail.BadPath("cannot determine filesystem root")
	}
	return filepath.Clean(home), nil
}

// resolve maps a caller path to an absolute target confined to root, returning a
// *Error (bad_path) for anything that escapes. An empty path, "~", or "~/…"
// resolves from the root; a relative path resolves from the root; an absolute
// path is honored as-is but must fall inside the root. The target need not exist
// (List reports a missing directory as not_found); a symlink anywhere along the
// existing portion is followed and its real target checked against the root.
func resolve(root, path string) (string, error) {
	root = filepath.Clean(root)
	p := strings.TrimSpace(path)

	// Expand a leading ~ to the root. "~" alone (and "~/") means the root itself;
	// "~/sub" means sub relative to the root.
	switch {
	case p == "~":
		p = ""
	case strings.HasPrefix(p, "~/"):
		p = p[2:]
	}

	var target string
	switch {
	case p == "":
		target = root
	case filepath.IsAbs(p):
		target = filepath.Clean(p)
	default:
		target = filepath.Join(root, p)
	}
	target = filepath.Clean(target)

	// Lexical containment: target must be the root or live beneath it. filepath
	// .Join/Clean above have already collapsed any .., so an escaping relative or
	// out-of-root absolute path lands outside root here.
	if !within(root, target) {
		return "", pathjail.BadPath("path escapes the filesystem root")
	}

	// Symlink containment: follow symlinks on the deepest existing ancestor and
	// re-check, so a symlink inside root pointing out is refused.
	realRoot, err := pathjail.ResolveExisting(root)
	if err != nil {
		return "", pathjail.BadPath("cannot resolve filesystem root")
	}
	realTarget, err := pathjail.ResolveExisting(target)
	if err != nil {
		return "", pathjail.BadPath("cannot resolve path")
	}
	if !within(realRoot, realTarget) {
		return "", pathjail.BadPath("path escapes the filesystem root (symlink)")
	}
	return target, nil
}

// List returns the child directories of the directory addressed by path,
// confined to root. Non-directory entries are filtered out and dotfile dirs are
// kept; entries are sorted case-insensitively. Parent is the absolute parent
// directory, or "" when the listed directory is the root itself.
func List(root, path string) (Listing, error) {
	target, err := resolve(root, path)
	if err != nil {
		return Listing{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return Listing{}, pathjail.NotFound("directory not found")
		}
		if pathjail.IsNotDir(err) {
			return Listing{}, pathjail.NotDir("path is not a directory")
		}
		return Listing{}, err
	}
	if !info.IsDir() {
		return Listing{}, pathjail.NotDir("path is not a directory")
	}
	des, err := os.ReadDir(target)
	if err != nil {
		return Listing{}, err
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		if !de.IsDir() {
			continue // directories only — a cwd picker never needs files
		}
		entries = append(entries, Entry{Name: de.Name(), Dir: true})
	}
	sort.Slice(entries, func(i, j int) bool {
		li, lj := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if li != lj {
			return li < lj
		}
		return entries[i].Name < entries[j].Name // stable tie-break for e.g. "A" vs "a"
	})

	root = filepath.Clean(root)
	parent := ""
	if target != root {
		parent = filepath.Dir(target)
	}
	return Listing{Path: target, Parent: parent, Entries: entries}, nil
}
