package telegramplugin

import (
	"context"
	"testing"
)

func TestProcessUpdateDenyAllAdvancesOffsetWithoutPublishing(t *testing.T) {
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Configure("token", nil); err != nil {
		t.Fatal(err)
	}
	daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
	server := NewServer(state, nil, daemon)
	update := Update{UpdateID: 10, Message: &TelegramMessage{
		From: &TelegramUser{ID: 11}, Chat: TelegramChat{ID: -100123, Type: "supergroup"},
		MessageThreadID: 9, Text: "hello",
	}}
	if err := server.ProcessUpdate(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().Offset != 11 || len(daemon.published) != 0 {
		t.Fatalf("offset=%d published=%v", state.Snapshot().Offset, daemon.published)
	}
}

func TestProcessUpdatePublishesOnlyAuthorizedLiveAgentTopic(t *testing.T) {
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Configure("token", []int64{11}); err != nil {
		t.Fatal(err)
	}
	if err := state.BindChat(-100123, 7); err != nil {
		t.Fatal(err)
	}
	if err := state.SetAgentTopic("worker", "worker", 9); err != nil {
		t.Fatal(err)
	}
	daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
	server := NewServer(state, nil, daemon)
	message := TelegramMessage{
		From: &TelegramUser{ID: 11}, Chat: TelegramChat{ID: -100123, Type: "supergroup"},
		MessageThreadID: 9, Text: "hello agent",
	}
	for id, mutate := range []func(*TelegramMessage){
		func(m *TelegramMessage) { m.From.ID = 99 },
		func(m *TelegramMessage) { m.Chat.ID = -100999 },
		func(m *TelegramMessage) { m.MessageThreadID = 99 },
	} {
		copy := message
		copy.From = &TelegramUser{ID: message.From.ID}
		mutate(&copy)
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: int64(id + 1), Message: &copy}); err != nil {
			t.Fatal(err)
		}
	}
	if len(daemon.published) != 0 {
		t.Fatalf("unauthorized messages published: %#v", daemon.published)
	}
	if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 20, Message: &message}); err != nil {
		t.Fatal(err)
	}
	if len(daemon.published) != 1 {
		t.Fatalf("published = %#v", daemon.published)
	}
	got := daemon.published[0]
	if got.Channel != "chat:telegram:worker" || got.Text != "hello agent" || got.UpdateID != 20 {
		t.Fatalf("published message = %#v", got)
	}
}
