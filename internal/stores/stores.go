// Package stores manages per-daemon image source catalogs.
package stores

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	storedb "github.com/alekzonder/tariboy/internal/store"
)

var (
	ErrInvalid  = errors.New("invalid Store")
	ErrExists   = errors.New("Store already exists")
	ErrNotFound = errors.New("Store not found")
	ErrUnsafe   = errors.New("unsafe Store path")
	ErrRefresh  = errors.New("Store refresh failed")
	// ErrBadRef marks a malformed ref string; it also matches ErrInvalid.
	ErrBadRef = fmt.Errorf("%w ref", ErrInvalid)
	// ErrRefNotFound marks a ref that is no branch, tag or commit of the Store.
	ErrRefNotFound = errors.New("Store ref not found")

	errDatabaseUnavailable = errors.New("Store database is unavailable")
)

// Store is a registered Store. Ref is the branch, tag or commit its managed
// clone is checked out at ("" is the default branch); Commit is the checked-out
// commit, "" for a local Store that is no Git checkout.
type Store struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
}

type StoreImage struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	BuiltVersion string `json:"built_version"`
	UpdateNeeded bool   `json:"update_needed"`
	Error        string `json:"error,omitempty"`
	LatestStatus string `json:"latest_status"`
	LatestError  string `json:"latest_error,omitempty"`
}

type Detail struct {
	Name      string          `json:"name"`
	Source    string          `json:"source"`
	Path      string          `json:"path"`
	Ref       string          `json:"ref"`
	Commit    string          `json:"commit"`
	Images    []StoreImage    `json:"images"`
	Workflows []StoreWorkflow `json:"workflows"`
	Auto      AutoBuild       `json:"auto"`
}

type PreparedBuild struct {
	Name string
	Path string
}

type Catalog struct {
	DB      *sql.DB
	BaseDir string
	// Now overrides the automatic-build clock in tests.
	Now func() time.Time
}

// ponytail: one daemon-wide lock; use per-Store locks if catalog concurrency becomes measurable.
var catalogMu sync.Mutex

const commandTimeout = 2 * time.Minute

// maxCommandError bounds the command stderr tail carried in an error.
const maxCommandError = 2048

var scpSource = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?[A-Za-z0-9.-]+:[^[:space:]]+$`)

func New(db *storedb.Store, baseDir string) *Catalog {
	c := &Catalog{BaseDir: baseDir}
	if db != nil {
		c.DB = db.DB
	}
	return c
}

// Add registers a Store. A non-empty ref checks a Git Store's managed clone out
// at that branch, tag or commit; a local Store takes no ref.
func (c *Catalog) Add(ctx context.Context, name, source, ref string) (Store, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if err := validName(name); err != nil {
		return Store{}, err
	}
	kind, normalized, err := classifySource(source)
	if err != nil {
		return Store{}, err
	}
	if err := validRef(ref); err != nil {
		return Store{}, err
	}
	if ref != "" && kind == sourceLocal {
		return Store{}, fmt.Errorf("%w: a ref needs a Git Store; check the branch out in the local directory instead", ErrInvalid)
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
		if err := c.clone(ctx, name, normalized, ref); err != nil {
			return Store{}, err
		}
	}
	if c.DB == nil {
		return Store{}, errDatabaseUnavailable
	}
	if _, err := c.DB.ExecContext(ctx, `INSERT INTO image_stores(name,source,ref) VALUES(?,?,?)`, name, normalized, ref); err != nil {
		if kind == sourceGit {
			if rmErr := c.removeManaged(name); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove managed clone of store %s: %w", name, rmErr))
			}
		}
		return Store{}, err
	}
	return withCommit(ctx, c.store(name, normalized, ref)), nil
}

func (c *Catalog) List(ctx context.Context) ([]Store, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if c.DB == nil {
		return nil, errDatabaseUnavailable
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT name,source,ref FROM image_stores ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Store{}
	for rows.Next() {
		var name, source, ref string
		if err := rows.Scan(&name, &source, &ref); err != nil {
			return nil, err
		}
		result = append(result, c.store(name, source, ref))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range result {
		result[i] = withCommit(ctx, result[i])
	}
	return result, nil
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
	// A managed clone is daemon-owned: discard local edits so it always
	// matches the remote ref. A local Store is user work and only fast-forwards.
	if kind == sourceGit {
		if err := fetchAndCheckout(ctx, store.Path, name, store.Ref); err != nil {
			return Detail{}, err
		}
	} else if git, err := isGitCheckout(ctx, store.Path); err != nil {
		return Detail{}, err
	} else if git {
		if err := run(ctx, store.Path, "git", "pull", "--ff-only", "--no-autostash"); err != nil {
			return Detail{}, fmt.Errorf("%w: %s: %w", ErrRefresh, name, err)
		}
	}
	return c.detail(ctx, name)
}

// SetRef checks a Git Store's managed clone out at ref ("" is the default
// branch) and stores ref for later refreshes. A failure before the checkout
// leaves the tree and the stored ref unchanged.
func (c *Catalog) SetRef(ctx context.Context, name, ref string) (Detail, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if err := validRef(ref); err != nil {
		return Detail{}, err
	}
	store, err := c.get(name)
	if err != nil {
		return Detail{}, err
	}
	if kind, _, err := classifySource(store.Source); err != nil {
		return Detail{}, err
	} else if kind != sourceGit {
		return Detail{}, fmt.Errorf("%w: Store %s is local; only a Git Store takes a ref", ErrInvalid, name)
	}
	if err := fetchAndCheckout(ctx, store.Path, name, ref); err != nil {
		return Detail{}, err
	}
	if _, err := c.DB.ExecContext(ctx, `UPDATE image_stores SET ref=? WHERE name=?`, ref, name); err != nil {
		return Detail{}, err
	}
	return c.detail(ctx, name)
}

// fetchAndCheckout fetches the managed clone in dir and checks out ref. The
// fetch and the ref lookup do not touch the tree, so an unknown ref keeps it.
func fetchAndCheckout(ctx context.Context, dir, name, ref string) error {
	if err := run(ctx, dir, "git", "fetch", "--prune", "--force", "--tags", "origin"); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrRefresh, name, err)
	}
	return checkoutRef(ctx, dir, name, ref)
}

func checkoutRef(ctx context.Context, dir, name, ref string) error {
	commit, err := resolveRef(ctx, dir, ref)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for _, args := range [][]string{{"checkout", "--quiet", "--force", "--detach", commit}, {"clean", "-fd"}} {
		if err := run(ctx, dir, "git", args...); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrRefresh, name, err)
		}
	}
	return nil
}

var commitRef = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// resolveRef finds the commit of ref in the clone at dir: a remote branch, then
// a tag, then a commit id of 7 to 40 hex digits. "" is the remote's default
// branch.
func resolveRef(ctx context.Context, dir, ref string) (string, error) {
	candidates := []string{"refs/remotes/origin/HEAD"}
	if ref != "" {
		candidates = []string{"refs/remotes/origin/" + ref, "refs/tags/" + ref}
		if commitRef.MatchString(ref) {
			candidates = append(candidates, ref)
		}
	}
	for _, candidate := range candidates {
		commit, err := output(ctx, dir, "git", "rev-parse", "--verify", "--quiet", candidate+"^{commit}")
		if err == nil {
			return commit, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	if ref == "" {
		ref = "default branch"
	}
	return "", fmt.Errorf("%w: %s", ErrRefNotFound, ref)
}

// validRef rejects a ref that git could read as an option or a range before
// any git command sees it. "" is the default branch.
func validRef(ref string) error {
	if ref == "" {
		return nil
	}
	if len(ref) > 255 || strings.HasPrefix(ref, "-") || strings.Contains(ref, "..") || strings.IndexFunc(ref, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%w %q", ErrBadRef, ref)
	}
	return nil
}

// withCommit fills s.Commit from the Git checkout at s.Path. A Store that is
// no Git checkout, or a failing git, leaves it empty.
func withCommit(ctx context.Context, s Store) Store {
	if git, err := isGitCheckout(ctx, s.Path); err == nil && git {
		s.Commit, _ = output(ctx, s.Path, "git", "rev-parse", "HEAD")
	}
	return s
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
// through lock restoration and image source freezing prevents Refresh from
// changing the selected tree.
func (c *Catalog) PrepareBuild(selector string) (prepared PreparedBuild, release func(), err error) {
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
	return PreparedBuild{Name: parts[1], Path: imageDir}, release, nil
}

func (c *Catalog) RestoreBuildLocks(ctx context.Context, prepared PreparedBuild, out io.Writer) error {
	if err := c.RestoreStoreLock(ctx, prepared, out); err != nil {
		return err
	}
	return RestoreLock(ctx, prepared.Path, out)
}

// RestoreStoreLock restores only the Store root lock. An image that uses
// extends restores each layer's lock while its layers are assembled.
func (c *Catalog) RestoreStoreLock(ctx context.Context, prepared PreparedBuild, out io.Writer) error {
	return RestoreLock(ctx, filepath.Dir(filepath.Dir(prepared.Path)), out)
}

// RestoreLock runs the skills installer in dir when it holds skills-lock.json
// and leaves the lock file as it was. A non-nil out receives the installer's
// output.
func RestoreLock(ctx context.Context, dir string, out io.Writer) error {
	lock := filepath.Join(dir, "skills-lock.json")
	if _, err := os.Lstat(lock); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := regularFile(lock); err != nil {
		return err
	}
	if _, err := exec.LookPath("npx"); err != nil {
		return errors.New("building this image requires npx because it has skills-lock.json; install Node.js (npm ships npx) on the daemon host")
	}
	original, err := os.ReadFile(lock)
	if err != nil {
		return err
	}
	if out != nil {
		fmt.Fprintf(out, "npx skills experimental_install in %s\n", dir)
	}
	installErr := runTo(ctx, dir, out, "npx", "skills", "experimental_install")
	// The installer rewrites the tracked lock; restoring it keeps the
	// Store checkout clean so a later Refresh can fast-forward.
	if err := os.Remove(lock); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(installErr, fmt.Errorf("restore Store skills lock: %w", err))
	}
	if err := os.WriteFile(lock, original, 0o644); err != nil {
		return errors.Join(installErr, fmt.Errorf("restore Store skills lock: %w", err))
	}
	if installErr != nil {
		return fmt.Errorf("install Store skills lock: %w", installErr)
	}
	return nil
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
	built := &image.Store{Dir: filepath.Join(c.BaseDir, "images")}
	for i := range images {
		ref := image.Ref{Name: images[i].Name, Tag: "latest"}
		manifest, err := built.Inspect(ref)
		if err != nil {
			// No tag pointer means the image was never built here; anything
			// else is a real read failure worth surfacing.
			exists, statErr := built.TagExists(ref)
			if statErr == nil && !exists {
				images[i].LatestStatus = "missing"
			} else {
				images[i].LatestStatus = "error"
				images[i].LatestError = err.Error()
				if statErr != nil {
					images[i].LatestError = statErr.Error()
				}
			}
			continue
		}
		images[i].BuiltVersion = manifest.ImageVersion
		if manifest.ImageVersion == "" {
			images[i].LatestStatus = "unversioned"
		} else {
			images[i].LatestStatus = "built"
		}
		images[i].UpdateNeeded = images[i].Error == "" && images[i].Version != "" && images[i].BuiltVersion != "" && images[i].Version != images[i].BuiltVersion
	}
	workflows, err := workflowInventory(store.Path)
	if err != nil {
		return Detail{}, err
	}
	auto, err := c.readAuto(store.Name)
	if err != nil {
		return Detail{}, err
	}
	store = withCommit(ctx, store)
	return Detail{Name: store.Name, Source: store.Source, Path: store.Path, Ref: store.Ref, Commit: store.Commit, Images: images, Workflows: workflows, Auto: auto}, nil
}

func (c *Catalog) get(name string) (Store, error) {
	if err := validName(name); err != nil {
		return Store{}, err
	}
	if c.DB == nil {
		return Store{}, errDatabaseUnavailable
	}
	var source, ref string
	err := c.DB.QueryRow(`SELECT source,ref FROM image_stores WHERE name=?`, name).Scan(&source, &ref)
	if errors.Is(err, sql.ErrNoRows) {
		return Store{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return Store{}, err
	}
	return c.store(name, source, ref), nil
}

func (c *Catalog) store(name, source, ref string) Store {
	path := source
	if kind, _, _ := classifySource(source); kind == sourceGit {
		path = c.managedPath(name)
	}
	return Store{Name: name, Source: source, Path: path, Ref: ref}
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

func (c *Catalog) clone(ctx context.Context, name, source, ref string) error {
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
	if ref != "" {
		if err := checkoutRef(ctx, stage, name, ref); err != nil {
			return err
		}
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
		parsed, err := imagefile.ParseV2(dir)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Version = parsed.ImageVersion
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

// ValidName reports whether name is a legal Store or Store workflow name, by
// the rule every Store selector applies.
func ValidName(name string) bool { return validName(name) == nil }

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
	return runTo(ctx, dir, nil, name, args...)
}

// lockedWriter serializes writes to w.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// runTo is run that also copies the command's stdout and stderr to a non-nil
// out.
func runTo(ctx context.Context, dir string, out io.Writer, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if out != nil {
		// exec copies stdout and stderr in two goroutines; both write to out.
		shared := &lockedWriter{w: out}
		cmd.Stdout, cmd.Stderr = shared, io.MultiWriter(&stderr, shared)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > maxCommandError {
			detail = "..." + strings.ToValidUTF8(detail[len(detail)-maxCommandError:], "")
		}
		if detail == "" {
			return fmt.Errorf("%s failed: %w", name, err)
		}
		return fmt.Errorf("%s failed: %w: %s", name, err, detail)
	}
	return nil
}
