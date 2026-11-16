package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

// ValidatePrepared verifies a skill rebuilt from archive members, with the
// rules of agentskills.ValidatePrepared: it derives integrity from the member
// bytes instead of trusting the metadata the archive supplies. files must be
// sorted by relative path. A root-level evals member is judged like any other.
func ValidatePrepared(meta Metadata, files []File) error {
	if len(files) == 0 || len(files) > MaxFilesPerSkill {
		return fmt.Errorf("skill %q has invalid file count", meta.Name)
	}
	if meta.ArchiveRoot != "skills/"+meta.Name {
		return fmt.Errorf("skill %q has invalid archive root", meta.Name)
	}
	skillBody, total, err := validateMembers(meta.Name, files)
	if err != nil {
		return err
	}
	if skillBody == nil {
		return fmt.Errorf("skill %q is missing SKILL.md", meta.Name)
	}
	front, err := parseFrontmatter(skillBody)
	if err != nil {
		return err
	}
	if front.Name != meta.Name || front.Description != meta.Description {
		return fmt.Errorf("skill %q frontmatter does not match manifest", meta.Name)
	}
	if meta.FileCount != len(files) || meta.Size != total {
		return fmt.Errorf("skill %q aggregate metadata mismatch", meta.Name)
	}
	hash, err := treeHash(files)
	if err != nil {
		return err
	}
	if meta.TreeSHA256 != hash {
		return fmt.Errorf("skill %q tree integrity mismatch", meta.Name)
	}
	return nil
}

// validateMembers checks each member's path, order, size and digest. It
// returns the SKILL.md body, nil when there is none, and the total size.
func validateMembers(name string, files []File) ([]byte, int64, error) {
	var total int64
	var skillBody []byte
	previous := ""
	for i, file := range files {
		if !validMemberPath(file.RelativePath) {
			return nil, 0, fmt.Errorf("skill %q has invalid member path %q", name, file.RelativePath)
		}
		if i > 0 && file.RelativePath <= previous {
			return nil, 0, fmt.Errorf("skill %q members are duplicate or unsorted", name)
		}
		previous = file.RelativePath
		if file.Size < 0 || file.Size > MaxFileBytes || int64(len(file.Body)) != file.Size {
			return nil, 0, fmt.Errorf("skill %q member %q has invalid size", name, file.RelativePath)
		}
		if total > MaxSkillBytes-file.Size {
			return nil, 0, fmt.Errorf("skill %q exceeds %d aggregate bytes", name, MaxSkillBytes)
		}
		total += file.Size
		sum := sha256.Sum256(file.Body)
		if file.SHA256 != hex.EncodeToString(sum[:]) {
			return nil, 0, fmt.Errorf("skill %q member %q integrity mismatch", name, file.RelativePath)
		}
		if file.RelativePath == "SKILL.md" {
			skillBody = file.Body
		}
	}
	return skillBody, total, nil
}

// validMemberPath reports whether rel is a clean relative slash path that
// stays inside the skill.
func validMemberPath(rel string) bool {
	clean := path.Clean(rel)
	return rel != "" && clean == rel && clean != "." && !strings.HasPrefix(clean, "../") && !path.IsAbs(clean) && !strings.ContainsRune(rel, '\\')
}
