package storesvc

import (
	"path/filepath"
	"testing"
)

func TestMandatoryTLSGuard(t *testing.T) {
	dir := t.TempDir()
	// No cert/key and no --allow-insecure -> fatal at construction.
	if _, err := New(Config{DataDir: dir, DBPath: filepath.Join(dir, "a.db")}); err == nil {
		t.Fatal("New without TLS and without AllowInsecure must error")
	}
	// --allow-insecure is the explicit escape hatch.
	s, err := New(Config{AllowInsecure: true, DataDir: dir, DBPath: filepath.Join(dir, "b.db")})
	if err != nil {
		t.Fatalf("New with AllowInsecure: %v", err)
	}
	s.Close()
}
