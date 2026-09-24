package commands

import (
	"errors"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/maintenance"
	"github.com/alekzonder/tariboy/internal/registry"
)

func requireMaintenance(c *registry.Ctx) (*maintenance.Service, error) {
	if c.Maintenance == nil {
		return nil, api.UserError{Code: "no_maintenance", Msg: "maintenance is not available"}
	}
	return c.Maintenance, nil
}

func maintenanceGet() registry.Command {
	return registry.Command{
		Path:    "maintenance.get",
		Summary: "Show database maintenance settings and the last run",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/maintenance"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			m, err := requireMaintenance(c)
			if err != nil {
				return nil, err
			}
			set, err := m.Settings()
			if err != nil {
				return nil, err
			}
			last, err := m.LastResult()
			if err != nil {
				return nil, err
			}
			out := map[string]any{"settings": set, "last_run": nil}
			if last != nil {
				out["last_run"] = *last
			}
			return out, nil
		},
	}
}

func maintenanceSet() registry.Command {
	return registry.Command{
		Path:    "maintenance.set",
		Summary: "Change database maintenance settings; unset flags keep their value",
		Args: []registry.Arg{
			{Name: "enabled", Flag: "enabled", Type: registry.Bool, Help: "run backup and cleanup every night"},
			{Name: "time", Flag: "time", Type: registry.String, Help: "nightly run time HH:MM, daemon host local time"},
			{Name: "keep-backups", Flag: "keep-backups", Type: registry.Int, Help: "number of database backups to keep (>= 1)"},
			{Name: "retention-days", Flag: "retention-days", Type: registry.Int, Help: "delete data older than N days (0 = keep everything, else >= 31)"},
			{Name: "compact", Flag: "compact", Type: registry.Bool, Help: "VACUUM the database after cleanup"},
			{Name: "compact-threshold-pct", Flag: "compact-threshold-pct", Type: registry.Int, Help: "VACUUM only when at least this percent of pages is free"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/maintenance"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			m, err := requireMaintenance(c)
			if err != nil {
				return nil, err
			}
			set, err := m.Settings()
			if err != nil {
				return nil, err
			}
			if v, ok := p["enabled"].(bool); ok {
				set.Enabled = v
			}
			if v, ok := p["compact"].(bool); ok {
				set.Compact = v
			}
			if _, ok := p["time"]; ok {
				set.Time = str(p, "time")
			}
			set.KeepBackups = intOf(p, "keep-backups", set.KeepBackups)
			set.RetentionDays = intOf(p, "retention-days", set.RetentionDays)
			set.CompactThresholdPct = intOf(p, "compact-threshold-pct", set.CompactThresholdPct)
			if err := set.Validate(); err != nil {
				return nil, api.UserError{Code: "bad_value", Msg: err.Error()}
			}
			if err := m.SetSettings(set); err != nil {
				return nil, err
			}
			return set, nil
		},
	}
}

func maintenanceRun() registry.Command {
	return registry.Command{
		Path:    "maintenance.run",
		Summary: "Back up the database, delete expired data, and compact it now",
		HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/maintenance/run"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			m, err := requireMaintenance(c)
			if err != nil {
				return nil, err
			}
			res, err := m.Run()
			if errors.Is(err, maintenance.ErrBusy) {
				return nil, api.UserError{Code: "maintenance_busy", Msg: err.Error()}
			}
			if err != nil && res.StartedAt == "" {
				return nil, err
			}
			// A run that started reports its own error in the result.
			return res, nil
		},
	}
}
