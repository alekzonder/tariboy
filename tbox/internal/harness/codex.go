package harness

// Probe of Codex 0.166.0 (`codex-cli 0.166.0`) on 2026-11-16. Every run used
// a fresh CODEX_HOME under a temp dir and only `codex --version` and
// `codex --profile <p> debug prompt-input hi`, which renders the model input
// locally without a login.
//
// Established before this task (see the design spec):
//   - A plugin in $CODEX_HOME/plugins/cache/tbox/<plugin>/<version>/ with
//     .codex-plugin/plugin.json ({"name","version","description",
//     "skills":"./skills/"}) and skills/<name>/SKILL.md, enabled by
//     [plugins."<plugin>@tbox"] enabled = true, lists <plugin>:<skill> in the
//     model input.
//   - The tables work from $CODEX_HOME/<p>.config.toml with `--profile <p>`;
//     without the flag the skill is absent. `-c` overrides do not enable the
//     plugin. A profile outside $CODEX_HOME is rejected.
//
// Found in this task:
//   - (a) The [marketplaces.tbox] table is not needed: the cache directory plus
//     [plugins."demo@tbox"] in the profile alone list demo:hello-tbox (the
//     skill root shown is $CODEX_HOME/plugins/cache/tbox). tbox therefore
//     writes no marketplace and nothing under <home>/run for Codex.
//   - (b) Instruction keys, as a top-level profile key:
//     `developer_instructions = "<text>"` adds the text as the start of the
//     developer message that follows Codex's own "You are Codex, ..."
//     message, which stays. `instructions` and `model_instructions_file`
//     put the text in place of that base message, so they replace Codex's
//     own instructions. `experimental_instructions_file` has no effect.
//     tbox uses `developer_instructions`.
//
// Not verified: that an interactive session behaves like `debug prompt-input`
// (no session was started), and whether Codex ever prunes a cache entry that
// no marketplace lists.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

const (
	codexMinimum     = "0.166.0"
	codexMarketplace = "tbox"
	codexDigestFile  = ".tbox-digest"
	codexDescription = "Skills of a tbox image"
)

type codex struct{}

func (codex) Name() string { return "codex" }

// codexHome is $CODEX_HOME, else $HOME/.codex, else empty.
func codexHome(host Host) string {
	if v := host.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	if h := host.Getenv("HOME"); h != "" {
		return filepath.Join(h, ".codex")
	}
	return ""
}

// codexCacheRoot is the tbox marketplace directory of the plugin cache. It
// refuses a symlink, so tbox never writes or removes through one.
func codexCacheRoot(home string) (string, error) {
	root := filepath.Join(home, "plugins", "cache", codexMarketplace)
	info, err := os.Lstat(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return root, nil
	case err != nil:
		return "", err
	case info.Mode()&fs.ModeSymlink != 0:
		return "", fmt.Errorf("%s is a symlink; tbox does not write through it", root)
	case !info.IsDir():
		return "", fmt.Errorf("%s is not a directory", root)
	}
	return root, nil
}

func codexProfileName(image string) string { return "tbox-" + pluginName(image) }

func codexProfilePath(home, image string) string {
	return filepath.Join(home, codexProfileName(image)+".config.toml")
}

func (codex) Prepare(ctx context.Context, host Host, run rundir.Run, extra []string) (Launch, error) {
	path, err := lookPath(host, "codex")
	if err != nil {
		return Launch{}, err
	}
	if err := checkCodexVersion(ctx, host, path); err != nil {
		return Launch{}, err
	}
	home := codexHome(host)
	if home == "" {
		return Launch{}, errors.New("neither CODEX_HOME nor HOME is set")
	}
	launch := Launch{Path: path, Env: imageEnv(run)}
	if err := prepareCodexFiles(home, run, &launch); err != nil {
		return Launch{}, err
	}
	launch.Argv = append([]string{"codex", "--profile", codexProfileName(run.Name)}, extra...)
	return launch, nil
}

func checkCodexVersion(ctx context.Context, host Host, path string) error {
	out, err := host.Output(ctx, nil, path, "--version")
	if err != nil {
		return fmt.Errorf("probe codex version: %w", err)
	}
	if atLeast(string(out), codexMinimum) {
		return nil
	}
	found := "unknown"
	if _, v, ok := parseVersion(string(out)); ok {
		found = v
	}
	return ErrUnsupportedVersion{Harness: "codex", Found: found, Minimum: codexMinimum}
}

// prepareCodexFiles writes the plugin cache and the profile under the cache
// root lock, so simultaneous launches do not race.
func prepareCodexFiles(home string, run rundir.Run, launch *Launch) error {
	root, err := codexCacheRoot(home)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	unlock, err := rundir.LockDir(root)
	if err != nil {
		return err
	}
	defer unlock()
	sweepStaging(root)
	sweepTempProfiles(home, "*")
	if len(run.Skills) > 0 {
		note, err := writeCodexPlugin(root, run)
		if err != nil {
			return err
		}
		if note != "" {
			launch.Notes = append(launch.Notes, note)
		}
	}
	profile, err := codexProfile(run)
	if err != nil {
		return err
	}
	return writeCodexProfile(home, run.Name, profile)
}

// codexRecord is the content of .tbox-digest: which image and digest the
// cache directory holds.
type codexRecord struct {
	Image  string `json:"image"`
	Digest string `json:"digest"`
}

func readCodexRecord(dir string) (codexRecord, bool) {
	var rec codexRecord
	body, err := os.ReadFile(filepath.Join(dir, codexDigestFile))
	if err != nil || json.Unmarshal(body, &rec) != nil {
		return codexRecord{}, false
	}
	return rec, true
}

// writeCodexPlugin builds plugins/cache/tbox/<plugin>/ holding .tbox-digest
// and <version>/ with the manifest and a copy of the skills. A directory
// whose record matches the image and digest is reused untouched; any other is
// replaced. The note reports a directory that held another image whose name
// maps to the same plugin name.
func writeCodexPlugin(root string, run rundir.Run) (string, error) {
	name := pluginName(run.Name)
	target := filepath.Join(root, name)
	want := codexRecord{Image: run.Name, Digest: run.Digest}
	old, found := readCodexRecord(target)
	if found && old == want {
		return "", nil
	}
	note := ""
	if found && old.Image != run.Name {
		note = fmt.Sprintf("tbox: codex plugin %s of image %q is replaced by image %q", name, old.Image, run.Name)
	}
	stage, err := os.MkdirTemp(root, stagePrefix)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := fillCodexPlugin(stage, run, want); err != nil {
		return "", err
	}
	return note, swapDir(stage, target)
}

func fillCodexPlugin(stage string, run rundir.Run, rec codexRecord) error {
	version := codexPluginVersion(run)
	dir := filepath.Join(stage, version)
	if err := copyTree(run.SkillsDir, filepath.Join(dir, "skills")); err != nil {
		return fmt.Errorf("copy skills into the codex plugin: %w", err)
	}
	manifest, err := json.MarshalIndent(struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Skills      string `json:"skills"`
	}{pluginName(run.Name), version, codexDescription + " " + run.Name, "./skills/"}, "", "  ")
	if err != nil {
		return err
	}
	if err := writeIfChanged(filepath.Join(dir, ".codex-plugin", "plugin.json"), append(manifest, '\n')); err != nil {
		return err
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, codexDigestFile), append(body, '\n'), 0o600)
}

// codexPluginVersion is the image version, else the first 12 digest
// characters. A version that is not a safe single path element falls back
// to the digest too.
func codexPluginVersion(run rundir.Run) string {
	v := strings.TrimSpace(run.ImageVersion)
	if v != "" && v != "." && v != ".." && !strings.ContainsAny(v, `/\`) {
		return v
	}
	d := run.Digest
	if len(d) > 12 {
		d = d[:12]
	}
	return d
}

// codexProfile renders tbox-<plugin>.config.toml. The top-level key comes
// before the table, as TOML requires.
func codexProfile(run rundir.Run) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# Written by tbox for image " + run.Name + ". `tbox clean` removes it.\n")
	empty, err := promptEmpty(run)
	if err != nil {
		return nil, err
	}
	if !empty {
		prompt, err := os.ReadFile(run.PromptPath)
		if err != nil {
			return nil, fmt.Errorf("read prompt: %w", err)
		}
		b.WriteString("developer_instructions = " + tomlString(string(prompt)) + "\n")
	}
	if len(run.Skills) > 0 {
		fmt.Fprintf(&b, "\n[plugins.%s]\nenabled = true\n", tomlString(pluginName(run.Name)+"@"+codexMarketplace))
	}
	return []byte(b.String()), nil
}

// tomlString quotes s as a TOML basic string. Invalid UTF-8 becomes U+FFFD.
func tomlString(s string) string {
	s = strings.ToValidUTF8(s, string(utf8.RuneError))
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// writeCodexProfile replaces $CODEX_HOME/tbox-<plugin>.config.toml through a
// temporary file named tbox-<plugin>.tmp-*.config.toml, which `tbox clean`
// also matches, and a rename. An identical regular file is left untouched.
func writeCodexProfile(home, image string, body []byte) error {
	path := codexProfilePath(home, image)
	if sameFile(path, body) {
		return nil
	}
	f, err := os.CreateTemp(home, codexProfileName(image)+".tmp-*.config.toml")
	if err != nil {
		return err
	}
	_, werr := f.Write(body)
	if err := errors.Join(werr, f.Close()); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// codexProfileHeader starts the first line of every profile tbox writes.
const codexProfileHeader = "# Written by tbox"

// sweepTempProfiles removes the temporary profiles tbox-<plugin>.tmp-*.config.toml
// of plugin, or of every plugin for "*", left by an interrupted launch.
func sweepTempProfiles(home, plugin string) {
	tmps, _ := filepath.Glob(filepath.Join(home, "tbox-"+plugin+".tmp-*.config.toml"))
	for _, p := range tmps {
		_ = removeIfExists(p)
	}
}

// writtenByTbox reports whether path is a regular file whose first line is
// the tbox profile header.
func writtenByTbox(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, len(codexProfileHeader))
	_, err = io.ReadFull(f, head)
	return err == nil && string(head) == codexProfileHeader
}

// removeProfile removes a profile that tbox wrote. A file of that name that
// tbox did not write is kept and reported.
func removeProfile(host Host, path string) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if !writtenByTbox(path) {
		host.note("tbox: kept %s: it does not start with the %q header of a tbox profile", path, codexProfileHeader)
		return nil
	}
	return removeIfExists(path)
}

// Clean removes plugins/cache/tbox/<plugin> and tbox-<plugin>.config.toml of
// image, or with an empty image plugins/cache/tbox and every
// tbox-*.config.toml, together with leftover temporary profiles. A profile
// is removed only when its first line is the tbox header. It never touches
// config.toml or another file. Without CODEX_HOME and HOME there is nowhere
// tbox could have written.
func (codex) Clean(host Host, image string) error {
	home := codexHome(host)
	if home == "" {
		return nil
	}
	root, err := codexCacheRoot(home)
	if err != nil {
		return err
	}
	if image != "" {
		sweepTempProfiles(home, pluginName(image))
		return errors.Join(
			os.RemoveAll(filepath.Join(root, pluginName(image))),
			removeProfile(host, codexProfilePath(home, image)),
		)
	}
	sweepTempProfiles(home, "*")
	errs := []error{os.RemoveAll(root)}
	profiles, err := filepath.Glob(filepath.Join(home, "tbox-*.config.toml"))
	errs = append(errs, err)
	for _, p := range profiles {
		errs = append(errs, removeProfile(host, p))
	}
	return errors.Join(errs...)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
