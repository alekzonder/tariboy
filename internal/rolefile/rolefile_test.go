package rolefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFromBytesKeepsText(t *testing.T) {
	r, err := FromBytes([]byte("# Reviewer\nreview code\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Text != "# Reviewer\nreview code\n" || r.Truncated || r.LimitBytes != LimitBytes {
		t.Fatalf("role = %+v", r)
	}
}

func TestFromBytesTreatsWhitespaceAsMissing(t *testing.T) {
	for _, body := range []string{"", " \n\t\r\n  ", " \n"} {
		r, err := FromBytes([]byte(body))
		if err != nil || r != nil {
			t.Fatalf("FromBytes(%q) = %+v, %v; want nil, nil", body, r, err)
		}
	}
}

func TestFromBytesRejectsInvalidUTF8(t *testing.T) {
	_, err := FromBytes([]byte("ok \xff\xfe"))
	if err == nil || !strings.Contains(err.Error(), "ROLE.md is not valid UTF-8") {
		t.Fatalf("err = %v", err)
	}
}

func TestFromBytesAtLimitIsNotTruncated(t *testing.T) {
	body := strings.Repeat("a", LimitBytes)
	r, err := FromBytes([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if r.Truncated || len(r.Text) != LimitBytes {
		t.Fatalf("truncated=%v len=%d", r.Truncated, len(r.Text))
	}
}

func TestFromBytesTruncatesAtRuneBoundary(t *testing.T) {
	// "я" is two bytes; it starts at LimitBytes-1 and ends past the limit.
	body := strings.Repeat("a", LimitBytes-1) + "я" + "tail"
	r, err := FromBytes([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Truncated || r.LimitBytes != LimitBytes {
		t.Fatalf("role = truncated %v limit %d", r.Truncated, r.LimitBytes)
	}
	if len(r.Text) != LimitBytes-1 || !utf8.ValidString(r.Text) {
		t.Fatalf("len = %d valid = %v", len(r.Text), utf8.ValidString(r.Text))
	}
}

func TestReadMissingFile(t *testing.T) {
	r, size, err := Read(t.TempDir())
	if err != nil || r != nil || size != 0 {
		t.Fatalf("Read = %+v, %d, %v", r, size, err)
	}
}

func TestReadReturnsRoleAndSize(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("b", LimitBytes+10)
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	r, size, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(body)) || !r.Truncated || len(r.Text) != LimitBytes {
		t.Fatalf("size=%d role truncated=%v len=%d", size, r.Truncated, len(r.Text))
	}
}

func TestReadRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, FileName)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(dir); err == nil || !strings.Contains(err.Error(), FileName) {
		t.Fatalf("err = %v, want a ROLE.md error", err)
	}
}

func TestValidate(t *testing.T) {
	ok := Role{Text: "role", LimitBytes: LimitBytes}
	if err := Validate(ok); err != nil {
		t.Fatalf("Validate(ok) = %v", err)
	}
	cases := map[string]Role{
		"empty":       {Text: " \n", LimitBytes: LimitBytes},
		"too long":    {Text: strings.Repeat("a", MinLimitBytes+1), LimitBytes: MinLimitBytes},
		"small limit": {Text: "role", LimitBytes: MinLimitBytes - 1},
		"bad utf8":    {Text: "a\xff", LimitBytes: LimitBytes},
	}
	for name, r := range cases {
		if err := Validate(r); err == nil {
			t.Errorf("Validate(%s) = nil, want error", name)
		}
	}
}

func TestWarningNamesSizeAndLimit(t *testing.T) {
	w := Warning(80000)
	if !strings.Contains(w, "80000") || !strings.Contains(w, "65536") || !strings.Contains(w, FileName) {
		t.Fatalf("warning = %q", w)
	}
}
