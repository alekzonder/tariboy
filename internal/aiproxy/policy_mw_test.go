package aiproxy

import (
	"net/http/httptest"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
)

func newPolicyProxy(cache *PolicyCache) *Proxy {
	return &Proxy{cfg: Config{Policy: cache, Audit: func(string, string, string) {}}}
}

func TestPolicyMiddlewareDeny(t *testing.T) {
	_, ai := newAIStore(t)
	ai.SetRule(PolicyRule{ID: "d", Priority: 1, Scope: "agent:bob", Kind: "model-policy",
		Deny: []string{"claude-opus-*"}, Enabled: true})
	cache := NewPolicyCache(ai, policyClock())
	cache.Refresh()
	p := newPolicyProxy(cache)
	rec := httptest.NewRecorder()
	ex := &Exchange{W: rec, Attr: Attribution{Agent: "bob"}, ReqBody: []byte(`{"model":"claude-opus-4-8"}`)}
	called := false
	h := p.policy(func(*Exchange) error { called = true; return nil })
	if err := h(ex); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 403 || called {
		t.Fatalf("expected 403 short-circuit, code=%d called=%v", rec.Code, called)
	}
	if ex.Status != "model_denied" {
		t.Fatalf("status = %q", ex.Status)
	}
}

func TestPolicyMiddlewareRoute(t *testing.T) {
	_, ai := newAIStore(t)
	ai.SetRule(PolicyRule{ID: "r", Priority: 1, Scope: "agent:dave", Kind: "model-policy",
		Route: "claude-sonnet-4", Enabled: true})
	cache := NewPolicyCache(ai, policyClock())
	cache.Refresh()
	p := newPolicyProxy(cache)
	rec := httptest.NewRecorder()
	ex := &Exchange{W: rec, Attr: Attribution{Agent: "dave"}, ReqBody: []byte(`{"model":"claude-opus-4-8","max_tokens":16}`)}
	var forwarded []byte
	h := p.policy(func(e *Exchange) error { forwarded = e.ReqBody; return nil })
	if err := h(ex); err != nil {
		t.Fatal(err)
	}
	if !containsJSON(forwarded, `"model":"claude-sonnet-4"`) || !containsJSON(forwarded, `"max_tokens":16`) {
		t.Fatalf("route rewrite failed: %s", forwarded)
	}
}

func TestPolicyMiddlewareRateLimit(t *testing.T) {
	base, ai := newAIStore(t)
	_ = agent.NewStore(base)
	seedReq(t, ai, "rlbot", 1)
	ai.SetRule(PolicyRule{ID: "rl", Priority: 1, Scope: "agent:rlbot", Kind: "rate-limit",
		MaxRequests: 1, WindowS: 86400, Enabled: true})
	cache := NewPolicyCache(ai, policyClock())
	cache.Refresh()
	p := newPolicyProxy(cache)
	rec := httptest.NewRecorder()
	ex := &Exchange{W: rec, Attr: Attribution{Agent: "rlbot"}, ReqBody: []byte(`{"model":"claude-opus-4-8"}`)}
	called := false
	h := p.policy(func(*Exchange) error { called = true; return nil })
	if err := h(ex); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 429 || called || ex.Status != "rate_limited" {
		t.Fatalf("expected 429 short-circuit, code=%d called=%v status=%q", rec.Code, called, ex.Status)
	}
}

// When a request is both denied by model-policy AND over a rate-limit window,
// Deny (403) takes precedence over RateLimited (429).
func TestPolicyMiddlewareDenyBeatsRateLimit(t *testing.T) {
	_, ai := newAIStore(t)
	seedReq(t, ai, "both", 1)
	ai.SetRule(PolicyRule{ID: "d", Priority: 1, Scope: "agent:both", Kind: "model-policy",
		Deny: []string{"claude-opus-*"}, Enabled: true})
	ai.SetRule(PolicyRule{ID: "rl", Priority: 1, Scope: "agent:both", Kind: "rate-limit",
		MaxRequests: 1, WindowS: 86400, Enabled: true})
	cache := NewPolicyCache(ai, policyClock())
	cache.Refresh()
	p := newPolicyProxy(cache)
	rec := httptest.NewRecorder()
	ex := &Exchange{W: rec, Attr: Attribution{Agent: "both"}, ReqBody: []byte(`{"model":"claude-opus-4-8"}`)}
	called := false
	h := p.policy(func(*Exchange) error { called = true; return nil })
	if err := h(ex); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 403 || called || ex.Status != "model_denied" {
		t.Fatalf("expected 403 (deny precedence), code=%d called=%v status=%q", rec.Code, called, ex.Status)
	}
}

// No matching rule ⇒ the request is forwarded unchanged.
func TestPolicyMiddlewareNoDecisionForwards(t *testing.T) {
	_, ai := newAIStore(t)
	cache := NewPolicyCache(ai, policyClock())
	cache.Refresh()
	p := newPolicyProxy(cache)
	rec := httptest.NewRecorder()
	ex := &Exchange{W: rec, Attr: Attribution{Agent: "free"}, ReqBody: []byte(`{"model":"claude-opus-4-8"}`)}
	var forwarded []byte
	h := p.policy(func(e *Exchange) error { forwarded = e.ReqBody; return nil })
	if err := h(ex); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !containsJSON(forwarded, `"model":"claude-opus-4-8"`) {
		t.Fatalf("expected pass-through, code=%d forwarded=%s", rec.Code, forwarded)
	}
}

// A nil policy engine disables the middleware entirely: middlewares() must not
// append policy, so the request is allowed (fail-safe default, no fail-open).
func TestPolicyMiddlewareNilEngineAllowed(t *testing.T) {
	p := &Proxy{cfg: Config{Tokens: NewTokenRegistry(nil)}}
	for _, mw := range p.middlewares() {
		if mw == nil {
			t.Fatal("nil middleware in chain")
		}
	}
	// With Policy nil, the chain is auth+route guard+record (no policy
	// link). Assert the count matches the no-budget/no-policy baseline.
	if got := len(p.middlewares()); got != 3 {
		t.Fatalf("expected 3 baseline middlewares with nil Policy, got %d", got)
	}
}
