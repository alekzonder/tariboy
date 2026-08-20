package builtinimages

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
)

var basicRef = image.Ref{Name: "basic", Tag: "latest"}

// EnsureBasic installs the bundle embedded by make build-basic-image.
func EnsureBasic(store *image.Store, log *slog.Logger) error {
	return ensureBasicFS(FS, store, log)
}

func ensureBasicFS(bundle fs.FS, store *image.Store, log *slog.Logger) error {
	versionBytes, err := fs.ReadFile(bundle, "generated/VERSION")
	if err != nil {
		return fmt.Errorf("read embedded basic version: %w", err)
	}
	bundleVersion := strings.TrimSpace(string(versionBytes))
	if bundleVersion == "" {
		return fmt.Errorf("embedded basic version is empty")
	}
	markerPath := filepath.Join(store.Dir, ".builtin-basic-version")
	installedVersion, _ := os.ReadFile(markerPath)
	if strings.TrimSpace(string(installedVersion)) == bundleVersion {
		if manifest, err := store.Inspect(basicRef); err == nil && manifest.Name == basicRef.Name && manifest.Tag == basicRef.Tag {
			log.Info("builtin basic image up to date", "version", bundleVersion)
			return nil
		}
	}
	archive, err := fs.ReadFile(bundle, "generated/basic.tar.gz")
	if err != nil {
		return fmt.Errorf("read embedded basic archive: %w", err)
	}
	if err := store.InstallManagedArchive(basicRef, archive); err != nil {
		return err
	}
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		return err
	}
	if err := writeAtomic(markerPath, []byte(bundleVersion+"\n")); err != nil {
		return fmt.Errorf("write embedded basic version: %w", err)
	}
	log.Info("builtin basic image installed", "version", bundleVersion)
	return nil
}
