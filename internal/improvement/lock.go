package improvement

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxLockedPromptBytes = 4 << 20

type Lock struct {
	SchemaVersion      int                         `yaml:"schema_version" json:"schema_version"`
	TariboyVersion     string                      `yaml:"tariboy_version" json:"tariboy_version"`
	PromptDependencies map[string]PromptDependency `yaml:"prompt_dependencies" json:"prompt_dependencies"`
}

type PromptDependency struct {
	Repository     string `yaml:"repository" json:"repository"`
	UpstreamCommit string `yaml:"upstream_commit" json:"upstream_commit"`
	UpstreamPath   string `yaml:"upstream_path" json:"upstream_path"`
	UpstreamSHA256 string `yaml:"upstream_sha256" json:"upstream_sha256"`
	LocalPath      string `yaml:"local_path" json:"local_path"`
	LocalSHA256    string `yaml:"local_sha256" json:"local_sha256,omitempty"`
	Mode           string `yaml:"mode" json:"mode"`
}

func LoadLock(sourceDir string) (Lock, error) {
	raw, err := os.ReadFile(filepath.Join(sourceDir, "tariboy.lock.yaml"))
	if err != nil {
		return Lock{}, err
	}
	var lock Lock
	if err := yaml.Unmarshal(raw, &lock); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func ValidateLock(sourceDir string, lock Lock) error {
	if lock.SchemaVersion != 1 || len(lock.PromptDependencies) == 0 {
		return fmt.Errorf("invalid tariboy lock schema or empty dependencies")
	}
	seen := map[string]bool{}
	for name, dependency := range lock.PromptDependencies {
		if name == "" || dependency.Repository == "" || dependency.UpstreamCommit == "" || !safeRelativePath(dependency.UpstreamPath) || !safeRelativePath(dependency.LocalPath) || seen[dependency.LocalPath] {
			return fmt.Errorf("invalid locked dependency %q", name)
		}
		seen[dependency.LocalPath] = true
		if dependency.Mode != "upstream" && dependency.Mode != "fork" {
			return fmt.Errorf("invalid lock mode for %q", name)
		}
		if dependency.Mode == "fork" && dependency.LocalSHA256 == "" {
			return fmt.Errorf("fork %q requires local_sha256", name)
		}
		info, err := os.Lstat(filepath.Join(sourceDir, filepath.FromSlash(dependency.LocalPath)))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxLockedPromptBytes {
			return fmt.Errorf("invalid locked file %q", dependency.LocalPath)
		}
		raw, err := os.ReadFile(filepath.Join(sourceDir, filepath.FromSlash(dependency.LocalPath)))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		actual := hex.EncodeToString(sum[:])
		expected := dependency.UpstreamSHA256
		if dependency.Mode == "fork" {
			expected = dependency.LocalSHA256
		}
		if strings.TrimPrefix(expected, "sha256:") != actual || strings.TrimSpace(dependency.UpstreamSHA256) == "" {
			return fmt.Errorf("locked file hash mismatch for %q", dependency.LocalPath)
		}
	}
	return nil
}
