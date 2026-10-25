package commands

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/maintenance"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestMaintenanceCommands(t *testing.T) {
	t.Parallel()
	c, _, _ := ctxWithStore(t)
	c.Maintenance = maintenance.New(c.Store, filepath.Join(t.TempDir(), "backups"), nil, nil)

	got, err := h(t, "maintenance.get")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(MaintenanceView)
	if s := maintenance.Settings(m.Settings); s != maintenance.DefaultSettings() {
		t.Fatalf("settings = %+v", s)
	}
	if data, err := json.Marshal(m.LastRun); err != nil || string(data) != "null" {
		t.Fatalf("last_run = %s %v, want null before any run", data, err)
	}

	// Unset flags keep their current value.
	if _, err := h(t, "maintenance.set")(c, registry.Params{"retention-days": 120, "enabled": false, "time": "04:30"}); err != nil {
		t.Fatal(err)
	}
	s, _ := c.Maintenance.Settings()
	want := maintenance.DefaultSettings()
	want.RetentionDays, want.Enabled, want.Time = 120, false, "04:30"
	if s != want {
		t.Fatalf("settings = %+v, want %+v", s, want)
	}
	if _, err := h(t, "maintenance.set")(c, registry.Params{"retention-days": 7}); err == nil {
		t.Fatal("retention-days 7 accepted")
	}
	if _, err := h(t, "maintenance.set")(c, registry.Params{"chat-retention-days": 3}); err != nil {
		t.Fatal(err)
	}
	if s, _ := c.Maintenance.Settings(); s.ChatRetentionDays != 3 || s.RetentionDays != 120 {
		t.Fatalf("settings = %+v, want chat retention 3", s)
	}
	if _, err := h(t, "maintenance.set")(c, registry.Params{"chat-retention-days": -1}); err == nil {
		t.Fatal("chat-retention-days -1 accepted")
	}

	run, err := h(t, "maintenance.run")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if r := run.(MaintenanceRun); r.Backup == "" || r.Error != "" {
		t.Fatalf("run = %+v", r)
	}
	got, _ = h(t, "maintenance.get")(c, registry.Params{})
	if data, err := json.Marshal(got.(MaintenanceView).LastRun); err != nil || string(data) == "null" {
		t.Fatal("last_run missing after run")
	}
}
