package scripts

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestUIModulePathsDifferBeyondCase guards the macOS release build: on a
// case-insensitive filesystem an extensionless import such as "./workflowProblem"
// also resolves to WorkflowProblem.tsx, which breaks the UI typecheck there while
// Linux CI stays green.
func TestUIModulePathsDifferBeyondCase(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "ui", "src")
	seen := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		switch ext {
		case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
		default:
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		module := filepath.ToSlash(strings.TrimSuffix(rel, ext))
		key := strings.ToLower(module)
		if other, found := seen[key]; found && other != rel {
			t.Errorf("ui/src/%s and ui/src/%s resolve to the same module on a case-insensitive filesystem", other, rel)
			return nil
		}
		seen[key] = rel
		return nil
	})
	if err != nil {
		t.Fatalf("walk ui/src: %v", err)
	}
}
