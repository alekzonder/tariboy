package imagefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxExtendsDepth = 16
	// ExtendsPromptDir keeps each layer's prompt files apart, so equal paths
	// declared by different images stay different files.
	ExtendsPromptDir = ".tariboy-extends"
	skillsLockFile   = "skills-lock.json"
	// ExtendsUnassembledMessage explains why a build consumer that reads a
	// source in place refuses extends.
	ExtendsUnassembledMessage = "extends is supported only when building from a path or a Store"
)

// Layers returns the image directories an extends chain assembles: every
// parent's own parents first, then the parent, in declaration order, with dir
// last. A directory reached twice (a diamond) keeps its first position.
func Layers(dir string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	var visit func(dir string, stack []string) error
	visit = func(dir string, stack []string) error {
		canonical, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return fmt.Errorf("extends: image %s is missing: %w", dir, err)
		}
		if slices.Contains(stack, canonical) {
			return fmt.Errorf("extends: cycle through %s", canonical)
		}
		if len(stack) >= maxExtendsDepth {
			return fmt.Errorf("extends: chain is deeper than %d images", maxExtendsDepth)
		}
		if seen[canonical] {
			return nil
		}
		src, err := ParseV2(canonical)
		if err != nil {
			return fmt.Errorf("extends: %s: %w", canonical, err)
		}
		stack = append(stack, canonical)
		for _, parent := range src.Extends {
			if !filepath.IsAbs(parent) {
				parent = filepath.Join(canonical, parent)
			}
			if err := visit(parent, stack); err != nil {
				return err
			}
		}
		seen[canonical] = true
		out = append(out, canonical)
		return nil
	}
	if err := visit(dir, nil); err != nil {
		return nil, err
	}
	return out, nil
}

// Assemble builds an extends chain into one plain schema-v2 source under a new
// owner-only directory in root. Layers are copied into it in order, each over
// the previous one; after each copy install, when set, restores that layer's
// skills lock in the build directory, and the lock is then removed. The
// merged Tariboyfile has no extends, so the ordinary build consumes it.
func Assemble(dir, root string, roots ResolveRoots, install func(dir string) error) (string, func(), error) {
	layers, err := Layers(dir)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", nil, err
	}
	build, err := os.MkdirTemp(root, "extends-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(build) }
	merged := V2{SchemaVersion: 2}
	for k, layer := range layers {
		src, err := ParseV2(layer)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		if err := assembleLayer(k, layer, build, src, install); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("extends: layer %s: %w", layer, err)
		}
		merged.ImageVersion = src.ImageVersion
		merged.Plugins = append(merged.Plugins, src.Plugins...)
		merged.Skills = append(merged.Skills, src.Skills...)
		merged.Prompts = append(merged.Prompts, src.Prompts...)
	}
	merged.Plugins = uniquePlugins(merged.Plugins)
	merged.Skills = uniqueSkills(merged.Skills)
	if merged.Prompts, err = uniquePrompts(build, merged.Prompts, roots); err != nil {
		cleanup()
		return "", nil, err
	}
	body, err := yaml.Marshal(merged)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(build, DefaultFilename), body, 0o600); err != nil {
		cleanup()
		return "", nil, err
	}
	return build, cleanup, nil
}

// assembleLayer copies one layer into build and rewrites src's paths so they
// keep naming the same files from build.
func assembleLayer(k int, layer, build string, src *V2, install func(string) error) error {
	if err := copyTree(layer, build); err != nil {
		return err
	}
	lock := filepath.Join(build, skillsLockFile)
	if _, err := os.Lstat(lock); err == nil {
		if install != nil {
			if err := install(build); err != nil {
				return fmt.Errorf("install skills lock: %w", err)
			}
		}
		if err := os.Remove(lock); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for i, skill := range src.Skills {
		if strings.HasPrefix(skill.Dir, "../") {
			src.Skills[i].Dir = filepath.Join(layer, skill.Dir)
		}
	}
	for i, prompt := range src.Prompts {
		switch {
		case strings.HasPrefix(prompt.File, "../"):
			src.Prompts[i].File = filepath.Join(layer, prompt.File)
		case strings.HasPrefix(prompt.File, "./"):
			rel := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(prompt.File, "./")))
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("prompt path %q escapes its root", prompt.File)
			}
			kept := filepath.Join(ExtendsPromptDir, strconv.Itoa(k), rel)
			if err := copyFile(filepath.Join(build, rel), filepath.Join(build, kept)); err != nil {
				return fmt.Errorf("prompt %q: %w", prompt.File, err)
			}
			src.Prompts[i].File = "./" + filepath.ToSlash(kept)
		}
	}
	return nil
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if rel == ExtendsPromptDir && entry.IsDir() {
			return fmt.Errorf("source uses reserved directory %s", ExtendsPromptDir)
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("symlink %s", rel)
		case entry.IsDir():
			return os.MkdirAll(filepath.Join(to, rel), 0o700)
		case !entry.Type().IsRegular():
			return fmt.Errorf("non-regular file %s", rel)
		}
		return copyFile(path, filepath.Join(to, rel))
	})
}

func copyFile(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", from)
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	// Remove first so a later layer replaces the file and its mode.
	if err := os.Remove(to); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(to, data, info.Mode().Perm()|0o600)
}

func uniquePlugins(plugins []V2Plugin) []V2Plugin {
	seen := map[string]bool{}
	out := []V2Plugin{}
	for _, plugin := range plugins {
		if !seen[plugin.Name] {
			seen[plugin.Name] = true
			out = append(out, plugin)
		}
	}
	return out
}

// uniqueSkills keeps the last entry of each skill name; a skill directory's
// base name is its name, so a later layer replaces an earlier one.
func uniqueSkills(skills []SkillEntry) []SkillEntry {
	last := map[string]int{}
	for i, skill := range skills {
		last[filepath.Base(filepath.Clean(skill.Dir))] = i
	}
	out := []SkillEntry{}
	for i, skill := range skills {
		if last[filepath.Base(filepath.Clean(skill.Dir))] == i {
			out = append(out, skill)
		}
	}
	return out
}

// uniquePrompts keeps the first entry of each runtime placeholder and of each
// prompt file content.
func uniquePrompts(build string, prompts []PromptEntry, roots ResolveRoots) ([]PromptEntry, error) {
	seen := map[string]bool{}
	out := []PromptEntry{}
	for _, prompt := range prompts {
		key := "runtime:" + prompt.Runtime
		if prompt.File != "" {
			resolved, err := ResolvePromptFile(build, prompt.File, roots)
			if err != nil {
				return nil, err
			}
			key = "file:" + resolved.SHA256
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, prompt)
		}
	}
	return out, nil
}
