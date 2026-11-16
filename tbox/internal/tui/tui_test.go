package tui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
)

type fakeTerm struct {
	keys     []Key
	keyErr   error
	panicEnd bool
	cols     int
	rows     int
	writes   [][]byte
	closes   int
}

func newFake(cols, rows int, keys ...Key) *fakeTerm {
	return &fakeTerm{keys: keys, cols: cols, rows: rows}
}

func (f *fakeTerm) ReadKey() (Key, error) {
	if len(f.keys) == 0 {
		if f.panicEnd {
			panic("boom")
		}
		if f.keyErr != nil {
			return Key{}, f.keyErr
		}
		return Key{}, io.EOF
	}
	k := f.keys[0]
	f.keys = f.keys[1:]
	return k, nil
}

func (f *fakeTerm) Write(p []byte) (int, error) {
	f.writes = append(f.writes, append([]byte(nil), p...))
	return len(p), nil
}
func (f *fakeTerm) Size() (int, int) { return f.cols, f.rows }
func (f *fakeTerm) Close() error     { f.closes++; return nil }

func (f *fakeTerm) all() string { return string(bytes.Join(f.writes, nil)) }

func (f *fakeTerm) cursorVisible() bool {
	s := f.all()
	return strings.LastIndex(s, showCursor) > strings.LastIndex(s, hideCursor)
}

func rk(r rune) Key    { return Key{Code: KeyRune, Rune: r} }
func ck(c KeyCode) Key { return Key{Code: c} }

func items(n int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{Label: fmt.Sprintf("item%03d", i), Detail: "detail"}
	}
	return out
}

func TestSelectMoveAndEnter(t *testing.T) {
	f := newFake(80, 24, ck(KeyDown), ck(KeyDown), ck(KeyEnter))
	got, err := Select(f, "pick", items(5))
	if err != nil || got != 2 {
		t.Fatalf("got %d, %v", got, err)
	}
	if !strings.Contains(f.all(), "pick: item002") {
		t.Fatalf("no summary line in %q", f.all())
	}
	if !f.cursorVisible() {
		t.Fatal("cursor left hidden")
	}
}

func TestSelectJKAndQ(t *testing.T) {
	f := newFake(80, 24, rk('j'), rk('j'), rk('k'), ck(KeyEnter))
	if got, err := Select(f, "p", items(5)); err != nil || got != 1 {
		t.Fatalf("got %d, %v", got, err)
	}
	f = newFake(80, 24, rk('q'))
	if _, err := Select(f, "p", items(5)); !errors.Is(err, ErrCancelled) {
		t.Fatalf("q: %v", err)
	}
}

func TestSelectCancel(t *testing.T) {
	for _, k := range []Key{ck(KeyEsc), ck(KeyCtrlC)} {
		f := newFake(80, 24, k)
		if _, err := Select(f, "p", items(3)); !errors.Is(err, ErrCancelled) {
			t.Fatalf("%v: %v", k, err)
		}
		if !f.cursorVisible() {
			t.Fatal("cursor left hidden")
		}
	}
}

func TestSelectFilterReturnsOriginalIndex(t *testing.T) {
	its := []Item{{Label: "alpha"}, {Label: "xx", Detail: "has AB inside"}, {Label: "cab"}}
	f := newFake(80, 24, rk('a'), rk('b'), ck(KeyEnter))
	got, err := Select(f, "p", its)
	if err != nil || got != 1 {
		t.Fatalf("got %d, %v", got, err)
	}
}

func TestFilterDisablesShortcuts(t *testing.T) {
	its := []Item{{Label: "aj"}, {Label: "ak"}, {Label: "ab"}}
	f := newFake(80, 24, rk('a'), rk('k'), ck(KeyEnter))
	got, err := Select(f, "p", its)
	if err != nil || got != 1 {
		t.Fatalf("filter ak: got %d, %v", got, err)
	}
	f = newFake(80, 24, rk('a'), rk('q'), ck(KeyBackspace), ck(KeyEnter))
	got, err = Select(f, "p", its)
	if err != nil || got != 0 {
		t.Fatalf("filter q: got %d, %v", got, err)
	}
}

func TestBackspaceRemovesFilter(t *testing.T) {
	its := []Item{{Label: "one"}, {Label: "two"}}
	f := newFake(80, 24, rk('t'), ck(KeyBackspace), ck(KeyEnter))
	got, err := Select(f, "p", its)
	if err != nil || got != 0 {
		t.Fatalf("got %d, %v", got, err)
	}
}

func TestEnterWithNoMatchIsIgnored(t *testing.T) {
	f := newFake(80, 24, rk('z'), ck(KeyEnter), ck(KeyBackspace), ck(KeyEnter))
	got, err := Select(f, "p", items(2))
	if err != nil || got != 0 {
		t.Fatalf("got %d, %v", got, err)
	}
}

func TestMultiSelect(t *testing.T) {
	f := newFake(80, 24, ck(KeySpace), ck(KeyDown), ck(KeySpace), ck(KeyEnter))
	got, err := MultiSelect(f, "p", items(3), nil)
	if err != nil || len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestMultiSelectPresetAndFilterKeepsChecked(t *testing.T) {
	f := newFake(80, 24, rk('2'), ck(KeySpace), ck(KeyEnter))
	got, err := MultiSelect(f, "p", items(3), []bool{true, false, false})
	if err != nil || len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	f = newFake(80, 24, ck(KeyEnter))
	got, err = MultiSelect(f, "p", items(3), []bool{false, true, true})
	if err != nil || len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("preset: %v, %v", got, err)
	}
}

func TestScrollWindow(t *testing.T) {
	keys := make([]Key, 0, 100)
	for i := 0; i < 99; i++ {
		keys = append(keys, ck(KeyDown))
	}
	keys = append(keys, ck(KeyEnter))
	f := newFake(80, 10, keys...)
	got, err := Select(f, "p", items(100))
	if err != nil || got != 99 {
		t.Fatalf("got %d, %v", got, err)
	}
	for _, w := range f.writes {
		if n := strings.Count(string(w), "item"); n > 6 {
			t.Fatalf("frame has %d item lines: %q", n, w)
		}
	}
	if !strings.Contains(f.all(), "> item099") {
		t.Fatal("last item never shown under the cursor")
	}
}

func TestPageKeys(t *testing.T) {
	f := newFake(80, 10, ck(KeyPageDown), ck(KeyEnter))
	got, err := Select(f, "p", items(100))
	if err != nil || got != 6 {
		t.Fatalf("pgdn: %d, %v", got, err)
	}
	f = newFake(80, 10, ck(KeyPageDown), ck(KeyPageDown), ck(KeyPageUp), ck(KeyEnter))
	got, err = Select(f, "p", items(100))
	if err != nil || got != 6 {
		t.Fatalf("pgup: %d, %v", got, err)
	}
}

func TestTruncation(t *testing.T) {
	its := []Item{{Label: "a-very-long-label-indeed", Detail: "and a detail"}}
	f := newFake(20, 10, ck(KeyEnter))
	if _, err := Select(f, "p", its); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.all(), "…") {
		t.Fatalf("no ellipsis in %q", f.all())
	}
	for _, w := range f.writes {
		for _, line := range strings.Split(string(w), "\r\n") {
			line = ownEscapes.ReplaceAllString(line, "")
			if strings.Contains(line, "a-very") && len([]rune(line)) > 20 {
				t.Fatalf("line too wide: %q", line)
			}
		}
	}
}

func TestDetailShownWhenFits(t *testing.T) {
	f := newFake(80, 10, ck(KeyEnter))
	if _, err := Select(f, "p", []Item{{Label: "lab", Detail: "det"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.all(), "lab  det") {
		t.Fatalf("detail missing: %q", f.all())
	}
}

var ownEscapes = regexp.MustCompile("\x1b\\[(\\d+A|J|\\?25[lh])")

func TestControlTextSanitized(t *testing.T) {
	f := newFake(80, 10, ck(KeyEnter))
	if _, err := Select(f, "ti\x1b[2Jtle", []Item{{Label: "a\tb\x1b[31m\nc", Detail: "d\x07"}}); err != nil {
		t.Fatal(err)
	}
	for _, w := range f.writes {
		s := ownEscapes.ReplaceAllString(string(w), "")
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", ""), "\r", "")
		if strings.ContainsAny(s, "\x1b\t\n\x07") {
			t.Fatalf("control bytes survived: %q", w)
		}
	}
}

func TestEmptyItems(t *testing.T) {
	f := newFake(80, 10)
	if _, err := Select(f, "p", nil); err == nil {
		t.Fatal("want error")
	}
	if _, err := MultiSelect(f, "p", nil, nil); err == nil {
		t.Fatal("want error")
	}
	if len(f.writes) != 0 {
		t.Fatalf("drew: %q", f.all())
	}
}

func TestReadKeyErrorLeavesTerminalToCaller(t *testing.T) {
	boom := errors.New("read failed")
	f := newFake(80, 10, ck(KeyDown))
	f.keyErr = boom
	if _, err := Select(f, "p", items(3)); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if f.closes != 0 {
		t.Fatalf("closes = %d", f.closes)
	}
	if !f.cursorVisible() {
		t.Fatal("cursor left hidden after error")
	}
	f = newFake(80, 10)
	f.keyErr = boom
	if _, err := MultiSelect(f, "p", items(3), nil); !errors.Is(err, boom) || !f.cursorVisible() {
		t.Fatalf("multi: %v", err)
	}
}

func TestPanicShowsCursor(t *testing.T) {
	f := newFake(80, 10)
	f.panicEnd = true
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("no panic")
			}
		}()
		_, _ = Select(f, "p", items(3))
	}()
	if !f.cursorVisible() {
		t.Fatal("cursor left hidden after panic")
	}
}

func TestInput(t *testing.T) {
	f := newFake(80, 10, rk('a'), rk('b'), rk('c'), ck(KeyBackspace), ck(KeyEnter))
	got, err := Input(f, "name", "")
	if err != nil || got != "ab" {
		t.Fatalf("got %q, %v", got, err)
	}
	f = newFake(80, 10, rk('x'), ck(KeySpace), rk('y'), ck(KeyEnter))
	got, _ = Input(f, "name", "in")
	if got != "inx y" {
		t.Fatalf("initial: %q", got)
	}
	if !f.cursorVisible() {
		t.Fatal("cursor hidden")
	}
}

func TestInputCancelAndError(t *testing.T) {
	f := newFake(80, 10, ck(KeyEsc))
	if _, err := Input(f, "n", "x"); !errors.Is(err, ErrCancelled) {
		t.Fatalf("got %v", err)
	}
	f = newFake(80, 10)
	if _, err := Input(f, "n", ""); !errors.Is(err, io.EOF) || !f.cursorVisible() || f.closes != 0 {
		t.Fatalf("got %v", err)
	}
}

func TestDecodeKey(t *testing.T) {
	cases := []struct {
		in   string
		want Key
		n    int
	}{
		{"\x1b[A", ck(KeyUp), 3},
		{"\x1b[B", ck(KeyDown), 3},
		{"\x1bOA", ck(KeyUp), 3},
		{"\x1bOB", ck(KeyDown), 3},
		{"\x1b[5~", ck(KeyPageUp), 4},
		{"\x1b[6~", ck(KeyPageDown), 4},
		{"\r", ck(KeyEnter), 1},
		{"\n", ck(KeyEnter), 1},
		{" ", ck(KeySpace), 1},
		{"\x03", ck(KeyCtrlC), 1},
		{"\x7f", ck(KeyBackspace), 1},
		{"\x08", ck(KeyBackspace), 1},
		{"a", rk('a'), 1},
		{"é", rk('é'), 2},
		{"\x1b[3~x", Key{}, 4},
		{"\x1b[1;5Ax", Key{}, 6},
		{"\x01", Key{}, 1},
		{"\x1bx", ck(KeyEsc), 1},
	}
	for _, c := range cases {
		got, n := decodeKey([]byte(c.in))
		if got != c.want || n != c.n {
			t.Errorf("%q: got %+v,%d want %+v,%d", c.in, got, n, c.want, c.n)
		}
	}
	for _, in := range []string{"\x1b", "\x1b[", "\x1b[5", "\x1bO", "\xc3", "\x1b[1;5"} {
		if _, n := decodeKey([]byte(in)); n != 0 {
			t.Errorf("%q should be incomplete, n=%d", in, n)
		}
	}
	if k, n := decodeKey([]byte("\xff")); n != 1 || k.Code != KeyRune || k.Rune != 0 {
		t.Errorf("invalid utf8: %+v %d", k, n)
	}
}

var upMove = regexp.MustCompile("^\r\x1b\\[(\\d+)A")

// checkFrames asserts that every frame moves up by the line count of the
// previous frame and that no line is longer than limit runes (limit < 0
// disables the width check).
func checkFrames(t *testing.T, f *fakeTerm, limit int) {
	t.Helper()
	prev := 0
	for _, w := range f.writes {
		s := string(w)
		if s == hideCursor || s == showCursor {
			continue
		}
		up := 0
		if m := upMove.FindStringSubmatch(s); m != nil {
			fmt.Sscan(m[1], &up)
		}
		if up != prev {
			t.Fatalf("frame moves up %d, previous frame wrote %d lines: %q", up, prev, s)
		}
		body := upMove.ReplaceAllString(s, "")
		body = ownEscapes.ReplaceAllString(body, "")
		prev = strings.Count(body, "\r\n")
		if limit < 0 {
			continue
		}
		for _, line := range strings.Split(body, "\r\n") {
			if n := len([]rune(line)); n > limit {
				t.Fatalf("line of %d runes exceeds %d: %q", n, limit, line)
			}
		}
	}
}

func TestInputSanitizesInitial(t *testing.T) {
	initial := "a\x1b[2J\nb"
	f := newFake(80, 10, ck(KeyEnter))
	got, err := Input(f, "name", initial)
	if err != nil || got != initial {
		t.Fatalf("value changed: %q, %v", got, err)
	}
	for _, w := range f.writes {
		s := upMove.ReplaceAllString(string(w), "")
		s = strings.ReplaceAll(ownEscapes.ReplaceAllString(s, ""), "\r\n", "")
		if strings.ContainsAny(s, "\x1b\n\x07") {
			t.Fatalf("control bytes survived: %q", w)
		}
		if strings.Count(string(w), "\r\n") > 1 {
			t.Fatalf("more than one line: %q", w)
		}
	}
	checkFrames(t, f, -1)
}

func TestC1ControlsRejected(t *testing.T) {
	f := newFake(80, 10, rk(0x9b), rk(0x85), ck(KeyEnter))
	got, err := Select(f, "p", items(2))
	if err != nil || got != 0 {
		t.Fatalf("got %d, %v", got, err)
	}
	if strings.ContainsRune(f.all(), 0x9b) || strings.ContainsRune(f.all(), 0x85) {
		t.Fatalf("C1 control written: %q", f.all())
	}
	f = newFake(80, 10, rk(0x9b), rk('x'), ck(KeyEnter))
	if got, _ := Input(f, "n", ""); got != "x" {
		t.Fatalf("input accepted C1: %q", got)
	}
}

func TestTinyTerminals(t *testing.T) {
	sizes := [][2]int{{1, 5}, {5, 0}, {0, 0}, {1, 1}, {19, 3}, {-1, -1}}
	for _, sz := range sizes {
		limit := sz[0] - 1
		if sz[0] <= 0 {
			limit = 19
		}
		mk := func() *fakeTerm {
			return newFake(sz[0], sz[1], ck(KeyDown), rk('i'), rk('t'), ck(KeyBackspace), ck(KeySpace), ck(KeyEnter))
		}
		f := mk()
		if _, err := Select(f, "a title", items(30)); err != nil {
			t.Fatalf("%v select: %v", sz, err)
		}
		checkFrames(t, f, limit)
		f = mk()
		if _, err := MultiSelect(f, "a title", items(30), nil); err != nil {
			t.Fatalf("%v multi: %v", sz, err)
		}
		checkFrames(t, f, limit)
		f = newFake(sz[0], sz[1], rk('a'), rk('b'), ck(KeyEnter))
		if _, err := Input(f, "a long prompt", "initial"); err != nil {
			t.Fatalf("%v input: %v", sz, err)
		}
		checkFrames(t, f, limit)
	}
}

func TestFilterShrinkAndGrowMoveUp(t *testing.T) {
	its := []Item{{Label: "a1"}, {Label: "a2"}, {Label: "a3"}, {Label: "a4"}, {Label: "a5"}, {Label: "b1"}}
	f := newFake(80, 10, rk('b'), ck(KeyBackspace), rk('a'), rk('3'), ck(KeyBackspace), ck(KeyBackspace), ck(KeyEnter))
	if _, err := Select(f, "p", its); err != nil {
		t.Fatal(err)
	}
	checkFrames(t, f, 79)
}

func TestNoMatchesLine(t *testing.T) {
	f := newFake(80, 10, rk('z'), ck(KeyEnter), ck(KeyEsc))
	if _, err := Select(f, "p", items(3)); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Enter on no match must do nothing: %v", err)
	}
	if !strings.Contains(f.all(), "no matches") {
		t.Fatalf("no 'no matches' line: %q", f.all())
	}
	checkFrames(t, f, 79)
}

func TestDropIncomplete(t *testing.T) {
	for _, in := range []string{"\x1b", "\x1b[", "\x1bO", "\x1b[1;5"} {
		k, rest := dropIncomplete([]byte(in))
		if k.Code != KeyEsc || len(rest) != 0 {
			t.Errorf("%q: %+v %q", in, k, rest)
		}
	}
	if k, rest := dropIncomplete([]byte("\xc3")); k != (Key{}) || len(rest) != 0 {
		t.Errorf("broken rune: %+v %q", k, rest)
	}
}
