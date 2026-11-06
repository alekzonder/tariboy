package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/imagecontract"
	"github.com/alekzonder/tariboy/internal/portablearchive"
	"github.com/alekzonder/tariboy/internal/tidy"
	"github.com/alekzonder/tariboy/internal/version"
)

// downloader is the raw download of the daemon client (*client.Client).
type downloader interface {
	Download(route string, w io.Writer) (int64, error)
}

// runArchiveVerb runs pack, unpack, validate --archive and the deprecated
// archive and import aliases; ok is false for any other verb.
func runArchiveVerb(call Caller, o Options, out, errOut io.Writer) (code int, ok bool) {
	switch o.Verb {
	case "archive":
		fmt.Fprintln(errOut, "compose archive is deprecated and will be removed in the next release; use compose pack")
		o.Verb = "pack"
	case "import":
		fmt.Fprintln(errOut, "compose import is deprecated and will be removed in the next release; use compose unpack")
		o.Verb = "unpack"
	}
	switch {
	case o.Verb == "pack":
		file, err := resolveFile(o.File)
		if err != nil {
			fmt.Fprintf(errOut, "compose: %v\n", err)
			return 1, true
		}
		return composePack(call, file, o, out, errOut), true
	case o.Verb == "unpack":
		return composeUnpack(call, o, out, errOut), true
	case o.Verb == "validate" && o.Archive != "":
		return composeValidateArchive(call, o.Archive, o.Daemon, out, errOut), true
	}
	return 0, false
}

func resolveFile(file string) (string, error) {
	if file != "" {
		return file, nil
	}
	return FindFile(".")
}

// composePack builds the file's workflows and images like compose build
// (unless NoBuild), then writes the compose file and the exported archive of
// every agent image and workflow image to a new compose archive.
func composePack(call Caller, file string, o Options, out, errOut io.Writer) int {
	if o.Output == "" {
		fmt.Fprintln(errOut, "compose pack: -o FILE is required")
		return 2
	}
	f, workdir, err := loadComposeFile(file)
	if err == nil {
		err = f.Validate()
	}
	if err != nil {
		fmt.Fprintf(errOut, "compose pack: %v\n", err)
		return 1
	}
	if err := checkDaemonVersion(call, f, file); err != nil {
		return reportRunError("pack", err, errOut)
	}
	r := NewRunner(call, workdir, out)
	if !o.NoBuild {
		if err := r.Build(f); err != nil {
			return reportRunError("pack", err, errOut)
		}
	}
	tmp, err := os.MkdirTemp("", "tariboy-compose-pack-")
	if err != nil {
		return reportRunError("pack", err, errOut)
	}
	defer tidy.RemoveAll(nil, tmp)
	meta, err := r.collectPack(f, file, tmp)
	if err == nil {
		err = writePackFile(o.Output, tmp, meta)
	}
	if err != nil {
		return reportRunError("pack", err, errOut)
	}
	fmt.Fprintln(out, o.Output)
	return 0
}

// collectPack copies the compose file into dir and downloads the archive of
// every agent image and workflow image there, checking each one's digest
// against the daemon's.
func (r *Runner) collectPack(f File, file, dir string) (packMeta, error) {
	meta := packMeta{Schema: packSchema, ComposeFile: filepath.Base(file), Images: []packImage{}, Workflows: []packWorkflow{}}
	v, err := daemonVersion(r.call)
	if err != nil {
		return meta, err
	}
	meta.ProducerVersion = v.String()
	body, err := os.ReadFile(file)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, packComposeRel), body, 0o600)
	}
	if err != nil {
		return meta, err
	}
	p := &packed{dir: dir, plugins: map[string][]string{}}
	for _, ref := range packImageRefs(f) {
		img, err := r.packImage(p, ref)
		if err != nil {
			return meta, err
		}
		meta.Images = append(meta.Images, img)
	}
	for _, ref := range packWorkflowRefs(f) {
		if meta.Workflows, err = r.packWorkflow(p, ref, meta.Workflows); err != nil {
			return meta, err
		}
	}
	return meta, nil
}

func (r *Runner) packImage(p *packed, ref string) (packImage, error) {
	live, found, err := r.inspectImage(ref)
	if err != nil {
		return packImage{}, fmt.Errorf("image %s: %w", ref, err)
	}
	if !found {
		return packImage{}, fmt.Errorf("image %s is not built; build it, or pack without --no-build", ref)
	}
	img := packImage{Ref: ref, Digest: live.Digest, File: imageMemberPath(ref)}
	if err := r.download("/api/images/"+ref+"/export", p.member(img.File)); err != nil {
		return packImage{}, fmt.Errorf("export image %s: %w", ref, err)
	}
	if err := p.verifyImage(img); err != nil {
		return packImage{}, fmt.Errorf("image %s: %w", ref, err)
	}
	return img, nil
}

// packWorkflow adds the workflow image a ref names. An archive holds one
// workflow image per name, since import moves its latest tag.
func (r *Runner) packWorkflow(p *packed, ref string, list []packWorkflow) ([]packWorkflow, error) {
	live, found, err := r.inspectWorkflowImage(ref)
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", ref, err)
	}
	if !found {
		return nil, fmt.Errorf("workflow %s is not built; build it, or pack without --no-build", ref)
	}
	for _, wf := range list {
		if wf.Name == live.Name && wf.Digest == live.Digest {
			return list, nil
		}
		if wf.Name == live.Name {
			return nil, fmt.Errorf("workflow %s: the file uses two workflow images of %s (%s and %s); an archive holds one per name", ref, live.Name, wf.Version, live.Version)
		}
	}
	tag := ref[strings.LastIndex(ref, ":")+1:]
	wf := packWorkflow{Name: live.Name, Version: live.Version, Digest: live.Digest, Tags: []string{live.Version, "latest"},
		File: workflowMemberPath(live.Name, live.Version)}
	if err := r.download("/api/workflow-images/"+live.Name+"/"+tag+"/export", p.member(wf.File)); err != nil {
		return nil, fmt.Errorf("export workflow %s: %w", ref, err)
	}
	if err := p.verifyWorkflow(wf); err != nil {
		return nil, fmt.Errorf("workflow %s: %w", ref, err)
	}
	return append(list, wf), nil
}

// download writes the body of an export route to a new file; a body over the
// member limit is refused.
func (r *Runner) download(route, file string) error {
	d, ok := r.call.(downloader)
	if !ok {
		return errors.New("archive download is not available")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	w, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, err := d.Download(route, w)
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if err == nil && n > portablearchive.ComposeMemberBytes {
		err = fmt.Errorf("the archive is %d bytes, over the %d byte limit of one image in a compose archive", n, portablearchive.ComposeMemberBytes)
	}
	return err
}

// writePackFile writes the archive to a new file and removes it on error.
func writePackFile(output, dir string, meta packMeta) error {
	w, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	err = writePack(w, dir, meta)
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		tidy.Remove(nil, output)
	}
	return err
}

// openPack stages and checks an archive in a new temporary directory. A team
// archive comes back with team set and no checks. cleanup removes the
// directory.
func openPack(file string) (p *packed, team bool, cleanup func(), problems []error) {
	tmp, err := os.MkdirTemp("", "tariboy-compose-unpack-")
	if err != nil {
		return nil, false, nil, []error{err}
	}
	cleanup = func() { tidy.RemoveAll(nil, tmp) }
	stage := filepath.Join(tmp, "archive")
	m, err := stagePack(file, stage)
	switch {
	case err != nil:
		return nil, false, cleanup, []error{err}
	case m.Kind == "team":
		return nil, true, cleanup, nil
	case m.Kind != packKind:
		return nil, false, cleanup, []error{fmt.Errorf("archive kind %q is not a compose archive", m.Kind)}
	}
	p, problems = verifyPack(stage, m)
	return p, false, cleanup, problems
}

// composeUnpack checks the archive as a whole before any change, then the
// daemon version, then applies the file by phase with the images and
// workflows of the archive. A team v1 archive goes to the team import.
func composeUnpack(call Caller, o Options, out, errOut io.Writer) int {
	file := o.Archive
	if len(o.Args) > 0 {
		file = o.Args[0]
	}
	if file == "" {
		fmt.Fprintln(errOut, "compose unpack: the archive FILE is required")
		return 2
	}
	p, team, cleanup, problems := openPack(file)
	if cleanup != nil {
		defer cleanup()
	}
	if team {
		if o.DryRun {
			return composeImport(call, file, false, strings.NewReader(""), out, errOut)
		}
		return composeImport(call, file, o.Yes, o.Stdin, out, errOut)
	}
	if len(problems) > 0 {
		return printProblems("unpack", file, problems, errOut)
	}
	if err := checkUnpackVersions(call, p, file); err != nil {
		return reportRunError("unpack", err, errOut)
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return reportRunError("unpack", err, errOut)
	}
	r := NewRunner(call, filepath.Dir(abs), out)
	r.archive = p
	if o.DryRun || !o.Yes {
		if err := r.DryRun(p.file); err != nil || o.DryRun {
			return reportRunError("unpack", err, errOut)
		}
		if !confirm("Apply this archive? [y/N] ", o.Stdin, errOut) {
			fmt.Fprintln(errOut, "archive not applied")
			return 0
		}
	}
	plan, err := r.Plan(p.file)
	if err != nil {
		return reportRunError("unpack", err, errOut)
	}
	rep := r.Apply(plan)
	r.printReport("unpack", rep)
	return reportRunError("unpack", rep.summaryErr(), errOut)
}

func confirm(question string, stdin io.Reader, errOut io.Writer) bool {
	fmt.Fprint(errOut, question)
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var answer string
	if _, err := fmt.Fscan(stdin, &answer); err != nil {
		return false
	}
	return answer == "y" || answer == "Y" || answer == "yes"
}

// checkUnpackVersions requires the daemon inside the file's tariboy range and
// not older than the daemon that packed the archive.
func checkUnpackVersions(call Caller, p *packed, file string) error {
	if err := checkDaemonVersion(call, p.file, file); err != nil {
		return err
	}
	v, err := daemonVersion(call)
	if err != nil {
		return err
	}
	return producerConflict("daemon", v, p.meta.ProducerVersion)
}

// producerConflict fails when v is older than the version that packed the
// archive.
func producerConflict(what string, v version.SemVer, producer string) error {
	pv, err := version.ParseSemVer(producer)
	if err != nil {
		return fmt.Errorf("archive producer_version: %w", err)
	}
	if v.Compare(pv) < 0 {
		return fmt.Errorf("%s version %s is older than %s, the daemon that packed the archive", what, v, pv)
	}
	return nil
}

func printProblems(verb, file string, problems []error, errOut io.Writer) int {
	for _, p := range problems {
		fmt.Fprintf(errOut, "- %v\n", p)
	}
	fmt.Fprintf(errOut, "compose %s: %d problem(s) in %s\n", verb, len(problems), file)
	return 1
}

// composeValidateArchive checks an archive offline: the files of its
// manifest, the digest of every image and workflow image, the compose file,
// and the versions against the CLI. With daemon it also reads the daemon's
// version and plugins; it never writes to the daemon.
func composeValidateArchive(call Caller, file string, daemon bool, out, errOut io.Writer) int {
	p, team, cleanup, problems := openPack(file)
	if cleanup != nil {
		defer cleanup()
	}
	if team {
		problems = append(problems, errors.New("a team v1 archive is not checked; compose unpack imports it"))
	}
	if p != nil {
		problems = append(problems, archiveVersionProblems(call, p, file, daemon)...)
	}
	if len(problems) > 0 {
		return printProblems("validate", file, problems, errOut)
	}
	fmt.Fprintf(out, "%s: valid\n", file)
	return 0
}

func archiveVersionProblems(call Caller, p *packed, file string, daemon bool) []error {
	var out []error
	c, err := p.file.VersionConstraint()
	cli, cliErr := version.ParseSemVer(version.Version)
	if cliErr != nil {
		return []error{fmt.Errorf("CLI %w", cliErr)}
	}
	if err == nil && !c.IsZero() {
		if err := checkVersion("CLI", cli, c, file); err != nil {
			out = append(out, err)
		}
	}
	if err := producerConflict("CLI", cli, p.meta.ProducerVersion); err != nil {
		out = append(out, err)
	}
	if !daemon {
		return out
	}
	if err := checkUnpackVersions(call, p, file); err != nil {
		if client.IsDaemonDown(err) {
			return append(out, errors.New("tariboyd is not running; cannot check the daemon"))
		}
		out = append(out, err)
	}
	return append(out, pluginProblems(call, p)...)
}

// pluginProblems names each external plugin an image needs that the daemon
// has not installed.
func pluginProblems(call Caller, p *packed) []error {
	raw, err := call.Call("GET", "/api/plugins", map[string]string{})
	if err != nil {
		return []error{fmt.Errorf("read plugins: %w", err)}
	}
	var list struct {
		Plugins []struct {
			Name string `json:"name"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return []error{fmt.Errorf("read plugins: %w", err)}
	}
	installed := map[string]bool{}
	for _, pl := range list.Plugins {
		installed[pl.Name] = true
	}
	var out []error
	for _, ref := range sortedKeys(p.plugins) {
		for _, name := range p.plugins[ref] {
			if _, builtin := imagecontract.Builtin(name); !builtin && !installed[name] {
				out = append(out, fmt.Errorf("image %s needs plugin %s, which the daemon has not installed", ref, name))
			}
		}
	}
	return out
}
