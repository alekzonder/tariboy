package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateWritesCertAndKey(t *testing.T) {
	dir := t.TempDir()
	if err := generate(dir); err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, f := range []string{"cert.pem", "key.pem"} {
		fi, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if fi.Size() == 0 {
			t.Fatalf("%s is empty", f)
		}
	}
}
