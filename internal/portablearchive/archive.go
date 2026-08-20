package portablearchive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func cleanArchivePath(name string, maxBytes int) (string, error) {
	if name == "" || len(name) > maxBytes || strings.IndexByte(name, 0) >= 0 || strings.Contains(name, `\`) || path.IsAbs(name) {
		return "", fmt.Errorf("portable archive: invalid path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != name {
		return "", fmt.Errorf("portable archive: invalid path %q", name)
	}
	return clean, nil
}

func Stage(r io.Reader, compressedSize int64, destination string, limits Limits) (Manifest, error) {
	if compressedSize < 0 || compressedSize > limits.MaxCompressedBytes || limits.MaxFiles <= 0 || limits.MaxExpandedBytes <= 0 || limits.MaxPathBytes <= 0 {
		return Manifest{}, errors.New("portable archive: limit exceeded")
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return Manifest{}, err
	}
	stage, err := os.MkdirTemp(parent, ".portable-stage-")
	if err != nil {
		return Manifest{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stage)
		}
	}()
	limited := &io.LimitedReader{R: r, N: limits.MaxCompressedBytes + 1}
	gz, err := gzip.NewReader(limited)
	if err != nil {
		return Manifest{}, fmt.Errorf("portable archive: gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	written := map[string]File{}
	var manifest Manifest
	manifestSeen := false
	var expanded int64
	var count int
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, fmt.Errorf("portable archive: tar: %w", err)
		}
		name, err := cleanArchivePath(h.Name, limits.MaxPathBytes)
		if err != nil {
			return Manifest{}, err
		}
		if seen[name] {
			return Manifest{}, fmt.Errorf("portable archive: duplicate path %s", name)
		}
		seen[name] = true
		count++
		if count > limits.MaxFiles {
			return Manifest{}, errors.New("portable archive: too many entries")
		}
		if h.Size < 0 || expanded > limits.MaxExpandedBytes-h.Size {
			return Manifest{}, errors.New("portable archive: expanded size exceeded")
		}
		expanded += h.Size
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA && h.Typeflag != tar.TypeDir {
			return Manifest{}, fmt.Errorf("portable archive: unsupported entry type for %s", name)
		}
		if name == "manifest.json" {
			if h.Typeflag == tar.TypeDir {
				return Manifest{}, errors.New("portable archive: manifest is not a file")
			}
			data, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
			if err != nil || int64(len(data)) != h.Size {
				return Manifest{}, errors.New("portable archive: invalid manifest body")
			}
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&manifest); err != nil {
				return Manifest{}, fmt.Errorf("portable archive: manifest: %w", err)
			}
			if manifest.Format != "tariboy-portable" || manifest.Version != 1 || manifest.Kind == "" {
				return Manifest{}, errors.New("portable archive: unsupported manifest")
			}
			manifestSeen = true
			continue
		}
		target := filepath.Join(stage, filepath.FromSlash(name))
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return Manifest{}, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return Manifest{}, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return Manifest{}, err
		}
		hash := sha256.New()
		n, copyErr := io.CopyN(io.MultiWriter(out, hash), tr, h.Size)
		closeErr := out.Close()
		if copyErr != nil || n != h.Size {
			return Manifest{}, errors.New("portable archive: truncated file")
		}
		if closeErr != nil {
			return Manifest{}, closeErr
		}
		written[name] = File{Path: name, Size: h.Size, SHA256: hex.EncodeToString(hash.Sum(nil))}
	}
	if !manifestSeen {
		return Manifest{}, errors.New("portable archive: manifest missing")
	}
	want := map[string]File{}
	for _, f := range manifest.Files {
		clean, err := cleanArchivePath(f.Path, limits.MaxPathBytes)
		if err != nil || clean == "manifest.json" || want[clean].Path != "" {
			return Manifest{}, errors.New("portable archive: invalid manifest file list")
		}
		want[clean] = f
	}
	if len(want) != len(written) {
		return Manifest{}, errors.New("portable archive: manifest does not match files")
	}
	for name, got := range written {
		expected, ok := want[name]
		if !ok || got.Size != expected.Size || got.SHA256 != expected.SHA256 {
			return Manifest{}, fmt.Errorf("portable archive: file mismatch %s", name)
		}
	}
	if _, err := os.Lstat(destination); err == nil {
		return Manifest{}, errors.New("portable archive: destination exists")
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}
	if err := os.Rename(stage, destination); err != nil {
		return Manifest{}, err
	}
	published = true
	return manifest, nil
}

func Write(w io.Writer, root string, manifest Manifest, paths []string) error {
	if manifest.Format != "tariboy-portable" || manifest.Version != 1 {
		return errors.New("portable archive: invalid manifest")
	}
	files := append([]string(nil), paths...)
	sort.Strings(files)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(w)
	gz.Header.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(gz)
	closeWith := func(err error) error {
		if closeErr := tw.Close(); err == nil {
			err = closeErr
		}
		if closeErr := gz.Close(); err == nil {
			err = closeErr
		}
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o600, Size: int64(len(manifestBytes)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}); err != nil {
		return closeWith(err)
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return closeWith(err)
	}
	for _, name := range files {
		clean, err := cleanArchivePath(name, DefaultLimits().MaxPathBytes)
		if err != nil {
			return closeWith(err)
		}
		full := filepath.Join(root, filepath.FromSlash(clean))
		info, err := os.Lstat(full)
		if err != nil {
			return closeWith(err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return closeWith(fmt.Errorf("portable archive: unsafe source %s", clean))
		}
		in, err := os.Open(full)
		if err != nil {
			return closeWith(err)
		}
		if err := tw.WriteHeader(&tar.Header{Name: clean, Mode: 0o600, Size: info.Size(), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}); err != nil {
			in.Close()
			return closeWith(err)
		}
		_, copyErr := io.Copy(tw, in)
		closeErr := in.Close()
		if copyErr != nil {
			return closeWith(copyErr)
		}
		if closeErr != nil {
			return closeWith(closeErr)
		}
	}
	return closeWith(nil)
}
