package daemon

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type pricingHTTPClientFunc func(*http.Request) (*http.Response, error)

func (f pricingHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

// daemonTestOptions keeps in-process daemon fixtures deterministic. Production
// startup intentionally resolves the account's login-shell PATH, but that
// external probe is neither part of daemon readiness tests nor safe to put on
// their critical path. userpath_test.go passes its own resolver when it needs
// to exercise that production behavior.
func daemonTestOptions(options Options) Options {
	if options.UserPathResolver == nil {
		options.UserPathResolver = func(context.Context, string) (string, error) {
			return os.Getenv("PATH"), nil
		}
	}
	if options.PricingHTTPClient == nil {
		options.PricingHTTPClient = pricingHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("pricing refresh disabled in daemon test")
		})
	}
	return options
}

func TestDaemonTestOptionsBypassExternalUserPathProbe(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "external-path-probe-called")
	probe := filepath.Join(root, "probe-shell")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\ntouch \""+marker+"\"\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", probe)

	// Force Run to return immediately after PATH setup. This isolates the
	// assertion from unrelated daemon startup duration, including -race load.
	baseFile := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(baseFile, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), daemonTestOptions(Options{BaseDir: baseFile, Listen: "unix", LogLevel: "error"})); err == nil {
		t.Fatal("Run error = nil, want isolated invalid-base failure")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("external PATH probe ran despite injected resolver: stat error = %v", err)
	}
}
