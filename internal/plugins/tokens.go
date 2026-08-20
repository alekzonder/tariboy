package plugins

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
)

// Identity is the scoped identity a valid plugin-token resolves to (spec §13).
type Identity struct {
	Name      string
	Publish   []string // channel globs the plugin may publish to
	Subscribe []string // channel globs the plugin drains as a sink
	Provide   []string // provided channel names the plugin owns (spec §6.2)
	// Sink holds the declared sink subscribe patterns, populated ONLY for a
	// channel-sink plugin (empty for every other type). It is what the publish
	// handler matches a concrete target channel against to decide whether to seed
	// a concrete sink subscription (host startSink registers only the literal
	// concrete entries; glob entries like chat:* live here and are realized into
	// concrete subscriptions lazily as inbound flows through publish).
	Sink []string
}

// CanPublish reports whether the plugin may publish to channel. Fail-closed:
// a plugin that declared no publish patterns can publish to nothing.
func (id Identity) CanPublish(channel string) bool {
	return matchesAnyGlob(id.Publish, channel)
}

// MatchesSink reports whether channel falls within this plugin's declared sink
// patterns. Only channel-sink plugins carry Sink patterns, so a non-sink always
// returns false — the publish handler uses it to gate concrete-subscription
// seeding to sinks whose declared glob (e.g. chat:*) covers the concrete target.
func (id Identity) MatchesSink(channel string) bool {
	return matchesAnyGlob(id.Sink, channel)
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
