package aiproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type handoffState struct {
	ListenAddr string                 `json:"listen_addr"`
	Tokens     map[string]Attribution `json:"tokens"`
}

// OpenTokenRegistry loads a registry whose active leases and listener address
// survive a short daemon restart. A missing file is an empty first-run state;
// malformed state fails closed so live harnesses are never silently orphaned.
func OpenTokenRegistry(path string, r io.Reader) (*TokenRegistry, error) {
	reg := newTokenRegistry(r)
	reg.persistPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("read AI proxy handoff state: %w", err)
	}
	var state handoffState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode AI proxy handoff state: %w", err)
	}
	if state.Tokens == nil {
		state.Tokens = map[string]Attribution{}
	}
	reg.listenAddr = state.ListenAddr
	reg.tokens = state.Tokens
	return reg, nil
}

func (t *TokenRegistry) persistLocked() error {
	if t.persistPath == "" {
		return nil
	}
	dir := filepath.Dir(t.persistPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create AI proxy handoff directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".aiproxy-handoff-*.tmp")
	if err != nil {
		return fmt.Errorf("create AI proxy handoff temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod AI proxy handoff temporary file: %w", err)
	}
	enc := json.NewEncoder(tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(handoffState{ListenAddr: t.listenAddr, Tokens: t.tokens}); err != nil {
		cleanup()
		return fmt.Errorf("encode AI proxy handoff state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync AI proxy handoff temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close AI proxy handoff temporary file: %w", err)
	}
	if err := os.Rename(tmpName, t.persistPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("publish AI proxy handoff state: %w", err)
	}
	parent, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open AI proxy handoff directory: %w", err)
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("sync AI proxy handoff directory: %w", err)
	}
	return nil
}

// ListenAddr is the concrete loopback endpoint carried across daemon restarts.
func (t *TokenRegistry) ListenAddr() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.listenAddr
}

// SetListenAddr publishes the concrete endpoint before the proxy starts serving.
func (t *TokenRegistry) SetListenAddr(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	old := t.listenAddr
	t.listenAddr = addr
	if err := t.persistLocked(); err != nil {
		t.listenAddr = old
		return err
	}
	return nil
}

// Prune removes leases that startup reconciliation proves are no longer live.
func (t *TokenRegistry) Prune(keep func(Attribution) bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	old := cloneAttributions(t.tokens)
	for token, attr := range t.tokens {
		if !keep(attr) {
			delete(t.tokens, token)
		}
	}
	if err := t.persistLocked(); err != nil {
		t.tokens = old
		return err
	}
	return nil
}

func cloneAttributions(in map[string]Attribution) map[string]Attribution {
	out := make(map[string]Attribution, len(in))
	for token, attr := range in {
		out[token] = attr
	}
	return out
}
