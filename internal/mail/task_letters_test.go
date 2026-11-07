package mail_test

import (
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/mail"
)

func aboutTask(key string) map[string]any { return map[string]any{"task_key": key} }

func TestOpenTaskKeysFollowRenderOrderWithoutSideEffects(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	n1 := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "n", Data: aboutTask("DEV-1")})
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "plain"})
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "r", NeedsReply: true, Data: aboutTask("DEV-2")})
	f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:bob", Body: "not alice's", Data: aboutTask("DEV-9")})

	keys, err := f.m.OpenTaskKeys("alice")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"DEV-2", "DEV-1", ""}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v; want %v", keys, want)
	}
	if got := f.show(t, "agent:alice", n1.ID); got.Attempts != 0 {
		t.Fatalf("reading the keys counted an attempt: %+v", got)
	}
}

func TestRenderIterationTaskRendersOnlyThatTask(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	a := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "a", Data: aboutTask("DEV-1")})
	b := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "b", NeedsReply: true, Data: aboutTask("DEV-2")})
	plain := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "plain"})

	batch, err := f.m.RenderIterationTask("alice", "it-1", "DEV-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(batch), []string{a.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("batch = %v; want %v", got, want)
	}
	for _, id := range []string{b.ID, plain.ID} {
		if got := f.show(t, "agent:alice", id); got.Attempts != 0 || !got.Open() {
			t.Fatalf("a letter of another task was touched: %+v", got)
		}
	}
	batch, err = f.m.RenderIterationTask("alice", "it-2", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(batch), []string{plain.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("letters without a task = %v; want %v", got, want)
	}
}

func TestInstantTakeForTaskHoldsLettersOfAnotherTask(t *testing.T) {
	t.Parallel()
	f := newInstantFixture(t)
	own := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "own", Instant: true, Data: aboutTask("DEV-1")})
	plain := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "plain", Instant: true})
	other := f.sendOne(t, mail.Letter{From: "user:customer", To: "agent:alice", Body: "other", Instant: true, Data: aboutTask("DEV-2")})

	got, err := f.m.TakeInstantForTask("alice", "it-1", "DEV-1")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{own.ID, plain.ID}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("taken = %v; want %v", ids(got), want)
	}
	if stored := f.show(t, "agent:alice", other.ID); stored.Attempts != 0 || stored.InstantState == mail.InstantDelivered {
		t.Fatalf("the letter of another task was delivered: %+v", stored)
	}
}
