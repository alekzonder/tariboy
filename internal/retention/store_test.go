package retention

import (
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/store"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s)
}

func TestDefaultAndPerAgent(t *testing.T) {
	s := newStore(t)
	// No default set yet -> zero policy with Archive true (unlimited, archive on).
	def, err := s.Default()
	if err != nil {
		t.Fatal(err)
	}
	if def.KeepIterations != 0 || !def.Archive {
		t.Fatalf("empty default = %+v", def)
	}
	if err := s.SetDefault(Policy{KeepIterations: 5, KeepDays: 30, MaxBytes: 1 << 20, Archive: true}); err != nil {
		t.Fatal(err)
	}
	// Effective(agent) with no per-agent row == default.
	eff, err := s.Effective("bot")
	if err != nil {
		t.Fatal(err)
	}
	if eff.KeepIterations != 5 || eff.MaxBytes != 1<<20 {
		t.Fatalf("effective(default) = %+v", eff)
	}
	// Per-agent row wins.
	if err := s.Set("bot", Policy{KeepIterations: 1, Archive: false}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("bot")
	if err != nil || !ok || got.KeepIterations != 1 || got.Archive {
		t.Fatalf("get = %+v ok=%v err=%v", got, ok, err)
	}
	eff, _ = s.Effective("bot")
	if eff.KeepIterations != 1 || eff.Archive {
		t.Fatalf("effective(per-agent) = %+v", eff)
	}
	if err := s.Delete("bot"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("bot"); ok {
		t.Fatal("delete did not remove")
	}
}
