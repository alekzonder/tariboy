// Package pricingcatalog maintains the validated LiteLLM model price catalog.
package pricingcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/store"
)

const (
	SourceURL        = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	RefreshInterval  = 24 * time.Hour
	MaxDocumentBytes = 8 << 20

	minimumModelPrices = 100
	downloadTimeout    = 10 * time.Second
)

const (
	DiagnosticLoaded    = "pricing_catalog_loaded"
	DiagnosticRefreshed = "pricing_catalog_refreshed"
	DiagnosticError     = "pricing_catalog_error"
)

var controlModels = [...]string{
	"gpt-4o",
	"claude-sonnet-4-6",
	"claude-opus-4-8",
}

// HTTPClient is the bounded external dependency used to retrieve the catalog.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Diagnostic reports a bounded catalog lifecycle outcome without catalog data.
type Diagnostic struct {
	Kind           string
	Source         string
	At             time.Time
	AcceptedModels int
	Err            error
}

// Config contains the catalog's daemon-owned dependencies and test seams.
type Config struct {
	HTTPClient HTTPClient
	Clock      func() time.Time
	After      func(time.Duration) <-chan time.Time
	SourceURL  string
	CachePath  string
	Store      *store.Store
	Pricing    *aiproxy.Pricing
	Diagnostic func(Diagnostic)
}

// Catalog validates and publishes complete LiteLLM pricing generations.
type Catalog struct {
	httpClient HTTPClient
	clock      func() time.Time
	after      func(time.Duration) <-chan time.Time
	sourceURL  string
	cachePath  string
	store      *store.Store
	pricing    *aiproxy.Pricing
	diagnostic func(Diagnostic)
}

// New constructs a catalog. Production callers leave the network seams unset
// to use the fixed HTTPS source, ten-second timeout, and daily timer.
func New(cfg Config) *Catalog {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: downloadTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	after := cfg.After
	if after == nil {
		after = time.After
	}
	sourceURL := cfg.SourceURL
	if sourceURL == "" {
		sourceURL = SourceURL
	}
	return &Catalog{
		httpClient: client,
		clock:      clock,
		after:      after,
		sourceURL:  sourceURL,
		cachePath:  cfg.CachePath,
		store:      cfg.Store,
		pricing:    cfg.Pricing,
		diagnostic: cfg.Diagnostic,
	}
}

// LoadCache validates and publishes an existing cache. A missing cache is an
// expected first-run state and leaves the current pricing generation intact.
func (c *Catalog) LoadCache(ctx context.Context) error {
	data, err := readBoundedFile(c.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return c.failure("cache", fmt.Errorf("read LiteLLM pricing cache: %w", err))
	}
	prices, err := parseCatalog(data)
	if err != nil {
		return c.failure("cache", err)
	}
	if err := c.publish(ctx, prices); err != nil {
		return c.failure("cache", err)
	}
	c.emit(DiagnosticLoaded, "cache", len(prices), nil)
	return nil
}

// Refresh retrieves, validates, caches, and publishes one complete catalog.
func (c *Catalog) Refresh(ctx context.Context) error {
	requestCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.sourceURL, nil)
	if err != nil {
		return c.failure("download", fmt.Errorf("create LiteLLM pricing request: %w", err))
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.failure("download", fmt.Errorf("download LiteLLM pricing catalog: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return c.failure("download", fmt.Errorf("download LiteLLM pricing catalog: HTTP status %d", resp.StatusCode))
	}
	data, err := readBounded(resp.Body)
	if err != nil {
		return c.failure("download", err)
	}
	prices, err := parseCatalog(data)
	if err != nil {
		return c.failure("download", err)
	}
	if err := writeAtomicCache(c.cachePath, data); err != nil {
		return c.failure("cache", err)
	}
	if err := c.publish(ctx, prices); err != nil {
		return c.failure("database", err)
	}
	c.emit(DiagnosticRefreshed, c.sourceURL, len(prices), nil)
	return nil
}

// Run refreshes once when the cache is absent or stale, then checks every day
// until daemon cancellation.
func (c *Catalog) Run(ctx context.Context) {
	if c.cacheStale() {
		_ = c.Refresh(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.after(RefreshInterval):
			_ = c.Refresh(ctx)
		}
	}
}

func (c *Catalog) cacheStale() bool {
	info, err := os.Stat(c.cachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			c.emit(DiagnosticError, "cache", 0, fmt.Errorf("stat LiteLLM pricing cache: %w", err))
		}
		return true
	}
	return !c.clock().Before(info.ModTime().Add(RefreshInterval))
}

func (c *Catalog) publish(ctx context.Context, prices map[string]aiproxy.ModelPrice) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.store == nil {
		return fmt.Errorf("publish LiteLLM pricing catalog: store is not configured")
	}
	if c.pricing == nil {
		return fmt.Errorf("publish LiteLLM pricing catalog: runtime pricing is not configured")
	}
	if err := aiproxy.ReplaceLiteLLMPrices(c.store, prices); err != nil {
		return fmt.Errorf("publish LiteLLM pricing database rows: %w", err)
	}
	c.pricing.Replace(prices)
	return nil
}

func (c *Catalog) failure(source string, err error) error {
	c.emit(DiagnosticError, source, 0, err)
	return err
}

func (c *Catalog) emit(kind, source string, accepted int, err error) {
	if c.diagnostic != nil {
		c.diagnostic(Diagnostic{
			Kind:           kind,
			Source:         source,
			At:             c.clock(),
			AcceptedModels: accepted,
			Err:            err,
		})
	}
}

func readBoundedFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBounded(f)
}

func readBounded(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read LiteLLM pricing catalog: %w", err)
	}
	if len(data) > MaxDocumentBytes {
		return nil, fmt.Errorf("LiteLLM pricing catalog exceeds %d bytes", MaxDocumentBytes)
	}
	return data, nil
}

func writeAtomicCache(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".model-prices-*.tmp")
	if err != nil {
		return fmt.Errorf("create LiteLLM pricing cache temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod LiteLLM pricing cache temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write LiteLLM pricing cache temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync LiteLLM pricing cache temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close LiteLLM pricing cache temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("publish LiteLLM pricing cache: %w", err)
	}
	return nil
}

type catalogEntry struct {
	InputPerToken      *float64 `json:"input_cost_per_token"`
	OutputPerToken     *float64 `json:"output_cost_per_token"`
	CacheWritePerToken *float64 `json:"cache_creation_input_token_cost"`
	CacheReadPerToken  *float64 `json:"cache_read_input_token_cost"`
	Aliases            []string `json:"aliases"`
}

func parseCatalog(data []byte) (map[string]aiproxy.ModelPrice, error) {
	var rawEntries map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawEntries); err != nil {
		return nil, fmt.Errorf("decode LiteLLM pricing catalog: %w", err)
	}

	sourcePrices := make(map[string]aiproxy.ModelPrice, len(rawEntries))
	aliases := make(map[string][]string, len(rawEntries))
	for model, raw := range rawEntries {
		var entry catalogEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("decode LiteLLM pricing entry %q: %w", model, err)
		}
		if err := validatePrices(entry); err != nil {
			return nil, fmt.Errorf("validate LiteLLM pricing entry %q: %w", model, err)
		}
		if entry.InputPerToken == nil && entry.OutputPerToken == nil {
			continue
		}

		price := aiproxy.ModelPrice{
			InputPerMtok:  perMillion(entry.InputPerToken),
			OutputPerMtok: perMillion(entry.OutputPerToken),
		}
		if entry.CacheWritePerToken == nil {
			price.CacheWritePerMtok = price.InputPerMtok
		} else {
			price.CacheWritePerMtok = perMillion(entry.CacheWritePerToken)
		}
		if entry.CacheReadPerToken == nil {
			price.CacheReadPerMtok = price.InputPerMtok
		} else {
			price.CacheReadPerMtok = perMillion(entry.CacheReadPerToken)
		}
		sourcePrices[model] = price
		aliases[model] = entry.Aliases
	}

	if len(sourcePrices) < minimumModelPrices {
		return nil, fmt.Errorf("LiteLLM pricing catalog has %d usable model prices; need at least %d", len(sourcePrices), minimumModelPrices)
	}
	for _, model := range controlModels {
		if _, ok := sourcePrices[model]; !ok {
			return nil, fmt.Errorf("LiteLLM pricing catalog is missing control model %q", model)
		}
	}

	prices := make(map[string]aiproxy.ModelPrice, len(sourcePrices))
	for model, price := range sourcePrices {
		prices[model] = price
	}
	for model, modelAliases := range aliases {
		price := sourcePrices[model]
		for _, alias := range modelAliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			if existing, ok := prices[alias]; ok && existing != price {
				return nil, fmt.Errorf("LiteLLM pricing alias %q resolves to conflicting prices", alias)
			}
			prices[alias] = price
		}
	}
	return prices, nil
}

func validatePrices(entry catalogEntry) error {
	for name, value := range map[string]*float64{
		"input_cost_per_token":            entry.InputPerToken,
		"output_cost_per_token":           entry.OutputPerToken,
		"cache_creation_input_token_cost": entry.CacheWritePerToken,
		"cache_read_input_token_cost":     entry.CacheReadPerToken,
	} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || math.Signbit(*value) || *value > math.MaxFloat64/1_000_000) {
			return fmt.Errorf("%s must convert to a finite, non-negative per-million price", name)
		}
	}
	return nil
}

func perMillion(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value * 1_000_000
}
