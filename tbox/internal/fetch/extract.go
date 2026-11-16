package fetch

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Limits bound an extraction.
type Limits struct {
	MaxCompressed int64
	MaxExpanded   int64
	MaxMembers    int
	MaxPathBytes  int
}

// DefaultLimits are the stage 2 archive limits: 64 MiB compressed, 256 MiB
// expanded, 4096 members and 512-byte paths.
var DefaultLimits = Limits{
	MaxCompressed: 64 << 20,
	MaxExpanded:   256 << 20,
	MaxMembers:    4096,
	MaxPathBytes:  512,
}

// ExtractTar extracts the plain tar stream r into the new directory dest. It
// accepts regular files and directories only, rejects symlinks, hardlinks,
// absolute and escaping paths, and enforces the member, path and expanded
// size limits of lim. Files are written 0644, or 0755 when any executable bit
// is set. The tree is staged in a sibling temporary directory and renamed, so
// dest never holds a partial extraction.
func ExtractTar(r io.Reader, dest string, lim Limits) error {
	return stageAndRename(dest, nil, func(stage string) error {
		return extractTarInto(r, stage, lim)
	})
}

// stageAndRename runs fill on a sibling temporary directory of dest and
// renames it to dest. With flatten, a stage holding exactly one directory
// whose name flatten accepts is replaced by that directory.
func stageAndRename(dest string, flatten func(name string) bool, fill func(stage string) error) error {
	dest = filepath.Clean(dest)
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("destination %s already exists", dest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := fill(stage); err != nil {
		return err
	}
	src := stage
	if flatten != nil {
		src, err = singleTopDir(stage, flatten)
		if err != nil {
			return err
		}
	}
	if err := os.Chmod(src, 0o755); err != nil {
		return err
	}
	return os.Rename(src, dest)
}

// singleTopDir returns the only entry of stage when it is a directory whose
// name strip accepts, and stage itself otherwise.
func singleTopDir(stage string, strip func(name string) bool) (string, error) {
	entries, err := os.ReadDir(stage)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() && strip(entries[0].Name()) {
		return filepath.Join(stage, entries[0].Name()), nil
	}
	return stage, nil
}

var errTooMany = errors.New("archive has too many members")

// streamOverhead is the tar framing allowed on top of MaxExpanded: a header
// block and padding for every member plus room for extended headers.
func streamOverhead(lim Limits) int64 { return int64(lim.MaxMembers)*1536 + 64<<10 }

// capStream fails once more than MaxExpanded plus the framing overhead has
// been read from r, bounding the decompressed size of a stream whatever its
// headers claim.
func capStream(r io.Reader, lim Limits) io.Reader {
	return &cappedReader{r: r, left: lim.MaxExpanded + streamOverhead(lim)}
}

type cappedReader struct {
	r    io.Reader
	left int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	if c.left < 0 {
		return 0, errExpanded
	}
	if int64(len(p)) > c.left+1 {
		p = p[:c.left+1]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	if c.left < 0 {
		return n, errExpanded
	}
	return n, err
}

var errExpanded = errors.New("archive exceeds expanded size limit")

// counter tracks the member and byte limits of one extraction.
type counter struct {
	lim      Limits
	members  int
	expanded int64
	seen     map[string]bool
}

func newCounter(lim Limits) *counter {
	return &counter{lim: lim, seen: map[string]bool{}}
}

// admit checks the member name and size against the limits and returns the
// cleaned relative name. An empty name means the member is the root and is
// skipped.
func (c *counter) admit(name string, size int64) (string, error) {
	c.members++
	if c.members > c.lim.MaxMembers {
		return "", errTooMany
	}
	clean, err := cleanMember(name, c.lim.MaxPathBytes)
	if err != nil {
		return "", err
	}
	if size < 0 || c.expanded > c.lim.MaxExpanded-size {
		return "", errExpanded
	}
	c.expanded += size
	return clean, nil
}

// cleanMember validates an archive member name and strips a leading "./" and
// a trailing "/".
func cleanMember(name string, maxBytes int) (string, error) {
	if len(name) > maxBytes {
		return "", fmt.Errorf("archive member path is longer than %d bytes", maxBytes)
	}
	if name == "" || strings.HasPrefix(name, "/") || strings.IndexByte(name, 0) >= 0 || strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("invalid archive member path %q", name)
	}
	n := strings.TrimSuffix(strings.TrimPrefix(name, "./"), "/")
	if n == "" || n == "." {
		return "", nil
	}
	if path.Clean(n) != n || slices.Contains(strings.Split(n, "/"), "..") {
		return "", fmt.Errorf("invalid archive member path %q", name)
	}
	return n, nil
}

func extractTarInto(r io.Reader, stage string, lim Limits) error {
	tr := tar.NewReader(capStream(r, lim))
	c := newCounter(lim)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			if c.members++; c.members > lim.MaxMembers {
				return errTooMany
			}
			continue
		}
		if err := extractTarMember(tr, h, c, stage); err != nil {
			return err
		}
	}
}

// LinkError is the error for a symbolic or hard link member, which is never
// extracted. Name is the member path as stored in the archive.
type LinkError struct{ Name string }

func (e *LinkError) Error() string {
	return fmt.Sprintf("invalid archive member %q: regular files and directories only", e.Name)
}

func extractTarMember(tr *tar.Reader, h *tar.Header, c *counter, stage string) error {
	switch h.Typeflag {
	case tar.TypeReg, tar.TypeDir:
	case tar.TypeSymlink, tar.TypeLink:
		return &LinkError{Name: h.Name}
	default:
		return fmt.Errorf("invalid archive member %q: regular files and directories only", h.Name)
	}
	rel, err := c.admit(h.Name, h.Size)
	if err != nil {
		return err
	}
	if rel == "" {
		return nil
	}
	if h.Typeflag == tar.TypeDir {
		return makeDir(stage, rel)
	}
	return writeFile(stage, rel, tr, h.Size, h.Mode&0o111 != 0, c)
}

func makeDir(stage, rel string) error {
	return os.MkdirAll(filepath.Join(stage, filepath.FromSlash(rel)), 0o755)
}

// writeFile creates rel under stage and copies exactly size bytes of r into
// it. The file must not exist yet, which also rejects duplicate members.
func writeFile(stage, rel string, r io.Reader, size int64, exec bool, c *counter) error {
	if c.seen[rel] {
		return fmt.Errorf("duplicate archive member %q", rel)
	}
	c.seen[rel] = true
	full := filepath.Join(stage, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("archive member %q size mismatch", rel)
	}
	return os.Chmod(full, mode)
}

// ExtractArchive extracts a .tar.gz or .zip archive, detected by content,
// into the new directory dest with DefaultLimits. A single top-level
// directory is stripped.
func ExtractArchive(data []byte, dest string) error {
	return ExtractArchiveWith(data, dest, DefaultLimits)
}

// storeLayoutDirs are the top-level directories of a Store; an archive whose
// only top-level entry is one of them keeps it.
var storeLayoutDirs = map[string]bool{"skills": true, "images": true, "prompts": true, "workflows": true}

// ExtractStoreArchive is ExtractArchive for a Store download: a single
// top-level directory named skills, images, prompts or workflows is part of
// the Store layout and is not stripped.
func ExtractStoreArchive(data []byte, dest string) error {
	return extractArchive(data, dest, DefaultLimits, func(name string) bool { return !storeLayoutDirs[name] })
}

// ExtractArchiveWith is ExtractArchive with explicit limits.
func ExtractArchiveWith(data []byte, dest string, lim Limits) error {
	return extractArchive(data, dest, lim, func(string) bool { return true })
}

func extractArchive(data []byte, dest string, lim Limits, strip func(name string) bool) error {
	if int64(len(data)) > lim.MaxCompressed {
		return errors.New("archive exceeds compressed size limit")
	}
	var fill func(stage string) error
	switch {
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		fill = func(stage string) error { return extractTarGz(data, stage, lim) }
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		fill = func(stage string) error { return extractZip(data, stage, lim) }
	default:
		return errors.New("unsupported archive format: want .tar.gz or .zip")
	}
	return stageAndRename(dest, strip, fill)
}

func extractTarGz(data []byte, stage string, lim Limits) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read gzip: %w", err)
	}
	defer gz.Close()
	return extractTarInto(gz, stage, lim)
}

func extractZip(data []byte, stage string, lim Limits) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("read zip: %w", err)
	}
	if len(zr.File) > lim.MaxMembers {
		return errTooMany
	}
	c := newCounter(lim)
	for _, f := range zr.File {
		if err := extractZipMember(f, c, stage); err != nil {
			return err
		}
	}
	return nil
}

func extractZipMember(f *zip.File, c *counter, stage string) error {
	mode := f.Mode()
	if mode&os.ModeSymlink != 0 {
		return &LinkError{Name: f.Name}
	}
	if !mode.IsRegular() && !mode.IsDir() {
		return fmt.Errorf("invalid archive member %q: regular files and directories only", f.Name)
	}
	rel, err := c.admit(f.Name, int64(min(f.UncompressedSize64, 1<<62)))
	if err != nil {
		return err
	}
	if rel == "" {
		return nil
	}
	if mode.IsDir() {
		return makeDir(stage, rel)
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return writeFile(stage, rel, rc, int64(f.UncompressedSize64), mode&0o111 != 0, c)
}
