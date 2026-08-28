package commands

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/registry"
)

func requirePlugins(c *registry.Ctx) (registry.PluginControl, error) {
	if c.Plugins == nil {
		return nil, api.UserError{Code: "no_plugin_host", Msg: "plugin host is not available"}
	}
	return c.Plugins, nil
}

// checkPluginName rejects a traversing/invalid name at the CLI layer with a
// clean 400 (bad_name), before any host primitive runs. This mirrors the
// primitive-level guard in plugins.Host so a bad name is refused in depth.
func checkPluginName(name string) error {
	if !plugins.ValidName(name) {
		return api.UserError{Code: "bad_name", Msg: "invalid plugin name " + name}
	}
	return nil
}

func pluginInstall() registry.Command {
	return registry.Command{
		Path:    "plugin.install",
		Summary: "Install and start a plugin from a directory with plugin.json",
		Args: []registry.Arg{
			{Name: "path", Type: registry.String, Required: true, Help: "path to the plugin dir (daemon-accessible)"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/plugins"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			path := str(p, "path")
			if path == "" {
				return nil, api.UserError{Code: "missing_path", Msg: "path is required"}
			}
			res, err := pc.Install(path)
			if err != nil {
				return nil, api.UserError{Code: "install_failed", Msg: err.Error()}
			}
			return res, nil
		},
	}
}

func pluginLs() registry.Command {
	return registry.Command{
		Path:    "plugin.ls",
		Summary: "List installed plugins (name/version/types/state/health)",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/plugins"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			list, err := pc.List()
			if err != nil {
				return nil, err
			}
			return map[string]any{"plugins": list, "count": len(list)}, nil
		},
	}
}

type pluginContributionsControl interface {
	Contributions() ([]plugins.Contribution, error)
}

func pluginContributions() registry.Command {
	return registry.Command{
		Path:    "plugin.contributions",
		Summary: "List enabled plugin CLI and settings contributions",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/plugin-contributions"},
		Handler: func(c *registry.Ctx, _ registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			contributor, ok := pc.(pluginContributionsControl)
			if !ok {
				return nil, api.UserError{Code: "no_plugin_contributions", Msg: "plugin host does not support contributions"}
			}
			items, err := contributor.Contributions()
			if err != nil {
				return nil, api.UserError{Code: "contributions_failed", Msg: err.Error()}
			}
			return map[string]any{"plugins": items, "count": len(items)}, nil
		},
	}
}

func pluginInspect() registry.Command {
	return registry.Command{
		Path:    "plugin.inspect",
		Summary: "Show one plugin's manifest, state and socket",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "plugin name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/plugins/{name}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkPluginName(name); err != nil {
				return nil, err
			}
			res, err := pc.Inspect(name)
			if err != nil {
				if errors.Is(err, plugins.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "plugin " + name + " not found"}
				}
				return nil, api.UserError{Code: "inspect_failed", Msg: err.Error()}
			}
			return res, nil
		},
	}
}

func pluginRm() registry.Command {
	return registry.Command{
		Path:    "plugin.rm",
		Summary: "Stop and remove a plugin",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "plugin name"},
			{Name: "force", Flag: "force", Short: "f", Type: registry.Bool, Help: "stop and remove even if running"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/plugins/{name}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkPluginName(name); err != nil {
				return nil, err
			}
			// Without --force, refuse to remove a running plugin (mirrors
			// `agent rm`): the operator must stop it or opt in explicitly.
			if !toBool(p["force"]) {
				if info, err := pc.Inspect(name); err == nil {
					if state, _ := info["state"].(string); state == "running" {
						return nil, api.UserError{Code: "running", Msg: "plugin " + name + " is running; use --force"}
					}
				}
			}
			if err := pc.Remove(name); err != nil {
				return nil, api.UserError{Code: "rm_failed", Msg: err.Error()}
			}
			return map[string]any{"removed": name}, nil
		},
	}
}

func pluginRestart() registry.Command {
	return registry.Command{
		Path:    "plugin.restart",
		Summary: "Restart a single plugin in place (stop + start) without touching others",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "plugin name"}},
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/plugins/{name}/restart"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkPluginName(name); err != nil {
				return nil, err
			}
			if err := pc.Restart(name); err != nil {
				if errors.Is(err, plugins.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "plugin " + name + " not found"}
				}
				return nil, api.UserError{Code: "restart_failed", Msg: err.Error()}
			}
			return map[string]any{"restarted": name}, nil
		},
	}
}

func pluginLogs() registry.Command {
	return registry.Command{
		Path:    "plugin.logs",
		Summary: "Show captured plugin stdout/stderr",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "plugin name"},
			{Name: "tail", Flag: "tail", Short: "t", Type: registry.Int, Help: "last N lines (default 200)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/plugins/{name}/logs"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkPluginName(name); err != nil {
				return nil, err
			}
			lines, err := pc.Logs(name, intOf(p, "tail", 200))
			if err != nil {
				return nil, err
			}
			return map[string]any{"lines": lines, "count": len(lines)}, nil
		},
	}
}

func pluginRoutes() registry.Command {
	return registry.Command{
		Path:    "plugin.routes",
		Summary: "Show a plugin's external channel routes",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "plugin name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/plugins/{name}/routes"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkPluginName(name); err != nil {
				return nil, err
			}
			res, err := pc.PluginRoutes(name)
			if err != nil {
				return nil, pluginActionErr(name, err)
			}
			return res, nil
		},
	}
}

func pluginAction() registry.Command {
	return registry.Command{
		Path:    "plugin.action",
		Summary: "Invoke a generic action on a running plugin",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "plugin name"},
			{Name: "action", Type: registry.String, Required: true, Help: "plugin-defined action name"},
			{Name: "data", Flag: "data", Type: registry.String, Help: "plugin-defined JSON object (default {})"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/plugins/{name}/action"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			pc, err := requirePlugins(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkPluginName(name); err != nil {
				return nil, err
			}
			action := str(p, "action")
			if action == "" {
				return nil, api.UserError{Code: "missing_action", Msg: "action is required"}
			}
			raw := str(p, "data")
			if raw == "" {
				raw = "{}"
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(raw), &body); err != nil || body == nil {
				return nil, api.UserError{Code: "bad_data", Msg: "data must be a JSON object"}
			}
			if _, exists := body["action"]; exists {
				return nil, api.UserError{Code: "bad_data", Msg: "data must not contain action"}
			}
			if validator, ok := pc.(interface {
				ValidateOperatorAction(name, action string, body map[string]any) error
			}); ok {
				if err := validator.ValidateOperatorAction(name, action, body); err != nil {
					return nil, api.UserError{Code: "bad_action_data", Msg: err.Error()}
				}
			}
			body["action"] = action
			res, err := pc.PluginAction(name, body)
			if err != nil {
				return nil, pluginActionErr(name, err)
			}
			if err := pc.ApplyActionSubscriptions(name, res); err != nil {
				return nil, api.UserError{Code: "bad_action_effect", Msg: err.Error()}
			}
			if result, ok := res["result"]; ok {
				return result, nil
			}
			public := make(map[string]any, len(res))
			for key, value := range res {
				if key != "subscriptions" {
					public[key] = value
				}
			}
			return public, nil
		},
	}
}

// pluginActionErr maps host/plugin errors to clean UserErrors for the API.
func pluginActionErr(name string, err error) error {
	if errors.Is(err, plugins.ErrNotRunning) {
		return api.UserError{Code: "not_running", Msg: "plugin " + name + " is not running"}
	}
	var ae *plugins.ActionError
	if errors.As(err, &ae) {
		code := ae.Code
		if code == "" {
			code = "action_failed"
		}
		return api.UserError{Code: code, Msg: "plugin action failed: " + code}
	}
	return api.UserError{Code: "action_failed", Msg: err.Error()}
}

// intOf reads an int arg tolerant of the CLI's typed int, JSON float64 and the
// HTTP query string forms.
func intOf(p registry.Params, key string, def int) int {
	switch v := p[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
