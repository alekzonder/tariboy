package build

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

// checkTariboyOnlyScopes explains a ../ prompt that tfile.Load accepted
// against every skill directory but that lies only inside skills marked
// only: tariboy, which tbox drops. Without it the build would fail later
// with "outside every declared skill directory".
func checkTariboyOnlyScopes(full, tbox *tfile.File) error {
	kept := canonicalDirs(tbox.SkillDirs())
	for _, p := range tbox.Prompts {
		if !strings.HasPrefix(p.File, "../") {
			continue
		}
		abs, err := tfile.EntryPath(p.BaseDir, p.File, "prompt")
		if err != nil || insideAny(abs, kept) {
			continue
		}
		for _, s := range full.Skills {
			if s.Only != "tariboy" {
				continue
			}
			dirs := canonicalDirs((&tfile.File{Skills: []tfile.SkillEntry{s}}).SkillDirs())
			if insideAny(abs, dirs) {
				return fmt.Errorf("prompt %s is inside skill %s, which is marked only: tariboy; mark the prompt only: tariboy too", p.File, skillLabel(s))
			}
		}
	}
	return nil
}

func canonicalDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if canon, err := filepath.EvalSymlinks(d); err == nil {
			d = canon
		}
		out = append(out, d)
	}
	return out
}

func insideAny(path string, dirs []string) bool {
	for _, d := range dirs {
		rel, err := filepath.Rel(d, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
