// Package stores keeps the registry of Stores: git repositories, https
// archives and local directories that hold images, skills and prompts. A
// Store is untrusted content; this package never executes anything from it.
package stores

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/flock"
	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
)

// Kind is the source kind of a Store.
type Kind string

// The Store kinds.
const (
	KindGit Kind = "git"
	KindURL Kind = "url"
	KindDir Kind = "dir"
)

const (
	registryVersion = 1
	maxRegistry     = 1 << 20
)

var (
	// ErrNotFound marks a Store name that is not registered.
	ErrNotFound = errors.New("store not found")

	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

// Store is one registered Store.
type Store struct {
	Name   string `json:"name"`
	Kind   Kind   `json:"kind"`
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
}

// Registry is the registry of a tbox home. HTTP and Git are the production
// clients when left zero.
type Registry struct {
	Home paths.Home
	Git  gitx.Git
	HTTP fetch.Client
	// Cwd resolves relative directory sources in Add.
	Cwd string
}

type registryFile struct {
	Version int     `json:"version"`
	Stores  []Store `json:"stores"`
}

func (r Registry) file() string     { return filepath.Join(string(r.Home), "stores.json") }
func (r Registry) lockFile() string { return filepath.Join(string(r.Home), "stores.lock") }

// Root is the directory that holds the content of s.
func (r Registry) Root(s Store) string {
	if s.Kind == KindDir {
		return s.Source
	}
	return filepath.Join(r.Home.StoresDir(), s.Name)
}

// ValidName reports whether name is a legal Store name.
func ValidName(name string) bool { return namePattern.MatchString(name) }

func (r Registry) load() ([]Store, error) {
	body, err := fsafe.ReadFileNoFollow(r.file(), maxRegistry)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read store registry: %w", err)
	}
	var f registryFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("parse store registry: %w", err)
	}
	if f.Version != registryVersion {
		return nil, fmt.Errorf("store registry has unsupported version %d", f.Version)
	}
	return f.Stores, nil
}

func (r Registry) save(list []Store) error {
	body, err := json.MarshalIndent(registryFile{Version: registryVersion, Stores: list}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(string(r.Home), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(string(r.Home), ".stores-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), r.file())
}

// List returns the registered Stores in registration order.
func (r Registry) List() ([]Store, error) { return r.load() }

// Get returns the Store called name.
func (r Registry) Get(name string) (Store, error) {
	list, err := r.load()
	if err != nil {
		return Store{}, err
	}
	for _, s := range list {
		if s.Name == name {
			return s, nil
		}
	}
	return Store{}, fmt.Errorf("%w: %s", ErrNotFound, name)
}

// update runs fn on the registry under the registry lock and saves the list
// when fn returns nil.
func (r Registry) update(fn func(list []Store) ([]Store, error)) error {
	unlock, err := flock.Lock(r.lockFile())
	if err != nil {
		return err
	}
	defer unlock()
	list, err := r.load()
	if err != nil {
		return err
	}
	list, err = fn(list)
	if err != nil {
		return err
	}
	return r.save(list)
}

func find(list []Store, name string) int {
	for i, s := range list {
		if s.Name == name {
			return i
		}
	}
	return -1
}

func isArchiveURL(s string) bool {
	if !strings.HasPrefix(s, "https://") {
		return false
	}
	p := strings.ToLower(s)
	return strings.HasSuffix(p, ".tar.gz") || strings.HasSuffix(p, ".tgz") || strings.HasSuffix(p, ".zip")
}

// Add registers a Store after it has been cloned, downloaded or checked.
func (r Registry) Add(ctx context.Context, name, source, ref string) (Store, error) {
	if !ValidName(name) {
		return Store{}, fmt.Errorf("invalid store name %q: want lower-case letters, digits, '.', '_' and '-'", name)
	}
	var out Store
	err := r.update(func(list []Store) ([]Store, error) {
		if find(list, name) >= 0 {
			return nil, fmt.Errorf("store %s already exists", name)
		}
		s, err := r.materialize(ctx, name, strings.TrimSpace(source), ref)
		if err != nil {
			return nil, err
		}
		out = s
		return append(list, s), nil
	})
	return out, err
}

func (r Registry) materialize(ctx context.Context, name, source, ref string) (Store, error) {
	if dir, ok := r.localDir(source); ok {
		if ref != "" {
			return Store{}, errors.New("--ref applies to git stores only")
		}
		return Store{Name: name, Kind: KindDir, Source: dir}, nil
	}
	dest := filepath.Join(r.Home.StoresDir(), name)
	if err := r.clearStale(dest); err != nil {
		return Store{}, err
	}
	if isArchiveURL(source) {
		if ref != "" {
			return Store{}, errors.New("--ref applies to git stores only")
		}
		if err := r.download(ctx, source, dest); err != nil {
			return Store{}, err
		}
		return Store{Name: name, Kind: KindURL, Source: source}, nil
	}
	url, err := gitx.NormalizeURL(source, r.Git.AllowLocal)
	if err != nil {
		return Store{}, err
	}
	commit, err := r.Git.Clone(ctx, url, ref, dest)
	if err != nil {
		return Store{}, err
	}
	return Store{Name: name, Kind: KindGit, Source: url, Ref: ref, Commit: commit}, nil
}

// localDir returns the absolute directory source names, if it names one.
func (r Registry) localDir(source string) (string, bool) {
	if source == "" || strings.Contains(source, "://") {
		return "", false
	}
	p := source
	if !filepath.IsAbs(p) {
		if r.Cwd == "" {
			return "", false
		}
		p = filepath.Join(r.Cwd, p)
	}
	p = filepath.Clean(p)
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		return "", false
	}
	return p, true
}

// clearStale removes a leftover directory of an unregistered Store.
func (r Registry) clearStale(dest string) error {
	if _, err := os.Lstat(dest); err != nil {
		return nil
	}
	return r.removeOwn(dest)
}

func (r Registry) download(ctx context.Context, url, dest string) error {
	data, err := r.HTTP.Get(ctx, url, fetch.DefaultLimits.MaxCompressed)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := fetch.ExtractArchive(data, dest); err != nil {
		_ = os.RemoveAll(dest)
		return fmt.Errorf("extract %s: %w", url, err)
	}
	return nil
}

// removeOwn deletes path only when it is a direct child of the stores
// directory. A symlink is removed itself, never followed.
func (r Registry) removeOwn(path string) error {
	path = filepath.Clean(path)
	if filepath.Dir(path) != filepath.Clean(r.Home.StoresDir()) || strings.HasPrefix(filepath.Base(path), "..") {
		return fmt.Errorf("refusing to remove %s outside %s", path, r.Home.StoresDir())
	}
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}

// Refresh updates the content of a Store: a git Store pulls its ref, a url
// Store downloads again and swaps the tree, a dir Store changes nothing.
func (r Registry) Refresh(ctx context.Context, name string) (Store, error) {
	return r.change(name, func(s Store) (Store, error) {
		switch s.Kind {
		case KindGit:
			commit, err := r.Git.Pull(ctx, r.Root(s), s.Ref)
			if err != nil {
				return s, err
			}
			s.Commit = commit
		case KindURL:
			if err := r.swapArchive(ctx, s); err != nil {
				return s, err
			}
		}
		return s, nil
	})
}

// swapArchive downloads the archive of s into a new directory and replaces
// the old tree with it. The old tree stays in place when anything fails.
func (r Registry) swapArchive(ctx context.Context, s Store) error {
	root := r.Root(s)
	fresh := filepath.Join(r.Home.StoresDir(), "."+s.Name+".new")
	old := filepath.Join(r.Home.StoresDir(), "."+s.Name+".old")
	_ = r.removeOwn(fresh)
	_ = r.removeOwn(old)
	if err := r.download(ctx, s.Source, fresh); err != nil {
		return err
	}
	if err := os.Rename(root, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = r.removeOwn(fresh)
		return err
	}
	if err := os.Rename(fresh, root); err != nil {
		_ = os.Rename(old, root)
		_ = r.removeOwn(fresh)
		return err
	}
	_ = r.removeOwn(old)
	return nil
}

// SetRef moves a git Store to ref; "" returns it to the default branch. The
// registration is unchanged when the checkout fails.
func (r Registry) SetRef(ctx context.Context, name, ref string) (Store, error) {
	return r.change(name, func(s Store) (Store, error) {
		if s.Kind != KindGit {
			return s, errors.New("ref applies to git stores only")
		}
		commit, err := r.Git.Pull(ctx, r.Root(s), ref)
		if err != nil {
			return s, err
		}
		s.Ref, s.Commit = ref, commit
		return s, nil
	})
}

// change applies fn to one Store under the registry lock.
func (r Registry) change(name string, fn func(Store) (Store, error)) (Store, error) {
	var out Store
	err := r.update(func(list []Store) ([]Store, error) {
		i := find(list, name)
		if i < 0 {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		s, err := fn(list[i])
		if err != nil {
			return nil, err
		}
		list[i], out = s, s
		return list, nil
	})
	return out, err
}

// Remove unregisters a Store and deletes tbox's own clone or extracted
// archive. A dir Store's directory and built images are never touched.
func (r Registry) Remove(name string) error {
	return r.update(func(list []Store) ([]Store, error) {
		i := find(list, name)
		if i < 0 {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		if s := list[i]; s.Kind != KindDir {
			if !ValidName(s.Name) {
				return nil, fmt.Errorf("refusing to remove store with invalid name %q", s.Name)
			}
			if err := r.removeOwn(r.Root(s)); err != nil {
				return nil, err
			}
		}
		return append(list[:i:i], list[i+1:]...), nil
	})
}

// Selector names one item of one Store as STORE/NAME.
type Selector struct{ Store, Name string }

// ParseSelector splits STORE/NAME; it is true only for exactly one '/' with
// both sides non-empty.
func ParseSelector(s string) (Selector, bool) {
	store, name, ok := strings.Cut(s, "/")
	if !ok || store == "" || name == "" || strings.Contains(name, "/") {
		return Selector{}, false
	}
	return Selector{Store: store, Name: name}, true
}
