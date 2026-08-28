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
	callResult any
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
func (d *fakeDaemon) Call(_ context.Context, method, path string, body, result any) error {
	d.calls = append(d.calls, daemonCall{method: method, path: path, body: body})
	if result != nil && d.callResult != nil {
		encoded, _ := json.Marshal(d.callResult)
		_ = json.Unmarshal(encoded, result)
	}
	return nil
}

func TestChatSetupRequiresForumSupergroupAndTopicPermission(t *testing.T) {
	chatType, forum, canManage := "supergroup", true, true
	created := []string{}
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
			name, _ := body["name"].(string)
			created = append(created, name)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_thread_id": len(created) + 8, "name": name}})
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
	daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
	server := NewServer(state, NewBotClient(api.URL), daemon)
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
	if len(created) != 2 || created[0] != "tariboyd" || created[1] != "worker" {
		t.Fatalf("created topics = %q", created)
	}
	got := state.Snapshot()
	if got.ChatID != -100123 || got.ManagementTopicID != 9 || got.AgentTopics["worker"].ThreadID != 10 {
		t.Fatalf("state = %+v", got)
	}
	if len(daemon.subscribed) != 1 || daemon.subscribed[0] != "worker=chat:telegram:worker" {
		t.Fatalf("subscriptions = %v", daemon.subscribed)
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

func TestChatSetupPreservesWorkingBindingAndResumesPendingTopics(t *testing.T) {
	failWorker := true
	created := []string{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": 77}})
		case strings.HasSuffix(r.URL.Path, "/getChat"):
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": -100222, "type": "supergroup", "is_forum": true}})
		case strings.HasSuffix(r.URL.Path, "/getChatMember"):
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"status": "administrator", "can_manage_topics": true}})
		case strings.HasSuffix(r.URL.Path, "/createForumTopic"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			name := body["name"].(string)
			created = append(created, name)
			if name == "worker" && failWorker {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error_code": 500})
				return
			}
			threadID := 20
			if name == "worker" {
				threadID = 21
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_thread_id": threadID, "name": name}})
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
	if err := state.BindChat(-100111, 7); err != nil {
		t.Fatal(err)
	}
	if err := state.SetAgentTopic("old", "old", 8); err != nil {
		t.Fatal(err)
	}
	server := NewServer(state, NewBotClient(api.URL), &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}})
	setup := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/action", bytes.NewBufferString(`{"action":"chat_setup","chat_id":-100222}`)))
		return response
	}
	if response := setup(); response.Code == http.StatusOK {
		t.Fatal("setup unexpectedly succeeded while agent topic failed")
	}
	if got := state.Snapshot(); got.ChatID != -100111 || got.AgentTopics["old"].ThreadID != 8 {
		t.Fatalf("failed setup replaced working binding: %+v", got)
	}
	failWorker = false
	if response := setup(); response.Code != http.StatusOK {
		t.Fatalf("resumed setup status=%d body=%s", response.Code, response.Body.String())
	}
	if got := state.Snapshot(); got.ChatID != -100222 || got.ManagementTopicID != 20 || got.AgentTopics["worker"].ThreadID != 21 {
		t.Fatalf("resumed binding = %+v", got)
	}
	if strings.Join(created, ",") != "tariboyd,worker,worker" {
		t.Fatalf("created topics = %v", created)
	}
}
