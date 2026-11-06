package telegramplugin

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func routedMessage(thread int64, text string) *TelegramMessage {
	return &TelegramMessage{
		From: &TelegramUser{ID: 11}, Chat: TelegramChat{ID: -100123, Type: "supergroup"},
		MessageThreadID: thread, Text: text,
	}
}

func newRoutingServer(t *testing.T) (*Server, *scriptedDaemon, *recordingBot, *StateStore) {
	t.Helper()
	state := configuredState(t)
	daemon := &scriptedDaemon{fakeDaemon: &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}}
	bot := &recordingBot{}
	return NewServer(state, bot, daemon), daemon, bot, state
}

func TestProcessUpdateIgnoresStaleUpdateAndAcceptsCurrentOffset(t *testing.T) {
	server, daemon, _, state := newRoutingServer(t)
	if err := state.SetOffset(50); err != nil {
		t.Fatal(err)
	}
	if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 49, Message: routedMessage(9, "old")}); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().Offset != 50 || len(daemon.published) != 0 {
		t.Fatalf("stale update: offset=%d published=%#v", state.Snapshot().Offset, daemon.published)
	}
	if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 50, Message: routedMessage(9, "new")}); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().Offset != 51 || len(daemon.published) != 1 {
		t.Fatalf("current update: offset=%d published=%#v", state.Snapshot().Offset, daemon.published)
	}
}

func TestProcessUpdateChecksAllowlistBeforeRunningCommands(t *testing.T) {
	command := func(change func(*TelegramMessage)) func() *TelegramMessage {
		return func() *TelegramMessage {
			message := routedMessage(7, "/agents")
			change(message)
			return message
		}
	}
	for name, tc := range map[string]struct {
		message    func() *TelegramMessage
		authorized bool
	}{
		"no message":    {message: func() *TelegramMessage { return nil }},
		"no sender":     {message: command(func(m *TelegramMessage) { m.From = nil })},
		"below list":    {message: command(func(m *TelegramMessage) { m.From.ID = 4 })},
		"between uids":  {message: command(func(m *TelegramMessage) { m.From.ID = 6 })},
		"above list":    {message: command(func(m *TelegramMessage) { m.From.ID = 21 })},
		"other chat":    {message: command(func(m *TelegramMessage) { m.Chat.ID = -100999 })},
		"first allowed": {message: command(func(m *TelegramMessage) { m.From.ID = 5 }), authorized: true},
		"last allowed":  {message: command(func(m *TelegramMessage) { m.From.ID = 20 }), authorized: true},
	} {
		t.Run(name, func(t *testing.T) {
			server, daemon, bot, state := newRoutingServer(t)
			if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: tc.message()}); err != nil {
				t.Fatal(err)
			}
			if state.Snapshot().Offset != 2 {
				t.Fatalf("offset = %d", state.Snapshot().Offset)
			}
			if tc.authorized {
				if len(daemon.calls) != 1 || len(bot.sent) != 1 {
					t.Fatalf("calls=%#v sent=%#v", daemon.calls, bot.sent)
				}
				return
			}
			if len(daemon.calls) != 0 || len(bot.calls) != 0 || daemon.listCalls != 0 {
				t.Fatalf("denied message reached collaborators: calls=%#v bot=%#v list=%d", daemon.calls, bot.calls, daemon.listCalls)
			}
		})
	}
}

func TestProcessUpdateEmptyAllowlistDeniesManagementCommand(t *testing.T) {
	server, daemon, bot, state := newRoutingServer(t)
	if err := state.Configure("", nil); err != nil {
		t.Fatal(err)
	}
	if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(7, "/agents")}); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().Offset != 2 || len(daemon.calls) != 0 || len(bot.calls) != 0 {
		t.Fatalf("offset=%d calls=%#v bot=%#v", state.Snapshot().Offset, daemon.calls, bot.calls)
	}
}

func TestProcessUpdateManagementTopic(t *testing.T) {
	t.Run("forwarded command is dropped", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		message := routedMessage(7, "/agents")
		message.ForwardOrigin = &MessageOrigin{Type: "hidden_user"}
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: message}); err != nil {
			t.Fatal(err)
		}
		if state.Snapshot().Offset != 2 || len(daemon.calls) != 0 || len(bot.calls) != 0 || len(daemon.published) != 0 {
			t.Fatalf("offset=%d calls=%#v bot=%#v published=%#v", state.Snapshot().Offset, daemon.calls, bot.calls, daemon.published)
		}
	})
	t.Run("plain text is dropped", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(7, "hello")}); err != nil {
			t.Fatal(err)
		}
		if state.Snapshot().Offset != 2 || len(daemon.calls) != 0 || len(bot.calls) != 0 || len(daemon.published) != 0 {
			t.Fatalf("offset=%d calls=%#v bot=%#v published=%#v", state.Snapshot().Offset, daemon.calls, bot.calls, daemon.published)
		}
	})
	t.Run("failed reply keeps the offset", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		sendErr := errors.New("telegram down")
		bot.fail = map[string]error{"send": sendErr}
		err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(7, "/agents")})
		if !errors.Is(err, sendErr) || state.Snapshot().Offset != 0 || len(daemon.calls) != 1 {
			t.Fatalf("err=%v offset=%d calls=%#v", err, state.Snapshot().Offset, daemon.calls)
		}
	})
	t.Run("failed command is answered and advances", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		daemon.callErr = errors.New("daemon down")
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(7, "/agents")}); err != nil {
			t.Fatal(err)
		}
		want := []recordedSend{{chatID: -100123, threadID: 7, text: "Error: daemon down"}}
		if state.Snapshot().Offset != 2 || !reflect.DeepEqual(bot.sent, want) {
			t.Fatalf("offset=%d sent=%#v", state.Snapshot().Offset, bot.sent)
		}
	})
}

func TestProcessUpdateAgentTopicDiscardsWithoutRecipient(t *testing.T) {
	t.Run("unknown thread never lists agents", func(t *testing.T) {
		server, daemon, _, state := newRoutingServer(t)
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(99, "/stop")}); err != nil {
			t.Fatal(err)
		}
		if state.Snapshot().Offset != 2 || daemon.listCalls != 0 || len(daemon.calls) != 0 {
			t.Fatalf("offset=%d list=%d calls=%#v", state.Snapshot().Offset, daemon.listCalls, daemon.calls)
		}
	})
	t.Run("empty text never lists agents", func(t *testing.T) {
		server, daemon, _, state := newRoutingServer(t)
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "")}); err != nil {
			t.Fatal(err)
		}
		if state.Snapshot().Offset != 2 || daemon.listCalls != 0 {
			t.Fatalf("offset=%d list=%d", state.Snapshot().Offset, daemon.listCalls)
		}
	})
	t.Run("no daemon", func(t *testing.T) {
		state := configuredState(t)
		bot := &recordingBot{}
		server := NewServer(state, bot)
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "/stop")}); err != nil {
			t.Fatal(err)
		}
		if state.Snapshot().Offset != 2 || len(bot.calls) != 0 {
			t.Fatalf("offset=%d bot=%#v", state.Snapshot().Offset, bot.calls)
		}
	})
	t.Run("agent gone does not run its commands", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		daemon.agents = []AgentInfo{{Name: "other"}}
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "/stop")}); err != nil {
			t.Fatal(err)
		}
		if state.Snapshot().Offset != 2 || len(daemon.calls) != 0 || len(bot.calls) != 0 || len(daemon.published) != 0 {
			t.Fatalf("offset=%d calls=%#v bot=%#v published=%#v", state.Snapshot().Offset, daemon.calls, bot.calls, daemon.published)
		}
	})
	t.Run("list failure keeps the offset", func(t *testing.T) {
		server, daemon, _, state := newRoutingServer(t)
		daemon.listErr = errors.New("daemon down")
		err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "hello")})
		if !errors.Is(err, daemon.listErr) || state.Snapshot().Offset != 0 || len(daemon.published) != 0 {
			t.Fatalf("err=%v offset=%d published=%#v", err, state.Snapshot().Offset, daemon.published)
		}
	})
}

func TestProcessUpdateAgentTopicCommands(t *testing.T) {
	t.Run("command is scoped to the topic and is not published", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "/stop")}); err != nil {
			t.Fatal(err)
		}
		if len(daemon.calls) != 1 || daemon.calls[0].path != "/api/agents/worker/stop" || len(daemon.published) != 0 {
			t.Fatalf("calls=%#v published=%#v", daemon.calls, daemon.published)
		}
		want := []recordedSend{{chatID: -100123, threadID: 9, text: "OK"}}
		if state.Snapshot().Offset != 2 || !reflect.DeepEqual(bot.sent, want) {
			t.Fatalf("offset=%d sent=%#v", state.Snapshot().Offset, bot.sent)
		}
	})
	t.Run("failed reply keeps the offset and does not publish", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		bot.fail = map[string]error{"send": errors.New("telegram down")}
		err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "/stop")})
		if !errors.Is(err, bot.fail["send"]) || state.Snapshot().Offset != 0 || len(daemon.published) != 0 {
			t.Fatalf("err=%v offset=%d published=%#v", err, state.Snapshot().Offset, daemon.published)
		}
	})
	t.Run("command in a caption stays text", func(t *testing.T) {
		server, daemon, _, state := newRoutingServer(t)
		message := routedMessage(9, "")
		message.Caption = "/stop"
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: message}); err != nil {
			t.Fatal(err)
		}
		if len(daemon.calls) != 0 || len(daemon.published) != 1 || daemon.published[0].Text != "/stop" || state.Snapshot().Offset != 2 {
			t.Fatalf("calls=%#v published=%#v offset=%d", daemon.calls, daemon.published, state.Snapshot().Offset)
		}
	})
}

func TestProcessUpdateSendFailures(t *testing.T) {
	inboxFull := &DaemonError{Code: "inbox_full", Message: "full"}
	t.Run("inbox full tells the topic and advances", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		daemon.sendErr = inboxFull
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "hello")}); err != nil {
			t.Fatal(err)
		}
		want := []recordedSend{{chatID: -100123, threadID: 9, text: "The inbox of worker is full; this message was not delivered. Try again later."}}
		if state.Snapshot().Offset != 2 || !reflect.DeepEqual(bot.sent, want) {
			t.Fatalf("offset=%d sent=%#v", state.Snapshot().Offset, bot.sent)
		}
	})
	t.Run("inbox full without a bot advances", func(t *testing.T) {
		state := configuredState(t)
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}, sendErr: inboxFull}
		server := NewServer(state, nil, daemon)
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "hello")}); err != nil {
			t.Fatal(err)
		}
		if state.Snapshot().Offset != 2 {
			t.Fatalf("offset = %d", state.Snapshot().Offset)
		}
	})
	t.Run("failed inbox notice keeps the offset", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		daemon.sendErr = inboxFull
		bot.fail = map[string]error{"send": errors.New("telegram down")}
		err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "hello")})
		if !errors.Is(err, bot.fail["send"]) || state.Snapshot().Offset != 0 {
			t.Fatalf("err=%v offset=%d", err, state.Snapshot().Offset)
		}
	})
	t.Run("other send error keeps the offset", func(t *testing.T) {
		server, daemon, bot, state := newRoutingServer(t)
		daemon.sendErr = errors.New("daemon down")
		err := server.ProcessUpdate(context.Background(), Update{UpdateID: 1, Message: routedMessage(9, "hello")})
		if !errors.Is(err, daemon.sendErr) || state.Snapshot().Offset != 0 || len(bot.calls) != 0 {
			t.Fatalf("err=%v offset=%d bot=%#v", err, state.Snapshot().Offset, bot.calls)
		}
	})
}
