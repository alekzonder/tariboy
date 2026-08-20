package agent

import (
	"testing"
)

func TestGroupColumnRoundTrip(t *testing.T) {
	s := openStore(t)
	if err := s.Create(Agent{Name: "scout", ImageRef: "img:latest", Group: "research"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Agent{Name: "writer", ImageRef: "img:latest"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("scout")
	if err != nil || got.Group != "research" {
		t.Fatalf("scout group = %q err=%v", got.Group, err)
	}
	// SetGroup joins writer; Update must NOT clobber the group column.
	if err := s.SetGroup("writer", "research"); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Get("writer")
	w.Model = "opus"
	if err := s.Update(w); err != nil {
		t.Fatal(err)
	}
	w2, _ := s.Get("writer")
	if w2.Group != "research" || w2.Model != "opus" {
		t.Fatalf("after update writer = %+v", w2)
	}
	members, err := s.ListByGroup("research")
	if err != nil || len(members) != 2 || members[0].Name != "scout" || members[1].Name != "writer" {
		t.Fatalf("members = %+v err=%v", members, err)
	}
	// Leaving clears the column.
	if err := s.SetGroup("writer", ""); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.ListByGroup("research"); len(m) != 1 {
		t.Fatalf("after leave members = %+v", m)
	}
}
