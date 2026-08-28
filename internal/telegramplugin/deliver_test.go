package telegramplugin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
