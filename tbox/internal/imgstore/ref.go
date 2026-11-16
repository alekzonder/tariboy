package imgstore

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

var refPart = regexp.MustCompile(`^[a-z0-9._-]+$`)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Ref names an image and one of its tags.
type Ref struct {
	Name string
	Tag  string
}

// String is the name:tag form.
func (r Ref) String() string { return r.Name + ":" + r.Tag }

// ParseRef parses name[:tag]. The tag defaults to latest. Name and tag follow
// the rule of internal/image ref.go: a name is a-z 0-9 . _ -, never "." or
// ".."; a tag is the same or a complete SemVer version.
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

// CheckName validates an image name.
func CheckName(name string) error {
	if !refPart.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid image name %q (allowed: a-z 0-9 . _ -)", name)
	}
	return nil
}

// CheckTag validates an image tag.
func CheckTag(tag string) error {
	if tag == "." || tag == ".." || (!refPart.MatchString(tag) && tfile.ValidImageVersion(tag) != nil) {
		return fmt.Errorf("invalid image tag %q (allowed: a-z 0-9 . _ -, or a SemVer version)", tag)
	}
	return nil
}
