// Package rolefile reads ROLE.md, the optional Markdown description of an
// image's or workflow image's role, into the manifest field "role". The text
// is metadata only: it never enters an agent prompt.
package rolefile

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/internal/safefs"
)

const (
	// FileName is the file at the root of a build context.
	FileName = "ROLE.md"
	// LimitBytes is the most text a manifest keeps; a longer file is cut.
	LimitBytes = 65536
	// MinLimitBytes is the smallest limit_bytes an imported manifest may declare.
	MinLimitBytes = 16384
	// readLimit bounds the read of the source file itself.
	readLimit = 4 << 20
)

// Role is the manifest field "role".
type Role struct {
	Text       string `json:"text"`
	Truncated  bool   `json:"truncated"`
	LimitBytes int    `json:"limit_bytes"`
}

// FromBytes turns the bytes of ROLE.md into a Role. Text of white space only
// counts as no file and returns nil. Text longer than LimitBytes is cut at the
// last UTF-8 character boundary within the limit.
func FromBytes(body []byte) (*Role, error) {
	if !utf8.Valid(body) {
		return nil, errors.New("ROLE.md is not valid UTF-8")
	}
	text := string(body)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	role := &Role{Text: text, LimitBytes: LimitBytes}
	if len(text) > LimitBytes {
		cut := LimitBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		role.Text = text[:cut]
		role.Truncated = true
	}
	return role, nil
}

// Read reads dir/ROLE.md without following a symbolic link. A missing file
// returns nil, 0, nil. The size is the length of the source file, for the
// truncation warning.
func Read(dir string) (*Role, int64, error) {
	body, _, err := safefs.ReadRegular(filepath.Join(dir, FileName), readLimit)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", FileName, err)
	}
	role, err := FromBytes(body)
	if err != nil {
		return nil, 0, err
	}
	return role, int64(len(body)), nil
}

// Validate checks a role read from an imported manifest.
func Validate(r Role) error {
	switch {
	case !utf8.ValidString(r.Text):
		return errors.New("text is not valid UTF-8")
	case strings.TrimSpace(r.Text) == "":
		return errors.New("text is empty")
	case r.LimitBytes < MinLimitBytes:
		return fmt.Errorf("limit_bytes %d is less than %d", r.LimitBytes, MinLimitBytes)
	case len(r.Text) > r.LimitBytes:
		return fmt.Errorf("text is %d bytes, more than limit_bytes %d", len(r.Text), r.LimitBytes)
	}
	return nil
}

// Warning is the build log line for a ROLE.md that was cut.
func Warning(size int64) string {
	return fmt.Sprintf("warning: %s is %d bytes; the manifest keeps the first %d bytes (truncated)", FileName, size, LimitBytes)
}
