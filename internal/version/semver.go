package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// semverRE is MAJOR.MINOR.PATCH[-PRE] of SemVer 2.0.0 without build metadata.
var semverRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?$`)

// SemVer is a parsed MAJOR.MINOR.PATCH[-PRE] version.
type SemVer struct {
	Major, Minor, Patch uint64
	Pre                 string
}

// ParseSemVer parses s as MAJOR.MINOR.PATCH[-PRE]. A "v" prefix, leading
// zeros, whitespace and build metadata ("+...") are refused.
func ParseSemVer(s string) (SemVer, error) {
	m := semverRE.FindStringSubmatch(s)
	if m == nil {
		return SemVer{}, fmt.Errorf("invalid version %q (want MAJOR.MINOR.PATCH[-PRERELEASE])", s)
	}
	var parts [3]uint64
	for i := range parts {
		n, err := strconv.ParseUint(m[i+1], 10, 64)
		if err != nil {
			return SemVer{}, fmt.Errorf("invalid version %q: %w", s, err)
		}
		parts[i] = n
	}
	return SemVer{Major: parts[0], Minor: parts[1], Patch: parts[2], Pre: m[4]}, nil
}

func (v SemVer) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare returns -1, 0 or 1 by SemVer 2.0.0 precedence: a prerelease is
// lower than its release (1.0.0-rc.1 < 1.0.0).
func (v SemVer) Compare(o SemVer) int {
	for _, d := range [][2]uint64{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if d[0] != d[1] {
			return cmpOrder(d[0] < d[1])
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	return comparePre(strings.Split(v.Pre, "."), strings.Split(o.Pre, "."))
}

func cmpOrder(less bool) int {
	if less {
		return -1
	}
	return 1
}

func comparePre(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		an, aErr := strconv.ParseUint(a[i], 10, 64)
		bn, bErr := strconv.ParseUint(b[i], 10, 64)
		switch {
		case aErr == nil && bErr == nil:
			return cmpOrder(an < bn)
		case aErr == nil:
			return -1 // numeric identifiers are lower than alphanumeric ones
		case bErr == nil:
			return 1
		default:
			return cmpOrder(a[i] < b[i])
		}
	}
	if len(a) == len(b) {
		return 0
	}
	return cmpOrder(len(a) < len(b))
}

// Constraint is an inclusive version range; a nil bound is open.
type Constraint struct {
	Min, Max *SemVer
}

// ParseConstraint parses the bounds of a range; an empty string is no bound.
func ParseConstraint(lo, hi string) (Constraint, error) {
	var c Constraint
	for _, b := range []struct {
		s   string
		dst **SemVer
	}{{lo, &c.Min}, {hi, &c.Max}} {
		if b.s == "" {
			continue
		}
		v, err := ParseSemVer(b.s)
		if err != nil {
			return Constraint{}, err
		}
		*b.dst = &v
	}
	if c.Min != nil && c.Max != nil && c.Min.Compare(*c.Max) > 0 {
		return Constraint{}, fmt.Errorf("min %s is greater than max %s", c.Min, c.Max)
	}
	return c, nil
}

// IsZero reports whether the constraint has no bound.
func (c Constraint) IsZero() bool { return c.Min == nil && c.Max == nil }

func (c Constraint) String() string {
	var parts []string
	if c.Min != nil {
		parts = append(parts, ">= "+c.Min.String())
	}
	if c.Max != nil {
		parts = append(parts, "<= "+c.Max.String())
	}
	return strings.Join(parts, ", ")
}

// Check returns nil when Min <= v <= Max.
func (c Constraint) Check(v SemVer) error {
	if (c.Min != nil && v.Compare(*c.Min) < 0) || (c.Max != nil && v.Compare(*c.Max) > 0) {
		return fmt.Errorf("version %s does not satisfy %s", v, c)
	}
	return nil
}
