package aiproxy

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
)

// Attribution is the identity a valid proxy token resolves to (spec §9).
type Attribution struct {
	Agent       string
	Iteration   string
	ImageName   string
	ImageTag    string
	ImageDigest string
	// TaskID/EpicID are the native task/root the iteration is currently working
	// on (epic dev-t-3e1 §2). Set live via UpdateTask; empty when untagged.
	TaskID string
	EpicID string
}

// TokenRegistry maps short-lived bearer tokens to attribution. Registries made
// with NewTokenRegistry are memory-only; OpenTokenRegistry persists active
// leases for daemon restart handoff. Tokens are minted at iteration start and
// revoked at terminal completion.
type TokenRegistry struct {
	mu          sync.RWMutex
	rand        io.Reader
	tokens      map[string]Attribution
	persistPath string
	listenAddr  string
}

func NewTokenRegistry(r io.Reader) *TokenRegistry {
	return newTokenRegistry(r)
}

func newTokenRegistry(r io.Reader) *TokenRegistry {
	if r == nil {
		r = rand.Reader
	}
	return &TokenRegistry{rand: r, tokens: map[string]Attribution{}}
}

func (t *TokenRegistry) Mint(a Attribution) (string, error) {
	var b [24]byte
	if _, err := io.ReadFull(t.rand, b[:]); err != nil {
		return "", err
	}
	tok := "sk-tariboy-" + hex.EncodeToString(b[:])
	t.mu.Lock()
	t.tokens[tok] = a
	if err := t.persistLocked(); err != nil {
		delete(t.tokens, tok)
		t.mu.Unlock()
		return "", err
	}
	t.mu.Unlock()
	return tok, nil
}

func (t *TokenRegistry) Resolve(token string) (Attribution, bool) {
	t.mu.RLock()
	a, ok := t.tokens[token]
	t.mu.RUnlock()
	return a, ok
}

// UpdateTask sets the native task/root attribution on the live token(s) matching
// key, which may be either the token string itself or an iteration id. Empty
// taskID/epicID clear the tags. The mutation happens under the same write lock
// as Mint/Revoke, so it is safe against concurrent Resolve. Unknown keys are a
// no-op. Returns the number of tokens updated.
func (t *TokenRegistry) UpdateTask(key, taskID, epicID string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	old := cloneAttributions(t.tokens)
	n := 0
	if a, ok := t.tokens[key]; ok {
		a.TaskID, a.EpicID = taskID, epicID
		t.tokens[key] = a
		n++
		if err := t.persistLocked(); err != nil {
			t.tokens = old
			return 0
		}
		return 1
	}
	for tok, a := range t.tokens {
		if a.Iteration == key {
			a.TaskID, a.EpicID = taskID, epicID
			t.tokens[tok] = a
			n++
		}
	}
	if n > 0 {
		if err := t.persistLocked(); err != nil {
			t.tokens = old
			return 0
		}
	}
	return n
}

func (t *TokenRegistry) Revoke(token string) {
	t.mu.Lock()
	delete(t.tokens, token)
	_ = t.persistLocked()
	t.mu.Unlock()
}

// RevokeIteration removes every lease that attributes requests to iteration.
func (t *TokenRegistry) RevokeIteration(iteration string) {
	t.mu.Lock()
	for token, attr := range t.tokens {
		if attr.Iteration == iteration {
			delete(t.tokens, token)
		}
	}
	_ = t.persistLocked()
	t.mu.Unlock()
}

func (t *TokenRegistry) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.tokens)
}
