package agentskills

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"golang.org/x/sys/unix"
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
	RelativePath string `json:"relative_path"`
	SourcePath   string `json:"-"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	Executable   bool   `json:"executable"`
	Body         []byte `json:"-"`
}

type Prepared struct {
	Metadata Metadata
	Files    []File
}

func isHardlinked(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}

// readRegular is used by best-effort scope discovery, where only one fixed
// SKILL.md member is inspected. Full image preparation uses descriptor-relative
// traversal below so every path component remains pinned.
func readRegular(path string, before os.FileInfo) ([]byte, os.FileInfo, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = syscall.Close(fd)
		return nil, nil, errors.New("open regular skill file")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) || isHardlinked(opened) {
		return nil, nil, errors.New("skill member changed or is linked")
	}
	body, err := readPinnedRegular(f, opened)
	if err != nil {
		return nil, nil, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !snapshotUnchanged(opened, after) {
		return nil, nil, errors.New("skill source changed while reading")
	}
	return body, opened, nil
}

func openSkillMember(parent *os.File, name, displayPath string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), displayPath)
	if f == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("open skill member")
	}
	return f, nil
}

func openPinnedSkillRoot(root string) (*os.File, error) {
	clean := filepath.Clean(root)
	if !filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return nil, errors.New("skill source root must be an absolute Unix path")
	}
	current, err := os.Open(string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	if clean == string(filepath.Separator) {
		return current, nil
	}
	walked := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			current.Close()
			return nil, errors.New("invalid skill source component")
		}
		walked = filepath.Join(walked, component)
		fd, openErr := unix.Openat(int(current.Fd()), component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			current.Close()
			return nil, fmt.Errorf("open skill source component %q: %w", walked, openErr)
		}
		next := os.NewFile(uintptr(fd), walked)
		if next == nil {
			_ = unix.Close(fd)
			current.Close()
			return nil, errors.New("open skill source component")
		}
		if closeErr := current.Close(); closeErr != nil {
			next.Close()
			return nil, closeErr
		}
		current = next
	}
	return current, nil
}

func snapshotUnchanged(before, after os.FileInfo) bool {
	return os.SameFile(before, after) &&
		before.Mode() == after.Mode() &&
		before.Size() == after.Size() &&
		before.ModTime().Equal(after.ModTime())
}

func readPinnedRegular(f *os.File, opened os.FileInfo) ([]byte, error) {
	if !opened.Mode().IsRegular() || isHardlinked(opened) {
		return nil, errors.New("skill member changed or is linked")
	}
	if opened.Size() > MaxFileBytes {
		return nil, fmt.Errorf("skill file exceeds %d bytes", MaxFileBytes)
	}
	body, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxFileBytes {
		return nil, fmt.Errorf("skill file exceeds %d bytes", MaxFileBytes)
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !snapshotUnchanged(opened, after) {
		return nil, errors.New("skill source changed while reading")
	}
	return body, nil
}

func verifyCurrentMember(parent *os.File, name, displayPath string, opened os.FileInfo) error {
	current, err := openSkillMember(parent, name, displayPath)
	if err != nil {
		return err
	}
	defer current.Close()
	currentInfo, err := current.Stat()
	if err != nil {
		return err
	}
	if !snapshotUnchanged(opened, currentInfo) {
		return errors.New("skill source changed while reading")
	}
	return nil
}

func walkSkillDirectory(dir *os.File, root, relDir string, beforeOpen func(string) error, files *[]File, total *int64) error {
	dirBefore, err := dir.Stat()
	if err != nil {
		return err
	}
	if !dirBefore.IsDir() {
		return errors.New("skill member is not a directory")
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) {
			return fmt.Errorf("invalid skill member name %q", name)
		}
		rel := name
		if relDir != "" {
			rel = filepath.Join(relDir, name)
		}
		relSlash := filepath.ToSlash(rel)
		if beforeOpen != nil {
			if err := beforeOpen(relSlash); err != nil {
				return err
			}
		}
		displayPath := filepath.Join(root, rel)
		member, err := openSkillMember(dir, name, displayPath)
		if err != nil {
			return fmt.Errorf("open skill member %q: %w", relSlash, err)
		}
		opened, statErr := member.Stat()
		if statErr != nil {
			member.Close()
			return statErr
		}
		switch {
		case opened.IsDir():
			err = walkSkillDirectory(member, root, rel, beforeOpen, files, total)
		case opened.Mode().IsRegular():
			if isHardlinked(opened) {
				err = fmt.Errorf("skill member %q is a hardlink", relSlash)
				break
			}
			if len(*files) >= MaxFilesPerSkill {
				err = fmt.Errorf("skill has more than %d files", MaxFilesPerSkill)
				break
			}
			var body []byte
			body, err = readPinnedRegular(member, opened)
			if err != nil {
				err = fmt.Errorf("read skill member %q: %w", relSlash, err)
				break
			}
			if *total > MaxSkillBytes-int64(len(body)) {
				err = fmt.Errorf("skill exceeds %d aggregate bytes", MaxSkillBytes)
				break
			}
			*total += int64(len(body))
			sum := sha256.Sum256(body)
			*files = append(*files, File{
				RelativePath: relSlash,
				SourcePath:   displayPath,
				Size:         int64(len(body)),
				SHA256:       hex.EncodeToString(sum[:]),
				Executable:   opened.Mode().Perm()&0o100 != 0,
				Body:         body,
			})
		default:
			err = fmt.Errorf("skill member %q is not a regular file or directory", relSlash)
		}
		closeErr := member.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err := verifyCurrentMember(dir, name, displayPath, opened); err != nil {
			return fmt.Errorf("verify skill member %q: %w", relSlash, err)
		}
	}
	dirAfter, err := dir.Stat()
	if err != nil {
		return err
	}
	if !snapshotUnchanged(dirBefore, dirAfter) {
		return errors.New("skill directory changed while reading")
	}
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

func Prepare(resolved imagefile.ResolvedDirectory) (Prepared, error) {
	return prepareWithHook(resolved, nil)
}

func prepareWithHook(resolved imagefile.ResolvedDirectory, beforeDescend func(string) error) (Prepared, error) {
	root, err := filepath.Abs(resolved.Path)
	if err != nil {
		return Prepared{}, err
	}
	rootFile, err := openPinnedSkillRoot(root)
	if err != nil {
		return Prepared{}, err
	}
	defer rootFile.Close()
	openedRootInfo, err := rootFile.Stat()
	if err != nil {
		return Prepared{}, err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return Prepared{}, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return Prepared{}, errors.New("skill source must be a regular directory")
	}
	if !openedRootInfo.IsDir() || !os.SameFile(rootInfo, openedRootInfo) {
		return Prepared{}, errors.New("skill source changed while opening")
	}
	files := make([]File, 0)
	var total int64
	err = walkSkillDirectory(rootFile, root, "", beforeDescend, &files, &total)
	if err != nil {
		return Prepared{}, err
	}
	currentRootInfo, err := os.Lstat(root)
	if err != nil || !snapshotUnchanged(openedRootInfo, currentRootInfo) {
		return Prepared{}, errors.New("skill source root changed while reading")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].RelativePath < files[j].RelativePath })
	var skillBody []byte
	for _, file := range files {
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
	if filepath.Base(root) != front.Name {
		return Prepared{}, fmt.Errorf("skill name %q must match source directory basename %q", front.Name, filepath.Base(root))
	}
	hash, err := treeHash(files)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{
		Metadata: Metadata{
			Name:        front.Name,
			Description: front.Description,
			Source:      resolved.Source,
			Category:    resolved.Category,
			ArchiveRoot: "skills/" + front.Name,
			FileCount:   len(files),
			Size:        total,
			TreeSHA256:  hash,
		},
		Files: files,
	}, nil
}

func ValidateSet(skills []Prepared) error {
	if len(skills) > MaxSkills {
		return fmt.Errorf("skills has %d entries, maximum is %d", len(skills), MaxSkills)
	}
	seen := make(map[string]bool, len(skills))
	var total int64
	for i, skill := range skills {
		if skill.Metadata.Name == "" {
			return fmt.Errorf("skills[%d]: name is required", i)
		}
		if seen[skill.Metadata.Name] {
			return fmt.Errorf("skills[%d]: duplicate skill name %q", i, skill.Metadata.Name)
		}
		seen[skill.Metadata.Name] = true
		if skill.Metadata.Size < 0 || total > MaxImageSkillBytes-skill.Metadata.Size {
			return fmt.Errorf("skills exceed %d aggregate bytes", MaxImageSkillBytes)
		}
		total += skill.Metadata.Size
	}
	return nil
}

// ValidatePrepared verifies a skill reconstructed from an immutable archive.
// It deliberately derives integrity from member bytes rather than trusting the
// manifest fields supplied by the archive.
func ValidatePrepared(skill Prepared) error {
	if len(skill.Files) == 0 || len(skill.Files) > MaxFilesPerSkill {
		return fmt.Errorf("skill %q has invalid file count", skill.Metadata.Name)
	}
	if skill.Metadata.ArchiveRoot != "skills/"+skill.Metadata.Name {
		return fmt.Errorf("skill %q has invalid archive root", skill.Metadata.Name)
	}
	var total int64
	var skillBody []byte
	previous := ""
	for i, file := range skill.Files {
		clean := path.Clean(file.RelativePath)
		if file.RelativePath == "" || clean != file.RelativePath || clean == "." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || strings.ContainsRune(file.RelativePath, '\\') {
			return fmt.Errorf("skill %q has invalid member path %q", skill.Metadata.Name, file.RelativePath)
		}
		if i > 0 && file.RelativePath <= previous {
			return fmt.Errorf("skill %q members are duplicate or unsorted", skill.Metadata.Name)
		}
		previous = file.RelativePath
		if file.Size < 0 || file.Size > MaxFileBytes || int64(len(file.Body)) != file.Size {
			return fmt.Errorf("skill %q member %q has invalid size", skill.Metadata.Name, file.RelativePath)
		}
		if total > MaxSkillBytes-file.Size {
			return fmt.Errorf("skill %q exceeds %d aggregate bytes", skill.Metadata.Name, MaxSkillBytes)
		}
		total += file.Size
		sum := sha256.Sum256(file.Body)
		if file.SHA256 != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("skill %q member %q integrity mismatch", skill.Metadata.Name, file.RelativePath)
		}
		if file.RelativePath == "SKILL.md" {
			skillBody = file.Body
		}
	}
	if skillBody == nil {
		return fmt.Errorf("skill %q is missing SKILL.md", skill.Metadata.Name)
	}
	front, err := parseFrontmatter(skillBody)
	if err != nil {
		return err
	}
	if front.Name != skill.Metadata.Name || front.Description != skill.Metadata.Description {
		return fmt.Errorf("skill %q frontmatter does not match manifest", skill.Metadata.Name)
	}
	if skill.Metadata.FileCount != len(skill.Files) || skill.Metadata.Size != total {
		return fmt.Errorf("skill %q aggregate metadata mismatch", skill.Metadata.Name)
	}
	hash, err := treeHash(skill.Files)
	if err != nil {
		return err
	}
	if skill.Metadata.TreeSHA256 != hash {
		return fmt.Errorf("skill %q tree integrity mismatch", skill.Metadata.Name)
	}
	return nil
}
