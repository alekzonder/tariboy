package telegramplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// roundTripDirectory knows agent worker with an inbox cap of 1, the customer
// and the plugin telegram.
type roundTripDirectory struct{}

func (roundTripDirectory) AgentExists(name string) (bool, error)     { return name == "worker", nil }
func (roundTripDirectory) InboxCap(string) (int, error)              { return 1, nil }
func (roundTripDirectory) CustomerLogin() string                     { return "customer" }
func (roundTripDirectory) PluginInstalled(name string) (bool, error) { return name == "telegram", nil }
func (d roundTripDirectory) PluginReceivesMail(name string) (bool, error) {
	return d.PluginInstalled(name)
}
func (roundTripDirectory) QueueExists(string) (bool, error)            { return false, nil }
func (roundTripDirectory) GroupMembers(string) ([]string, bool, error) { return nil, false, nil }

// TestTelegramInboundSendsLetterAndOutboundDelivers: a topic message reaches
// the agent as a request from plugin:telegram through the real plugin API, the
// agent's reply goes back to plugin:telegram, and its delivery lands in the
// agent's topic. With the inbox at its cap the next topic message is told so
// in the topic and polling moves on.
func TestTelegramInboundSendsLetterAndOutboundDelivers(t *testing.T) {
	st := storetest.Open(t)
	m := mail.New(st, nil, roundTripDirectory{})
	tokens := plugins.NewTokenRegistry(nil)
	token, err := tokens.Mint(plugins.Identity{Name: "telegram", Send: true})
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/plugin/", plugins.NewAPI(tokens, m, nil, nil))
	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"agents": []any{map[string]any{"name": "worker"}}}})
	})
	daemonServer := &http.Server{Handler: mux}
	go daemonServer.Serve(listener)
	t.Cleanup(func() { daemonServer.Shutdown(context.Background()) })

	var sent []map[string]any
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent = append(sent, body)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_id": len(sent)}})
	}))
	defer bot.Close()
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
	server := NewServer(state, NewBotClient(bot.URL), NewDaemonClient(socket, token))
	topicMessage := func(id int64, text string) {
		t.Helper()
		message := TelegramMessage{From: &TelegramUser{ID: 11}, Chat: TelegramChat{ID: -100123, Type: "supergroup"},
			MessageThreadID: 9, Text: text}
		if err := server.ProcessUpdate(context.Background(), Update{UpdateID: id, Message: &message}); err != nil {
			t.Fatal(err)
		}
	}

	topicMessage(1, "deploy please")
	inbox, _, err := m.List(mail.BoxInbox, "agent:worker", 10, "")
	if err != nil || len(inbox) != 1 {
		t.Fatalf("worker inbox = %+v, %v", inbox, err)
	}
	in := inbox[0]
	if in.From != "plugin:telegram" || !in.NeedsReply || in.Body != "deploy please" || in.Data["telegram_update_id"] != float64(1) {
		t.Fatalf("inbound letter = %+v", in)
	}

	// The inbox is at its cap of one: the next message is stored dead and the
	// topic is told.
	topicMessage(2, "and this")
	if state.Snapshot().Offset != 3 || len(sent) != 1 || !strings.Contains(sent[0]["text"].(string), "inbox of worker is full") {
		t.Fatalf("offset = %d, bot messages = %#v", state.Snapshot().Offset, sent)
	}

	if _, _, err := m.Reply(mail.ReplyRequest{As: "agent:worker", ID: in.ID, Text: "deployed"}); err != nil {
		t.Fatal(err)
	}
	out, err := m.DrainBatch("plugin:telegram", 10)
	if err != nil || len(out) != 1 {
		t.Fatalf("telegram inbox = %+v, %v", out, err)
	}
	body, _ := json.Marshal(map[string]any{"letter": out[0]})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/deliver", bytes.NewReader(body)))
	if response.Code != http.StatusOK || len(sent) != 2 || sent[1]["text"] != "deployed" || sent[1]["message_thread_id"] != float64(9) {
		t.Fatalf("deliver status = %d, bot messages = %#v", response.Code, sent)
	}
}
