package commands

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
)

func ctx(t *testing.T) *registry.Ctx {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &registry.Ctx{
		Store: s, Version: "v-test", BaseDir: "/base",
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now().Add(-3 * time.Second),
	}
}

func call(t *testing.T, c *registry.Ctx, path string, p registry.Params) map[string]any {
	t.Helper()
	reg := BuildRegistry()
	cmd, ok := reg.Get(path)
	if !ok {
		t.Fatalf("command %s not registered", path)
	}
	res, err := cmd.Handler(c, p)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("%s returned %T", path, res)
	}
	return m
}

func TestDaemonStatus(t *testing.T) {
	c := ctx(t)
	m := call(t, c, "daemon.status", registry.Params{})
	if m["version"] != "v-test" || m["base_dir"] != "/base" {
		t.Fatalf("bad status %v", m)
	}
	if m["uptime_seconds"].(int64) < 3 {
		t.Fatalf("uptime %v", m["uptime_seconds"])
	}
}

func TestDaemonConfigRoundTrip(t *testing.T) {
	c := ctx(t)
	call(t, c, "daemon.config.set", registry.Params{"key": "log_level", "value": "debug"})
	got := call(t, c, "daemon.config.get", registry.Params{"key": "log_level"})
	if got["log_level"] != "debug" {
		t.Fatalf("get after set: %v", got)
	}
	all := call(t, c, "daemon.config.get", registry.Params{})
	if all["log_level"] != "debug" {
		t.Fatalf("get all: %v", all)
	}
	// config_set must be audited
	var n int
	c.Store.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE kind='config_set'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit events = %d, want 1", n)
	}
}

func TestDaemonConfigSetValidates(t *testing.T) {
	c := ctx(t)
	reg := BuildRegistry()
	cmd, _ := reg.Get("daemon.config.set")
	if _, err := cmd.Handler(c, registry.Params{"value": "x"}); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestDaemonStatusIncludesHTTPAddr(t *testing.T) {
	c := ctx(t)
	c.HTTPAddr = "127.0.0.1:9990"
	m := call(t, c, "daemon.status", registry.Params{})
	if m["http_addr"] != "127.0.0.1:9990" {
		t.Fatalf("http_addr = %v, want 127.0.0.1:9990", m["http_addr"])
	}
}

// A daemon started with an empty --http-addr has no TCP listener at all; the
// desktop app renders a "restart me with a listener" banner off this field, so
// the key must be present and empty rather than absent.
func TestDaemonStatusHTTPAddrEmptyWhenDisabled(t *testing.T) {
	c := ctx(t)
	m := call(t, c, "daemon.status", registry.Params{})
	v, ok := m["http_addr"]
	if !ok || v != "" {
		t.Fatalf("http_addr = %v (present=%v), want present and empty", v, ok)
	}
}

func TestBuildRegistryOmitsRetiredCommandAndRoute(t *testing.T) {
	reg := BuildRegistry()
	if _, ok := reg.Get("bd"); ok {
		t.Fatal("retired command is still registered")
	}
	for _, command := range reg.Commands() {
		if command.HTTP != nil && command.HTTP.Path == "/api/customer-attention" {
			t.Fatalf("retired route is still registered by %s", command.Path)
		}
	}
}

func TestUsageGroupFilterHelpDescribesFunctionalFilter(t *testing.T) {
	cmd, ok := BuildRegistry().Get("usage")
	if !ok {
		t.Fatal("usage command not registered")
	}
	for _, arg := range cmd.Args {
		if arg.Name != "group" {
			continue
		}
		if strings.Contains(strings.ToLower(arg.Help), "no-op") || !strings.Contains(arg.Help, "__ungrouped__") {
			t.Fatalf("group help = %q, want functional filter including the ungrouped sentinel", arg.Help)
		}
		return
	}
	t.Fatal("usage command has no group argument")
}
