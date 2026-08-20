package builtinimages

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

// Generate builds the canonical basic source in an isolated image store and
// publishes the archive plus its owning daemon version into outputDir.
func Generate(sourceDir, outputDir, daemonVersion string) error {
	daemonVersion = strings.TrimSpace(daemonVersion)
	if daemonVersion == "" {
		return fmt.Errorf("daemon version is required")
	}
	parsed, err := imagefile.ParseAny(sourceDir)
	if err != nil {
		return fmt.Errorf("parse basic image: %w", err)
	}
	workDir, err := os.MkdirTemp("", "tariboy-basic-image-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	store := &image.Store{Dir: filepath.Join(workDir, "images")}
	ref := image.Ref{Name: "basic", Tag: "latest"}
	storeAssets := filepath.Clean(filepath.Join(sourceDir, "..", "..", "..", "store"))
	if parsed.Version != 2 {
		return fmt.Errorf("basic image must use schema_version 2")
	}
	if _, err := image.BuildV2(parsed.V2, imagefile.ResolveRoots{CurrentVersionStore: storeAssets, Store: storeAssets}, ref, store, time.Now, nil); err != nil {
		return fmt.Errorf("build basic image: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return err
	}
	if err := copyAtomic(filepath.Join(store.Dir, "basic", "latest.tar.gz"), filepath.Join(outputDir, "basic.tar.gz")); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(outputDir, "VERSION"), []byte(daemonVersion+"\n"))
}

func copyAtomic(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".basic-archive-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func writeAtomic(target string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".basic-version-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}
