package plugins

import (
	"errors"
	"testing"
)

// fakeLister satisfies RecordLister: it returns a fixed record set, or an error
// to simulate a transient plugin-store read failure at subscribe time.
type fakeLister struct {
	recs []Record
	err  error
}

func (f fakeLister) List() ([]Record, error) { return f.recs, f.err }

// TestParamsValidatorForSwallowsListError proves review finding #2: when the
// plugin store's List() fails, a subscribe to a core (non-provider) channel must
// still succeed — the store-read failure must not propagate to every subscribe —
// while a readable store still enforces a provided channel's required-params
// gate loudly.
func TestParamsValidatorForSwallowsListError(t *testing.T) {
	// Store unreadable: every subscribe is allowed, target treated as non-provider.
	var gotChannel string
	var gotErr error
	failing := ParamsValidatorFor(
		fakeLister{err: errors.New("boom")},
		func(channel string, err error) { gotChannel, gotErr = channel, err },
	)
	if err := failing("chat:room", nil); err != nil {
		t.Fatalf("core subscribe must succeed on List() error, got: %v", err)
	}
	// Even a channel a provider *would* gate is allowed while the store is down —
	// we can't know it's a provider channel when we can't read the store.
	if err := failing("issue-provider:query", map[string]any{"junk": true}); err != nil {
		t.Fatalf("subscribe must be allowed while store unreadable, got: %v", err)
	}
	if gotErr == nil || gotChannel != "issue-provider:query" {
		t.Fatalf("onListErr not reported: channel=%q err=%v", gotChannel, gotErr)
	}

	// Store readable: the provided channel's params_schema gate still fires.
	ok := ParamsValidatorFor(fakeLister{recs: providerRecords()}, nil)
	if err := ok("chat:room", map[string]any{"x": 1}); err != nil {
		t.Fatalf("non-provider channel must pass with readable store, got: %v", err)
	}
	if err := ok("issue-provider:query", map[string]any{"query": "is:open"}); err != nil {
		t.Fatalf("valid provider params must pass, got: %v", err)
	}
	if err := ok("issue-provider:query", map[string]any{"limit": float64(5)}); err == nil {
		t.Fatalf("missing required param must fail loudly with readable store")
	}
}

// TestParamsValidatorForNilOnErrCallback ensures a nil onListErr callback is
// tolerated (the daemon always passes one, but the helper must not panic).
func TestParamsValidatorForNilOnErrCallback(t *testing.T) {
	v := ParamsValidatorFor(fakeLister{err: errors.New("boom")}, nil)
	if err := v("chat:room", nil); err != nil {
		t.Fatalf("expected nil error with nil callback, got: %v", err)
	}
}
