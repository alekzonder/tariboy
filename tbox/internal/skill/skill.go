// Package skill validates and packages Agent Skill directories with the same
// rules and the same tree hash as the tariboy agentskills package.
package skill

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

const (
	MaxSkills          = 128
	MaxFilesPerSkill   = 1024
	MaxFileBytes       = 8 << 20
	MaxSkillBytes      = 32 << 20
	MaxImageSkillBytes = 128 << 20
)

type Metadata struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Source        string `json:"source"`
	Category      string `json:"category"`
	ClientVersion string `json:"client_version,omitempty"`
	ArchiveRoot   string `json:"archive_root"`
	FileCount     int    `json:"file_count"`
	Size          int64  `json:"size"`
	TreeSHA256    string `json:"tree_sha256"`
}

type File struct {
	RelativePath string
	Size         int64
	SHA256       string
	Executable   bool
	Body         []byte
}

type Prepared struct {
	Metadata Metadata
	Files    []File
}

func isHardlinked(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}

func snapshotUnchanged(before, after os.FileInfo) bool {
	return os.SameFile(before, after) &&
		before.Mode() == after.Mode() &&
		before.Size() == after.Size() &&
		before.ModTime().Equal(after.ModTime())
}

type walker struct {
	root  *os.Root
	files []File
	total int64
}

// Prepare reads the skill directory dir. Symlinks, hardlinks and special files
// are rejected, a root-level evals directory is skipped, and the skill name
// must equal the directory name.
func Prepare(dir, source, category string) (Prepared, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Prepared{}, err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Prepared{}, err
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return Prepared{}, err
	}
	defer root.Close()
	rootInfo, err := os.Lstat(canonical)
	if err != nil {
		return Prepared{}, err
	}
	if !rootInfo.IsDir() {
		return Prepared{}, errors.New("skill source must be a regular directory")
	}
	w := &walker{root: root, files: make([]File, 0)}
	if err := w.walk("."); err != nil {
		return Prepared{}, err
	}
	after, err := os.Lstat(canonical)
	if err != nil || !snapshotUnchanged(rootInfo, after) {
		return Prepared{}, errors.New("skill source root changed while reading")
	}
	return w.finish(canonical, source, category)
}

func (w *walker) finish(canonical, source, category string) (Prepared, error) {
	sort.Slice(w.files, func(i, j int) bool { return w.files[i].RelativePath < w.files[j].RelativePath })
	var skillBody []byte
	for _, file := range w.files {
		if file.RelativePath == "SKILL.md" {
			skillBody = file.Body
			break
		}
	}
	if skillBody == nil {
		return Prepared{}, errors.New("skill source must contain regular SKILL.md")
	}
	front, err := parseFrontmatter(skillBody)
	if err != nil {
		return Prepared{}, err
	}
	if filepath.Base(canonical) != front.Name {
		return Prepared{}, fmt.Errorf("skill name %q must match source directory basename %q", front.Name, filepath.Base(canonical))
	}
	hash, err := treeHash(w.files)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{
		Metadata: Metadata{
			Name:        front.Name,
			Description: front.Description,
			Source:      source,
			Category:    category,
			ArchiveRoot: "skills/" + front.Name,
			FileCount:   len(w.files),
			Size:        w.total,
			TreeSHA256:  hash,
		},
		Files: w.files,
	}, nil
}

func (w *walker) walk(rel string) error {
	dir, err := w.root.Open(rel)
	if err != nil {
		return fmt.Errorf("open skill member %q: %w", rel, err)
	}
	defer dir.Close()
	before, err := dir.Stat()
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := w.entry(rel, entry); err != nil {
			return err
		}
	}
	after, err := dir.Stat()
	if err != nil {
		return err
	}
	if !snapshotUnchanged(before, after) {
		return errors.New("skill directory changed while reading")
	}
	return nil
}

func (w *walker) entry(parent string, entry os.DirEntry) error {
	name := entry.Name()
	// Skill evals are authoring evidence, so a root-level evals directory
	// stays out of the package.
	if parent == "." && name == "evals" && entry.IsDir() {
		return nil
	}
	rel := name
	if parent != "." {
		rel = parent + "/" + name
	}
	info, err := w.root.Lstat(rel)
	if err != nil {
		return fmt.Errorf("stat skill member %q: %w", rel, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("skill member %q is a symlink", rel)
	case info.IsDir():
		return w.walk(rel)
	case info.Mode().IsRegular():
		return w.collect(rel, info)
	default:
		return fmt.Errorf("skill member %q is not a regular file or directory", rel)
	}
}

func (w *walker) collect(rel string, listed os.FileInfo) error {
	if isHardlinked(listed) {
		return fmt.Errorf("skill member %q is a hardlink", rel)
	}
	if len(w.files) >= MaxFilesPerSkill {
		return fmt.Errorf("skill has more than %d files", MaxFilesPerSkill)
	}
	f, err := w.root.Open(rel)
	if err != nil {
		return fmt.Errorf("open skill member %q: %w", rel, err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(listed, opened) || isHardlinked(opened) {
		return fmt.Errorf("skill member %q changed or is linked", rel)
	}
	if opened.Size() > MaxFileBytes {
		return fmt.Errorf("skill member %q exceeds %d bytes", rel, MaxFileBytes)
	}
	body, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return fmt.Errorf("read skill member %q: %w", rel, err)
	}
	if len(body) > MaxFileBytes {
		return fmt.Errorf("skill member %q exceeds %d bytes", rel, MaxFileBytes)
	}
	current, err := f.Stat()
	if err != nil {
		return err
	}
	if !snapshotUnchanged(opened, current) {
		return errors.New("skill source changed while reading")
	}
	if w.total > MaxSkillBytes-int64(len(body)) {
		return fmt.Errorf("skill exceeds %d aggregate bytes", MaxSkillBytes)
	}
	w.total += int64(len(body))
	sum := sha256.Sum256(body)
	w.files = append(w.files, File{
		RelativePath: rel,
		Size:         int64(len(body)),
		SHA256:       hex.EncodeToString(sum[:]),
		Executable:   opened.Mode().Perm()&0o100 != 0,
		Body:         body,
	})
	return nil
}

func treeHash(files []File) (string, error) {
	h := sha256.New()
	for _, file := range files {
		name := filepath.ToSlash(file.RelativePath)
		if err := binary.Write(h, binary.BigEndian, uint32(len(name))); err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, name)
		mode := uint32(0o600)
		if file.Executable {
			mode = 0o700
		}
		if err := binary.Write(h, binary.BigEndian, mode); err != nil {
			return "", err
		}
		if err := binary.Write(h, binary.BigEndian, uint64(file.Size)); err != nil {
			return "", err
		}
		digest, err := hex.DecodeString(file.SHA256)
		if err != nil {
			return "", err
		}
		_, _ = h.Write(digest)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ValidateSet checks that the skills of one image have unique names and stay
// within the aggregate limits.
func ValidateSet(skills []Prepared) error {
	if len(skills) > MaxSkills {
		return fmt.Errorf("skills has %d entries, maximum is %d", len(skills), MaxSkills)
	}
	seen := make(map[string]bool, len(skills))
	var total int64
	for i, s := range skills {
		if s.Metadata.Name == "" {
			return fmt.Errorf("skills[%d]: name is required", i)
		}
		if seen[s.Metadata.Name] {
			return fmt.Errorf("skills[%d]: duplicate skill name %q", i, s.Metadata.Name)
		}
		seen[s.Metadata.Name] = true
		if s.Metadata.Size < 0 || total > MaxImageSkillBytes-s.Metadata.Size {
			return fmt.Errorf("skills exceed %d aggregate bytes", MaxImageSkillBytes)
		}
		total += s.Metadata.Size
	}
	return nil
}
