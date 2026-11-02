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
	// LimitChars is the most Unicode characters (code points) ROLE.md may hold.
	LimitChars = 500
	// readLimit bounds the read of the source file itself.
	readLimit = 4 << 20
)

// Role is the manifest field "role".
type Role struct {
	Text string `json:"text"`
}

// FromBytes turns the bytes of ROLE.md into a Role. Text of white space only
// counts as no file and returns nil. Text longer than LimitChars is an error.
func FromBytes(body []byte) (*Role, error) {
	if !utf8.Valid(body) {
		return nil, errors.New("ROLE.md is not valid UTF-8")
	}
	text := string(body)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	if n := utf8.RuneCountInString(text); n > LimitChars {
		return nil, fmt.Errorf("%s is %d characters, more than the limit of %d characters", FileName, n, LimitChars)
	}
	return &Role{Text: text}, nil
}

// Read reads dir/ROLE.md without following a symbolic link. A missing file
// returns nil, nil.
func Read(dir string) (*Role, error) {
	body, _, err := safefs.ReadRegular(filepath.Join(dir, FileName), readLimit)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	return FromBytes(body)
}

// Validate checks a role read from an imported manifest.
func Validate(r Role) error {
	switch {
	case !utf8.ValidString(r.Text):
		return errors.New("text is not valid UTF-8")
	case strings.TrimSpace(r.Text) == "":
		return errors.New("text is empty")
	}
	if n := utf8.RuneCountInString(r.Text); n > LimitChars {
		return fmt.Errorf("text is %d characters, more than the limit of %d characters", n, LimitChars)
	}
	return nil
}
