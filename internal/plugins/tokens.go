package plugins

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
)

// Identity is the scoped identity a valid plugin-token resolves to: the
// plugin and whether it may send mail.
type Identity struct {
	Name string
	Send bool
}

// TokenRegistry maps short-lived plugin-tokens to scoped identities. In-memory
// only; minted at plugin start, revoked at stop, re-minted on restart.
type TokenRegistry struct {
	mu     sync.RWMutex
	rand   io.Reader
	tokens map[string]Identity
}

func NewTokenRegistry(r io.Reader) *TokenRegistry {
	if r == nil {
		r = rand.Reader
	}
	return &TokenRegistry{rand: r, tokens: map[string]Identity{}}
}

func (t *TokenRegistry) Mint(id Identity) (string, error) {
	var b [24]byte
	if _, err := io.ReadFull(t.rand, b[:]); err != nil {
		return "", err
	}
	tok := "plg-" + hex.EncodeToString(b[:])
	t.mu.Lock()
	t.tokens[tok] = id
	t.mu.Unlock()
	return tok, nil
}

func (t *TokenRegistry) Resolve(token string) (Identity, bool) {
	t.mu.RLock()
	id, ok := t.tokens[token]
	t.mu.RUnlock()
	return id, ok
}

func (t *TokenRegistry) Revoke(token string) {
	t.mu.Lock()
	delete(t.tokens, token)
	t.mu.Unlock()
}

func (t *TokenRegistry) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.tokens)
}
