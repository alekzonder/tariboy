package skillslock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// Hash returns the computedHash the skills installer writes into
// skills-lock.json for the skill directory dir, as tariboy's
// internal/stores.SkillHash computes it: sha256 over every file, each as its
// slash-separated relative path followed by its content. Files are ordered
// like JavaScript localeCompare; .git and node_modules directories below dir
// are skipped. Unlike tariboy, a symlink or other special file is an error,
// so Hash never reads outside dir, and a file larger than a skill member may
// be is an error; such a skill cannot be packaged anyway.
func Hash(dir string) (string, error) {
	type member struct{ rel, path string }
	var files []member
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != dir && (entry.Name() == ".git" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", filepath.ToSlash(rel))
		}
		files = append(files, member{rel: filepath.ToSlash(rel), path: path})
		return nil
	})
	if err != nil {
		return "", err
	}
	order := collate.New(language.Und)
	sort.SliceStable(files, func(i, j int) bool { return order.CompareString(files[i].rel, files[j].rel) < 0 })
	h := sha256.New()
	for _, file := range files {
		body, err := fsafe.ReadFileNoFollow(file.path, skill.MaxFileBytes)
		if err != nil {
			return "", err
		}
		h.Write([]byte(file.rel))
		h.Write(body)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
