// Package image builds, stores and inspects agent images (spec §8).
package image

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrExists    = errors.New("image ref already exists")
	ErrImmutable = errors.New("image ref is immutable")
	ErrReserved  = errors.New("image ref is daemon-managed")
)

var refPart = regexp.MustCompile(`^[a-z0-9._-]+$`)

type Ref struct {
	Name string
	Tag  string
}

func ParseRef(s string) (Ref, error) {
	name, tag := s, "latest"
	if i := strings.LastIndex(s, ":"); i >= 0 {
		name, tag = s[:i], s[i+1:]
	}
	if !refPart.MatchString(name) {
		return Ref{}, fmt.Errorf("invalid image name %q (allowed: a-z 0-9 . _ -)", name)
	}
	if !refPart.MatchString(tag) {
		return Ref{}, fmt.Errorf("invalid image tag %q (allowed: a-z 0-9 . _ -)", tag)
	}
	return Ref{Name: name, Tag: tag}, nil
}

func (r Ref) String() string { return r.Name + ":" + r.Tag }

// IsReserved reports refs owned by daemon startup rather than public image
// authoring. Other tags under the same names remain ordinary immutable refs.
func IsReserved(r Ref) bool {
	return r.Tag == "latest" && (r.Name == "bare" || r.Name == "basic")
}
