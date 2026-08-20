package aiproxy

import (
	"bytes"
	"strings"
	"testing"
)

func TestTokenLifecycle(t *testing.T) {
	// A deterministic reader makes minted tokens reproducible.
	reg := NewTokenRegistry(bytes.NewReader(bytes.Repeat([]byte{0xAB}, 1024)))
	attr := Attribution{Agent: "alice", Iteration: "alice-1", ImageName: "basic",
		ImageTag: "latest", ImageDigest: "sha256:x"}
	tok, err := reg.Mint(attr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "sk-tariboy-") {
		t.Fatalf("token shape wrong: %q", tok)
	}
	got, ok := reg.Resolve(tok)
	if !ok || got.Agent != "alice" || got.Iteration != "alice-1" || got.ImageDigest != "sha256:x" {
		t.Fatalf("resolve = %+v ok=%v", got, ok)
	}
	if reg.Count() != 1 {
		t.Fatalf("count = %d", reg.Count())
	}
	// Revoked tokens no longer resolve.
	reg.Revoke(tok)
	if _, ok := reg.Resolve(tok); ok {
		t.Fatal("revoked token still resolves")
	}
	if reg.Count() != 0 {
		t.Fatalf("count after revoke = %d", reg.Count())
	}
	// Unknown token never resolves.
	if _, ok := reg.Resolve("sk-tariboy-deadbeef"); ok {
		t.Fatal("unknown token resolved")
	}
}

func TestMintUnique(t *testing.T) {
	reg := NewTokenRegistry(nil) // crypto/rand
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, err := reg.Mint(Attribution{Agent: "a", Iteration: "a-1"})
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
	}
	if reg.Count() != 100 {
		t.Fatalf("count = %d", reg.Count())
	}
}
