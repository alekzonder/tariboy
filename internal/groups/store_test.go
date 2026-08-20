package groups

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func newGroupStore(t *testing.T) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s, func() time.Time { return time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC) })
}

func TestGroupStoreCRUD(t *testing.T) {
	s := newGroupStore(t)
	if err := s.Upsert(Group{Name: "research", Lead: "scout"}); err != nil {
		t.Fatal(err)
	}
	// Idempotent upsert refreshes without duplicating.
	if err := s.Upsert(Group{Name: "research", Lead: "scout"}); err != nil {
		t.Fatal(err)
	}
	g, ok, err := s.Get("research")
	if err != nil || !ok || g.Lead != "scout" || g.CreatedAt == "" {
		t.Fatalf("get = %+v ok=%v err=%v", g, ok, err)
	}
	if err := s.SetLead("research", "writer"); err != nil {
		t.Fatal(err)
	}
	g2, _, _ := s.Get("research")
	if g2.Lead != "writer" {
		t.Fatalf("lead not changed: %+v", g2)
	}
	if err := s.Upsert(Group{Name: "ops"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 2 || list[0].Name != "ops" || list[1].Name != "research" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := s.Delete("ops"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("ops"); ok {
		t.Fatal("delete did not remove")
	}
}

func TestGroupStoreRejectsBadName(t *testing.T) {
	s := newGroupStore(t)
	if err := s.Upsert(Group{Name: "../evil"}); err == nil {
		t.Fatal("traversing name must be rejected")
	}
}
