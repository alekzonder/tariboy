package agent

import (
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/store"
)

func newAliasStore(t *testing.T) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s)
}

func TestSetAliasAndNotesRoundTrip(t *testing.T) {
	as := newAliasStore(t)
	if err := as.Create(Agent{Name: "a1", OnTimeout: "restart", OnError: "restart"}); err != nil {
		t.Fatal(err)
	}
	if err := as.SetAlias("a1", "Friendly"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetNotes("a1", "hello notes"); err != nil {
		t.Fatal(err)
	}
	a, err := as.Get("a1")
	if err != nil {
		t.Fatal(err)
	}
	if a.Alias != "Friendly" || a.Notes != "hello notes" {
		t.Fatalf("alias=%q notes=%q", a.Alias, a.Notes)
	}
	// Update must NOT clobber alias/notes (owned columns).
	a.Model = "opus"
	if err := as.Update(a); err != nil {
		t.Fatal(err)
	}
	a2, _ := as.Get("a1")
	if a2.Alias != "Friendly" || a2.Notes != "hello notes" {
		t.Fatalf("clobbered by Update: alias=%q notes=%q", a2.Alias, a2.Notes)
	}
}
