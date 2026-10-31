package aiproxy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
)

// ProviderRoute binds an OpenCode provider to its original upstream.
type ProviderRoute struct {
	BaseURL  string
	Provider string
}

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
	Routes map[string]ProviderRoute `json:",omitempty"`
}

func validProviderRoutes(routes map[string]ProviderRoute) bool {
	if routes != nil && len(routes) == 0 {
		return false
	}
	for id, route := range routes {
		if id == "" || strings.Trim(id, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
			return false
		}
		u, err := url.Parse(route.BaseURL)
		if err != nil || u.Hostname() == "" || u.User != nil || strings.ContainsAny(route.BaseURL, "?#") ||
			(u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && !isSafeEscapedRoutePath(u.Path, u.EscapedPath())) ||
			(route.Provider != string(ProviderOpenAI) && route.Provider != string(ProviderAnthropic)) {
			return false
		}
	}
	return true
}

// RegisterProviderRoutes publishes an iteration's routes together with its lease.
func (t *TokenRegistry) RegisterProviderRoutes(token string, routes map[string]ProviderRoute) error {
	if len(routes) == 0 || !validProviderRoutes(routes) {
		return errors.New("invalid provider routes")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.tokens[token]
	if !ok {
		return errors.New("unknown proxy token")
	}
	old := a
	a.Routes = make(map[string]ProviderRoute, len(routes))
	for id, route := range routes {
		a.Routes[id] = route
	}
	t.tokens[token] = a
	if err := t.persistLocked(); err != nil {
		t.tokens[token] = old
		return err
	}
	return nil
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
	if !validProviderRoutes(a.Routes) {
		return "", errors.New("invalid provider routes")
	}
	a.Routes = copyProviderRoutes(a.Routes)
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
	a.Routes = copyProviderRoutes(a.Routes)
	t.mu.RUnlock()
	return a, ok
}

func copyProviderRoutes(routes map[string]ProviderRoute) map[string]ProviderRoute {
	if routes == nil {
		return nil
	}
	out := make(map[string]ProviderRoute, len(routes))
	for id, route := range routes {
		out[id] = route
	}
	return out
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

// SetTaskIfEmpty attributes matching live leases only when they are untagged
// or already carry the same task/root pair. It returns the matched count and
// whether any lease changed.
func (t *TokenRegistry) SetTaskIfEmpty(key, taskID, epicID string) (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	matched := make([]string, 0, 1)
	if _, ok := t.tokens[key]; ok {
		matched = append(matched, key)
	} else {
		for token, attr := range t.tokens {
			if attr.Iteration == key {
				matched = append(matched, token)
			}
		}
	}
	if len(matched) == 0 {
		return 0, false
	}
	for _, token := range matched {
		attr := t.tokens[token]
		if attr.TaskID != "" && (attr.TaskID != taskID || attr.EpicID != epicID) {
			return 0, false
		}
	}

	old := cloneAttributions(t.tokens)
	changed := false
	for _, token := range matched {
		attr := t.tokens[token]
		if attr.TaskID == "" {
			attr.TaskID, attr.EpicID = taskID, epicID
			t.tokens[token] = attr
			changed = true
		}
	}
	if changed {
		if err := t.persistLocked(); err != nil {
			t.tokens = old
			return 0, false
		}
	}
	return len(matched), changed
}

// Revoke removes token from memory unconditionally (fail closed) and returns
// the error from persisting the removal. A persist failure means a restart can
// bring the revoked token back from disk, so callers must report it.
func (t *TokenRegistry) Revoke(token string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tokens, token)
	return t.persistLocked()
}

// RevokeIteration removes every lease that attributes requests to iteration.
// Like Revoke, the in-memory removal is unconditional and the persist error is
// returned.
func (t *TokenRegistry) RevokeIteration(iteration string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for token, attr := range t.tokens {
		if attr.Iteration == iteration {
			delete(t.tokens, token)
		}
	}
	return t.persistLocked()
}

func (t *TokenRegistry) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.tokens)
}
