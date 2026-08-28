package telegramplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeDaemon struct {
	agents     []AgentInfo
	subscribed []string
	published  []PublishedMessage
	calls      []daemonCall
}

type daemonCall struct {
	method, path string
	body         any
}

func (d *fakeDaemon) ListAgents(context.Context) ([]AgentInfo, error) { return d.agents, nil }
func (d *fakeDaemon) Subscribe(_ context.Context, agent, channel string) error {
	d.subscribed = append(d.subscribed, agent+"="+channel)
	return nil
}
func (d *fakeDaemon) Publish(_ context.Context, message PublishedMessage) error {
	d.published = append(d.published, message)
	return nil
}
func (d *fakeDaemon) Call(_ context.Context, method, path string, body, _ any) error {
	d.calls = append(d.calls, daemonCall{method: method, path: path, body: body})
	return nil
}

func TestChatSetupRequiresForumSupergroupAndTopicPermission(t *testing.T) {
	chatType, forum, canManage := "supergroup", true, true
	created := ""
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": 77, "username": "bot"}})
		case strings.HasSuffix(r.URL.Path, "/getChat"):
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": -100123, "type": chatType, "title": "Ops", "is_forum": forum}})
		case strings.HasSuffix(r.URL.Path, "/getChatMember"):
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"status": "administrator", "can_manage_topics": canManage}})
		case strings.HasSuffix(r.URL.Path, "/createForumTopic"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			created, _ = body["name"].(string)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_thread_id": 9, "name": created}})
		default:
			t.Fatalf("unexpected method %s", r.URL.Path)
		}
	}))
	defer api.Close()
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Configure("token", []int64{11}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(state, NewBotClient(api.URL))
	setup := func() *httptest.ResponseRecorder {
		body := bytes.NewBufferString(`{"action":"chat_setup","chat_id":-100123}`)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/action", body))
		return response
	}

	chatType = "group"
	if response := setup(); response.Code == http.StatusOK {
		t.Fatal("plain group accepted")
	}
	chatType, forum = "supergroup", false
	if response := setup(); response.Code == http.StatusOK {
		t.Fatal("non-forum supergroup accepted")
	}
	forum, canManage = true, false
	if response := setup(); response.Code == http.StatusOK {
		t.Fatal("bot without topic permission accepted")
	}
	canManage = true
	response := setup()
	if response.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", response.Code, response.Body.String())
	}
	if created != "tariboyd" {
		t.Fatalf("created topic = %q", created)
	}
	got := state.Snapshot()
	if got.ChatID != -100123 || got.ManagementTopicID != 9 {
		t.Fatalf("state = %+v", got)
	}
}

func TestReconcileCreatesMissingAgentTopicsAndRetainsDeletedMappings(t *testing.T) {
	created := []string{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/createForumTopic") {
			t.Fatalf("unexpected method %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		name := body["name"].(string)
		created = append(created, name)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_thread_id": len(created) + 20, "name": name}})
	}))
	defer api.Close()
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Configure("token", []int64{11}); err != nil {
		t.Fatal(err)
	}
	if err := state.BindChat(-100123, 9); err != nil {
		t.Fatal(err)
	}
	daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}, {Name: "analyst"}}}
	server := NewServer(state, NewBotClient(api.URL), daemon)
	if err := server.ReconcileTopics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || len(state.Snapshot().AgentTopics) != 2 || len(daemon.subscribed) != 2 {
		t.Fatalf("created=%v topics=%v subscriptions=%v", created, state.Snapshot().AgentTopics, daemon.subscribed)
	}
	daemon.agents = []AgentInfo{{Name: "worker"}}
	if err := server.ReconcileTopics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || len(state.Snapshot().AgentTopics) != 2 {
		t.Fatalf("deleted mapping was removed: created=%v topics=%v", created, state.Snapshot().AgentTopics)
	}
}
