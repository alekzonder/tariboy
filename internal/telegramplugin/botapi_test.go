package telegramplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotClientGetMeAndRedactedUnauthorizedError(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "bad-token") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error_code": 401, "description": "bad-token rejected"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": 77, "username": "tariboy_bot"}})
	}))
	defer api.Close()
	client := NewBotClient(api.URL)
	bot, err := client.GetMe(context.Background(), "good-token")
	if err != nil || bot.ID != 77 || bot.Username != "tariboy_bot" {
		t.Fatalf("bot=%+v err=%v", bot, err)
	}
	_, err = client.GetMe(context.Background(), "bad-token")
	if err == nil || strings.Contains(err.Error(), "bad-token") {
		t.Fatalf("unauthorized error is not redacted: %v", err)
	}
}

func TestConfigureActionValidatesBeforeReplacingToken(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "invalid") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error_code": 401})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"id": 77, "username": "bot"}})
	}))
	defer api.Close()
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(state, NewBotClient(api.URL))
	postAction := func(body map[string]any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader(encoded)))
		return response
	}
	response := postAction(map[string]any{"action": "configure", "token": "valid", "allowed_uids": []int64{22, 11}})
	if response.Code != http.StatusOK {
		t.Fatalf("configure status=%d body=%s", response.Code, response.Body.String())
	}
	if got := state.Snapshot(); got.Token != "valid" || len(got.AllowedUIDs) != 2 {
		t.Fatalf("configured state = %+v", got)
	}
	response = postAction(map[string]any{"action": "configure", "token": "invalid-secret-token", "allowed_uids": []int64{9}})
	if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "invalid-secret-token") {
		t.Fatalf("invalid token response status=%d body=%s", response.Code, response.Body.String())
	}
	if got := state.Snapshot(); got.Token != "valid" || len(got.AllowedUIDs) != 2 {
		t.Fatalf("failed validation changed state = %+v", got)
	}
}

func TestBotClientGetUpdatesReturnsRateLimitMetadata(t *testing.T) {
	limited := true
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limited {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"ok": false, "error_code": 429, "parameters": map[string]any{"retry_after": 3},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": []any{
			map[string]any{"update_id": 9, "message": map[string]any{
				"message_id": 1, "message_thread_id": 7, "from": map[string]any{"id": 11},
				"chat": map[string]any{"id": -100123, "type": "supergroup"}, "text": "hello",
			}},
		}})
	}))
	defer api.Close()
	client := NewBotClient(api.URL)
	_, err := client.GetUpdates(context.Background(), "token", 9)
	var botErr *BotError
	if !errors.As(err, &botErr) || botErr.Code != 429 || botErr.RetryAfter != 3 {
		t.Fatalf("rate limit error = %#v", err)
	}
	limited = false
	updates, err := client.GetUpdates(context.Background(), "token", 9)
	if err != nil || len(updates) != 1 || updates[0].UpdateID != 9 || updates[0].Message.Text != "hello" {
		t.Fatalf("updates=%+v err=%v", updates, err)
	}
}
