package aiproxy

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/store"
)

func TestCost(t *testing.T) {
	p := &Pricing{table: map[string]ModelPrice{
		"claude-opus-4-8": {InputPerMtok: 5, OutputPerMtok: 25, CacheWritePerMtok: 6.25, CacheReadPerMtok: 0.5},
	}}
	// 1e6 input -> $5; 1e6 output -> $25; totals scale linearly.
	got := p.Cost("claude-opus-4-8", Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000,
		CacheWriteTokens: 1_000_000, CacheReadTokens: 1_000_000})
	want := 5.0 + 25.0 + 6.25 + 0.5
	if got != want {
		t.Fatalf("Cost = %v, want %v", got, want)
	}
	// Small realistic counts.
	small := p.Cost("claude-opus-4-8", Usage{InputTokens: 1000, OutputTokens: 500})
	if small != (1000*5.0/1e6 + 500*25.0/1e6) {
		t.Fatalf("small Cost = %v", small)
	}
	// Unknown model -> 0.
	if p.Cost("mystery", Usage{InputTokens: 1_000_000}) != 0 {
		t.Fatal("unknown model should cost 0")
	}
}

func TestPricingPriceReportsUnknownModel(t *testing.T) {
	p := NewPricing(nil, nil, nil)
	cost, known := p.Price("mystery", Usage{InputTokens: 1_000_000})
	if known || cost != 0 {
		t.Fatalf("unknown model Price = (%v, %v), want (0, false)", cost, known)
	}
}

func TestPricingPriceUsesCompleteGenerationDuringReplace(t *testing.T) {
	old := ModelPrice{InputPerMtok: 1, OutputPerMtok: 2, CacheWritePerMtok: 4, CacheReadPerMtok: 8}
	new := ModelPrice{InputPerMtok: 10, OutputPerMtok: 20, CacheWritePerMtok: 40, CacheReadPerMtok: 80}
	p := NewPricing(map[string]ModelPrice{"model": old}, nil, nil)
	u := Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheWriteTokens: 1_000_000, CacheReadTokens: 1_000_000}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		p.Replace(map[string]ModelPrice{"model": new})
	}()
	var cost float64
	var known bool
	go func() {
		defer wg.Done()
		<-start
		cost, known = p.Price("model", u)
	}()
	close(start)
	wg.Wait()

	if !known {
		t.Fatal("known model was not priced")
	}
	if cost != 15 && cost != 150 {
		t.Fatalf("racing Price = %v, want one complete generation (15 or 150)", cost)
	}
}

func TestLoadFallbackRoundTrip(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := SeedDefaults(s); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(s); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ai_pricing`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("SeedDefaults rows=%d err=%v, want 0", rows, err)
	}
	p, err := LoadPricing(s, DefaultPricing())
	if err != nil {
		t.Fatal(err)
	}
	if p.Cost("claude-opus-4-8", Usage{InputTokens: 1_000_000}) != 5.0 {
		t.Fatalf("fallback opus input price wrong: %v", p.Cost("claude-opus-4-8", Usage{InputTokens: 1_000_000}))
	}
	// Set overrides at runtime.
	p.Set("claude-opus-4-8", ModelPrice{InputPerMtok: 99})
	if p.Cost("claude-opus-4-8", Usage{InputTokens: 1_000_000}) != 99.0 {
		t.Fatal("Set did not override")
	}
}

func TestPricingSourcePrecedenceAndReplace(t *testing.T) {
	fallback := map[string]ModelPrice{"same": {InputPerMtok: 1}, "fallback": {InputPerMtok: 2}}
	p := NewPricing(fallback, map[string]ModelPrice{"same": {InputPerMtok: 3}, "remote": {InputPerMtok: 4}}, map[string]ModelPrice{"same": {InputPerMtok: 5}})
	assertCost(t, p, "same", 5)
	assertCost(t, p, "remote", 4)
	assertCost(t, p, "fallback", 2)
	p.Replace(map[string]ModelPrice{"remote": {InputPerMtok: 9}})
	assertCost(t, p, "remote", 9)
	assertCost(t, p, "same", 5)
}

func TestPricingLoadAndReplaceLiteLLM(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "pricing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB.Exec(`INSERT INTO ai_pricing(model, source, input_per_mtok) VALUES ('same', 'manual', 5)`); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceLiteLLMPrices(s, map[string]ModelPrice{
		"same":   {InputPerMtok: 3},
		"remote": {InputPerMtok: 4},
	}); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPricing(s, map[string]ModelPrice{"same": {InputPerMtok: 1}, "fallback": {InputPerMtok: 2}})
	if err != nil {
		t.Fatal(err)
	}
	assertCost(t, p, "same", 5)
	assertCost(t, p, "remote", 4)
	assertCost(t, p, "fallback", 2)
	if err := ReplaceLiteLLMPrices(s, map[string]ModelPrice{"remote": {InputPerMtok: 9}}); err != nil {
		t.Fatal(err)
	}
	var source string
	if err := s.DB.QueryRow(`SELECT source FROM ai_pricing WHERE model='same'`).Scan(&source); err != nil || source != "manual" {
		t.Fatalf("manual row source=%q err=%v, want manual", source, err)
	}
	p, err = LoadPricing(s, map[string]ModelPrice{"same": {InputPerMtok: 1}, "fallback": {InputPerMtok: 2}})
	if err != nil {
		t.Fatal(err)
	}
	assertCost(t, p, "remote", 9)
	assertCost(t, p, "same", 5)
}

func assertCost(t *testing.T, p *Pricing, model string, want float64) {
	t.Helper()
	if got := p.Cost(model, Usage{InputTokens: 1_000_000}); got != want {
		t.Fatalf("Cost(%q) = %v, want %v", model, got, want)
	}
}
