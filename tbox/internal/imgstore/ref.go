package imgstore

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

var refPart = regexp.MustCompile(`^[a-z0-9._-]+$`)

// namePattern is refPart with a first character in [a-z0-9], so a name is
// never hidden, never an option and never a temporary file.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Ref names an image and one of its tags.
type Ref struct {
	Name string
	Tag  string
}

// String is the name:tag form.
func (r Ref) String() string { return r.Name + ":" + r.Tag }

// ParseRef parses name[:tag]. The tag defaults to latest. Name and tag follow
// CheckName and CheckTag, a subset of the rule of internal/image ref.go.
func ParseRef(s string) (Ref, error) {
	name, tag := s, "latest"
	if i := strings.LastIndex(s, ":"); i >= 0 {
		name, tag = s[:i], s[i+1:]
	}
	if err := CheckName(name); err != nil {
		return Ref{}, err
	}
	if err := CheckTag(tag); err != nil {
		return Ref{}, err
	}
	return Ref{Name: name, Tag: tag}, nil
}

// CheckName validates an image name: a-z 0-9 . _ -, starting with a-z or
// 0-9. This is stricter than internal/image, which also accepts a leading
// . _ or -.
func CheckName(name string) error {
	if !refPart.MatchString(name) {
		return fmt.Errorf("invalid image name %q (allowed: a-z 0-9 . _ -)", name)
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid image name %q: it must start with a lower-case letter or digit", name)
	}
	return nil
}

// CheckTag validates an image tag. A tag never starts with ".": the store
// keeps temporary files there and a listing skips them.
func CheckTag(tag string) error {
	if strings.HasPrefix(tag, ".") || (!refPart.MatchString(tag) && tfile.ValidImageVersion(tag) != nil) {
		return fmt.Errorf("invalid image tag %q (allowed: a-z 0-9 . _ -, not starting with ., or a SemVer version)", tag)
	}
	return nil
}
