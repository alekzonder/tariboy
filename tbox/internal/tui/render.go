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

func (s *screen) draw(lines []string, limit int) error {
	var b bytes.Buffer
	if s.lines > 0 {
		fmt.Fprintf(&b, "\r\x1b[%dA", s.lines)
	}
	b.WriteString(clearDown)
	for _, l := range lines {
		b.WriteString(hardCut(l, limit))
		b.WriteString("\r\n")
	}
	s.lines = len(lines)
	_, err := s.t.Write(b.Bytes())
	return err
}

// clear removes the current frame, if any.
func (s *screen) clear() {
	if s.lines > 0 {
		_ = s.draw(nil, 0)
	}
}

// leave replaces the frame with one permanent line.
func (s *screen) leave(line string, limit int) error {
	err := s.draw([]string{line}, limit)
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

// hardCut cuts s to at most limit runes with no ellipsis; a negative limit
// means no cut.
func hardCut(s string, limit int) string {
	if limit < 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

const (
	minCols = 20
	minRows = 5
)

// geometry returns the layout size and the longest line that may be written.
// A terminal smaller than minCols x minRows, or of unknown size, is laid out
// as if it had that minimum, but no line is longer than the real cols-1.
func geometry(t Terminal) (cols, rows, limit int) {
	c, r := t.Size()
	cols, rows = max(c, minCols), max(r, minRows)
	limit = cols - 1
	if c > 0 {
		limit = c - 1
	}
	return cols, rows, limit
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) }

// display makes untrusted or typed text safe for one line without trimming
// or collapsing spaces: control characters, C1 controls and invalid runes
// become spaces.
func display(s string) string {
	return strings.Map(func(r rune) rune {
		if isControl(r) || r == utf8.RuneError {
			return ' '
		}
		return r
	}, s)
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
