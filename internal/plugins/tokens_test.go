package plugins

import (
	"bytes"
	"testing"
)

func TestCanPublish(t *testing.T) {
	id := Identity{Name: "echo", Publish: []string{"chat:*", "user:ops"}}
	for ch, want := range map[string]bool{
		"chat:echo-out": true,
		"chat:anything": true,
		"user:ops":      true,
		"user:other":    false,
		"agent:a:inbox": false,
	} {
		if got := id.CanPublish(ch); got != want {
			t.Errorf("CanPublish(%q)=%v want %v", ch, got, want)
		}
	}
	// No publish patterns -> fail closed.
	if (Identity{Name: "x"}).CanPublish("chat:anything") {
		t.Fatal("empty publish scope must deny")
	}
}

func TestTokenLifecycle(t *testing.T) {
	reg := NewTokenRegistry(bytes.NewReader(bytes.Repeat([]byte{0xCD}, 1024)))
	tok, err := reg.Mint(Identity{Name: "echo", Publish: []string{"chat:*"}, Subscribe: []string{"chat:in"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 4 || tok[:4] != "plg-" {
		t.Fatalf("token shape wrong: %q", tok)
	}
	id, ok := reg.Resolve(tok)
	if !ok || id.Name != "echo" || !id.CanPublish("chat:x") {
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
