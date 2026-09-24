package commands

import (
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/maintenance"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestMaintenanceCommands(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	c.Maintenance = maintenance.New(c.Store, filepath.Join(t.TempDir(), "backups"), nil, nil)

	got, err := h(t, "maintenance.get")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if s := m["settings"].(maintenance.Settings); s != maintenance.DefaultSettings() {
		t.Fatalf("settings = %+v", s)
	}
	if m["last_run"] != nil {
		t.Fatalf("last_run = %+v, want nil before any run", m["last_run"])
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

	run, err := h(t, "maintenance.run")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if r := run.(maintenance.Result); r.Backup == "" || r.Error != "" {
		t.Fatalf("run = %+v", r)
	}
	got, _ = h(t, "maintenance.get")(c, registry.Params{})
	if got.(map[string]any)["last_run"] == nil {
		t.Fatal("last_run missing after run")
	}
}
