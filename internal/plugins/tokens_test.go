package plugins

import (
	"bytes"
	"testing"
)

func TestTokenLifecycle(t *testing.T) {
	t.Parallel()
	reg := NewTokenRegistry(bytes.NewReader(bytes.Repeat([]byte{0xCD}, 1024)))
	tok, err := reg.Mint(Identity{Name: "echo", Send: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 4 || tok[:4] != "plg-" {
		t.Fatalf("token shape wrong: %q", tok)
	}
	id, ok := reg.Resolve(tok)
	if !ok || id.Name != "echo" || !id.Send {
		t.Fatalf("resolve = %+v ok=%v", id, ok)
	}
	reg.Revoke(tok)
	if _, ok := reg.Resolve(tok); ok {
		t.Fatal("revoked token still resolves")
	}
	if _, ok := reg.Resolve("plg-bogus"); ok {
		t.Fatal("unknown token resolved")
	}
}

func TestMintUnique(t *testing.T) {
	t.Parallel()
	reg := NewTokenRegistry(nil)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, err := reg.Mint(Identity{Name: "a"})
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
