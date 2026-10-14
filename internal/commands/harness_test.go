package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestHarnessModelsRoute(t *testing.T) {
	cmd, ok := BuildRegistry().Get("harness.models")
	if !ok {
		t.Fatal("harness.models not registered")
	}
	if cmd.HTTP == nil || cmd.HTTP.Method != "GET" || cmd.HTTP.Path != "/api/harnesses/{type}/models" {
		t.Fatalf("route = %+v", cmd.HTTP)
	}
}

func TestHarnessModelsReturnsCatalogAndCLIErrors(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	dir := t.TempDir()
	script := "#!/bin/sh\necho openai/gpt-5\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	res, err := h(t, "harness.models")(c, registry.Params{"type": "opencode", "refresh": "true"})
	if err != nil {
		t.Fatal(err)
	}
	catalog := res.(harness.Catalog)
	if catalog.Error != "" || len(catalog.Models) != 1 || catalog.Models[0].ID != "openai/gpt-5" {
		t.Fatalf("catalog = %+v", catalog)
	}

	res, err = h(t, "harness.models")(c, registry.Params{"type": "codex", "refresh": true})
	if err != nil {
		t.Fatal(err)
	}
	if catalog := res.(harness.Catalog); catalog.Error != "codex not found on PATH" || len(catalog.Models) != 0 {
		t.Fatalf("missing CLI catalog = %+v", catalog)
	}

	if _, err := h(t, "harness.models")(c, registry.Params{"type": "nope"}); !isUserErr(err, "bad_harness") {
		t.Fatalf("unknown harness err = %v", err)
	}
}

func TestHarnessCodexApprovalGetSet(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	get, set := h(t, "harness.codex-approval.get"), h(t, "harness.codex-approval.set")

	res, err := get(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.(map[string]any)["mode"]; got != harness.CodexApprovalYolo {
		t.Fatalf("default mode = %v", got)
	}

	for _, mode := range []string{harness.CodexApprovalApproveForMe, harness.CodexApprovalYolo} {
		if _, err := set(c, registry.Params{"mode": mode}); err != nil {
			t.Fatal(err)
		}
		if v, _, _ := c.Store.ConfigGet(harness.CodexApprovalConfigKey); v != mode {
			t.Fatalf("stored = %q, want %q", v, mode)
		}
		res, err := get(c, registry.Params{})
		if err != nil || res.(map[string]any)["mode"] != mode {
			t.Fatalf("get after set %q = %v, %v", mode, res, err)
		}
	}

	if _, err := set(c, registry.Params{"mode": "never"}); !isUserErr(err, "bad_value") {
		t.Fatalf("bad mode err = %v", err)
	}
}

func TestHarnessCodexApprovalRoutes(t *testing.T) {
	r := BuildRegistry()
	for path, want := range map[string]string{
		"harness.codex-approval.get": "GET",
		"harness.codex-approval.set": "POST",
	} {
		cmd, ok := r.Get(path)
		if !ok {
			t.Fatalf("%s not registered", path)
		}
		if cmd.HTTP == nil || cmd.HTTP.Method != want || cmd.HTTP.Path != "/api/harnesses/codex/approval-mode" {
			t.Fatalf("%s route = %+v", path, cmd.HTTP)
		}
	}
}
