package telegramplugin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeliverSendsAgentReplyToMappedTopicAndSplitsLongText(t *testing.T) {
	sent := []map[string]any{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Fatalf("unexpected method %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent = append(sent, body)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_id": len(sent)}})
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
	server := NewServer(state, NewBotClient(api.URL))
	body, _ := json.Marshal(map[string]any{"message": map[string]any{
		"id": "m1", "channel": "chat:telegram:worker", "text": strings.Repeat("x", 4100),
		"produced_by_agent": "worker",
	}})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/deliver", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("deliver status=%d body=%s", response.Code, response.Body.String())
	}
	if len(sent) != 2 || sent[0]["message_thread_id"] != float64(9) || len(sent[0]["text"].(string)) != 4096 {
		t.Fatalf("sent = %#v", sent)
	}
}

func TestDeliverHonorsTelegramRetryAfter(t *testing.T) {
	attempts := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"ok": false, "error_code": 429, "parameters": map[string]any{"retry_after": 1},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_id": 2}})
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
	server := NewServer(state, NewBotClient(api.URL))
	body := bytes.NewBufferString(`{"message":{"channel":"chat:telegram:worker","text":"hello","produced_by_agent":"worker"}}`)
	response := httptest.NewRecorder()
	started := time.Now()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/deliver", body))
	if response.Code != http.StatusOK || attempts != 2 || time.Since(started) < 900*time.Millisecond {
		t.Fatalf("status=%d attempts=%d elapsed=%s", response.Code, attempts, time.Since(started))
	}
}

func TestDeliverIgnoresInboundPluginEcho(t *testing.T) {
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(state, nil)
	body := bytes.NewBufferString(`{"message":{"channel":"chat:telegram:worker","text":"inbound","produced_by_plugin":"telegram"}}`)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/deliver", body))
	if response.Code != http.StatusOK {
		t.Fatalf("echo should be acknowledged, status=%d", response.Code)
	}
}

func TestDeliverReplacesMissingAgentTopicAndRetriesOnce(t *testing.T) {
	sentThreads := []int64{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			thread := int64(body["message_thread_id"].(float64))
			sentThreads = append(sentThreads, thread)
			if len(sentThreads) == 1 {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"ok": false, "error_code": 400, "description": "Bad Request: message thread not found",
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_id": 2}})
		case strings.HasSuffix(r.URL.Path, "/createForumTopic"):
			if body["name"] != "worker" {
				t.Fatalf("replacement name = %#v", body["name"])
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_thread_id": 17, "name": "worker"}})
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
	if err := state.BindChat(-100123, 7); err != nil {
		t.Fatal(err)
	}
	if err := state.SetAgentTopic("worker", "worker", 9); err != nil {
		t.Fatal(err)
	}
	server := NewServer(state, NewBotClient(api.URL))
	body, _ := json.Marshal(map[string]any{"message": map[string]any{
		"id": "m1", "channel": "chat:telegram:worker", "text": "hello", "produced_by_agent": "worker",
	}})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/deliver", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("deliver status=%d body=%s", response.Code, response.Body.String())
	}
	if len(sentThreads) != 2 || sentThreads[0] != 9 || sentThreads[1] != 17 {
		t.Fatalf("sent threads = %v", sentThreads)
	}
	if got := state.Snapshot().AgentTopics["worker"].ThreadID; got != 17 {
		t.Fatalf("stored replacement thread = %d", got)
	}
}
