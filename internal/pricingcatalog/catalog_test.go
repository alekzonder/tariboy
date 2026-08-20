package pricingcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/store"
)

func TestParseCatalogConvertsPricesAndAliases(t *testing.T) {
	got := parseValid(t, catalogFixture(map[string]any{
		"gpt-4o": map[string]any{
			"input_cost_per_token":        2.5e-6,
			"output_cost_per_token":       1e-5,
			"cache_read_input_token_cost": 1.25e-6,
			"aliases":                     []string{"openai/gpt-4o"},
			"litellm_provider":            "openai",
		},
	}))
	want := aiproxy.ModelPrice{
		InputPerMtok:      2.5,
		OutputPerMtok:     10,
		CacheWritePerMtok: 2.5,
		CacheReadPerMtok:  1.25,
	}
	if got["gpt-4o"] != want {
		t.Fatalf("gpt-4o = %+v, want %+v", got["gpt-4o"], want)
	}
	if got["openai/gpt-4o"] != got["gpt-4o"] {
		t.Fatal("alias does not resolve to the source model price")
	}
	if _, ok := got["openai/openai/gpt-4o"]; ok {
		t.Fatal("litellm_provider unexpectedly changed the source model key")
	}
}

func TestParseCatalogUsesExplicitCachePrices(t *testing.T) {
	got := parseValid(t, catalogFixture(map[string]any{
		"gpt-4o": map[string]any{
			"input_cost_per_token":            2.5e-6,
			"output_cost_per_token":           1e-5,
			"cache_creation_input_token_cost": 3.125e-6,
			"cache_read_input_token_cost":     1.25e-6,
		},
	}))
	if got["gpt-4o"].CacheWritePerMtok != 3.125 {
		t.Fatalf("cache write price = %v, want 3.125", got["gpt-4o"].CacheWritePerMtok)
	}
	if got["gpt-4o"].CacheReadPerMtok != 1.25 {
		t.Fatalf("cache read price = %v, want 1.25", got["gpt-4o"].CacheReadPerMtok)
	}
}

func TestValidateCatalogRejectsInvalidDocuments(t *testing.T) {
	valid := catalogFixture(nil)
	var missingControl map[string]any
	if err := json.Unmarshal(valid, &missingControl); err != nil {
		t.Fatal(err)
	}
	delete(missingControl, "claude-opus-4-8")

	tests := []struct {
		name string
		data []byte
	}{
		{name: "malformed JSON", data: []byte(`{"gpt-4o":`)},
		{name: "NaN", data: prependRawEntry(valid, `"invalid-number":{"input_cost_per_token":NaN}`)},
		{name: "overflow", data: prependRawEntry(valid, `"invalid-number":{"input_cost_per_token":1e309}`)},
		{name: "per-million overflow", data: replacePrice(t, valid, "gpt-4o", "input_cost_per_token", 1e308)},
		{name: "negative", data: replacePrice(t, valid, "gpt-4o", "input_cost_per_token", -1e-6)},
		{name: "negative signed zero", data: prependRawEntry(valid, `"negative-zero":{"input_cost_per_token":-0}`)},
		{name: "negative underflow", data: prependRawEntry(valid, `"negative-underflow":{"input_cost_per_token":-1e-1000}`)},
		{name: "undersized", data: mustJSON(t, map[string]any{
			"gpt-4o":            pricedEntry(1e-6),
			"claude-sonnet-4-6": pricedEntry(1e-6),
			"claude-opus-4-8":   pricedEntry(1e-6),
		})},
		{name: "missing control", data: mustJSON(t, missingControl)},
		{name: "control without usable prices", data: replaceEntry(t, valid, "gpt-4o", map[string]any{"max_tokens": 128_000})},
		{name: "ambiguous alias", data: replaceEntry(t, valid, "model-000", map[string]any{
			"input_cost_per_token": 9e-6,
			"aliases":              []string{"gpt-4o"},
		})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseCatalog(tc.data); err == nil {
				t.Fatal("parseCatalog accepted invalid document")
			}
		})
	}
}

func TestParseCatalogSkipsEntriesWithoutUsablePrices(t *testing.T) {
	got := parseValid(t, catalogFixture(map[string]any{
		"metadata-only": map[string]any{"max_tokens": 128_000},
	}))
	if _, ok := got["metadata-only"]; ok {
		t.Fatal("entry without input or output pricing was published")
	}
}

func TestRefreshPublishesValidCatalogAndOwnerOnlyCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "model-prices-litellm.json")
	if err := os.WriteFile(cachePath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := openStore(t)
	pricing := aiproxy.NewPricing(map[string]aiproxy.ModelPrice{
		"gpt-4o": {InputPerMtok: 1},
	}, nil, nil)
	var diagnostics []Diagnostic
	c := New(Config{
		HTTPClient: clientFunc(func(req *http.Request) (*http.Response, error) {
			if got, want := req.URL.String(), "https://catalog.test/prices.json"; got != want {
				t.Fatalf("request URL = %q, want %q", got, want)
			}
			return httpResponse(http.StatusOK, catalogFixture(nil)), nil
		}),
		SourceURL: "https://catalog.test/prices.json",
		CachePath: cachePath,
		Store:     s,
		Pricing:   pricing,
		Diagnostic: func(d Diagnostic) {
			diagnostics = append(diagnostics, d)
		},
	})

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("cache mode = %o, want 600", got)
	}
	assertInputPrice(t, pricing, "gpt-4o", 2.5)
	var input, output float64
	if err := s.DB.QueryRow(`SELECT input_per_mtok, output_per_mtok FROM ai_pricing WHERE model='gpt-4o' AND source='litellm'`).Scan(&input, &output); err != nil {
		t.Fatal(err)
	}
	if input != 2.5 || output != 12.5 {
		t.Fatalf("stored gpt-4o prices = (%v, %v), want (2.5, 12.5)", input, output)
	}
	if len(diagnostics) != 1 || diagnostics[0].Kind != DiagnosticRefreshed || diagnostics[0].AcceptedModels < 103 {
		t.Fatalf("diagnostics = %+v, want one refresh success", diagnostics)
	}
}

func TestRefreshRejectsHTTPAndResponseFailures(t *testing.T) {
	tests := []struct {
		name   string
		client HTTPClient
	}{
		{
			name: "non-success status",
			client: clientFunc(func(*http.Request) (*http.Response, error) {
				return httpResponse(http.StatusBadGateway, []byte("upstream unavailable")), nil
			}),
		},
		{
			name: "timeout",
			client: clientFunc(func(*http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			}),
		},
		{
			name: "oversized response",
			client: clientFunc(func(*http.Request) (*http.Response, error) {
				return httpResponse(http.StatusOK, []byte(strings.Repeat("x", MaxDocumentBytes+1))), nil
			}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := New(Config{
				HTTPClient: tc.client,
				SourceURL:  "https://catalog.test/prices.json",
				CachePath:  filepath.Join(t.TempDir(), "prices.json"),
				Store:      openStore(t),
				Pricing:    aiproxy.NewPricing(nil, nil, nil),
			})
			if err := c.Refresh(context.Background()); err == nil {
				t.Fatal("Refresh accepted failed response")
			}
			if _, err := os.Stat(c.cachePath); !os.IsNotExist(err) {
				t.Fatalf("cache exists after failed refresh: %v", err)
			}
		})
	}
}

func TestRefreshBoundsRequestByTenSecondTimeout(t *testing.T) {
	c := New(Config{
		HTTPClient: clientFunc(func(req *http.Request) (*http.Response, error) {
			deadline, ok := req.Context().Deadline()
			if !ok {
				t.Fatal("request context has no deadline")
			}
			remaining := time.Until(deadline)
			if remaining < 9*time.Second || remaining > 10*time.Second {
				t.Fatalf("request deadline remaining = %v, want approximately 10s", remaining)
			}
			return nil, context.DeadlineExceeded
		}),
		SourceURL: "https://catalog.test/prices.json",
		CachePath: filepath.Join(t.TempDir(), "prices.json"),
		Store:     openStore(t),
		Pricing:   aiproxy.NewPricing(nil, nil, nil),
	})
	if err := c.Refresh(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Refresh error = %v, want context deadline exceeded", err)
	}
}

func TestRefreshInvalidCandidateRetainsLastKnownGood(t *testing.T) {
	s := openStore(t)
	old := map[string]aiproxy.ModelPrice{"gpt-4o": {InputPerMtok: 7}}
	if err := aiproxy.ReplaceLiteLLMPrices(s, old); err != nil {
		t.Fatal(err)
	}
	pricing := aiproxy.NewPricing(nil, old, nil)
	cachePath := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(cachePath, []byte("last known good"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New(Config{
		HTTPClient: clientFunc(func(*http.Request) (*http.Response, error) {
			return httpResponse(http.StatusOK, []byte(`{"gpt-4o":`)), nil
		}),
		SourceURL: "https://catalog.test/prices.json",
		CachePath: cachePath,
		Store:     s,
		Pricing:   pricing,
	})
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh accepted malformed candidate")
	}
	assertInputPrice(t, pricing, "gpt-4o", 7)
	var stored float64
	if err := s.DB.QueryRow(`SELECT input_per_mtok FROM ai_pricing WHERE model='gpt-4o' AND source='litellm'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 7 {
		t.Fatalf("stored price = %v, want 7", stored)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "last known good" {
		t.Fatalf("cache changed to %q", data)
	}
}

func TestCacheLoadPublishesValidatedCatalog(t *testing.T) {
	s := openStore(t)
	pricing := aiproxy.NewPricing(map[string]aiproxy.ModelPrice{"gpt-4o": {InputPerMtok: 1}}, nil, nil)
	cachePath := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(cachePath, catalogFixture(nil), 0o600); err != nil {
		t.Fatal(err)
	}
	var diagnostic Diagnostic
	c := New(Config{
		CachePath: cachePath,
		Store:     s,
		Pricing:   pricing,
		Diagnostic: func(d Diagnostic) {
			diagnostic = d
		},
	})
	if err := c.LoadCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertInputPrice(t, pricing, "gpt-4o", 2.5)
	if diagnostic.Kind != DiagnosticLoaded || diagnostic.AcceptedModels < 103 {
		t.Fatalf("diagnostic = %+v, want cache loaded", diagnostic)
	}
}

func TestCacheLoadMissingIsNoOp(t *testing.T) {
	pricing := aiproxy.NewPricing(map[string]aiproxy.ModelPrice{"gpt-4o": {InputPerMtok: 1}}, nil, nil)
	c := New(Config{
		CachePath: filepath.Join(t.TempDir(), "missing.json"),
		Store:     openStore(t),
		Pricing:   pricing,
	})
	if err := c.LoadCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertInputPrice(t, pricing, "gpt-4o", 1)
}

func TestRefreshDatabaseFailureKeepsRuntimeAndLeavesReconcileableCache(t *testing.T) {
	s := openStore(t)
	pricing := aiproxy.NewPricing(nil, map[string]aiproxy.ModelPrice{"gpt-4o": {InputPerMtok: 7}}, nil)
	cachePath := filepath.Join(t.TempDir(), "prices.json")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	c := New(Config{
		HTTPClient: clientFunc(func(*http.Request) (*http.Response, error) {
			return httpResponse(http.StatusOK, catalogFixture(nil)), nil
		}),
		SourceURL: "https://catalog.test/prices.json",
		CachePath: cachePath,
		Store:     s,
		Pricing:   pricing,
	})
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh succeeded with closed database")
	}
	assertInputPrice(t, pricing, "gpt-4o", 7)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseCatalog(data); err != nil {
		t.Fatalf("renamed cache is not valid for startup reconciliation: %v", err)
	}
}

func TestRunRefreshesStaleCacheAtStartup(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	cachePath := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(cachePath, catalogFixture(nil), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := now.Add(-RefreshInterval)
	if err := os.Chtimes(cachePath, stale, stale); err != nil {
		t.Fatal(err)
	}
	called := make(chan struct{}, 1)
	timer := newManualAfter()
	c := New(Config{
		HTTPClient: clientFunc(func(*http.Request) (*http.Response, error) {
			called <- struct{}{}
			return httpResponse(http.StatusOK, catalogFixture(nil)), nil
		}),
		Clock:     func() time.Time { return now },
		After:     timer.After,
		SourceURL: "https://catalog.test/prices.json",
		CachePath: cachePath,
		Store:     openStore(t),
		Pricing:   aiproxy.NewPricing(nil, nil, nil),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()
	waitSignal(t, called, "startup refresh")
	if got := waitDuration(t, timer.calls); got != RefreshInterval {
		t.Fatalf("timer duration = %v, want %v", got, RefreshInterval)
	}
	cancel()
	waitSignal(t, done, "Run cancellation")
}

func TestRunRepeatsEveryTwentyFourHours(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	cachePath := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(cachePath, catalogFixture(nil), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(cachePath, now, now); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := 0
	called := make(chan struct{}, 2)
	timer := newManualAfter()
	c := New(Config{
		HTTPClient: clientFunc(func(*http.Request) (*http.Response, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			called <- struct{}{}
			return httpResponse(http.StatusOK, catalogFixture(nil)), nil
		}),
		Clock:     func() time.Time { return now },
		After:     timer.After,
		SourceURL: "https://catalog.test/prices.json",
		CachePath: cachePath,
		Store:     openStore(t),
		Pricing:   aiproxy.NewPricing(nil, nil, nil),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()
	for i := 0; i < 2; i++ {
		if got := waitDuration(t, timer.calls); got != RefreshInterval {
			t.Fatalf("timer %d duration = %v, want %v", i, got, RefreshInterval)
		}
		timer.ticks <- now.Add(time.Duration(i+1) * RefreshInterval)
		waitSignal(t, called, fmt.Sprintf("refresh %d", i+1))
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 2 {
		t.Fatalf("refresh calls = %d, want 2", gotCalls)
	}
	cancel()
	waitSignal(t, done, "Run cancellation")
}

func TestRunCancellationStopsWaitingWorker(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	cachePath := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(cachePath, catalogFixture(nil), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(cachePath, now, now); err != nil {
		t.Fatal(err)
	}
	timer := newManualAfter()
	c := New(Config{
		HTTPClient: clientFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("fresh startup unexpectedly refreshed")
			return nil, errors.New("unexpected refresh")
		}),
		Clock:     func() time.Time { return now },
		After:     timer.After,
		SourceURL: "https://catalog.test/prices.json",
		CachePath: cachePath,
		Store:     openStore(t),
		Pricing:   aiproxy.NewPricing(nil, nil, nil),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()
	waitDuration(t, timer.calls)
	cancel()
	waitSignal(t, done, "Run cancellation")
}

type clientFunc func(*http.Request) (*http.Response, error)

func (f clientFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func httpResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Header:     make(http.Header),
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func assertInputPrice(t *testing.T, pricing *aiproxy.Pricing, model string, want float64) {
	t.Helper()
	got, ok := pricing.Lookup(model)
	if !ok {
		t.Fatalf("price %q is missing", model)
	}
	if got.InputPerMtok != want {
		t.Fatalf("%s input price = %v, want %v", model, got.InputPerMtok, want)
	}
}

type manualAfter struct {
	calls chan time.Duration
	ticks chan time.Time
}

func newManualAfter() *manualAfter {
	return &manualAfter{
		calls: make(chan time.Duration, 4),
		ticks: make(chan time.Time, 4),
	}
}

func (m *manualAfter) After(d time.Duration) <-chan time.Time {
	m.calls <- d
	return m.ticks
}

func waitDuration(t *testing.T, ch <-chan time.Duration) time.Duration {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for timer")
		return 0
	}
}

func waitSignal(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func parseValid(t *testing.T, data []byte) map[string]aiproxy.ModelPrice {
	t.Helper()
	got, err := parseCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func catalogFixture(overrides map[string]any) []byte {
	entries := make(map[string]any, 103+len(overrides))
	for i := 0; i < 100; i++ {
		entries[fmt.Sprintf("model-%03d", i)] = pricedEntry(float64(i+1) * 1e-7)
	}
	entries["gpt-4o"] = pricedEntry(2.5e-6)
	entries["claude-sonnet-4-6"] = pricedEntry(3e-6)
	entries["claude-opus-4-8"] = pricedEntry(5e-6)
	for model, entry := range overrides {
		entries[model] = entry
	}
	data, err := json.Marshal(entries)
	if err != nil {
		panic(err)
	}
	return data
}

func pricedEntry(input float64) map[string]any {
	return map[string]any{
		"input_cost_per_token":  input,
		"output_cost_per_token": input * 5,
	}
}

func replacePrice(t *testing.T, data []byte, model, field string, value any) []byte {
	t.Helper()
	var entries map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	entry := entries[model].(map[string]any)
	entry[field] = value
	return mustJSON(t, entries)
}

func replaceEntry(t *testing.T, data []byte, model string, entry any) []byte {
	t.Helper()
	var entries map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	entries[model] = entry
	return mustJSON(t, entries)
}

func prependRawEntry(data []byte, entry string) []byte {
	return append([]byte("{"+entry+","), data[1:]...)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
