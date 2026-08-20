package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

func TestVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, "", &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != version.Version {
		t.Fatalf("version=%q want %q", got, version.Version)
	}
}
