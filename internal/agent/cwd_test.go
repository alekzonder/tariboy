package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCwd(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"empty", "", true},
		{"absolute existing dir", dir, true},
		{"relative", "rel/path", false},
		{"non-existent", filepath.Join(dir, "nope"), false},
		{"file not dir", file, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateCwd(c.in)
			if c.ok && err != nil {
				t.Fatalf("want ok, got %v", err)
			}
			if !c.ok && err == nil {
				t.Fatalf("want error, got nil")
			}
		})
	}
}
