// Package tui draws small interactive lists on a terminal. The functions take
// a Terminal so tests can replay keys without a real tty.
package tui

import (
	"errors"
	"strings"
)

// KeyCode names a decoded key.
type KeyCode int

const (
	// KeyRune is a printable rune in Key.Rune. A KeyRune with Rune 0 is a
	// key the decoder ignores.
	KeyRune KeyCode = iota
	KeyUp
	KeyDown
	KeyEnter
	KeySpace
	KeyEsc
	KeyCtrlC
	KeyBackspace
	KeyPageUp
	KeyPageDown
)

// Key is one decoded key press.
type Key struct {
	Rune rune
	Code KeyCode
}

// Terminal is what the lists need from a terminal. The caller owns Close.
type Terminal interface {
	ReadKey() (Key, error)
	Write(p []byte) (int, error)
	Size() (cols, rows int)
	Close() error
}

// ErrCancelled is returned when the user leaves a list with Esc, q or Ctrl-C.
var ErrCancelled = errors.New("cancelled")

// Item is one list row. Detail is shown after Label when it fits.
type Item struct {
	Label  string
	Detail string
}

// Select shows items and returns the index of the chosen one.
func Select(t Terminal, title string, items []Item) (int, error) {
	l, err := runList(t, title, items, nil, false)
	if err != nil {
		return 0, err
	}
	return l.matches[l.cursor], nil
}

// MultiSelect lets the user toggle items and returns the checked indices in
// ascending order. checked presets the toggles; it may be nil or shorter than
// items.
func MultiSelect(t Terminal, title string, items []Item, checked []bool) ([]int, error) {
	l, err := runList(t, title, items, checked, true)
	if err != nil {
		return nil, err
	}
	return l.chosen(), nil
}

// Input reads one line of text starting from initial.
func Input(t Terminal, prompt, initial string) (string, error) {
	s := &screen{t: t}
	defer s.showCursor()
	defer s.clear()
	prompt = clean(prompt)
	text := []rune(initial)
	for {
		cols, _, limit := geometry(t)
		if err := s.draw([]string{truncate(prompt+": "+display(string(text)), cols-1)}, limit); err != nil {
			return "", err
		}
		k, err := t.ReadKey()
		if err != nil {
			return "", err
		}
		switch k.Code {
		case KeyEnter:
			out := string(text)
			return out, s.leave(truncate(prompt+": "+clean(out), cols-1), limit)
		case KeyEsc, KeyCtrlC:
			return "", ErrCancelled
		case KeyBackspace:
			if len(text) > 0 {
				text = text[:len(text)-1]
			}
		case KeySpace:
			text = append(text, ' ')
		case KeyRune:
			if !isControl(k.Rune) {
				text = append(text, k.Rune)
			}
		}
	}
}

type action int

const (
	actNone action = iota
	actDone
	actCancel
)

type list struct {
	title   string
	labels  []string
	details []string
	lower   []string
	checked []bool
	multi   bool
	filter  []rune
	matches []int
	cursor  int
	top     int
}

func newList(title string, items []Item, checked []bool, multi bool) *list {
	l := &list{title: clean(title), multi: multi, checked: make([]bool, len(items))}
	copy(l.checked, checked)
	for _, it := range items {
		lab, det := clean(it.Label), clean(it.Detail)
		l.labels = append(l.labels, lab)
		l.details = append(l.details, det)
		l.lower = append(l.lower, strings.ToLower(lab+"\n"+det))
	}
	l.refilter()
	return l
}

func (l *list) refilter() {
	needle := strings.ToLower(string(l.filter))
	l.matches = l.matches[:0]
	for i, s := range l.lower {
		if strings.Contains(s, needle) {
			l.matches = append(l.matches, i)
		}
	}
	l.cursor, l.top = 0, 0
}

func (l *list) move(delta int) {
	if len(l.matches) == 0 {
		return
	}
	l.cursor = min(max(l.cursor+delta, 0), len(l.matches)-1)
}

func (l *list) chosen() []int {
	var out []int
	for i, c := range l.checked {
		if c {
			out = append(out, i)
		}
	}
	return out
}

func (l *list) typeRune(r rune) action {
	if isControl(r) {
		return actNone
	}
	if len(l.filter) == 0 {
		switch r {
		case 'k':
			l.move(-1)
			return actNone
		case 'j':
			l.move(1)
			return actNone
		case 'q':
			return actCancel
		}
	}
	l.filter = append(l.filter, r)
	l.refilter()
	return actNone
}

func (l *list) backspace() {
	if len(l.filter) > 0 {
		l.filter = l.filter[:len(l.filter)-1]
		l.refilter()
	}
}

func (l *list) handle(k Key, page int) action {
	switch k.Code {
	case KeyUp:
		l.move(-1)
	case KeyDown:
		l.move(1)
	case KeyPageUp:
		l.move(-page)
	case KeyPageDown:
		l.move(page)
	case KeyEnter:
		if len(l.matches) > 0 || l.multi {
			return actDone
		}
	case KeySpace:
		if l.multi && len(l.matches) > 0 {
			i := l.matches[l.cursor]
			l.checked[i] = !l.checked[i]
		}
	case KeyEsc, KeyCtrlC:
		return actCancel
	case KeyBackspace:
		l.backspace()
	case KeyRune:
		return l.typeRune(k.Rune)
	}
	return actNone
}

// pageSize is the number of item rows: rows-4, at least 1.
func pageSize(rows int) int { return max(rows-4, 1) }

func (l *list) view(cols, rows int) []string {
	h := pageSize(rows)
	if l.cursor < l.top {
		l.top = l.cursor
	}
	if l.cursor >= l.top+h {
		l.top = l.cursor - h + 1
	}
	w := max(cols-4, 1)
	lines := []string{truncate(l.title, cols-1), truncate("filter: "+display(string(l.filter)), cols-1)}
	textW := w
	if l.multi {
		textW = max(w-4, 1)
	}
	for p := l.top; p < len(l.matches) && p < l.top+h; p++ {
		i := l.matches[p]
		line := cursorMark(p == l.cursor)
		if l.multi {
			line += checkMark(l.checked[i])
		}
		lines = append(lines, line+itemText(l.labels[i], l.details[i], textW))
	}
	if len(l.matches) == 0 {
		lines = append(lines, "  (no matches)")
	}
	hint := "up/down move, Enter confirm, Esc cancel, type to filter"
	if l.multi {
		hint = "up/down move, Space toggle, Enter confirm, Esc cancel, type to filter"
	}
	return append(lines, truncate(hint, cols-1))
}

func (l *list) summary() string {
	var labels []string
	if l.multi {
		for _, i := range l.chosen() {
			labels = append(labels, l.labels[i])
		}
	} else {
		labels = []string{l.labels[l.matches[l.cursor]]}
	}
	return l.title + ": " + joinLabels(labels)
}

func runList(t Terminal, title string, items []Item, checked []bool, multi bool) (*list, error) {
	if len(items) == 0 {
		return nil, errors.New("nothing to choose from")
	}
	l := newList(title, items, checked, multi)
	s := &screen{t: t}
	s.hideCursor()
	defer s.showCursor()
	defer s.clear()
	for {
		cols, rows, limit := geometry(t)
		if err := s.draw(l.view(cols, rows), limit); err != nil {
			return nil, err
		}
		k, err := t.ReadKey()
		if err != nil {
			return nil, err
		}
		switch l.handle(k, pageSize(rows)) {
		case actCancel:
			return nil, ErrCancelled
		case actDone:
			return l, s.leave(truncate(l.summary(), cols-1), limit)
		}
	}
}
