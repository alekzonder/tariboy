package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EnsureBundled installs a trusted sibling binary through the ordinary plugin
// package path. A missing binary is normal in `go run` developer builds.
func (h *Host) EnsureBundled(executable string, manifest Manifest) (bool, error) {
	info, err := os.Stat(executable)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("bundled plugin executable is not a regular file")
	}
	if manifest.Exec == "" || filepath.Base(executable) != manifest.Exec {
		return false, fmt.Errorf("bundled plugin exec %q does not match %q", manifest.Exec, filepath.Base(executable))
	}
	if err := manifest.Validate(); err != nil {
		return false, err
	}
	if err := os.MkdirAll(h.cfg.PluginsDir, 0o700); err != nil {
		return false, err
	}
	staging, err := os.MkdirTemp(h.cfg.PluginsDir, ".bundled-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(staging)
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(staging, "plugin.json"), encoded, 0o600); err != nil {
		return false, err
	}
	if err := copyBundledExecutable(executable, filepath.Join(staging, manifest.Exec)); err != nil {
		return false, err
	}
	if _, err := h.Install(staging); err != nil {
		return false, err
	}
	record, ok, err := h.cfg.Store.Get(manifest.Name)
	if err != nil {
		return false, err
	}
	if ok {
		record.SourcePath = filepath.Dir(executable)
		if err := h.cfg.Store.Upsert(record); err != nil {
			return false, err
		}
	}
	return true, nil
}

func copyBundledExecutable(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
