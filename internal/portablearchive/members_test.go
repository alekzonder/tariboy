package portablearchive

import (
	"archive/tar"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestMemberRulesTable checks every row of MemberRules: its type and each of
// its modes are accepted for its format, a mode outside the row is refused
// with ErrMemberMode, and the type is refused for every format without a row
// for it.
func TestMemberRulesTable(t *testing.T) {
	t.Parallel()
	formats := []Format{FormatPortable, FormatImage}
	for i, row := range MemberRules {
		t.Run(fmt.Sprintf("%d %s %q", i, row.Format, row.Typeflag), func(t *testing.T) {
			modes := row.Modes
			if modes == nil {
				modes = []int64{0o600, 0o644, 0o755, 0o000}
			}
			for _, mode := range modes {
				h := &tar.Header{Name: "a", Typeflag: row.Typeflag, Mode: mode}
				if err := CheckMemberType(row.Format, h); err != nil {
					t.Fatalf("mode %#o refused: %v", mode, err)
				}
			}
			if row.Modes != nil {
				h := &tar.Header{Name: "a", Typeflag: row.Typeflag, Mode: 0o644}
				if err := CheckMemberType(row.Format, h); !errors.Is(err, ErrMemberMode) {
					t.Fatalf("mode 0644 error = %v, want ErrMemberMode", err)
				}
			}
			for _, other := range formats {
				if hasRow(other, row.Typeflag) {
					continue
				}
				h := &tar.Header{Name: "a", Typeflag: row.Typeflag, Mode: 0o600}
				if err := CheckMemberType(other, h); !errors.Is(err, ErrMemberType) {
					t.Fatalf("format %s type %q error = %v, want ErrMemberType", other, row.Typeflag, err)
				}
			}
		})
	}
}

func hasRow(format Format, typeflag byte) bool {
	for _, row := range MemberRules {
		if row.Format == format && row.Typeflag == typeflag {
			return true
		}
	}
	return false
}

func TestMemberRulesRejectLinksAndSpecialFiles(t *testing.T) {
	t.Parallel()
	for _, format := range []Format{FormatPortable, FormatImage} {
		for _, typeflag := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo, tar.TypeXGlobalHeader} {
			h := &tar.Header{Name: "a", Typeflag: typeflag, Mode: 0o600}
			if err := CheckMemberType(format, h); !errors.Is(err, ErrMemberType) {
				t.Fatalf("format %s type %q error = %v, want ErrMemberType", format, typeflag, err)
			}
		}
	}
	if err := CheckMemberType(FormatImage, &tar.Header{Name: "a", Typeflag: tar.TypeDir, Mode: 0o700}); !errors.Is(err, ErrMemberType) {
		t.Fatalf("image directory error = %v, want ErrMemberType", err)
	}
}

func TestMemberChecker(t *testing.T) {
	t.Parallel()
	limits := MemberLimits{MaxPathBytes: 8, MaxMembers: 2, MaxExpanded: 10}
	reg := func(name string, size int64) *tar.Header {
		return &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Size: size}
	}
	cases := []struct {
		name    string
		headers []*tar.Header
		wantErr error
	}{
		{"within limits", []*tar.Header{reg("a", 4), reg("b", 6)}, nil},
		{"too many members", []*tar.Header{reg("a", 1), reg("b", 1), reg("c", 1)}, ErrMemberCount},
		{"expanded size", []*tar.Header{reg("a", 6), reg("b", 5)}, ErrMemberSize},
		{"negative size", []*tar.Header{reg("a", -1)}, ErrMemberSize},
		{"path too long", []*tar.Header{reg(strings.Repeat("a", 9), 1)}, ErrMemberPath},
		{"empty path", []*tar.Header{reg("", 1)}, ErrMemberPath},
		{"type", []*tar.Header{{Name: "a", Typeflag: tar.TypeSymlink, Mode: 0o600}}, ErrMemberType},
		{"mode", []*tar.Header{{Name: "a", Typeflag: tar.TypeReg, Mode: 0o644}}, ErrMemberMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checker := MemberChecker{Format: FormatImage, Limits: limits}
			var err error
			for _, h := range tc.headers {
				if err = checker.Check(h); err != nil {
					break
				}
			}
			if tc.wantErr == nil && err != nil || tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("Check error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
