package plugins

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s, func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) })
}

func sampleRecord(name string) Record {
	return Record{
		Name: name, Version: "0.1.0", Types: []string{"channel-source", "channel-sink"},
		ProtocolVersion: 1, Exec: "echo.py", SourcePath: "/tmp/" + name,
		Channels: Channels{Publish: []string{"chat:*"}, Subscribe: []string{"chat:in"}},
		Enabled:  true, State: "installed", Health: "{}",
	}
}

func TestUpsertGetListDelete(t *testing.T) {
	s := newStore(t)
	if err := s.Upsert(sampleRecord("echo")); err != nil {
		t.Fatal(err)
	}
	// Upsert again with a new version: idempotent, no duplicate row.
	r2 := sampleRecord("echo")
	r2.Version = "0.2.0"
	if err := s.Upsert(r2); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("echo")
	if err != nil || !ok {
		t.Fatalf("get echo ok=%v err=%v", ok, err)
	}
	if got.Version != "0.2.0" || len(got.Types) != 2 || got.Channels.Subscribe[0] != "chat:in" {
		t.Fatalf("record = %+v", got)
	}
	if got.InstalledAt == "" {
		t.Fatal("installed_at not stamped")
	}
	if err := s.Upsert(sampleRecord("other")); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 2 || list[0].Name != "echo" || list[1].Name != "other" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := s.Delete("other"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("other"); ok {
		t.Fatal("delete did not remove")
	}
}

func TestScanRecordRejectsMalformedJSON(t *testing.T) {
	s := newStore(t)
	s.Upsert(sampleRecord("echo"))
	// Corrupt the types JSON directly.
	if _, err := s.db.Exec(`UPDATE plugins SET types='not json' WHERE name=?`, "echo"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get("echo"); err == nil {
		t.Fatal("malformed types JSON should surface an error")
	}
}

func TestSetEnabledAndState(t *testing.T) {
	s := newStore(t)
	s.Upsert(sampleRecord("echo"))
	if err := s.SetEnabled("echo", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetState("echo", "running", `{"checked_at":"t"}`); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Get("echo")
	if got.Enabled != false || got.State != "running" || got.Health != `{"checked_at":"t"}` {
		t.Fatalf("record = %+v", got)
	}
}
