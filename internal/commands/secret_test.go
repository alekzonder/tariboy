package commands

import (
	"errors"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestSecretStoreLsRm(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})

	if _, err := h(t, "secret.store")(c, registry.Params{"name": "smoke", "key": "TOKEN", "value": "s3cr3t"}); err != nil {
		t.Fatal(err)
	}
	ls, err := h(t, "secret.ls")(c, registry.Params{"name": "smoke"})
	if err != nil {
		t.Fatal(err)
	}
	keys := ls.(map[string]any)["keys"].([]string)
	if len(keys) != 1 || keys[0] != "TOKEN" {
		t.Fatalf("keys = %v", keys)
	}
	// values must never appear anywhere in the ls output
	for _, v := range ls.(map[string]any) {
		if s, ok := v.(string); ok && s == "s3cr3t" {
			t.Fatal("secret value leaked in ls output")
		}
	}
	if _, err := h(t, "secret.rm")(c, registry.Params{"name": "smoke", "key": "TOKEN"}); err != nil {
		t.Fatal(err)
	}
	if k, _ := as.SecretKeys("smoke"); len(k) != 0 {
		t.Fatalf("secret not removed: %v", k)
	}
}

// TestSecretRmNotFound ensures removing an absent secret yields a not_found
// UserError (and not a generic/DB error masquerading as not_found).
func TestSecretRmNotFound(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})

	_, err := h(t, "secret.rm")(c, registry.Params{"name": "smoke", "key": "MISSING"})
	if err == nil {
		t.Fatal("removing absent secret should error")
	}
	var ue api.UserError
	if !errors.As(err, &ue) || ue.Code != "not_found" {
		t.Fatalf("want not_found UserError, got %#v", err)
	}
}
