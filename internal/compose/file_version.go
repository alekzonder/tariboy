package compose

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/version"
)

// TariboySpec is the top-level tariboy key of a version 2 file: the inclusive
// range of tariboy versions the file works with. Both bounds are optional.
type TariboySpec struct {
	Min string `yaml:"min,omitempty"`
	Max string `yaml:"max,omitempty"`
}

// VersionConstraint returns the file's tariboy range; it is zero when the
// file has no tariboy key.
func (f File) VersionConstraint() (version.Constraint, error) {
	if f.Tariboy == nil {
		return version.Constraint{}, nil
	}
	return version.ParseConstraint(f.Tariboy.Min, f.Tariboy.Max)
}

func (f File) tariboyProblems() []error {
	if f.Tariboy == nil {
		return nil
	}
	if f.Version == 1 {
		return []error{fmt.Errorf("the tariboy key needs version: 2")}
	}
	if _, err := f.VersionConstraint(); err != nil {
		return []error{fmt.Errorf("tariboy: %w", err)}
	}
	return nil
}
