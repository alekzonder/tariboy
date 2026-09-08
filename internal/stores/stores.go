// Package stores manages per-daemon image source catalogs.
package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	storedb "github.com/alekzonder/tariboy/internal/store"
)

var (
	ErrInvalid  = errors.New("invalid Store")
	ErrExists   = errors.New("Store already exists")
	ErrNotFound = errors.New("Store not found")
	ErrUnsafe   = errors.New("unsafe Store path")
)

type Store struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type StoreImage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Error   string `json:"error,omitempty"`
}

type Detail struct {
	Name   string       `json:"name"`
	Source string       `json:"source"`
	Path   string       `json:"path"`
	Images []StoreImage `json:"images"`
}

type PreparedBuild struct {
	Name string
	Path string
}

type Catalog struct {
	DB      *sql.DB
	BaseDir string
}

// ponytail: one daemon-wide lock; use per-Store locks if catalog concurrency becomes measurable.
var catalogMu sync.Mutex

const commandTimeout = 2 * time.Minute

var scpSource = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?[A-Za-z0-9.-]+:[^[:space:]]+$`)

func New(db *storedb.Store, baseDir string) *Catalog {
	c := &Catalog{BaseDir: baseDir}
	if db != nil {
		c.DB = db.DB
	}
	return c
}

func (c *Catalog) Add(ctx context.Context, name, source string) (Store, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if err := validName(name); err != nil {
		return Store{}, err
	}
	kind, normalized, err := classifySource(source)
	if err != nil {
		return Store{}, err
	}
	if _, err := c.get(name); err == nil {
		return Store{}, fmt.Errorf("%w: %s", ErrExists, name)
	} else if !errors.Is(err, ErrNotFound) {
		return Store{}, err
	}
	if kind == sourceLocal {
		if err := realDirectory(normalized); err != nil {
			return Store{}, err
		}
	} else {
		normalized = strings.TrimSpace(source)
		if err := c.clone(ctx, name, normalized); err != nil {
			return Store{}, err
		}
	}
	if c.DB == nil {
		return Store{}, errors.New("Store database is unavailable")
	}
	if _, err := c.DB.ExecContext(ctx, `INSERT INTO image_stores(name,source) VALUES(?,?)`, name, normalized); err != nil {
		if kind == sourceGit {
			_ = c.removeManaged(name)
		}
		return Store{}, err
	}
	return c.store(name, normalized), nil
}

func (c *Catalog) List(ctx context.Context) ([]Store, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if c.DB == nil {
		return nil, errors.New("Store database is unavailable")
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT name,source FROM image_stores ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Store{}
	for rows.Next() {
		var name, source string
		if err := rows.Scan(&name, &source); err != nil {
			return nil, err
		}
		result = append(result, c.store(name, source))
	}
	return result, rows.Err()
}

func (c *Catalog) Detail(ctx context.Context, name string) (Detail, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	return c.detail(ctx, name)
}

func (c *Catalog) Refresh(ctx context.Context, name string) (Detail, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	store, err := c.get(name)
	if err != nil {
		return Detail{}, err
	}
	kind, _, err := classifySource(store.Source)
	if err != nil {
		return Detail{}, err
	}
	git := kind == sourceGit
	if !git {
		git, err = isGitCheckout(ctx, store.Path)
		if err != nil {
			return Detail{}, err
		}
	}
	if git {
		if err := run(ctx, store.Path, "git", "pull", "--ff-only"); err != nil {
			return Detail{}, fmt.Errorf("refresh Store %s: %w", name, err)
		}
	}
	return c.detail(ctx, name)
}

func (c *Catalog) Remove(ctx context.Context, name string) error {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	store, err := c.get(name)
	if err != nil {
		return err
	}
	kind, _, err := classifySource(store.Source)
	if err != nil {
		return err
	}
	if kind == sourceGit {
		path := c.managedPath(name)
		if path != store.Path {
			return fmt.Errorf("%w: managed clone mismatch", ErrUnsafe)
		}
		if err := c.removeManaged(name); err != nil {
			return fmt.Errorf("remove managed Store clone: %w", err)
		}
	}
	_, err = c.DB.ExecContext(ctx, `DELETE FROM image_stores WHERE name=?`, name)
	return err
}

// PrepareBuild holds the catalog lock until release is called. Keeping it
// through image source freezing prevents Refresh from changing the selected tree.
func (c *Catalog) PrepareBuild(ctx context.Context, selector string) (prepared PreparedBuild, release func(), err error) {
	catalogMu.Lock()
	release = catalogMu.Unlock
	fail := func(cause error) (PreparedBuild, func(), error) {
		release()
		return PreparedBuild{}, nil, cause
	}
	parts := strings.Split(selector, "/")
	if len(parts) != 2 || validName(parts[0]) != nil || validName(parts[1]) != nil {
		return fail(fmt.Errorf("%w selector %q (want store/image)", ErrInvalid, selector))
	}
	store, err := c.get(parts[0])
	if err != nil {
		return fail(err)
	}
	if err := realDirectory(store.Path); err != nil {
		return fail(err)
	}
	imagesDir := filepath.Join(store.Path, "images")
	if err := realDirectory(imagesDir); err != nil {
		return fail(err)
	}
	imageDir := filepath.Join(imagesDir, parts[1])
	if err := realDirectory(imageDir); err != nil {
		return fail(err)
	}
	imagefilePath := filepath.Join(imageDir, imagefile.DefaultFilename)
	if err := regularFile(imagefilePath); err != nil {
		return fail(err)
	}
	for _, dir := range []string{store.Path, imageDir} {
		lock := filepath.Join(dir, "skills-lock.json")
		if _, err := os.Lstat(lock); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return fail(err)
		}
		if err := regularFile(lock); err != nil {
			return fail(err)
		}
		if err := run(ctx, dir, "npx", "skills", "experimental_install"); err != nil {
			return fail(fmt.Errorf("install Store skills lock: %w", err))
		}
	}
	return PreparedBuild{Name: parts[1], Path: imageDir}, release, nil
}

func (c *Catalog) detail(ctx context.Context, name string) (Detail, error) {
	store, err := c.get(name)
	if err != nil {
		return Detail{}, err
	}
	images, err := inventory(store.Path)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Name: store.Name, Source: store.Source, Path: store.Path, Images: images}, nil
}

func (c *Catalog) get(name string) (Store, error) {
	if err := validName(name); err != nil {
		return Store{}, err
	}
	if c.DB == nil {
		return Store{}, errors.New("Store database is unavailable")
	}
	var source string
	err := c.DB.QueryRow(`SELECT source FROM image_stores WHERE name=?`, name).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return Store{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return Store{}, err
	}
	return c.store(name, source), nil
}

func (c *Catalog) store(name, source string) Store {
	path := source
	if kind, _, _ := classifySource(source); kind == sourceGit {
		path = c.managedPath(name)
	}
	return Store{Name: name, Source: source, Path: path}
}

func (c *Catalog) managedPath(name string) string { return filepath.Join(c.BaseDir, "stores", name) }

func (c *Catalog) removeManaged(name string) error {
	root, err := os.OpenRoot(c.BaseDir)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := root.Lstat("stores")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: managed Stores parent is not a real directory", ErrUnsafe)
	}
	rel := filepath.Join("stores", name)
	target, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !target.IsDir() || target.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: managed Store clone is not a real directory", ErrUnsafe)
	}
	return root.RemoveAll(rel)
}

func (c *Catalog) clone(ctx context.Context, name, source string) error {
	root := filepath.Join(c.BaseDir, "stores")
	if err := secureDirectory(root); err != nil {
		return err
	}
	target := c.managedPath(name)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("%w: managed path exists", ErrUnsafe)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.MkdirTemp(root, ".clone-"+name+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := run(ctx, "", "git", "clone", "--", source, stage); err != nil {
		return fmt.Errorf("clone Store: %w", err)
	}
	if err := os.Chmod(stage, 0o700); err != nil {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		return err
	}
	return nil
}

func inventory(root string) ([]StoreImage, error) {
	if err := realDirectory(root); err != nil {
		return nil, err
	}
	imagesDir := filepath.Join(root, "images")
	if _, err := os.Lstat(imagesDir); errors.Is(err, os.ErrNotExist) {
		return []StoreImage{}, nil
	} else if err != nil {
		return nil, err
	}
	if err := realDirectory(imagesDir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	result := make([]StoreImage, 0, len(entries))
	for _, entry := range entries {
		item := StoreImage{Name: entry.Name()}
		dir := filepath.Join(imagesDir, entry.Name())
		if err := realDirectory(dir); err != nil {
			item.Error = err.Error()
			result = append(result, item)
			continue
		}
		if err := regularFile(filepath.Join(dir, imagefile.DefaultFilename)); err != nil {
			item.Error = err.Error()
			result = append(result, item)
			continue
		}
		parsed, err := imagefile.ParseAny(dir)
		if err != nil {
			item.Error = err.Error()
		} else if parsed.Version == 2 {
			item.Version = parsed.V2.ImageVersion
		} else {
			item.Version = parsed.V1.ImageVersion
		}
		if item.Version == "" && item.Error == "" {
			item.Version = "latest"
		}
		result = append(result, item)
	}
	return result, nil
}

type sourceKind int

const (
	sourceLocal sourceKind = iota
	sourceGit
)

func classifySource(source string) (sourceKind, string, error) {
	source = strings.TrimSpace(source)
	if filepath.IsAbs(source) {
		return sourceLocal, filepath.Clean(source), nil
	}
	if scpSource.MatchString(source) && !strings.Contains(source, "://") {
		return sourceGit, source, nil
	}
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "https" && u.Scheme != "ssh") || u.Host == "" || strings.Trim(u.Path, "/") == "" || u.RawQuery != "" || u.Fragment != "" {
		return 0, "", fmt.Errorf("%w source", ErrInvalid)
	}
	if u.Scheme == "https" && u.User != nil {
		return 0, "", fmt.Errorf("%w source credentials", ErrInvalid)
	}
	if u.User != nil {
		if _, password := u.User.Password(); password {
			return 0, "", fmt.Errorf("%w source credentials", ErrInvalid)
		}
	}
	return sourceGit, source, nil
}

func validName(name string) error {
	ref, err := image.ParseRef(name + ":latest")
	if err != nil || ref.Name != name || name == "." || name == ".." {
		return fmt.Errorf("%w name %q", ErrInvalid, name)
	}
	return nil
}

func secureDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrUnsafe, path)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	} else {
		return err
	}
	return os.Chmod(path, 0o700)
}

func realDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is not a real directory", ErrUnsafe, path)
	}
	return nil
}

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is not a regular file", ErrUnsafe, path)
	}
	return nil
}

func isGitCheckout(ctx context.Context, dir string) (bool, error) {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return false, nil
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, err
	}
	return true, nil
}

func run(ctx context.Context, dir, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}
