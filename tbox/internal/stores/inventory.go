package stores

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

const errNoSkill = "SKILL.md not found"

const (
	maxSkillFile = 256 << 10
	// maxRows bounds the rows of each kind; a hostile Store may hold millions.
	maxRows = 1000
)

// capNames cuts names to maxRows and returns the number left out.
func capNames(names []string) ([]string, int) {
	if len(names) <= maxRows {
		return names, 0
	}
	return names[:maxRows], len(names) - maxRows
}

func moreText(n int) string { return fmt.Sprintf("… %d more not shown", n) }

// Image is an image source found in a Store.
type Image struct {
	Name, Dir, Version string
	Schema             int
	Err                string
}

// Skill is a skill found in a Store.
type Skill struct {
	Name, Dir, Description string
	Err                    string
}

// Prompt is a prompt file found in a Store.
type Prompt struct{ Name, Path, Err string }

// Inventory lists the content of a Store. Rows with a problem carry Err and
// do not hide the other rows.
type Inventory struct {
	Images  []Image
	Skills  []Skill
	Prompts []Prompt
}

// Inventory reads the layout of a Store. It parses files and executes
// nothing; it does not follow symlinks below the Store root.
func (r Registry) Inventory(s Store) (Inventory, error) {
	root := r.Root(s)
	fi, err := os.Stat(root)
	if err != nil {
		return Inventory{}, fmt.Errorf("store %s: %w", s.Name, err)
	}
	if !fi.IsDir() {
		return Inventory{}, fmt.Errorf("store %s: %s is not a directory", s.Name, root)
	}
	inv := Inventory{
		Images:  readImages(filepath.Join(root, "images")),
		Skills:  readSkills(root),
		Prompts: readPrompts(filepath.Join(root, "prompts")),
	}
	return inv, nil
}

// subdirs lists the real subdirectories of dir; a dir that is not itself a
// real directory (a symlink, a file, absent) yields none.
func subdirs(dir string) []string {
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func readImages(dir string) []Image {
	var out []Image
	names, more := capNames(subdirs(dir))
	for _, name := range names {
		imgDir := filepath.Join(dir, name)
		file := filepath.Join(imgDir, tfile.Filename)
		fi, err := os.Lstat(file)
		if err != nil {
			continue
		}
		row := Image{Name: name, Dir: imgDir}
		if !fi.Mode().IsRegular() {
			row.Err = tfile.Filename + " is not a regular file"
		} else if f, err := tfile.Parse(file); err != nil {
			row.Err = err.Error()
		} else {
			row.Version, row.Schema = f.ImageVersion, f.SchemaVersion
		}
		out = append(out, row)
	}
	if more > 0 {
		out = append(out, Image{Err: moreText(more)})
	}
	return out
}

// readSkills lists skills/<name>; without a skills/ directory it looks in the
// known places of other tools: the root, .agents/skills and .claude/skills.
func readSkills(root string) []Skill {
	if fi, err := os.Lstat(filepath.Join(root, "skills")); err == nil && fi.IsDir() {
		return skillRows(filepath.Join(root, "skills"))
	}
	var out []Skill
	if _, err := os.Lstat(filepath.Join(root, "SKILL.md")); err == nil {
		out = append(out, rootSkill(root))
	}
	out = append(out, skillRows(filepath.Join(root, ".agents", "skills"))...)
	out = append(out, skillRows(filepath.Join(root, ".claude", "skills"))...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func skillRows(dir string) []Skill {
	var out []Skill
	names, more := capNames(subdirs(dir))
	for _, name := range names {
		row := Skill{Name: name, Dir: filepath.Join(dir, name)}
		var fmName string
		row.Description, fmName, row.Err = describe(row.Dir)
		if row.Err == errNoSkill {
			continue
		}
		if row.Err == "" && fmName != name {
			row.Err = fmt.Sprintf("frontmatter name %q does not match directory %q", fmName, name)
		}
		out = append(out, row)
	}
	if more > 0 {
		out = append(out, Skill{Err: moreText(more)})
	}
	return out
}

// rootSkill is the skill whose SKILL.md sits at the Store root; its name
// comes from the frontmatter.
func rootSkill(root string) Skill {
	row := Skill{Name: filepath.Base(root), Dir: root}
	var name string
	row.Description, name, row.Err = describe(root)
	if name != "" {
		row.Name = name
	}
	return row
}

// describe reads the description and name from dir/SKILL.md, read with no
// follow. The error is returned as text for the row.
func describe(dir string) (description, name, errText string) {
	body, err := fsafe.ReadFileNoFollow(filepath.Join(dir, "SKILL.md"), maxSkillFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", errNoSkill
		}
		return "", "", err.Error()
	}
	name, description, err = skill.ReadMeta(body)
	if err != nil {
		return "", "", err.Error()
	}
	return description, name, ""
}

func readPrompts(dir string) []Prompt {
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Prompt
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".md") && e.Name() != ".md" {
			out = append(out, Prompt{Name: strings.TrimSuffix(e.Name(), ".md"), Path: filepath.Join(dir, e.Name())})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > maxRows {
		more := len(out) - maxRows
		out = append(out[:maxRows], Prompt{Err: moreText(more)})
	}
	return out
}
