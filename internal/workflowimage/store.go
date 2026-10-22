package workflowimage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/refstore"
	"github.com/alekzonder/tariboy/internal/safefs"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tidy"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

const (
	maxFiles     = 256
	maxFileSize  = 4 << 20
	maxTotalSize = 32 << 20

	latestTag    = "latest"
	manifestName = "manifest.json"
	tmpPrefix    = ".tmp-"
)

// IsDigest reports whether s has the form of an image digest.
func IsDigest(s string) bool { return refstore.IsID(s) }

// mu serializes publication and removal across every Store in the process.
var mu sync.Mutex

// beforeTagWrite runs before moveTags writes each tag. Tests replace it to
// inject failures between tag writes.
var beforeTagWrite = func(tag string) {}

// Store is the on-disk store of workflow images. Dir is <base-dir>/workflows.
//
//	<Dir>/<name>/refs/<digest>/  copied source tree plus manifest.json
//	<Dir>/<name>/tags/<tag>      file holding the digest
type Store struct{ Dir string }

// Listing is a manifest together with the tag that names it.
type Listing struct {
	Manifest
	Tag string
}

// sourceFile is a file read from the source, held in memory until it is
// copied so the digest and the stored bytes cannot differ.
type sourceFile struct {
	FileEntry
	content []byte
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// checkName rejects values that could escape the store when joined into a
// path, with this package's error text.
func checkName(kind, v string) error {
	if refstore.CheckName(v) != nil {
		return invalidf("%s %q is not a valid name", kind, v)
	}
	return nil
}

// refs is the shared name and tag layer under Dir. The unpacked tree kept at
// refs/<digest>/ is this package's own.
func (s *Store) refs() refstore.Store { return refstore.Store{Dir: s.Dir} }

// ContentDir returns the absolute path of the unpacked tree, or "" when name
// or digest is not a valid path component.
func (s *Store) ContentDir(name, digest string) string {
	if checkName("name", name) != nil || !IsDigest(digest) {
		return ""
	}
	p := filepath.Join(s.refs().RefsDir(name), digest)
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// FilePath returns the absolute path of a file inside the unpacked tree. It
// refuses a path that is absolute or leaves the tree, and a file that is not
// there.
func (s *Store) FilePath(name, digest, rel string) (string, error) {
	root := s.ContentDir(name, digest)
	if root == "" {
		return "", invalidf("invalid image reference %q %q", name, digest)
	}
	if rel == "" || !filepath.IsLocal(rel) || strings.Contains(rel, "\\") {
		return "", invalidf("path %q is outside the image", rel)
	}
	p := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: file %q", ErrNotFound, rel)
	}
	return p, nil
}

// readRegular reads a regular file without following a symlink at path, so a
// file swapped after the directory walk cannot pull in outside bytes. The
// type, size, and mode come from the opened descriptor, and the read stops
// one byte past limit.
func readRegular(path string, limit int64) ([]byte, fs.FileMode, error) {
	content, info, err := safefs.ReadRegular(path, limit)
	if err != nil {
		return nil, 0, err
	}
	return content, info.Mode(), nil
}

// readSourceFile is readRegular; tests replace it to simulate a file that
// grew between the walk and the read.
var readSourceFile = readRegular

// scanError keeps this package's texts for the entries and limits safefs
// refuses while scanning a source.
func scanError(err error) error {
	var pathErr *safefs.PathError
	if !errors.As(err, &pathErr) {
		return err
	}
	switch {
	case errors.Is(err, safefs.ErrUnsafeFile):
		return invalidf("%q is not a regular file (symlinks and special files are not allowed)", pathErr.Rel)
	case errors.Is(err, safefs.ErrTooManyFiles):
		return invalidf("source has more than %d files", maxFiles)
	case errors.Is(err, safefs.ErrFileTooLarge):
		return invalidf("file %q is larger than %d bytes", pathErr.Rel, maxFileSize)
	case errors.Is(err, safefs.ErrTotalTooLarge):
		return invalidf("source is larger than %d bytes", maxTotalSize)
	}
	return err
}

// scan reads the whole source directory. It rejects anything but regular
// files and directories, and enforces the source limits.
func scan(root string) ([]sourceFile, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, invalidf("source directory: %v", err)
	}
	var files []sourceFile
	// MaxTotal is an early check on Lstat sizes; total counts the bytes
	// actually read, so a file that grew after the walk is still bounded.
	var total int64
	opts := safefs.WalkOptions{Dirs: true, MaxFiles: maxFiles, MaxFileSize: maxFileSize, MaxTotal: maxTotalSize}
	err = safefs.WalkRegular(root, opts, func(rel string, d fs.DirEntry, _ fs.FileInfo) error {
		if d.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == manifestName {
			return invalidf("%q is reserved for the stored manifest", rel)
		}
		content, mode, err := readSourceFile(filepath.Join(root, filepath.FromSlash(rel)), maxFileSize)
		if err != nil {
			return invalidf("%q: %v", rel, err)
		}
		total += int64(len(content))
		if total > maxTotalSize {
			return invalidf("source is larger than %d bytes", maxTotalSize)
		}
		sum := sha256.Sum256(content)
		files = append(files, sourceFile{
			FileEntry: FileEntry{
				Path:       rel,
				SHA256:     hex.EncodeToString(sum[:]),
				Executable: mode.Perm()&0o111 != 0,
				Size:       int64(len(content)),
			},
			content: content,
		})
		return nil
	})
	err = scanError(err)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return nil, err
		}
		return nil, fmt.Errorf("scan source: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// checkScanned verifies against the scanned entries, not the file system,
// that the manifest file and every file the manifest names were captured, and
// that every script is executable. Validate looked at the file system earlier;
// this closes the gap between the two looks.
func checkScanned(src *workflowfile.File, entries []FileEntry) error {
	byPath := make(map[string]FileEntry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}
	if _, ok := byPath[workflowfile.DefaultFilename]; !ok {
		return invalidf("%q was not captured from the source", workflowfile.DefaultFilename)
	}
	for _, p := range src.Files() {
		if _, ok := byPath[p]; !ok {
			return invalidf("file %q named by the manifest was not captured from the source", "./"+p)
		}
	}
	var scripts []string
	for _, s := range src.Statuses {
		if s.Watch != nil {
			scripts = append(scripts, s.Watch.Script)
		}
		for _, t := range s.Transitions {
			for _, c := range t.Checks {
				scripts = append(scripts, c.Script)
			}
		}
	}
	for _, script := range scripts {
		p := path.Clean(script)
		if e, ok := byPath[p]; !ok || !e.Executable {
			return invalidf("script %q is not an executable file in the captured source", "./"+p)
		}
	}
	return nil
}

// Publish validates src, copies the whole source directory, and points the
// version tag and "latest" at it. The digest covers every file's path,
// executable bit, and content, Workflowfile.yaml included, and nothing else.
// A version whose stored content differs is refused, whether or not its tag
// still exists. The bool reports whether the content was new.
func (s *Store) Publish(src *workflowfile.File, now time.Time) (Manifest, bool, error) {
	mu.Lock()
	defer mu.Unlock()
	return s.publishLocked(src, "", now)
}

// Validate runs every check Publish makes before it writes anything: the
// manifest validation, then the source scan. A source the scan refuses is
// reported as one source_invalid error. It reads src.Dir and writes nothing.
func Validate(src *workflowfile.File) []workflowfile.ValidationError {
	_, _, err := prepareSource(src)
	var invalid *InvalidError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &invalid):
		return invalid.Errors
	default:
		return []workflowfile.ValidationError{{Code: "source_invalid", Path: "source", Message: err.Error()}}
	}
}

// storeNameErrors reports a name or version that could escape the store when
// joined into a path, under the manifest field that holds it. The manifest
// rules already refuse such values; this is the store's own guard.
func storeNameErrors(src *workflowfile.File) []workflowfile.ValidationError {
	var errs []workflowfile.ValidationError
	if err := checkName("workflow name", src.Name); err != nil {
		errs = append(errs, workflowfile.ValidationError{Code: "name_invalid", Path: "name", Message: err.Error()})
	}
	if err := checkName("workflow version", src.WorkflowVersion); err != nil {
		errs = append(errs, workflowfile.ValidationError{Code: "version_invalid", Path: "workflow_version", Message: err.Error()})
	}
	return errs
}

// prepareSource validates src and reads its whole directory.
func prepareSource(src *workflowfile.File) ([]sourceFile, []FileEntry, error) {
	if errs := workflowfile.Validate(src); len(errs) > 0 {
		return nil, nil, &InvalidError{Errors: errs}
	}
	if errs := storeNameErrors(src); len(errs) > 0 {
		return nil, nil, &InvalidError{Errors: errs}
	}
	files, err := scan(src.Dir)
	if err != nil {
		return nil, nil, err
	}
	entries := make([]FileEntry, len(files))
	for i, f := range files {
		entries[i] = f.FileEntry
	}
	if err := checkScanned(src, entries); err != nil {
		return nil, nil, err
	}
	return files, entries, nil
}

// publishLocked is Publish for a caller that already holds mu. A non-empty
// want refuses a source whose digest differs, before anything is written.
func (s *Store) publishLocked(src *workflowfile.File, want string, now time.Time) (Manifest, bool, error) {
	files, entries, err := prepareSource(src)
	if err != nil {
		return Manifest{}, false, err
	}
	digest := computeDigest(entries)
	if want != "" && digest != want {
		return Manifest{}, false, fmt.Errorf("%w: archive names %s, content is %s", ErrDigestMismatch, want, digest)
	}

	name, version := src.Name, src.WorkflowVersion
	prev, err := s.readTag(name, version)
	switch {
	case err == nil && prev == digest:
		m, err := s.readManifest(name, digest)
		return m, false, err
	case err == nil:
		return Manifest{}, false, fmt.Errorf("%w: %s %s is %s", ErrVersionPublished, name, version, prev)
	case !errors.Is(err, ErrNotFound):
		return Manifest{}, false, err
	}
	// The version tag can be gone while another tag still holds content of
	// that version; the version stays taken until that content is deleted.
	if other, err := s.storedVersionDigest(name, version, digest); err != nil {
		return Manifest{}, false, err
	} else if other != "" {
		return Manifest{}, false, fmt.Errorf("%w: %s %s is %s", ErrVersionPublished, name, version, other)
	}

	var m Manifest
	created := false
	if _, statErr := os.Stat(s.ContentDir(name, digest)); statErr == nil {
		if m, err = s.readManifest(name, digest); err != nil {
			return Manifest{}, false, err
		}
	} else if errors.Is(statErr, fs.ErrNotExist) {
		def := *src
		def.Dir = "" // the stored definition does not carry the source path
		m = Manifest{
			SchemaVersion: src.SchemaVersion,
			Name:          name,
			Version:       version,
			Digest:        digest,
			BuiltAt:       store.FormatTime(now),
			Definition:    def,
			Files:         entries,
		}
		if err := s.install(m, files); err != nil {
			return Manifest{}, false, err
		}
		created = true
	} else {
		return Manifest{}, false, statErr
	}

	if err := s.moveTags(name, digest, version, latestTag); err != nil {
		// Delete new content only when the tags are back as they were; a tag
		// that could not be restored still names it.
		if created && !errors.Is(err, errRestoreTags) {
			if rmErr := removeAll(s.ContentDir(name, digest)); rmErr != nil {
				err = errors.Join(err, fmt.Errorf("remove new content: %w", rmErr))
			}
		}
		return Manifest{}, false, err
	}
	return m, created, nil
}

// storedVersionDigest returns the digest of stored content of name that has
// version but is not digest, or "" when there is none. It reads every
// refs/<digest>/manifest.json, whether or not a tag names it.
func (s *Store) storedVersionDigest(name, version, digest string) (string, error) {
	refs, err := os.ReadDir(s.refs().RefsDir(name))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		if !ref.IsDir() || !IsDigest(ref.Name()) || ref.Name() == digest {
			continue
		}
		m, err := s.readManifest(name, ref.Name())
		if err != nil {
			return "", err
		}
		if m.Version == version {
			return ref.Name(), nil
		}
	}
	return "", nil
}

// install writes the tree into a temporary sibling directory, makes it
// read-only, and renames it into place.
func (s *Store) install(m Manifest, files []sourceFile) (err error) {
	refs := s.refs().RefsDir(m.Name)
	if err := os.MkdirAll(refs, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(refs, tmpPrefix)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			removeAll(tmp)
		}
	}()
	dirs := map[string]bool{tmp: true}
	modes := map[string]fs.FileMode{}
	write := func(rel string, content []byte, mode fs.FileMode) error {
		p := filepath.Join(tmp, filepath.FromSlash(rel))
		for d := filepath.Dir(p); !dirs[d]; d = filepath.Dir(d) {
			dirs[d] = true
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, content, 0o600); err != nil {
			return err
		}
		modes[p] = mode
		return nil
	}
	for _, f := range files {
		mode := fs.FileMode(0o400)
		if f.Executable {
			mode = 0o500
		}
		if err := write(f.Path, f.content, mode); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := write(manifestName, append(data, '\n'), 0o400); err != nil {
		return err
	}
	for p, mode := range modes {
		if err := os.Chmod(p, mode); err != nil {
			return err
		}
	}
	// Deepest directories first, so a parent is still writable while its
	// children change.
	list := make([]string, 0, len(dirs))
	for d := range dirs {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	for _, d := range list {
		if err := os.Chmod(d, 0o500); err != nil {
			return err
		}
	}
	return os.Rename(tmp, s.ContentDir(m.Name, m.Digest))
}

// errRestoreTags marks a moveTags failure whose rollback also failed, so some
// tags may still name the new digest.
var errRestoreTags = errors.New("restore tags")

// moveTags points every tag at digest. If one fails it puts the earlier tags
// back as they were; when that also fails the error matches errRestoreTags.
func (s *Store) moveTags(name, digest string, tags ...string) error {
	var done []tagState
	for _, tag := range tags {
		beforeTagWrite(tag)
		old, err := s.readTag(name, tag)
		had := err == nil
		if err == nil || errors.Is(err, ErrNotFound) {
			err = s.writeTag(name, tag, digest)
		}
		if err != nil {
			if rerr := s.restoreTags(name, done); rerr != nil {
				err = errors.Join(err, fmt.Errorf("%w: %w", errRestoreTags, rerr))
			}
			return err
		}
		done = append(done, tagState{tag, old, had})
	}
	return nil
}

type tagState struct {
	tag    string
	digest string
	had    bool
}

func (s *Store) restoreTags(name string, states []tagState) error {
	var errs []error
	for _, st := range states {
		if st.had {
			errs = append(errs, s.writeTag(name, st.tag, st.digest))
		} else if err := s.refs().RemoveTag(name, st.tag); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// writeTag points tag at digest through refstore's atomic, synced write.
func (s *Store) writeTag(name, tag, digest string) error {
	return s.refs().WriteTag(name, tag, digest)
}

// readTag returns the digest a tag names, or ErrNotFound. A name or tag that
// cannot be a path component cannot exist and reads as not found.
func (s *Store) readTag(name, tag string) (string, error) {
	digest, err := s.refs().Resolve(name, tag)
	var pointer *refstore.PointerError
	switch {
	case err == nil:
		return digest, nil
	case errors.Is(err, refstore.ErrNotFound), errors.Is(err, refstore.ErrInvalidName):
		return "", fmt.Errorf("%w: %s %s", ErrNotFound, name, tag)
	case errors.As(err, &pointer):
		return "", fmt.Errorf("tag %s of %s holds %q, not a digest", tag, name, pointer.Value)
	}
	return "", err
}

func (s *Store) readManifest(name, digest string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(s.ContentDir(name, digest), manifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, fmt.Errorf("%w: %s %s", ErrNotFound, name, digest)
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("read manifest of %s %s: %w", name, digest, err)
	}
	return m, nil
}

// Resolve maps a tag, or a full digest that exists for name, to a digest. An
// empty tag means "latest".
func (s *Store) Resolve(name, tag string) (string, error) {
	if err := checkName("name", name); err != nil {
		return "", err
	}
	if tag == "" {
		tag = latestTag
	}
	if err := checkName("tag", tag); err != nil {
		return "", err
	}
	digest, err := s.readTag(name, tag)
	if err == nil {
		return digest, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	if IsDigest(tag) {
		if info, statErr := os.Stat(s.ContentDir(name, tag)); statErr == nil && info.IsDir() {
			return tag, nil
		}
	}
	return "", err
}

// Inspect returns the manifest a tag or digest names.
func (s *Store) Inspect(name, tagOrDigest string) (Manifest, error) {
	digest, err := s.Resolve(name, tagOrDigest)
	if err != nil {
		return Manifest{}, err
	}
	return s.readManifest(name, digest)
}

// List returns one entry per name and tag, sorted by name and then tag.
func (s *Store) List() ([]Listing, error) {
	names, err := s.refs().List()
	if err != nil {
		return nil, err
	}
	var out []Listing
	for _, name := range names {
		tags, err := s.refs().Tags(name)
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			digest, err := s.readTag(name, tag)
			if err != nil {
				return nil, err
			}
			m, err := s.readManifest(name, digest)
			if err != nil {
				return nil, err
			}
			out = append(out, Listing{Manifest: m, Tag: tag})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

// Tags returns the tags of name that point at digest, sorted.
func (s *Store) Tags(name, digest string) ([]string, error) {
	if err := checkName("name", name); err != nil {
		return nil, err
	}
	names, err := s.refs().Tags(name)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, tag := range names {
		d, err := s.readTag(name, tag)
		if err != nil {
			return nil, err
		}
		if d == digest {
			out = append(out, tag)
		}
	}
	return out, nil
}

// RemoveTag deletes a tag. When no other tag names the same digest, the
// content is deleted too.
func (s *Store) RemoveTag(name, tag string) (string, bool, error) {
	if err := checkName("name", name); err != nil {
		return "", false, err
	}
	if err := checkName("tag", tag); err != nil {
		return "", false, err
	}
	mu.Lock()
	defer mu.Unlock()
	return s.removeTagLocked(name, tag)
}

// removeTagLocked is RemoveTag for a caller that already holds mu and has
// checked the names.
func (s *Store) removeTagLocked(name, tag string) (string, bool, error) {
	digest, err := s.readTag(name, tag)
	if err != nil {
		return "", false, err
	}
	if err := s.refs().RemoveTag(name, tag); err != nil {
		return "", false, err
	}
	rest, err := s.Tags(name, digest)
	if err != nil {
		return digest, false, err
	}
	if len(rest) > 0 {
		return digest, false, nil
	}
	if err := removeAll(s.ContentDir(name, digest)); err != nil {
		return digest, false, err
	}
	return digest, true, nil
}

// makeWritable restores owner write permission on a stored tree so it can be
// deleted.
func makeWritable(root string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error { // best effort: unreadable entries are skipped and removal reports the failure
		if d != nil && d.IsDir() {
			tidy.Log(nil, "make stored tree writable", os.Chmod(p, 0o700), "path", p)
		}
		return nil
	})
}

// removeAll deletes a stored tree, restoring write permission first.
func removeAll(path string) error {
	makeWritable(path)
	return os.RemoveAll(path)
}
