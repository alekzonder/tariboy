// Package selfupdate implements `tariboy update`: it downloads a published
// server release from GitHub and installs it with the same fixed installer
// script the desktop app uploads, so both paths share one activation contract.
package selfupdate

import (
	"archive/tar"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/tariboy/internal/daemonctl"
	"github.com/alekzonder/tariboy/internal/tidy"
	"github.com/alekzonder/tariboy/internal/version"
)

// DefaultBaseURL is the GitHub releases root the published archives live under.
// TARIBOY_UPDATE_BASE_URL overrides it for a mirror or for tests.
const DefaultBaseURL = "https://github.com/alekzonder/tariboy/releases"

const (
	archiveFormat = "tariboy_%s_linux-%s.tar.gz"
	sumsName      = "SHA256SUMS"
	installerName = "remote-install.sh"
	// maxArchiveBytes bounds an untrusted download; the real archive is far smaller.
	maxArchiveBytes = 512 << 20
	// DefaultKeep is how many of the newest releases a cleanup keeps.
	DefaultKeep = 5
)

// archiveArches maps a supported GOARCH to the `uname -m` name in its archive.
var archiveArches = map[string]string{
	"amd64": "x86_64",
	"arm64": "aarch64",
}

func archiveName(target, goarch string) string {
	return fmt.Sprintf(archiveFormat, target, archiveArches[goarch])
}

// payloadBinaries are the executables every published release contains. The
// installer refuses to switch links without them.
var payloadBinaries = []string{
	"tariboyd",
	"tariboy",
	"tariboy-tasks",
	"tariboy-mail",
	"tariboy-shim",
	"tariboy-plugin-telegram",
}

type Options struct {
	// Version is an explicit release version or "latest".
	Version string
	// Force reinstalls the target version even when it is already active.
	Force bool
	// Cleanup deletes older releases after a successful update, keeping the
	// Keep newest ones (DefaultKeep when unset) and always the active one.
	Cleanup bool
	Keep    int
	BaseURL string
	Home    string
	GOOS    string
	GOARCH  string
	Getenv  func(string) string
	Client  *http.Client
	Out     io.Writer
}

func (o Options) getenv(key string) string {
	if o.Getenv == nil {
		return os.Getenv(key)
	}
	return o.Getenv(key)
}

func (o Options) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

// Run resolves, downloads, verifies, and installs a release, then restarts the
// daemon when one was running.
func Run(ctx context.Context, opts Options) error {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if _, ok := archiveArches[opts.GOARCH]; opts.GOOS != "linux" || !ok {
		return fmt.Errorf("tariboy update publishes only linux-x86_64 and linux-aarch64 releases; this host is %s-%s", opts.GOOS, opts.GOARCH)
	}
	if opts.Home == "" {
		return errors.New("HOME is required")
	}

	root := filepath.Join(opts.Home, ".local", "lib", "tariboy")
	current, err := managedVersion(opts.Home, root)
	if err != nil {
		return err
	}

	target := opts.Version
	if target == "" || target == "latest" {
		if target, err = resolveLatest(ctx, opts.client(), opts.BaseURL); err != nil {
			return err
		}
	}
	if err := validateVersion(target); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "target version %s (installed %s)\n", target, current)
	if target == current && !opts.Force {
		fmt.Fprintf(opts.Out, "%s is already installed; pass --force to reinstall\n", target)
		opts.cleanupAfterUpdate(root)
		return nil
	}

	archive, err := download(ctx, opts, target)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(archive) }()

	staging, err := stagingName()
	if err != nil {
		return err
	}
	stage := filepath.Join(root, staging)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", root, err)
	}
	if err := os.Mkdir(stage, 0o755); err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	// The installer publishes the staging directory by renaming it, so removing
	// it here is a no-op after a successful install and a cleanup after a failed
	// one.
	defer func() { _ = os.RemoveAll(stage) }()

	fmt.Fprintf(opts.Out, "unpacking %s\n", filepath.Base(archive))
	if err := extract(archive, stage); err != nil {
		return err
	}
	if err := install(ctx, opts, stage, staging, target); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "installed %s\n", target)
	// A failed restart keeps every release, so the previous one stays available
	// for a rollback.
	if err := restartDaemon(ctx, opts, target); err != nil {
		return err
	}
	opts.cleanupAfterUpdate(root)
	return nil
}

// cleanupAfterUpdate runs the requested cleanup. The update is already applied,
// so a cleanup failure is only reported.
func (o Options) cleanupAfterUpdate(root string) {
	if !o.Cleanup {
		return
	}
	keep := o.Keep
	if keep < 1 {
		keep = DefaultKeep
	}
	if err := cleanup(root, o.Home, keep, o.Out); err != nil {
		fmt.Fprintf(o.Out, "warning: cleanup: %v\n", err)
	}
}

// cleanup deletes all but the keep newest releases under root, counting and
// always keeping the active one. It removes only directories the installer
// published (a valid version name whose VERSION file matches) and holds the
// installer's lock so it never races an install.
func cleanup(root, home string, keep int, out io.Writer) error {
	lock, err := os.OpenFile(filepath.Join(root, ".install.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	// Same 60 s wait the installer uses for its own flock.
	for deadline := time.Now().Add(60 * time.Second); ; {
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("another tariboy install holds %s", lock.Name())
		}
		time.Sleep(200 * time.Millisecond)
	}

	active, err := managedVersion(home, root)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var releases []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == active || validateVersion(name) != nil {
			continue
		}
		recorded, err := os.ReadFile(filepath.Join(root, name, "VERSION"))
		if err != nil || strings.TrimSpace(string(recorded)) != name {
			continue
		}
		releases = append(releases, name)
	}
	slices.SortFunc(releases, func(a, b string) int { return compareVersions(b, a) })

	var errs []error
	for i, name := range releases {
		if i < keep-1 {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			errs = append(errs, err)
			continue
		}
		fmt.Fprintf(out, "removed release %s\n", name)
	}
	return errors.Join(errs...)
}

// compareVersions orders versions by their dot-separated components, numerically
// where both are numbers, so 0.10.0 sorts above 0.9.0.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		c := strings.Compare(as[i], bs[i])
		if aerr == nil && berr == nil {
			c = cmp.Compare(an, bn)
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}

// managedVersion returns the version the managed symlinks currently point at.
// It refuses any installation the fixed installer does not own, so an update
// never replaces a hand-placed binary or the macOS app-bundle links.
func managedVersion(home, root string) (string, error) {
	link := filepath.Join(home, ".local", "bin", "tariboy")
	target, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("%s is not a managed installation: %w", link, err)
	}
	if !strings.HasPrefix(target, root+string(filepath.Separator)) || filepath.Base(target) != "tariboy" {
		return "", fmt.Errorf("%s is not a managed installation: it points at %s", link, target)
	}
	current := filepath.Base(filepath.Dir(target))
	if err := validateVersion(current); err != nil {
		return "", fmt.Errorf("%s is not a managed installation: %w", link, err)
	}
	return current, nil
}

// resolveLatest reads the release tag from the redirect GitHub serves for
// /releases/latest, which needs no API token and no rate-limited request.
func resolveLatest(ctx context.Context, client *http.Client, baseURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/latest", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", version.UserAgent())
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := noRedirect.Do(request)
	if err != nil {
		return "", fmt.Errorf("resolve latest release: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))

	location := response.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("resolve latest release: no redirect from %s/latest (status %d)", baseURL, response.StatusCode)
	}
	tag := location[strings.LastIndex(location, "/")+1:]
	latest := strings.TrimPrefix(tag, "v")
	if err := validateVersion(latest); err != nil {
		return "", fmt.Errorf("resolve latest release: %w", err)
	}
	return latest, nil
}

// validateVersion accepts the same character set the installer accepts, so a
// version can never escape the release root or reach a shell as an option.
func validateVersion(candidate string) error {
	if candidate == "" || candidate == "latest" || strings.HasPrefix(candidate, ".") {
		return fmt.Errorf("invalid version: %q", candidate)
	}
	for _, r := range candidate {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == '.' || r == '_' || r == '-':
		default:
			return fmt.Errorf("invalid version: %q", candidate)
		}
	}
	return nil
}

// download fetches the release archive into a temporary file and verifies it
// against the release SHA256SUMS before anything is unpacked.
func download(ctx context.Context, opts Options, target string) (string, error) {
	name := archiveName(target, opts.GOARCH)
	base := fmt.Sprintf("%s/download/v%s/", opts.BaseURL, target)

	sums, err := fetch(ctx, opts, base+sumsName)
	if err != nil {
		return "", err
	}
	defer func() { _ = sums.Close() }()
	text, err := io.ReadAll(io.LimitReader(sums, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", sumsName, err)
	}
	expected, err := digestFor(string(text), name)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(opts.Out, "downloading %s\n", name)
	body, err := fetch(ctx, opts, base+name)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()

	file, err := os.CreateTemp("", "tariboy-update-*.tar.gz")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(body, maxArchiveBytes))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		tidy.Remove(nil, file.Name())
		return "", errors.Join(copyErr, closeErr)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != expected {
		tidy.Remove(nil, file.Name())
		return "", fmt.Errorf("checksum mismatch for %s: got %s, expected %s", name, got, expected)
	}
	return file.Name(), nil
}

func fetch(ctx context.Context, opts Options, url string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", version.UserAgent())
	response, err := opts.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if response.StatusCode != http.StatusOK {
		tidy.Close(nil, response.Body)
		return nil, fmt.Errorf("download %s: unexpected status %d", url, response.StatusCode)
	}
	return response.Body, nil
}

func digestFor(sums, name string) (string, error) {
	for line := range strings.SplitSeq(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%s does not list %s; this release publishes no server archive", sumsName, name)
}

func stagingName() (string, error) {
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return ".stage-" + hex.EncodeToString(token), nil
}

// extract unpacks the flat release payload. Only regular files at the archive
// root are accepted, so no entry can write outside the staging directory.
func extract(archivePath, stage string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag == tar.TypeDir && (name == "" || name == ".") {
			continue
		}
		if header.Typeflag != tar.TypeReg || name == "" || strings.ContainsRune(name, filepath.Separator) {
			return fmt.Errorf("unexpected archive entry: %q", header.Name)
		}
		mode := os.FileMode(0o644)
		if header.FileInfo().Mode()&0o111 != 0 {
			mode = 0o755
		}
		out, err := os.OpenFile(filepath.Join(stage, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(reader, maxArchiveBytes))
		if err := errors.Join(copyErr, out.Close()); err != nil {
			return err
		}
	}
}

// install runs the installer script the release carries. It verifies the
// payload checksums, serializes with flock, publishes the release directory,
// and switches the managed symlinks atomically or rolls back.
func install(ctx context.Context, opts Options, stage, staging, target string) error {
	for _, name := range append(payloadBinaries, "VERSION", sumsName, installerName) {
		if _, err := os.Stat(filepath.Join(stage, name)); err != nil {
			return fmt.Errorf("release archive is incomplete: %w", err)
		}
	}
	// The installer renames the staging directory it lives in, so it runs from a
	// copy outside that directory.
	script, err := os.CreateTemp("", "tariboy-install-*.sh")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(script.Name()) }()
	body, err := os.ReadFile(filepath.Join(stage, installerName))
	if err != nil {
		return err
	}
	if _, err := script.Write(body); err != nil {
		tidy.Close(nil, script)
		return err
	}
	if err := script.Close(); err != nil {
		return err
	}

	fmt.Fprintf(opts.Out, "installing %s\n", target)
	command := exec.CommandContext(ctx, "sh", script.Name(), target, staging)
	command.Env = append(os.Environ(), "HOME="+opts.Home)
	command.Stdout = opts.Out
	command.Stderr = opts.Out
	if err := command.Run(); err != nil {
		return fmt.Errorf("install %s: %w", target, err)
	}
	return nil
}

// restartDaemon restarts a running daemon onto the new release and reports the
// version it comes back with. A stopped daemon is left stopped.
func restartDaemon(ctx context.Context, opts Options, target string) error {
	cfg, err := daemonctl.ResolveConfig(opts.getenv)
	if err != nil {
		return err
	}
	// The running CLI is the previous release binary, so the daemon must come
	// from the managed link that now points at the new release.
	cfg.DaemonBin = filepath.Join(opts.Home, ".local", "bin", "tariboyd")
	if !daemonctl.GetStatus(cfg).Running {
		fmt.Fprintln(opts.Out, "daemon is not running; start it with `tariboy daemon start`")
		return nil
	}
	fmt.Fprintln(opts.Out, "restarting daemon")
	if err := daemonctl.Restart(ctx, cfg, opts.Out); err != nil {
		return fmt.Errorf("restart daemon: %w", err)
	}
	if running := daemonctl.GetStatus(cfg); running.Version != target {
		return fmt.Errorf("daemon reports version %q after the update, expected %q", running.Version, target)
	}
	fmt.Fprintf(opts.Out, "daemon is running %s\n", target)
	return nil
}
