package bus

import "testing"

func woken(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestWakeSignalsEachRecipientOncePerBurst(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	sink, other := b.Wake("plugin:sink"), b.Wake("plugin:other")
	if _, err := b.Subscribe("plugin:sink", "chat:in", Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := b.Publish(Message{Channel: "chat:in", Type: "chat.msg", Text: "hi"}); err != nil {
			t.Fatal(err)
		}
	}
	if !woken(sink) {
		t.Fatal("recipient was not woken by publish")
	}
	if woken(sink) {
		t.Fatal("wake did not coalesce a burst of publishes into one signal")
	}
	if woken(other) {
		t.Fatal("non-recipient was woken")
	}
	if b.Wake("plugin:sink") != sink {
		t.Fatal("Wake returned a different channel for the same subscriber")
	}
}

func TestWakeSignalsRequeueAndAnnounce(t *testing.T) {
	t.Parallel()
	b := newBus(t)
	wake := b.Wake("plugin:sink")
	if _, err := b.Subscribe("plugin:sink", "chat:in", Matcher{}, nil); err != nil {
		t.Fatal(err)
	}
	msg, err := b.Publish(Message{Channel: "chat:in", Type: "chat.msg", Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	woken(wake)
	if err := b.Ack("plugin:sink", []string{msg.ID}); err != nil {
		t.Fatal(err)
	}
	if woken(wake) {
		t.Fatal("ack woke the subscriber")
	}
	if err := b.Requeue("plugin:sink", msg.ID); err != nil {
		t.Fatal(err)
	}
	if !woken(wake) {
		t.Fatal("requeue did not wake the subscriber")
	}
	if err := b.Announce("plugin:sink", "chat:in"); err != nil {
		t.Fatal(err)
	}
	if !woken(wake) {
		t.Fatal("announce redelivery did not wake the subscriber")
	}
}
