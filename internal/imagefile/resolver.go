package imagefile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxPromptFileSize = 4 << 20

type ResolveRoots struct {
	Plugins string
	// SourceSkills pins source-relative declarations to an immutable snapshot.
	// A non-nil map is exhaustive: missing declarations fail closed.
	SourceSkills map[string]string
}

type ResolvedFile struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

type ResolvedDirectory struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	Category string `json:"category"`
}

func resolveExplicitPath(sourceDir, value string, roots ResolveRoots, kind string) (string, string, error) {
	var root, suffix, category string
	switch {
	case strings.HasPrefix(value, "$PLUGINS/"):
		root, suffix, category = roots.Plugins, strings.TrimPrefix(value, "$PLUGINS/"), "plugin"
	case strings.HasPrefix(value, "./"):
		root, suffix, category = sourceDir, strings.TrimPrefix(value, "./"), "source"
	case kind == "skill" && strings.HasPrefix(value, "../"):
		root, suffix, category = sourceDir, value, "source"
	case filepath.IsAbs(value):
		category = "absolute"
	default:
		if kind == "skill" {
			return "", "", fmt.Errorf("skill path %q must use ./, ../, an absolute path, or $PLUGINS", value)
		}
		return "", "", fmt.Errorf("%s path %q must use ./, an absolute path, or $PLUGINS", kind, value)
	}
	var candidate string
	if category == "absolute" {
		candidate = filepath.Clean(value)
	} else {
		if root == "" || suffix == "" {
			return "", "", fmt.Errorf("%s path %q has an empty root or path", kind, value)
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			return "", "", err
		}
		candidate = filepath.Join(rootAbs, filepath.FromSlash(suffix))
		rel, err := filepath.Rel(rootAbs, candidate)
		if err != nil || ((rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) && !(kind == "skill" && category == "source")) {
			return "", "", fmt.Errorf("%s path %q escapes its root", kind, value)
		}
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", fmt.Errorf("resolve %s path %q: %w", kind, value, err)
	}
	if resolved != abs {
		return "", "", fmt.Errorf("%s path %q contains a symlink", kind, value)
	}
	return abs, category, nil
}

func ResolvePromptFile(sourceDir, value string, roots ResolveRoots) (ResolvedFile, error) {
	path := value
	fromSourceSkill := false
	if strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		path, fromSourceSkill = resolveSourceSkillFile(value, roots.SourceSkills)
	}
	abs, category, err := resolveExplicitPath(sourceDir, path, roots, "prompt")
	if err != nil {
		return ResolvedFile{}, err
	}
	if fromSourceSkill {
		category = "source"
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return ResolvedFile{}, err
	}
	if !info.Mode().IsRegular() {
		return ResolvedFile{}, fmt.Errorf("prompt path %q is not a regular file", value)
	}
	if info.Size() > maxPromptFileSize {
		return ResolvedFile{}, fmt.Errorf("prompt path %q exceeds %d bytes", value, maxPromptFileSize)
	}
	f, err := os.Open(abs)
	if err != nil {
		return ResolvedFile{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxPromptFileSize+1))
	if err != nil {
		return ResolvedFile{}, err
	}
	if n > maxPromptFileSize {
		return ResolvedFile{}, fmt.Errorf("prompt path %q exceeds %d bytes", value, maxPromptFileSize)
	}
	return ResolvedFile{Source: value, Path: abs, Category: category, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func resolveSourceSkillFile(value string, sourceSkills map[string]string) (string, bool) {
	path := filepath.Clean(filepath.FromSlash(value))
	var bestSource, bestRoot, bestRelative string
	for source, root := range sourceSkills {
		cleanSource := filepath.Clean(filepath.FromSlash(source))
		relative, err := filepath.Rel(cleanSource, path)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if len(cleanSource) > len(bestSource) {
			bestSource, bestRoot, bestRelative = cleanSource, root, relative
		}
	}
	if bestSource == "" {
		return value, false
	}
	return filepath.Join(bestRoot, bestRelative), true
}

func ResolveSkillDirectory(sourceDir, value string, roots ResolveRoots) (ResolvedDirectory, error) {
	path := value
	frozen := roots.SourceSkills != nil && (strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../"))
	if frozen {
		var ok bool
		path, ok = roots.SourceSkills[value]
		if !ok {
			return ResolvedDirectory{}, fmt.Errorf("skill path %q is missing from the source snapshot", value)
		}
	}
	abs, category, err := resolveExplicitPath(sourceDir, path, roots, "skill")
	if err != nil {
		return ResolvedDirectory{}, err
	}
	if frozen {
		category = "source"
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return ResolvedDirectory{}, err
	}
	if !info.IsDir() {
		return ResolvedDirectory{}, fmt.Errorf("skill path %q is not a directory", value)
	}
	return ResolvedDirectory{Source: value, Path: abs, Category: category}, nil
}
