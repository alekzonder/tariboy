package tfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillDirs returns the absolute directories of the file's local skill
// entries. A prompt may reach below one of them with ../.
func (f *File) SkillDirs() []string {
	var out []string
	for _, s := range f.Skills {
		if s.Dir == "" || strings.HasPrefix(s.Dir, pluginsPrefix) {
			continue
		}
		if abs, ok := lexicalPath(s.BaseDir, s.Dir); ok {
			out = append(out, abs)
		}
	}
	return out
}

func lexicalPath(base, value string) (string, bool) {
	if filepath.IsAbs(value) {
		return filepath.Clean(value), true
	}
	if !strings.HasPrefix(value, "./") && !strings.HasPrefix(value, "../") {
		return "", false
	}
	abs, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(value)))
	return abs, err == nil
}

// checkPlain verifies that abs exists and no component is a symlink.
func checkPlain(kind, value, abs string) error {
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("resolve %s path %q: %w", kind, value, err)
	}
	if resolved != abs {
		return fmt.Errorf("%s path %q contains a symlink", kind, value)
	}
	return nil
}

// locate maps value to an absolute path and its category. base is resolved
// first, so a symlinked base directory is not an error.
func locate(base, value, kind string) (string, string, error) {
	if strings.HasPrefix(value, pluginsPrefix) {
		return "", "", fmt.Errorf("%s %q: tbox has no plugin root to resolve $PLUGINS paths", kind, value)
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), "absolute", nil
	}
	if !strings.HasPrefix(value, "./") && !strings.HasPrefix(value, "../") {
		return "", "", fmt.Errorf("%s path %q must use ./, ../, an absolute path, or $PLUGINS", kind, value)
	}
	root, err := filepath.Abs(base)
	if err != nil {
		return "", "", err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return "", "", fmt.Errorf("resolve %s root: %w", kind, err)
	}
	return filepath.Join(root, filepath.FromSlash(value)), "source", nil
}

// ResolveSkillDir resolves a local dir entry. It returns the absolute path,
// the source as written and the category.
func ResolveSkillDir(e SkillEntry) (string, string, string, error) {
	if e.Git != "" || e.URL != "" {
		return "", "", "", ErrRemoteUnsupported
	}
	abs, category, err := locate(e.BaseDir, e.Dir, "skill")
	if err != nil {
		return "", "", "", err
	}
	if err := checkPlain("skill", e.Dir, abs); err != nil {
		return "", "", "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", "", "", err
	}
	if !info.IsDir() {
		return "", "", "", fmt.Errorf("skill path %q is not a directory", e.Dir)
	}
	return abs, e.Dir, category, nil
}

func below(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ResolvePromptFile resolves a local file entry. A ../ path is accepted only
// below one of skillDirs (absolute skill directories, see File.SkillDirs); a
// ./ path must stay inside the entry's BaseDir.
func ResolvePromptFile(e PromptEntry, skillDirs ...string) (string, string, string, error) {
	if e.Git != "" || e.URL != "" {
		return "", "", "", ErrRemoteUnsupported
	}
	if e.File == "" {
		return "", "", "", fmt.Errorf("prompt entry has no file")
	}
	abs, category, err := locate(e.BaseDir, e.File, "prompt")
	if err != nil {
		return "", "", "", err
	}
	if err := checkPromptScope(e, abs, skillDirs); err != nil {
		return "", "", "", err
	}
	if err := checkPlain("prompt", e.File, abs); err != nil {
		return "", "", "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", "", "", err
	}
	if !info.Mode().IsRegular() {
		return "", "", "", fmt.Errorf("prompt path %q is not a regular file", e.File)
	}
	if info.Size() > maxFileSize {
		return "", "", "", fmt.Errorf("prompt path %q exceeds %d bytes", e.File, maxFileSize)
	}
	return abs, e.File, category, nil
}

func checkPromptScope(e PromptEntry, abs string, skillDirs []string) error {
	switch {
	case strings.HasPrefix(e.File, "./"):
		root, err := filepath.EvalSymlinks(e.BaseDir)
		if err != nil {
			return err
		}
		if root, err = filepath.Abs(root); err != nil {
			return err
		}
		if !below(root, abs) {
			return fmt.Errorf("prompt path %q escapes its root", e.File)
		}
	case strings.HasPrefix(e.File, "../"):
		for _, dir := range skillDirs {
			if canon, err := filepath.EvalSymlinks(dir); err == nil {
				dir = canon
			}
			if below(dir, abs) {
				return nil
			}
		}
		return fmt.Errorf("prompt path %q is outside every declared skill directory", e.File)
	}
	return nil
}
