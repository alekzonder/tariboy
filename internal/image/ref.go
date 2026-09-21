// Package image builds, stores and inspects agent images (spec §8).
package image

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// ErrReserved marks the refs the daemon seeds itself; public authoring cannot
// replace or remove them.
var ErrReserved = errors.New("image ref is daemon-managed")

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
	if !refPart.MatchString(tag) && imagefile.ValidateImageVersion(tag) != nil {
		return Ref{}, fmt.Errorf("invalid image tag %q (allowed: a-z 0-9 . _ -, or a SemVer version)", tag)
	}
	return Ref{Name: name, Tag: tag}, nil
}

func (r Ref) String() string { return r.Name + ":" + r.Tag }

// IsReserved reports refs owned by daemon startup rather than public image
// authoring. Every other ref is ordinary and can be rebuilt at any time.
func IsReserved(r Ref) bool {
	return r.Tag == "latest" && (r.Name == "bare" || r.Name == "basic")
}

// ErrDuplicateTag and ErrTagReserved classify BuildRefs input so callers can
// map them onto their own error surfaces.
var (
	ErrDuplicateTag = errors.New("duplicate image tag")
	ErrTagReserved  = errors.New("image ref is managed by tariboyd")
)

// BuildRefs resolves the tags that one build publishes. No requested tag means
// the declared image_version plus latest. Every build path — the operator CLI
// and the agent tool alike — uses this one rule, and every returned ref points
// at the same content.
func BuildRefs(name string, tags []string, imageVersion string) ([]Ref, bool, error) {
	defaultTags := len(tags) == 0
	if defaultTags {
		if imageVersion == "" {
			imageVersion = "latest"
		}
		tags = []string{imageVersion}
		if imageVersion != "latest" {
			tags = append(tags, "latest")
		}
	}
	refs := make([]Ref, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		ref, err := ParseRef(name + ":" + tag)
		if err != nil {
			return nil, defaultTags, err
		}
		if seen[ref.String()] {
			return nil, defaultTags, fmt.Errorf("%w %s", ErrDuplicateTag, tag)
		}
		if IsReserved(ref) {
			return nil, defaultTags, fmt.Errorf("%w: %s", ErrTagReserved, ref.String())
		}
		seen[ref.String()] = true
		refs = append(refs, ref)
	}
	return refs, defaultTags, nil
}
