package storesvc

import (
	"path/filepath"
	"testing"
)

func TestTokenSeedAndLookup(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SeedToken("s3cr3t-rw", ScopeReadWrite); err != nil {
		t.Fatal(err)
	}
	scope, ok, err := db.LookupToken("s3cr3t-rw")
	if err != nil || !ok || scope != ScopeReadWrite {
		t.Fatalf("lookup = %q,%v,%v", scope, ok, err)
	}
	if _, ok, _ := db.LookupToken("wrong"); ok {
		t.Fatal("unknown token must not resolve")
	}
	// Idempotent re-seed updates the scope in place.
	if err := db.SeedToken("s3cr3t-rw", ScopeRead); err != nil {
		t.Fatal(err)
	}
	scope, _, _ = db.LookupToken("s3cr3t-rw")
	if scope != ScopeRead {
		t.Fatalf("re-seed scope = %q, want read", scope)
	}
}
