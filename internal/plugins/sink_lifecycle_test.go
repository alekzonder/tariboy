package plugins

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
)

func serveActionRoutes(t *testing.T, h *Host, name string, routes map[string]any) {
	t.Helper()
	socket := h.SocketPath(name)
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/routes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"routes": routes})
	})
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
}

func actionSink(t *testing.T, h *Host, store *Store, name string, subscribe []string) {
	t.Helper()
	rec := Record{
		Name: name, Types: []string{"channel-sink"},
		Channels: Channels{Publish: []string{"chat:*"}, Subscribe: subscribe},
	}
	if err := store.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	h.running[name] = &runningPlugin{rec: rec, state: "running"}
	h.mu.Unlock()
}

func TestApplyActionSubscriptionsAddsIdempotently(t *testing.T) {
	h, b, store := newHost(t, nil)
	actionSink(t, h, store, "messenger-provider", []string{"chat:*"})
	response := map[string]any{
		"subscriptions": map[string]any{"add": []any{"chat:new"}},
	}
	for range 2 {
		if err := h.ApplyActionSubscriptions("messenger-provider", response); err != nil {
			t.Fatal(err)
		}
	}
	subs, err := b.ListSubscriptions("plugin:messenger-provider")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Channel != "chat:new" {
		t.Fatalf("subscriptions = %+v", subs)
	}
}

func TestApplyActionSubscriptionsRejectsInvalidBatchWithoutPartialMutation(t *testing.T) {
	h, b, store := newHost(t, nil)
	actionSink(t, h, store, "messenger-provider", []string{"chat:*"})
	response := map[string]any{
		"subscriptions": map[string]any{"add": []any{"chat:valid", "outside:invalid"}},
	}
	if err := h.ApplyActionSubscriptions("messenger-provider", response); err == nil {
		t.Fatal("invalid subscription effect succeeded")
	}
	if subs, _ := b.ListSubscriptions("plugin:messenger-provider"); len(subs) != 0 {
		t.Fatalf("partial subscriptions = %+v", subs)
	}
}

func TestApplyActionSubscriptionsPreservesDeclaredConcreteSubscription(t *testing.T) {
	h, b, store := newHost(t, nil)
	actionSink(t, h, store, "messenger-provider", []string{"chat:*", "chat:standing"})
	if _, err := b.Subscribe("plugin:messenger-provider", "chat:standing", bus.Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	response := map[string]any{
		"subscriptions": map[string]any{"remove": []any{"chat:standing"}},
	}
	if err := h.ApplyActionSubscriptions("messenger-provider", response); err == nil {
		t.Fatal("declared subscription removal succeeded")
	}
	if subs, _ := b.ListSubscriptions("plugin:messenger-provider"); len(subs) != 1 {
		t.Fatalf("declared subscription was removed: %+v", subs)
	}
}

func TestApplyActionSubscriptionsRemovesOnlyUnusedRouteChannel(t *testing.T) {
	for _, test := range []struct {
		name      string
		routes    map[string]any
		wantError bool
	}{
		{name: "in use", routes: map[string]any{"external-2": "chat:shared"}, wantError: true},
		{name: "unused", routes: map[string]any{"external-2": "chat:other"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, b, store := newHost(t, nil)
			shortRoot, err := os.MkdirTemp("", "plugin-action-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(shortRoot) })
			h.cfg.PluginsDir = shortRoot
			actionSink(t, h, store, "messenger-provider", []string{"chat:*"})
			serveActionRoutes(t, h, "messenger-provider", test.routes)
			if _, err := b.Subscribe("plugin:messenger-provider", "chat:shared", bus.Matcher{}, nil); err != nil {
				t.Fatal(err)
			}
			err = h.ApplyActionSubscriptions("messenger-provider", map[string]any{
				"subscriptions": map[string]any{"remove": []any{"chat:shared"}},
			})
			if test.wantError && err == nil {
				t.Fatal("route-backed subscription removal succeeded")
			}
			if !test.wantError && err != nil {
				t.Fatal(err)
			}
			subs, _ := b.ListSubscriptions("plugin:messenger-provider")
			if test.wantError && len(subs) != 1 {
				t.Fatalf("route-backed subscription removed: %+v", subs)
			}
			if !test.wantError && len(subs) != 0 {
				t.Fatalf("unused subscription retained: %+v", subs)
			}
		})
	}
}

func TestApplyActionSubscriptionsRejectsMalformedMetadataAndNonSink(t *testing.T) {
	h, _, store := newHost(t, nil)
	actionSink(t, h, store, "messenger-provider", []string{"chat:*"})
	for _, response := range []map[string]any{
		{"subscriptions": "bad"},
		{"subscriptions": map[string]any{"add": "chat:new"}},
		{"subscriptions": map[string]any{"add": []any{1}}},
	} {
		if err := h.ApplyActionSubscriptions("messenger-provider", response); err == nil {
			t.Fatalf("malformed metadata succeeded: %#v", response)
		}
	}
	source := Record{Name: "source-provider", Types: []string{"channel-source"}, Channels: Channels{Publish: []string{"chat:*"}}}
	if err := store.Upsert(source); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	h.running[source.Name] = &runningPlugin{rec: source, state: "running"}
	h.mu.Unlock()
	if err := h.ApplyActionSubscriptions(source.Name, map[string]any{
		"subscriptions": map[string]any{"add": []any{"chat:new"}},
	}); err == nil {
		t.Fatal("non-sink effect succeeded")
	}
}
