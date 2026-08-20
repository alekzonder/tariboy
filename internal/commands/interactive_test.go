package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

func TestSendKeys_ItemsRouteRaw(t *testing.T) {
	c, _, fc := ctxWithStore(t)
	res, err := agentSendKeys().Handler(c, registry.Params{
		"name":  "a1",
		"items": []any{map[string]any{"text": "hi"}, map[string]any{"key": "Enter"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.itemsSent) != 2 || fc.itemsSent[0].Text != "hi" || fc.itemsSent[1].Key != "Enter" {
		t.Fatalf("items not routed raw: %+v", fc.itemsSent)
	}
	if res.(map[string]any)["sent"].(int) != 2 {
		t.Fatalf("sent count: %v", res)
	}
}

func TestSendKeys_LegacyKeysRoute(t *testing.T) {
	c, _, fc := ctxWithStore(t)
	if _, err := agentSendKeys().Handler(c, registry.Params{"name": "a1", "keys": "ls"}); err != nil {
		t.Fatal(err)
	}
	if fc.itemsSent != nil {
		t.Fatalf("legacy path should not call SendKeysItems: %+v", fc.itemsSent)
	}
}

func TestParseKeyItems_IgnoresEmptyAndNonObjects(t *testing.T) {
	items, ok := parseKeyItems([]any{
		map[string]any{"text": "a"},
		map[string]any{}, // empty → dropped
		"garbage",        // non-object → skipped
		map[string]any{"key": "Up"},
	})
	if !ok || len(items) != 2 || items[0].Text != "a" || items[1].Key != "Up" {
		t.Fatalf("parse = %+v ok=%v", items, ok)
	}
	if _, ok := parseKeyItems(nil); ok {
		t.Fatal("nil should yield ok=false")
	}
}
