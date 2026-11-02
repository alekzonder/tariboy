package rolefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFromBytesKeepsText(t *testing.T) {
	r, err := FromBytes([]byte("# Reviewer\nreview code\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || *r != (Role{Text: "# Reviewer\nreview code\n"}) {
		t.Fatalf("role = %+v", r)
	}
}

func TestFromBytesTreatsWhitespaceAsMissing(t *testing.T) {
	for _, body := range []string{"", " \n\t\r\n  ", " \n"} {
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

func TestFromBytesAcceptsLimitOfMultibyteCharacters(t *testing.T) {
	body := strings.Repeat("я", LimitChars)
	r, err := FromBytes([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Text != body {
		t.Fatalf("role = %+v", r)
	}
}

func TestFromBytesRejectsTextOverLimit(t *testing.T) {
	_, err := FromBytes([]byte(strings.Repeat("я", LimitChars+1)))
	if err == nil || !strings.Contains(err.Error(), "501 characters") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want size 501 and limit 500", err)
	}
}

func TestReadMissingFile(t *testing.T) {
	r, err := Read(t.TempDir())
	if err != nil || r != nil {
		t.Fatalf("Read = %+v, %v", r, err)
	}
}

func TestReadReturnsRole(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("# Role\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Read(dir)
	if err != nil || r == nil || r.Text != "# Role\n" {
		t.Fatalf("Read = %+v, %v", r, err)
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
	if _, err := Read(dir); err == nil || !strings.Contains(err.Error(), FileName) {
		t.Fatalf("err = %v, want a ROLE.md error", err)
	}
}

func TestValidate(t *testing.T) {
	for _, ok := range []Role{{Text: "role"}, {Text: strings.Repeat("я", LimitChars)}} {
		if err := Validate(ok); err != nil {
			t.Fatalf("Validate(%d chars) = %v", len([]rune(ok.Text)), err)
		}
	}
	cases := map[string]Role{
		"empty":    {Text: " \n"},
		"too long": {Text: strings.Repeat("я", LimitChars+1)},
		"bad utf8": {Text: "a\xff"},
	}
	for name, r := range cases {
		if err := Validate(r); err == nil {
			t.Errorf("Validate(%s) = nil, want error", name)
		}
	}
}
