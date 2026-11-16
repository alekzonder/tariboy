package tfile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const maxExtendsDepth = 16

// ErrRemoteUnsupported is returned for git and url entries until remote
// sources are implemented.
var ErrRemoteUnsupported = fmt.Errorf("remote sources are not supported yet")

// Load parses the Tariboyfile in dir and flattens its extends chain. Parents
// come first, the file itself last. Plugins keep the first declaration,
// skills keep the last one per skill identity, prompts are de-duplicated. The
// result has no Extends; entries keep the BaseDir of their own layer.
func Load(dir string) (*File, error) {
	layers, err := layers(dir)
	if err != nil {
		return nil, err
	}
	merged := &File{}
	for _, l := range layers {
		merged.Plugins = append(merged.Plugins, l.Plugins...)
		merged.Skills = append(merged.Skills, l.Skills...)
		merged.Prompts = append(merged.Prompts, l.Prompts...)
	}
	root := layers[len(layers)-1]
	merged.Dir = root.Dir
	merged.SchemaVersion = root.SchemaVersion
	merged.ImageVersion = root.ImageVersion
	merged.Plugins = uniquePlugins(merged.Plugins)
	merged.Skills = uniqueSkills(merged.Skills)
	merged.Prompts = uniquePrompts(merged.Prompts)
	return merged, nil
}

func layers(dir string) ([]*File, error) {
	var out []*File
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
		f, err := Parse(canonical)
		if err != nil {
			return fmt.Errorf("extends: %s: %w", canonical, err)
		}
		stack = append(stack, canonical)
		for _, parent := range f.Extends {
			if !filepath.IsAbs(parent) {
				parent = filepath.Join(canonical, parent)
			}
			if err := visit(parent, stack); err != nil {
				return err
			}
		}
		seen[canonical] = true
		out = append(out, f)
		return nil
	}
	if err := visit(dir, nil); err != nil {
		return nil, err
	}
	return out, nil
}

func uniquePlugins(plugins []Plugin) []Plugin {
	seen := map[string]bool{}
	out := []Plugin{}
	for _, p := range plugins {
		if !seen[p.Name] {
			seen[p.Name] = true
			out = append(out, p)
		}
	}
	return out
}

// skillIdentity is the skill directory name for dir entries, the base name of
// path for git entries and the URL for url entries.
func skillIdentity(s SkillEntry) string {
	switch {
	case s.Dir != "":
		return filepath.Base(filepath.Clean(s.Dir))
	case s.Git != "":
		return filepath.Base(filepath.Clean(s.Path))
	default:
		return s.URL
	}
}

func uniqueSkills(skills []SkillEntry) []SkillEntry {
	last := map[string]int{}
	for i, s := range skills {
		last[skillIdentity(s)] = i
	}
	out := []SkillEntry{}
	for i, s := range skills {
		if last[skillIdentity(s)] == i {
			out = append(out, s)
		}
	}
	return out
}

// promptKey identifies a prompt for de-duplication: a runtime name, a remote
// source, or the SHA-256 of a readable local file. An unreadable local file
// falls back to its declared location.
func promptKey(p PromptEntry) string {
	switch {
	case p.Runtime != "":
		return "runtime:" + p.Runtime
	case p.Git != "":
		return "git:" + p.Git + "|" + p.Path + "|" + p.Ref
	case p.URL != "":
		return "url:" + p.URL
	}
	if path, _, _, err := ResolvePromptFile(p); err == nil {
		if body, err := os.ReadFile(path); err == nil {
			sum := sha256.Sum256(body)
			return "file:" + hex.EncodeToString(sum[:])
		}
	}
	return "path:" + filepath.Join(p.BaseDir, p.File)
}

func uniquePrompts(prompts []PromptEntry) []PromptEntry {
	seen := map[string]bool{}
	out := []PromptEntry{}
	for _, p := range prompts {
		if key := promptKey(p); !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}
