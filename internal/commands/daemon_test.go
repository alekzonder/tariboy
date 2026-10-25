package commands

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

func ctx(t *testing.T) *registry.Ctx {
	t.Helper()
	s := storetest.Open(t)
	return &registry.Ctx{
		Store: s, Version: "v-test", BaseDir: "/base",
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now().Add(-3 * time.Second),
	}
}

func call(t *testing.T, c *registry.Ctx, path string, p registry.Params) map[string]any {
	t.Helper()
	return callAs[map[string]any](t, c, path, p)
}

// callAs runs the command at path and returns its result as T.
func callAs[T any](t *testing.T, c *registry.Ctx, path string, p registry.Params) T {
	t.Helper()
	reg := BuildRegistry()
	cmd, ok := reg.Get(path)
	if !ok {
		t.Fatalf("command %s not registered", path)
	}
	res, err := registry.Call(c, context.Background(), cmd, p)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	v, ok := res.(T)
	if !ok {
		t.Fatalf("%s returned %T", path, res)
	}
	return v
}

func TestDaemonStatus(t *testing.T) {
	t.Parallel()
	c := ctx(t)
	m := callAs[DaemonStatus](t, c, "daemon.status", registry.Params{})
	if m.Version != "v-test" || m.BaseDir != "/base" {
		t.Fatalf("bad status %v", m)
	}
	if m.UptimeSeconds < 3 {
		t.Fatalf("uptime %v", m.UptimeSeconds)
	}
}

func TestDaemonConfigRoundTrip(t *testing.T) {
	t.Parallel()
	c := ctx(t)
	callAs[DaemonConfigEntry](t, c, "daemon.config.set", registry.Params{"key": "log_level", "value": "debug"})
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
	t.Parallel()
	c := ctx(t)
	reg := BuildRegistry()
	cmd, _ := reg.Get("daemon.config.set")
	if _, err := registry.Call(c, context.Background(), cmd, registry.Params{"value": "x"}); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestDaemonConfigSetKeepsGenericStringBehaviorForUnrelatedKeys(t *testing.T) {
	t.Parallel()
	c := ctx(t)
	got := callAs[DaemonConfigEntry](t, c, "daemon.config.set", registry.Params{"key": "arbitrary", "value": "not json"})
	if got.Value != "not json" {
		t.Fatalf("value = %v, want generic string preserved", got.Value)
	}
}

func TestDaemonStatusIncludesHTTPAddr(t *testing.T) {
	t.Parallel()
	c := ctx(t)
	c.HTTPAddr = "127.0.0.1:9990"
	m := callAs[DaemonStatus](t, c, "daemon.status", registry.Params{})
	if m.HTTPAddr != "127.0.0.1:9990" {
		t.Fatalf("http_addr = %v, want 127.0.0.1:9990", m.HTTPAddr)
	}
}

// A daemon started with an empty --http-addr has no TCP listener at all; the
// desktop app renders a "restart me with a listener" banner off this field, so
// the key must be present and empty rather than absent.
func TestDaemonStatusHTTPAddrEmptyWhenDisabled(t *testing.T) {
	t.Parallel()
	c := ctx(t)
	data, err := json.Marshal(callAs[DaemonStatus](t, c, "daemon.status", registry.Params{}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	v, ok := m["http_addr"]
	if !ok || v != "" {
		t.Fatalf("http_addr = %v (present=%v), want present and empty", v, ok)
	}
}

func TestBuildRegistryOmitsRetiredCommandAndRoute(t *testing.T) {
	t.Parallel()
	reg := BuildRegistry()
	if _, ok := reg.Get("bd"); ok {
		t.Fatal("retired command is still registered")
	}
	for _, path := range []string{"rule.set", "rule.ls", "rule.rm"} {
		if _, ok := reg.Get(path); ok {
			t.Fatalf("retired command %s is still registered", path)
		}
	}
	for _, command := range reg.Commands() {
		if command.HTTP != nil && (command.HTTP.Path == "/api/customer-attention" || strings.HasPrefix(command.HTTP.Path, "/api/proxy-rules")) {
			t.Fatalf("retired route is still registered by %s", command.Path)
		}
	}
}

func TestUsageGroupFilterHelpDescribesFunctionalFilter(t *testing.T) {
	t.Parallel()
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

func TestDaemonStatusReportsMissingTools(t *testing.T) {
	// Sequential: sets PATH with t.Setenv.
	t.Setenv("PATH", t.TempDir())
	out, err := registry.Call(&registry.Ctx{StartedAt: time.Now()}, context.Background(), daemonStatus(), nil)
	if err != nil {
		t.Fatal(err)
	}
	status := out.(DaemonStatus)
	if status.State != "error" || status.Message == "" {
		t.Fatalf("status = %+v, want error state with message", status)
	}
	if missing := status.MissingTools; len(missing) == 0 || missing[0].Name != "tmux" {
		t.Fatalf("missing_tools = %#v, want tmux first", status.MissingTools)
	}
}
