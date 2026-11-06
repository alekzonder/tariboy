package compose

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/agent"
)

// ImageSpec is an images entry. Context is the version 1 build directory;
// version 2 declares a SourceSpec instead.
type ImageSpec struct {
	Context    string `yaml:"context,omitempty"`
	SourceSpec `yaml:",inline"`
}

// BuildPath is the local directory the image builds from: path in version 2,
// context in version 1.
func (s ImageSpec) BuildPath() string {
	if s.Path != "" {
		return s.Path
	}
	return s.Context
}

func (f File) imageProblems() []error {
	var out []error
	for _, name := range sortedKeys(f.Images) {
		s := f.Images[name]
		switch {
		case !agent.ValidName(name):
			out = append(out, fmt.Errorf("invalid image name %q", name))
		case f.Version == 1:
			if s.SourceSpec.declared() {
				out = append(out, fmt.Errorf("image %q uses path, store, source, tag or version, which need version: 2", name))
			}
		case s.Context != "":
			out = append(out, fmt.Errorf("image %q context is version 1 syntax; use path", name))
		default:
			out = append(out, f.sourceProblems("image", name, s.SourceSpec)...)
		}
	}
	return out
}
