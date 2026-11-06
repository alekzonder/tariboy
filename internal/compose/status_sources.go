package compose

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// sourceStatus adds the status line of a workflows or images entry: the
// version its NAME:TAG was built from against the version its source gives
// now. A Store source is read from the Store's current checkout, without a
// refresh; a path source from the local file.
func (p *planner) sourceStatus(kind, name string, s SourceSpec, live liveSources) {
	id := kind + ":" + name
	label := fmt.Sprintf("%-10s %-10s", kind, name)
	source, err := p.r.sourceVersion(kind, name, s, live)
	if err != nil {
		p.status(id, true, "%s UNKNOWN source (%v)", label, err)
		return
	}
	built, found, readErr := live.builtVersion(kind, name)
	want := s.Version
	if kind == "workflow" {
		want = s.workflowVersion()
	}
	switch {
	case readErr != nil:
		p.status(id, true, "%s UNKNOWN (%s)", label, apiCode(readErr))
	case want != "" && want != source:
		p.status(id, true, "%s version conflict: want=%s source=%s", label, want, versionText(source))
	case !found:
		p.status(id, true, "%s MISSING (not built; source %s)", label, versionText(source))
	case built != source:
		p.status(id, true, "%s drift: built=%s source=%s", label, versionText(built), versionText(source))
	default:
		p.status(id, false, "%s ok (version %s)", label, versionText(built))
	}
}

func versionText(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

// sourceVersion is the version the source of an entry gives now.
func (r *Runner) sourceVersion(kind, name string, s SourceSpec, live liveSources) (string, error) {
	if s.Store != "" {
		d, ok := live.details[s.Store]
		if !ok {
			return "", fmt.Errorf("store %s cannot be read", s.Store)
		}
		return d.entryVersion(kind, s.sourceName(name))
	}
	path := r.absPath(s.Path)
	if kind == "workflow" {
		wf, err := workflowfile.Parse(path)
		if err != nil {
			return "", err
		}
		return wf.WorkflowVersion, nil
	}
	v2, err := imagefile.ParseV2(path)
	if err != nil {
		return "", err
	}
	return v2.ImageVersion, nil
}

// builtVersion is the version the entry's NAME:TAG was built from; found is
// false when it is not built.
func (live liveSources) builtVersion(kind, name string) (version string, found bool, err error) {
	if kind == "workflow" {
		img, ok := live.workflows[name]
		return img.Version, ok, live.workflowErr[name]
	}
	img, ok := live.images[name]
	return img.ImageVersion, ok, live.imageErr[name]
}
