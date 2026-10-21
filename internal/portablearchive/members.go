package portablearchive

import (
	"archive/tar"
	"errors"
	"slices"
)

// Format names an archive layout whose tar members follow its rows of
// MemberRules.
type Format string

const (
	// FormatPortable is a tariboy-portable archive read by Stage.
	FormatPortable Format = "portable"
	// FormatImage is a runnable image tar.gz held by the image store.
	FormatImage Format = "image"
)

// MemberRule is one allowed tar member shape of a format: its type flag and
// the permission bits (Mode & 0o7777) it may carry. Nil Modes allows any.
type MemberRule struct {
	Format   Format
	Typeflag byte
	Modes    []int64
}

// MemberRules is the one table of tar members the archive validators accept.
// Anything without a row, links and special files included, is refused.
// archive/tar already reports the old '\x00' regular type as tar.TypeReg.
var MemberRules = []MemberRule{
	{Format: FormatPortable, Typeflag: tar.TypeReg},
	{Format: FormatPortable, Typeflag: tar.TypeDir},
	{Format: FormatImage, Typeflag: tar.TypeReg, Modes: []int64{0o600, 0o700}},
}

var (
	ErrMemberType  = errors.New("unsupported archive member type")
	ErrMemberMode  = errors.New("unsupported archive member mode")
	ErrMemberPath  = errors.New("invalid archive member path length")
	ErrMemberCount = errors.New("too many archive members")
	ErrMemberSize  = errors.New("archive expanded size exceeded")
)

// CheckMemberType matches h against the rows of format. It returns
// ErrMemberType when no row has h's type and ErrMemberMode when a row has the
// type but not h's mode.
func CheckMemberType(format Format, h *tar.Header) error {
	typeKnown := false
	for _, row := range MemberRules {
		if row.Format != format || row.Typeflag != h.Typeflag {
			continue
		}
		typeKnown = true
		if row.Modes == nil || slices.Contains(row.Modes, h.Mode&0o7777) {
			return nil
		}
	}
	if typeKnown {
		return ErrMemberMode
	}
	return ErrMemberType
}

// MemberLimits bounds the members of one archive.
type MemberLimits struct {
	MaxPathBytes int
	MaxMembers   int
	MaxExpanded  int64
}

// MemberChecker applies a format's MemberRules and the limits to the members
// of one archive, in order. Path canonical form and duplicates stay with the
// caller, which owns the cleaned name.
type MemberChecker struct {
	Format Format
	Limits MemberLimits

	count    int
	expanded int64
}

// Check counts h and checks, in order, the member count, type and mode, path
// length, and the running expanded size.
func (c *MemberChecker) Check(h *tar.Header) error {
	c.count++
	if c.count > c.Limits.MaxMembers {
		return ErrMemberCount
	}
	if err := CheckMemberType(c.Format, h); err != nil {
		return err
	}
	if len(h.Name) == 0 || len(h.Name) > c.Limits.MaxPathBytes {
		return ErrMemberPath
	}
	if h.Size < 0 || c.expanded > c.Limits.MaxExpanded-h.Size {
		return ErrMemberSize
	}
	c.expanded += h.Size
	return nil
}
