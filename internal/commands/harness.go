package commands

import (
	"context"

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
