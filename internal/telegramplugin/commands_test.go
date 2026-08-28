package telegramplugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTopicCommandsHelpAndLifecycleUseDaemonAPI(t *testing.T) {
	replies := []map[string]any{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Fatalf("unexpected Bot API method %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		replies = append(replies, body)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_id": len(replies)}})
	}))
	defer api.Close()
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
	server := NewServer(state, NewBotClient(api.URL), daemon)
	update := func(id, thread int64, text string) {
		err := server.ProcessUpdate(context.Background(), Update{UpdateID: id, Message: &TelegramMessage{
			From: &TelegramUser{ID: 11}, Chat: TelegramChat{ID: -100123, Type: "supergroup"},
			MessageThreadID: thread, Text: text,
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	update(1, 7, "/help@tariboy_bot")
	if len(replies) != 1 || !strings.Contains(replies[0]["text"].(string), "/agent create NAME IMAGE") || !strings.Contains(replies[0]["text"].(string), "/task status") {
		t.Fatalf("management help = %#v", replies)
	}
	update(2, 7, "/start worker")
	update(3, 9, "/start")
	if len(daemon.calls) != 2 || daemon.calls[0].path != "/api/agents/worker/start" || daemon.calls[1].path != "/api/agents/worker/start" {
		t.Fatalf("daemon calls = %#v", daemon.calls)
	}
}
