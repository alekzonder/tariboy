package aiproxy

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBudgetStoreCRUD(t *testing.T) {
	s := newStore(t)
	if err := s.SetBudget(Budget{Scope: "agent:alice", LimitUSD: 1.0, PeriodS: 3600, Mode: "block"}); err != nil {
		t.Fatal(err)
	}
	// Upsert.
	if err := s.SetBudget(Budget{Scope: "agent:alice", LimitUSD: 2.0, PeriodS: 3600, Mode: "warn"}); err != nil {
		t.Fatal(err)
	}
	b, ok, _ := s.GetBudget("agent:alice")
	if !ok || b.LimitUSD != 2.0 || b.Mode != "warn" {
		t.Fatalf("get budget = %+v ok=%v", b, ok)
	}
	list, _ := s.ListBudgets()
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}
	if err := s.DeleteBudget("agent:alice"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetBudget("agent:alice"); ok {
		t.Fatal("delete failed")
	}
}

func TestBudgetCacheCheck(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t)
	// Spend $0.80 in the last hour for alice.
	st.Insert(sampleReq("r1", "alice", "basic", 0.80, now.Add(-30*time.Minute)))
	st.SetBudget(Budget{Scope: "agent:alice", LimitUSD: 0.50, PeriodS: 3600, Mode: "block"})

	cache := NewBudgetCache(st, func() time.Time { return now })
	if err := cache.Refresh(); err != nil {
		t.Fatal(err)
	}
	d := cache.Check("alice")
	if !d.Over || d.Mode != "block" || d.Scope != "agent:alice" {
		t.Fatalf("decision = %+v", d)
	}
	// bob has no budget -> not over.
	if cache.Check("bob").Over {
		t.Fatal("bob should not be over")
	}
}

func TestBudgetBlockMiddleware(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t)
	st.Insert(sampleReq("r1", "alice", "basic", 0.80, now.Add(-time.Minute)))
	st.SetBudget(Budget{Scope: "agent:alice", LimitUSD: 0.50, PeriodS: 3600, Mode: "block"})
	cache := NewBudgetCache(st, func() time.Time { return now })
	cache.Refresh()

	p := testProxy(t)
	p.cfg.Budget = cache
	p.cfg.Store = st
	p.rebuild() // pick up the budget middleware now that a cache is present
	forwarded := false
	p.forward = func(ex *Exchange) error { forwarded = true; ex.W.WriteHeader(200); return nil }

	tok, _ := p.Mint(Attribution{Agent: "alice", Iteration: "alice-9", ImageName: "basic", ImageTag: "latest"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(`{"model":"claude-opus-4-8"}`))
	req.Header.Set("x-api-key", "real-provider-key")
	p.ServeHTTP(rr, req)

	if rr.Code != 429 {
		t.Fatalf("blocked request status = %d, want 429", rr.Code)
	}
	if forwarded {
		t.Fatal("blocked request must not reach the upstream")
	}
}

func TestBudgetWarnMiddlewareProceeds(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t)
	st.Insert(sampleReq("r1", "alice", "basic", 0.80, now.Add(-time.Minute)))
	st.SetBudget(Budget{Scope: "agent:alice", LimitUSD: 0.50, PeriodS: 3600, Mode: "warn"})
	cache := NewBudgetCache(st, func() time.Time { return now })
	cache.Refresh()

	p := testProxy(t)
	p.cfg.Budget = cache
	p.cfg.Store = st
	warned := 0
	p.cfg.Warn = func(agent string, d Decision) { warned++ }
	p.rebuild()
	forwarded := false
	p.forward = func(ex *Exchange) error { forwarded = true; ex.W.WriteHeader(200); return nil }

	tok, _ := p.Mint(Attribution{Agent: "alice", Iteration: "alice-9", ImageName: "basic", ImageTag: "latest"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(`{"model":"claude-opus-4-8"}`))
	req.Header.Set("x-api-key", "real-provider-key")
	p.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("warned request status = %d, want 200", rr.Code)
	}
	if !forwarded {
		t.Fatal("warn must proceed to the upstream")
	}
	if warned != 1 {
		t.Fatalf("warn callback called %d times, want 1", warned)
	}
}

// TestBudgetCacheRace exercises the refresher goroutine writing the cache while
// the request path reads it, so `go test -race` proves the RWMutex covers both.
func TestBudgetCacheRace(t *testing.T) {
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t)
	st.Insert(sampleReq("r1", "alice", "basic", 0.80, now.Add(-time.Minute)))
	st.SetBudget(Budget{Scope: "agent:alice", LimitUSD: 0.50, PeriodS: 3600, Mode: "block"})
	cache := NewBudgetCache(st, func() time.Time { return now })
	cache.Refresh()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = cache.Refresh()
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = cache.Check("alice")
		}
		close(stop)
	}()
	wg.Wait()
}
