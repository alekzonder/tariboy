package agent

import (
	"errors"
	"testing"
)

func tagStore(t *testing.T) *Store {
	t.Helper()
	s := openStore(t)
	if err := s.Create(Agent{Name: "worker", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Agent{Name: "other", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	for _, it := range []Iteration{
		{ID: "a", Agent: "worker", Status: "done", StartedAt: "2026-09-10T02:00:00Z"},
		{ID: "b", Agent: "worker", Status: "done", StartedAt: "2026-09-11T02:00:00Z"},
		{ID: "c", Agent: "worker", Status: "done", StartedAt: "2026-09-12T02:00:00Z"},
		{ID: "z", Agent: "other", Status: "done", StartedAt: "2026-09-11T02:00:00Z"},
	} {
		if err := s.CreateIteration(it); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestIterationTagsAddRemoveSetAreBatchedAndIdempotent(t *testing.T) {
	s := tagStore(t)

	got, err := s.MutateIterationTags("worker", TagOpAdd, []string{"a", "b"}, []string{"processed", "review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got["a"]) != 2 || got["a"][0] != "processed" || got["a"][1] != "review" {
		t.Fatalf("add a = %v, want sorted [processed review]", got["a"])
	}
	if len(got["b"]) != 2 {
		t.Fatalf("add b = %v", got["b"])
	}

	// Re-adding the same tag is a no-op, not an error.
	if got, err = s.MutateIterationTags("worker", TagOpAdd, []string{"a"}, []string{"processed"}); err != nil {
		t.Fatal(err)
	}
	if len(got["a"]) != 2 {
		t.Fatalf("re-add a = %v, want 2 tags", got["a"])
	}

	if got, err = s.MutateIterationTags("worker", TagOpRemove, []string{"a", "b"}, []string{"review"}); err != nil {
		t.Fatal(err)
	}
	if len(got["a"]) != 1 || got["a"][0] != "processed" {
		t.Fatalf("remove a = %v", got["a"])
	}

	// set replaces the whole set; an empty tag list clears it.
	if got, err = s.MutateIterationTags("worker", TagOpSet, []string{"a"}, []string{"fresh"}); err != nil {
		t.Fatal(err)
	}
	if len(got["a"]) != 1 || got["a"][0] != "fresh" {
		t.Fatalf("set a = %v", got["a"])
	}
	if got, err = s.MutateIterationTags("worker", TagOpSet, []string{"a"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(got["a"]) != 0 {
		t.Fatalf("clear a = %v", got["a"])
	}
}

func TestIterationTagsRejectForeignIDsWithoutWriting(t *testing.T) {
	s := tagStore(t)
	if _, err := s.MutateIterationTags("worker", TagOpAdd, []string{"a", "z"}, []string{"processed"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign id = %v, want ErrNotFound", err)
	}
	tags, err := s.IterationTags([]string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags["a"]) != 0 {
		t.Fatalf("partial write leaked tags: %v", tags)
	}
}

func TestIterationTagsInvalidTagRejected(t *testing.T) {
	s := tagStore(t)
	for _, bad := range []string{"", "   ", "a,b", "a b", "a\tb", string(make([]byte, 65))} {
		if _, err := s.MutateIterationTags("worker", TagOpAdd, []string{"a"}, []string{bad}); !errors.Is(err, ErrInvalidTag) {
			t.Fatalf("tag %q = %v, want ErrInvalidTag", bad, err)
		}
	}
}

func TestIterationTagsDeletedWithTheirIteration(t *testing.T) {
	s := tagStore(t)
	if _, err := s.MutateIterationTags("worker", TagOpAdd, []string{"a"}, []string{"processed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM iterations WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM iteration_tags WHERE iteration_id='a'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("orphan tag rows = %d, want 0", n)
	}
}

func TestFindIterationsFiltersByTagAndStartDate(t *testing.T) {
	s := tagStore(t)
	if _, err := s.MutateIterationTags("worker", TagOpAdd, []string{"a"}, []string{"processed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MutateIterationTags("worker", TagOpAdd, []string{"c"}, []string{"skipped"}); err != nil {
		t.Fatal(err)
	}

	ids := func(f IterationFilter) []string {
		t.Helper()
		its, err := s.FindIterations("worker", f)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(its))
		for _, it := range its {
			out = append(out, it.ID)
		}
		return out
	}

	if got := ids(IterationFilter{Tags: []string{"processed"}}); len(got) != 1 || got[0] != "a" {
		t.Fatalf("by tag = %v", got)
	}
	// Several tags match ANY of them.
	if got := ids(IterationFilter{Tags: []string{"processed", "skipped"}}); len(got) != 2 {
		t.Fatalf("by two tags = %v", got)
	}
	// Date-only bounds cover the whole named day, inclusive at both ends.
	if got := ids(IterationFilter{StartedAfter: "2026-09-11", StartedBefore: "2026-09-11"}); len(got) != 1 || got[0] != "b" {
		t.Fatalf("by day = %v", got)
	}
	if got := ids(IterationFilter{StartedAfter: "2026-09-11T02:00:00Z"}); len(got) != 2 {
		t.Fatalf("by rfc3339 after = %v", got)
	}
	// Tag and date compose.
	if got := ids(IterationFilter{Tags: []string{"processed"}, StartedAfter: "2026-09-11"}); len(got) != 0 {
		t.Fatalf("tag+date = %v", got)
	}
	if _, err := s.FindIterations("worker", IterationFilter{StartedAfter: "yesterday"}); !errors.Is(err, ErrInvalidTime) {
		t.Fatalf("bad time = %v, want ErrInvalidTime", err)
	}
}
