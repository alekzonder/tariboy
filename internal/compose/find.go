package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultFile is the compose file a verb reads when -f is not given.
const DefaultFile = "Tariboycompose.yaml"

// LegacyFile is the earlier default name, read when DefaultFile is absent.
const LegacyFile = "tariboy-compose.yaml"

// FindFile returns the compose file of dir: DefaultFile, else LegacyFile. A
// directory with both, or with neither, is an error that names both.
func FindFile(dir string) (string, error) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	var found []string
	for _, name := range []string{DefaultFile, LegacyFile} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			found = append(found, path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("no %s or %s in %s; pass -f FILE", DefaultFile, LegacyFile, dir)
	default:
		return "", fmt.Errorf("both %s and %s exist in %s; remove one or pass -f", DefaultFile, LegacyFile, dir)
	}
}
