package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/paths"
)

func TestPricingCatalogRunsWithDaemonLifecycle(t *testing.T) {
	base := t.TempDir()
	runtimeDir := t.TempDir()
	t.Setenv("TARIBOY_BASE_DIR", base)
	t.Setenv("TARIBOY_RUNTIME_DIR", runtimeDir)

	firstRelease := make(chan struct{})
	requestStarted := make(chan int, 3)
	cancelObserved := make(chan struct{})
	shutdownAcknowledged := make(chan struct{})
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	cachedCatalog := pricingCatalogFixture(t, 1.5)
	firstCatalog := pricingCatalogFixture(t, 2.5)
	secondCatalog := pricingCatalogFixture(t, 9)
	cachePath := paths.New(base).PricingCatalogFile()
	if err := os.WriteFile(cachePath, cachedCatalog, 0o600); err != nil {
		t.Fatal(err)
	}
	stale := now.Add(-24 * time.Hour)
	if err := os.Chtimes(cachePath, stale, stale); err != nil {
		t.Fatal(err)
	}
	var cancelOnce sync.Once
	var mu sync.Mutex
	requests := 0
	pricingClient := pricingHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requests++
		call := requests
		mu.Unlock()
		requestStarted <- call
		switch call {
		case 1:
			select {
			case <-firstRelease:
				return pricingResponse(firstCatalog), nil
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		case 2:
			return pricingResponse(secondCatalog), nil
		default:
			<-req.Context().Done()
			cancelOnce.Do(func() { close(cancelObserved) })
			<-shutdownAcknowledged
			return nil, req.Context().Err()
		}
	})

	timerCalls := make(chan time.Duration, 3)
	timerTicks := make(chan time.Time, 3)
	after := func(d time.Duration) <-chan time.Time {
		timerCalls <- d
		return timerTicks
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	daemonReturned := false
	opts := daemonTestOptions(Options{
		BaseDir:           base,
		Listen:            "unix",
		HTTPAddr:          "",
		LogLevel:          "error",
		PricingHTTPClient: pricingClient,
		PricingClock:      func() time.Time { return now },
		PricingAfter:      after,
		PricingSourceURL:  "https://catalog.test/prices.json",
	})
	go func() { done <- Run(ctx, opts) }()
	t.Cleanup(func() {
		if daemonReturned {
			return
		}
		cancel()
		cancelOnce.Do(func() { close(cancelObserved) })
		select {
		case <-shutdownAcknowledged:
		default:
			close(shutdownAcknowledged)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("daemon cleanup timed out")
		}
	})

	waitPricingRequest(t, requestStarted, 1)
	waitDaemonReady(t, base)
	db := openDaemonReadDB(t, filepath.Join(base, "tariboyd.db"))
	waitPricingEventCount(t, db, "pricing_catalog_loaded", 1)
	assertPricingEventIsBounded(t, db, "pricing_catalog_loaded", "cache")
	assertManagedInputPrice(t, db, 1.5)
	select {
	case err := <-done:
		daemonReturned = true
		t.Fatalf("daemon returned while initial pricing refresh was blocked: %v", err)
	default:
	}

	close(firstRelease)
	waitPricingTimer(t, timerCalls)
	waitPricingEventCount(t, db, "pricing_catalog_refreshed", 1)
	assertPricingEventIsBounded(t, db, "pricing_catalog_refreshed", "https://catalog.test/prices.json")

	timerTicks <- time.Now()
	waitPricingRequest(t, requestStarted, 2)
	waitPricingEventCount(t, db, "pricing_catalog_refreshed", 2)
	waitPricingTimer(t, timerCalls)

	timerTicks <- time.Now()
	waitPricingRequest(t, requestStarted, 3)
	cancel()
	select {
	case <-cancelObserved:
	case <-time.After(5 * time.Second):
		t.Fatal("pricing worker did not observe daemon cancellation")
	}
	select {
	case err := <-done:
		daemonReturned = true
		t.Fatalf("daemon returned before pricing worker acknowledged shutdown: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(shutdownAcknowledged)
	select {
	case err := <-done:
		daemonReturned = true
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not return after pricing worker acknowledged shutdown")
	}
	waitPricingEventCount(t, db, "pricing_catalog_error", 1)
	assertPricingErrorEventIsBounded(t, db)

	var input float64
	if err := db.QueryRow(`SELECT input_per_mtok FROM ai_pricing WHERE model='gpt-4o' AND source='litellm'`).Scan(&input); err != nil {
		t.Fatal(err)
	}
	if input != 9 {
		t.Fatalf("managed gpt-4o input price = %v, want 9 from the later tick", input)
	}
}

func waitDaemonReady(t *testing.T, base string) {
	t.Helper()
	c := client.New(paths.New(base).Socket())
	deadline := time.Now().Add(10 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		if _, err = c.Call("GET", "/api/daemon/status", nil); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("daemon never became ready: %v", err)
}

func waitPricingRequest(t *testing.T, started <-chan int, want int) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("pricing request = %d, want %d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for pricing request %d", want)
	}
}

func waitPricingTimer(t *testing.T, calls <-chan time.Duration) {
	t.Helper()
	select {
	case got := <-calls:
		if got != 24*time.Hour {
			t.Fatalf("pricing timer duration = %v, want 24h", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for pricing timer")
	}
}

func openDaemonReadDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(path)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func waitPricingEventCount(t *testing.T, db *sql.DB, kind string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var got int
		if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE kind=?`, kind).Scan(&got); err == nil && got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	var got int
	_ = db.QueryRow(`SELECT COUNT(*) FROM events WHERE kind=?`, kind).Scan(&got)
	t.Fatalf("%s event count = %d, want %d", kind, got, want)
}

func assertPricingEventIsBounded(t *testing.T, db *sql.DB, kind, wantSource string) {
	t.Helper()
	var raw string
	if err := db.QueryRow(`SELECT data FROM events WHERE kind=? ORDER BY id DESC LIMIT 1`, kind).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("decode pricing event %q: %v", raw, err)
	}
	allowed := map[string]bool{
		"source": true, "model_count": true, "generation_time": true, "error_class": true,
	}
	for key := range fields {
		if !allowed[key] {
			t.Fatalf("pricing event contains unsafe field %q: %s", key, raw)
		}
	}
	if fields["source"] != wantSource {
		t.Fatalf("pricing event source = %v", fields["source"])
	}
	if count, ok := fields["model_count"].(float64); !ok || count < 103 {
		t.Fatalf("pricing event model_count = %v", fields["model_count"])
	}
	if _, ok := fields["generation_time"].(string); !ok {
		t.Fatalf("pricing event generation_time = %v", fields["generation_time"])
	}
}

func assertManagedInputPrice(t *testing.T, db *sql.DB, want float64) {
	t.Helper()
	var got float64
	if err := db.QueryRow(`SELECT input_per_mtok FROM ai_pricing WHERE model='gpt-4o' AND source='litellm'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("managed gpt-4o input price = %v, want %v", got, want)
	}
}

func assertPricingErrorEventIsBounded(t *testing.T, db *sql.DB) {
	t.Helper()
	var raw string
	if err := db.QueryRow(`SELECT data FROM events WHERE kind='pricing_catalog_error' ORDER BY id DESC LIMIT 1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("decode pricing error event %q: %v", raw, err)
	}
	allowed := map[string]bool{
		"source": true, "model_count": true, "generation_time": true, "error_class": true,
	}
	for key := range fields {
		if !allowed[key] {
			t.Fatalf("pricing error event contains unsafe field %q: %s", key, raw)
		}
	}
	if fields["source"] != "download" {
		t.Fatalf("pricing error event source = %v, want download", fields["source"])
	}
	if fields["error_class"] != "canceled" {
		t.Fatalf("pricing error class = %v, want canceled", fields["error_class"])
	}
}

func pricingResponse(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Header:     make(http.Header),
	}
}

func pricingCatalogFixture(t *testing.T, gptInputPerMtok float64) []byte {
	t.Helper()
	entries := make(map[string]any, 103)
	for i := 0; i < 100; i++ {
		entries[fmt.Sprintf("model-%03d", i)] = map[string]any{
			"input_cost_per_token":  1e-7,
			"output_cost_per_token": 5e-7,
		}
	}
	for model, input := range map[string]float64{
		"gpt-4o":            gptInputPerMtok,
		"claude-sonnet-4-6": 3,
		"claude-opus-4-8":   5,
	} {
		entries[model] = map[string]any{
			"input_cost_per_token":  input / 1_000_000,
			"output_cost_per_token": input * 5 / 1_000_000,
		}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
