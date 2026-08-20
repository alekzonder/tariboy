package agentskills

import (
	"os"
	"path/filepath"
	"sort"
)

type ScopeDuplicate struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

func readMetadata(dir string) (Metadata, error) {
	root, err := os.Lstat(dir)
	if err != nil {
		return Metadata{}, err
	}
	if !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		return Metadata{}, os.ErrInvalid
	}
	path := filepath.Join(dir, "SKILL.md")
	info, err := os.Lstat(path)
	if err != nil {
		return Metadata{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || isHardlinked(info) {
		return Metadata{}, os.ErrInvalid
	}
	body, _, err := readRegular(path, info)
	if err != nil {
		return Metadata{}, err
	}
	front, err := parseFrontmatter(body)
	if err != nil {
		return Metadata{}, err
	}
	if filepath.Base(dir) != front.Name {
		return Metadata{}, os.ErrInvalid
	}
	return Metadata{Name: front.Name, Description: front.Description}, nil
}

func FindScopeDuplicates(names []string, cwd, home string) []ScopeDuplicate {
	requested := make(map[string]bool, len(names))
	for _, name := range names {
		if skillNamePattern.MatchString(name) {
			requested[name] = true
		}
	}
	type scopeRoot struct {
		scope string
		path  string
	}
	roots := []scopeRoot{
		{"cwd", filepath.Join(cwd, ".claude", "skills")},
		{"cwd", filepath.Join(cwd, ".agents", "skills")},
		{"cwd", filepath.Join(cwd, ".opencode", "skills")},
		{"global", filepath.Join(home, ".claude", "skills")},
		{"global", filepath.Join(home, ".agents", "skills")},
		{"global", filepath.Join(home, ".codex", "skills")},
		{"global", filepath.Join(home, ".config", "opencode", "skills")},
	}
	seen := map[ScopeDuplicate]bool{}
	for name := range requested {
		for _, root := range roots {
			meta, err := readMetadata(filepath.Join(root.path, name))
			if err != nil || meta.Name != name {
				continue
			}
			seen[ScopeDuplicate{Name: name, Scope: root.scope}] = true
		}
	}
	out := make([]ScopeDuplicate, 0, len(seen))
	for duplicate := range seen {
		out = append(out, duplicate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Scope == "cwd" && out[j].Scope != "cwd"
	})
	return out
}
