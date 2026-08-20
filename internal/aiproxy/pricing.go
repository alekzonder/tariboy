package aiproxy

import (
	"sync"

	"github.com/alekzonder/tariboy/internal/store"
)

// Usage is the token 4-tuple parsed from a provider response (spec §9).
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheWriteTokens int
	CacheReadTokens  int
}

// Normalized returns a non-negative token tuple. Provider parsers make the
// input and cache buckets mutually exclusive before constructing Usage.
func (u Usage) Normalized() Usage {
	if u.InputTokens < 0 {
		u.InputTokens = 0
	}
	if u.OutputTokens < 0 {
		u.OutputTokens = 0
	}
	if u.CacheWriteTokens < 0 {
		u.CacheWriteTokens = 0
	}
	if u.CacheReadTokens < 0 {
		u.CacheReadTokens = 0
	}
	return u
}

// ModelPrice is a model's price per million tokens for each token bucket.
type ModelPrice struct {
	InputPerMtok      float64
	OutputPerMtok     float64
	CacheWritePerMtok float64
	CacheReadPerMtok  float64
}

type Pricing struct {
	mu       sync.RWMutex
	fallback map[string]ModelPrice
	litellm  map[string]ModelPrice
	manual   map[string]ModelPrice
	table    map[string]ModelPrice
}

// NewPricing resolves manual, LiteLLM, and built-in fallback prices in that
// order. Every input map is copied so a caller cannot mutate a published
// pricing generation.
func NewPricing(fallback, litellm, manual map[string]ModelPrice) *Pricing {
	p := &Pricing{
		fallback: clonePrices(fallback),
		litellm:  clonePrices(litellm),
		manual:   clonePrices(manual),
	}
	p.table = resolvePrices(p.fallback, p.litellm, p.manual)
	return p
}

// Price returns tokens/1e6 * price, summed over the four buckets. Its boolean
// reports whether a resolved price was available for the model.
func (p *Pricing) Price(model string, u Usage) (float64, bool) {
	p.mu.RLock()
	mp, ok := p.table[model]
	p.mu.RUnlock()
	if !ok {
		return 0, false
	}
	u = u.Normalized()
	const m = 1_000_000.0
	return float64(u.InputTokens)*mp.InputPerMtok/m +
		float64(u.OutputTokens)*mp.OutputPerMtok/m +
		float64(u.CacheWriteTokens)*mp.CacheWritePerMtok/m +
		float64(u.CacheReadTokens)*mp.CacheReadPerMtok/m, true
}

// Cost is a compatibility wrapper around Price. Unknown model -> 0.
func (p *Pricing) Cost(model string, u Usage) float64 {
	cost, _ := p.Price(model, u)
	return cost
}

// Lookup returns the resolved price for one model.
func (p *Pricing) Lookup(model string) (ModelPrice, bool) {
	p.mu.RLock()
	mp, ok := p.table[model]
	p.mu.RUnlock()
	return mp, ok
}

// Replace publishes a complete LiteLLM pricing generation.
func (p *Pricing) Replace(litellm map[string]ModelPrice) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fallback == nil && p.litellm == nil && p.manual == nil {
		p.fallback = clonePrices(p.table)
	}
	p.litellm = clonePrices(litellm)
	p.table = resolvePrices(p.fallback, p.litellm, p.manual)
}

func (p *Pricing) Set(model string, mp ModelPrice) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fallback == nil && p.litellm == nil && p.manual == nil {
		p.fallback = clonePrices(p.table)
	}
	if p.manual == nil {
		p.manual = map[string]ModelPrice{}
	}
	p.manual[model] = mp
	p.table = resolvePrices(p.fallback, p.litellm, p.manual)
}

// DefaultPricing provides a few built-in fallback model prices (cache-write
// ~1.25x input, cache-read ~0.1x input).
func DefaultPricing() map[string]ModelPrice {
	return map[string]ModelPrice{
		"claude-opus-4-8":   {InputPerMtok: 5, OutputPerMtok: 25, CacheWritePerMtok: 6.25, CacheReadPerMtok: 0.5},
		"claude-sonnet-4-6": {InputPerMtok: 3, OutputPerMtok: 15, CacheWritePerMtok: 3.75, CacheReadPerMtok: 0.30},
		"claude-haiku-4-5":  {InputPerMtok: 1, OutputPerMtok: 5, CacheWritePerMtok: 1.25, CacheReadPerMtok: 0.10},
		"gpt-4o":            {InputPerMtok: 2.5, OutputPerMtok: 10, CacheReadPerMtok: 1.25},
	}
}

// SeedDefaults is retained for compatibility. Built-in prices are runtime
// fallback values and must not be persisted as manual overrides.
func SeedDefaults(s *store.Store) error {
	return nil
}

// ReplaceLiteLLMPrices atomically replaces only LiteLLM-managed pricing rows.
func ReplaceLiteLLMPrices(s *store.Store, prices map[string]ModelPrice) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM ai_pricing WHERE source='litellm'`); err != nil {
		return err
	}
	for model, mp := range prices {
		if _, err := tx.Exec(`INSERT INTO ai_pricing(
			model, source, input_per_mtok, output_per_mtok, cache_write_per_mtok, cache_read_per_mtok
		) VALUES (?, 'litellm', ?, ?, ?, ?)`,
			model, mp.InputPerMtok, mp.OutputPerMtok, mp.CacheWritePerMtok, mp.CacheReadPerMtok); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadPricing loads source-specific database rows over the provided built-in
// fallback prices.
func LoadPricing(s *store.Store, fallback map[string]ModelPrice) (*Pricing, error) {
	rows, err := s.DB.Query(`SELECT model, source, input_per_mtok, output_per_mtok,
		cache_write_per_mtok, cache_read_per_mtok FROM ai_pricing`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	litellm := map[string]ModelPrice{}
	manual := map[string]ModelPrice{}
	for rows.Next() {
		var model, source string
		var mp ModelPrice
		if err := rows.Scan(&model, &source, &mp.InputPerMtok, &mp.OutputPerMtok,
			&mp.CacheWritePerMtok, &mp.CacheReadPerMtok); err != nil {
			return nil, err
		}
		switch source {
		case "manual":
			manual[model] = mp
		case "litellm":
			litellm[model] = mp
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewPricing(fallback, litellm, manual), nil
}

func clonePrices(prices map[string]ModelPrice) map[string]ModelPrice {
	copy := make(map[string]ModelPrice, len(prices))
	for model, price := range prices {
		copy[model] = price
	}
	return copy
}

func resolvePrices(fallback, litellm, manual map[string]ModelPrice) map[string]ModelPrice {
	resolved := clonePrices(fallback)
	for model, price := range litellm {
		resolved[model] = price
	}
	for model, price := range manual {
		resolved[model] = price
	}
	return resolved
}
