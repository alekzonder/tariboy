package commands

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/registry"
)

// harnessModels asks the harness CLI on the daemon host which models and
// efforts it supports. CLI failures come back in the catalog's error field, so
// clients keep free-form input working.
func harnessModels() registry.Command {
	return registry.Command{
		Path:    "harness.models",
		Summary: "List the models and efforts a harness supports on the daemon host",
		Args: []registry.Arg{
			{Name: "type", Type: registry.String, Required: true, Help: "harness: claude|codex|opencode|cursor|stub"},
			{Name: "refresh", Flag: "refresh", Type: registry.Bool, Help: "ask the harness CLI again instead of the 5-minute cache"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/harnesses/{type}/models"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			refresh, _ := boolParam(p, "refresh")
			catalog, err := harness.ListModels(context.Background(), str(p, "type"), refresh)
			if err != nil {
				return nil, api.UserError{Code: "bad_harness", Msg: err.Error()}
			}
			return catalog, nil
		},
	}
}

// harnessCodexApprovalGet reports how this daemon starts codex: --yolo (the
// default when nothing is stored) or --approve-for-me.
func harnessCodexApprovalGet() registry.Command {
	return registry.Command{
		Path:    "harness.codex-approval.get",
		Summary: "Read the codex approval mode of this daemon (yolo|approve-for-me)",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/harnesses/codex/approval-mode"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			v, ok, err := c.Store.ConfigGet(harness.CodexApprovalConfigKey)
			if err != nil {
				return nil, err
			}
			if !ok || !slices.Contains(harness.CodexApprovalModes, v) {
				v = harness.CodexApprovalYolo
			}
			return map[string]any{"mode": v}, nil
		},
	}
}

// harnessCodexApprovalSet changes the codex approval mode; codex iterations
// started afterwards use it.
func harnessCodexApprovalSet() registry.Command {
	return registry.Command{
		Path:    "harness.codex-approval.set",
		Summary: "Set the codex approval mode of this daemon for the next iterations",
		Args: []registry.Arg{
			{Name: "mode", Type: registry.String, Required: true, Help: "yolo|approve-for-me"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/harnesses/codex/approval-mode"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			mode := str(p, "mode")
			if !slices.Contains(harness.CodexApprovalModes, mode) {
				return nil, api.UserError{Code: "bad_value", Msg: "mode must be one of " + strings.Join(harness.CodexApprovalModes, "|")}
			}
			if err := c.Store.ConfigSet(harness.CodexApprovalConfigKey, mode); err != nil {
				return nil, err
			}
			data, _ := json.Marshal(map[string]string{"key": harness.CodexApprovalConfigKey, "value": mode})
			if err := c.Store.AddEvent("", "config_set", string(data)); err != nil {
				return nil, err
			}
			return map[string]any{"mode": mode}, nil
		},
	}
}
