package compose

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/version"
)

// daemonVersion reads the version of the running daemon.
func daemonVersion(call Caller) (version.SemVer, error) {
	raw, err := call.Call("GET", "/api/daemon/status", nil)
	if err != nil {
		return version.SemVer{}, err
	}
	var status struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return version.SemVer{}, fmt.Errorf("read daemon status: %w", err)
	}
	v, err := version.ParseSemVer(status.Version)
	if err != nil {
		return version.SemVer{}, fmt.Errorf("daemon %w", err)
	}
	return v, nil
}

// checkDaemonVersion fails when the daemon is outside the file's tariboy
// range. A file without the key reads nothing.
func checkDaemonVersion(call Caller, f File, path string) error {
	c, err := f.VersionConstraint()
	if err != nil || c.IsZero() {
		return err
	}
	v, err := daemonVersion(call)
	if err != nil {
		return err
	}
	return checkVersion("daemon", v, c, path)
}

func checkVersion(what string, v version.SemVer, c version.Constraint, path string) error {
	if c.Check(v) == nil {
		return nil
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return fmt.Errorf("%s version %s does not satisfy tariboy %s in %s", what, v, c, path)
}

// UnappliedSections refuses a file with sections that up, build and team
// import do not apply yet (stores, workflows, images from a Store), so such
// a file is never applied in part.
func (f File) UnappliedSections() error {
	for _, s := range []struct {
		name string
		n    int
	}{{"stores", len(f.Stores)}, {"workflows", len(f.Workflows)}} {
		if s.n != 0 {
			return fmt.Errorf("section %s is not applied by compose up yet; run compose validate to check it", s.name)
		}
	}
	for _, name := range sortedKeys(f.Images) {
		if f.Images[name].Store != "" {
			return fmt.Errorf("image %q from a Store is not applied by compose up yet; run compose validate to check it", name)
		}
	}
	return nil
}
