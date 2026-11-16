package tui

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/tbox/internal/textsafe"
)

const (
	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	clearDown  = "\x1b[J"
)

// screen draws frames in place: each draw moves the cursor back to the top of
// the previous frame and clears everything below it. It never uses the
// alternate screen. Lines end with CR LF because raw mode disables output
// post-processing.
type screen struct {
	t     Terminal
	lines int
}

func (s *screen) draw(lines []string) error {
	var b bytes.Buffer
	if s.lines > 0 {
		fmt.Fprintf(&b, "\r\x1b[%dA", s.lines)
	}
	b.WriteString(clearDown)
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\r\n")
	}
	s.lines = len(lines)
	_, err := s.t.Write(b.Bytes())
	return err
}

// clear removes the current frame, if any.
func (s *screen) clear() {
	if s.lines > 0 {
		_ = s.draw(nil)
	}
}

// leave replaces the frame with one permanent line.
func (s *screen) leave(line string) error {
	err := s.draw([]string{line})
	s.lines = 0
	return err
}

func (s *screen) showCursor() { _, _ = s.t.Write([]byte(showCursor)) }
func (s *screen) hideCursor() { _, _ = s.t.Write([]byte(hideCursor)) }

// truncate cuts s to w runes, ending in an ellipsis when it cut. Every rune
// counts as one column.
func truncate(s string, w int) string {
	if w < 1 {
		w = 1
	}
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	r := []rune(s)
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// clean makes untrusted text one printable line.
func clean(s string) string { return textsafe.Line(s, 0) }

// itemText joins label and detail when both fit in w columns.
func itemText(label, detail string, w int) string {
	if detail != "" && utf8.RuneCountInString(label)+2+utf8.RuneCountInString(detail) <= w {
		return label + "  " + detail
	}
	return truncate(label, w)
}

func cursorMark(on bool) string {
	if on {
		return "> "
	}
	return "  "
}

func checkMark(on bool) string {
	if on {
		return "[x] "
	}
	return "[ ] "
}

func joinLabels(labels []string) string { return strings.Join(labels, ", ") }
