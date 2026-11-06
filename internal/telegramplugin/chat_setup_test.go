package telegramplugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	setupChatID  = -100222
	setupPayload = `{"action":"chat_setup","chat_id":-100222}`
)

func runChatSetup(t *testing.T, server *Server, payload string) (int, string) {
	t.Helper()
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/action", bytes.NewBufferString(payload)))
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return response.Code, body.Error
}

func setupStore(t *testing.T) *StateStore {
	t.Helper()
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Configure("token", []int64{11}); err != nil {
		t.Fatal(err)
	}
	return state
}

func forumBot() *recordingBot {
	return &recordingBot{
		chat:   Chat{ID: setupChatID, Type: "supergroup", IsForum: true},
		member: ChatMember{Status: "administrator", CanManageTopics: true},
	}
}

// breakSaves points the store at a missing directory so the next save fails.
func breakSaves(t *testing.T, state *StateStore) {
	t.Helper()
	state.mu.Lock()
	defer state.mu.Unlock()
	state.path = filepath.Join(t.TempDir(), "missing", "telegram.json")
}

func TestChatSetupRejectsBeforeCallingTelegram(t *testing.T) {
	for name, tc := range map[string]struct {
		payload string
		token   string
		allowed []int64
		code    string
	}{
		"bad chat id": {payload: `{"action":"chat_setup","chat_id":"x"}`, code: "bad_chat_id"},
		"no token":    {payload: setupPayload, allowed: []int64{11}, code: "token_required"},
		"no allowed":  {payload: setupPayload, token: "token", code: "allowed_uids_required"},
	} {
		t.Run(name, func(t *testing.T) {
			state, err := OpenState(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := state.Configure(tc.token, tc.allowed); err != nil {
				t.Fatal(err)
			}
			bot := forumBot()
			status, code := runChatSetup(t, NewServer(state, bot), tc.payload)
			if status != http.StatusBadRequest || code != tc.code || len(bot.calls) != 0 {
				t.Fatalf("status=%d code=%q bot=%#v", status, code, bot.calls)
			}
		})
	}
}

func TestChatSetupVerifiesChatThenBotInOrder(t *testing.T) {
	telegramDown := errors.New("telegram down")
	for name, tc := range map[string]struct {
		mutate func(*recordingBot)
		status int
		code   string
		calls  []string
	}{
		"getChat fails": {
			mutate: func(b *recordingBot) { b.fail = map[string]error{"getChat": telegramDown} },
			status: http.StatusBadGateway, code: "telegram_unavailable", calls: []string{"getChat"},
		},
		"not a supergroup": {
			mutate: func(b *recordingBot) { b.chat.Type = "group"; b.chat.IsForum = false },
			status: http.StatusBadRequest, code: "chat_must_be_supergroup", calls: []string{"getChat"},
		},
		"not a forum": {
			mutate: func(b *recordingBot) { b.chat.IsForum = false },
			status: http.StatusBadRequest, code: "topics_must_be_enabled", calls: []string{"getChat"},
		},
		"getMe fails": {
			mutate: func(b *recordingBot) { b.fail = map[string]error{"getMe": telegramDown} },
			status: http.StatusBadGateway, code: "telegram_unavailable", calls: []string{"getChat", "getMe"},
		},
		"getChatMember fails": {
			mutate: func(b *recordingBot) { b.fail = map[string]error{"getChatMember": telegramDown} },
			status: http.StatusBadGateway, code: "telegram_unavailable", calls: []string{"getChat", "getMe", "getChatMember"},
		},
		"plain member": {
			mutate: func(b *recordingBot) { b.member = ChatMember{Status: "member", CanManageTopics: true} },
			status: http.StatusBadRequest, code: "bot_must_manage_topics", calls: []string{"getChat", "getMe", "getChatMember"},
		},
		"admin without topics": {
			mutate: func(b *recordingBot) { b.member = ChatMember{Status: "administrator"} },
			status: http.StatusBadRequest, code: "bot_must_manage_topics", calls: []string{"getChat", "getMe", "getChatMember"},
		},
		"creator without flag": {
			mutate: func(b *recordingBot) { b.member = ChatMember{Status: "creator"} },
			status: http.StatusOK, calls: []string{"getChat", "getMe", "getChatMember", "topic:tariboyd"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := setupStore(t)
			bot := forumBot()
			tc.mutate(bot)
			status, code := runChatSetup(t, NewServer(state, bot), setupPayload)
			if status != tc.status || code != tc.code || !reflect.DeepEqual(bot.calls, tc.calls) {
				t.Fatalf("status=%d code=%q calls=%#v", status, code, bot.calls)
			}
		})
	}
}

func TestChatSetupExistingBindingOnlyReconciles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		state := setupStore(t)
		if err := state.BindChat(setupChatID, 7); err != nil {
			t.Fatal(err)
		}
		bot := forumBot()
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
		status, _ := runChatSetup(t, NewServer(state, bot, daemon), setupPayload)
		want := []string{"getChat", "getMe", "getChatMember", "topic:worker"}
		if status != http.StatusOK || !reflect.DeepEqual(bot.calls, want) {
			t.Fatalf("status=%d calls=%#v", status, bot.calls)
		}
		if got := state.Snapshot(); got.ManagementTopicID != 7 || got.PendingChat != nil || got.AgentTopics["worker"].ThreadID == 0 {
			t.Fatalf("state = %+v", got)
		}
	})
	t.Run("failure", func(t *testing.T) {
		state := setupStore(t)
		if err := state.BindChat(setupChatID, 7); err != nil {
			t.Fatal(err)
		}
		bot := forumBot()
		bot.fail = map[string]error{"topic:worker": errors.New("telegram down")}
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
		status, code := runChatSetup(t, NewServer(state, bot, daemon), setupPayload)
		if status != http.StatusBadGateway || code != "create_agent_topics_failed" {
			t.Fatalf("status=%d code=%q", status, code)
		}
	})
}

func TestChatSetupManagementTopicFailures(t *testing.T) {
	t.Run("create fails", func(t *testing.T) {
		state := setupStore(t)
		bot := forumBot()
		bot.fail = map[string]error{"topic:tariboyd": errors.New("telegram down")}
		status, code := runChatSetup(t, NewServer(state, bot), setupPayload)
		if status != http.StatusBadGateway || code != "create_topic_failed" || state.Snapshot().PendingChat != nil {
			t.Fatalf("status=%d code=%q state=%+v", status, code, state.Snapshot())
		}
	})
	t.Run("save fails", func(t *testing.T) {
		state := setupStore(t)
		bot := forumBot()
		bot.beforeTopic = func(string) { breakSaves(t, state) }
		status, code := runChatSetup(t, NewServer(state, bot), setupPayload)
		if status != http.StatusInternalServerError || code != "save_failed" || state.Snapshot().PendingChat != nil {
			t.Fatalf("status=%d code=%q state=%+v", status, code, state.Snapshot())
		}
	})
}

func TestChatSetupAgentTopicFailures(t *testing.T) {
	t.Run("list fails and keeps the pending chat", func(t *testing.T) {
		state := setupStore(t)
		daemon := &scriptedDaemon{fakeDaemon: &fakeDaemon{}, listErr: errors.New("daemon down")}
		status, code := runChatSetup(t, NewServer(state, forumBot(), daemon), setupPayload)
		got := state.Snapshot()
		if status != http.StatusBadGateway || code != "list_agents_failed" || got.PendingChat == nil || got.ChatID != 0 {
			t.Fatalf("status=%d code=%q state=%+v", status, code, got)
		}
	})
	t.Run("agent topic create fails", func(t *testing.T) {
		state := setupStore(t)
		bot := forumBot()
		bot.fail = map[string]error{"topic:worker": errors.New("telegram down")}
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
		status, code := runChatSetup(t, NewServer(state, bot, daemon), setupPayload)
		if status != http.StatusBadGateway || code != "create_agent_topics_failed" {
			t.Fatalf("status=%d code=%q", status, code)
		}
	})
	t.Run("agent topic save fails", func(t *testing.T) {
		state := setupStore(t)
		bot := forumBot()
		bot.beforeTopic = func(name string) {
			if name == "worker" {
				breakSaves(t, state)
			}
		}
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
		status, code := runChatSetup(t, NewServer(state, bot, daemon), setupPayload)
		if status != http.StatusInternalServerError || code != "save_failed" {
			t.Fatalf("status=%d code=%q", status, code)
		}
	})
	t.Run("commit fails", func(t *testing.T) {
		state := setupStore(t)
		daemon := &scriptedDaemon{fakeDaemon: &fakeDaemon{}, beforeList: func() { breakSaves(t, state) }}
		status, code := runChatSetup(t, NewServer(state, forumBot(), daemon), setupPayload)
		got := state.Snapshot()
		if status != http.StatusInternalServerError || code != "save_failed" || got.ChatID != 0 || got.PendingChat == nil {
			t.Fatalf("status=%d code=%q state=%+v", status, code, got)
		}
	})
}

func TestChatSetupWithoutDaemonCommitsManagementTopicOnly(t *testing.T) {
	state := setupStore(t)
	status, _ := runChatSetup(t, NewServer(state, forumBot()), setupPayload)
	got := state.Snapshot()
	if status != http.StatusOK || got.ChatID != setupChatID || got.ManagementTopicID == 0 || len(got.AgentTopics) != 0 || got.PendingChat != nil {
		t.Fatalf("status=%d state=%+v", status, got)
	}
}

func TestChatSetupRereadsStateAfterEachSavedTopic(t *testing.T) {
	t.Run("agent topics use the state saved with the pending chat", func(t *testing.T) {
		state := setupStore(t)
		bot := forumBot()
		bot.beforeTopic = func(name string) {
			if name == "tariboyd" {
				if err := state.Configure("rotated", []int64{11}); err != nil {
					t.Error(err)
				}
			}
		}
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}}}
		if status, code := runChatSetup(t, NewServer(state, bot, daemon), setupPayload); status != http.StatusOK {
			t.Fatalf("status=%d code=%q", status, code)
		}
		if bot.tokens["tariboyd"] != "token" || bot.tokens["worker"] != "rotated" {
			t.Fatalf("tokens = %#v", bot.tokens)
		}
	})
	t.Run("a repeated agent gets one topic", func(t *testing.T) {
		state := setupStore(t)
		bot := forumBot()
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "worker"}, {Name: "worker"}}}
		if status, code := runChatSetup(t, NewServer(state, bot, daemon), setupPayload); status != http.StatusOK {
			t.Fatalf("status=%d code=%q", status, code)
		}
		want := []string{"getChat", "getMe", "getChatMember", "topic:tariboyd", "topic:worker"}
		if !reflect.DeepEqual(bot.calls, want) {
			t.Fatalf("calls = %#v", bot.calls)
		}
	})
	t.Run("a resumed chat keeps its topics", func(t *testing.T) {
		state := setupStore(t)
		if err := state.BeginPendingChat(setupChatID, 50); err != nil {
			t.Fatal(err)
		}
		if err := state.SetPendingAgentTopic(setupChatID, "done", "done", 51); err != nil {
			t.Fatal(err)
		}
		bot := forumBot()
		daemon := &fakeDaemon{agents: []AgentInfo{{Name: "done"}, {Name: "worker"}}}
		if status, code := runChatSetup(t, NewServer(state, bot, daemon), setupPayload); status != http.StatusOK {
			t.Fatalf("status=%d code=%q", status, code)
		}
		want := []string{"getChat", "getMe", "getChatMember", "topic:worker"}
		got := state.Snapshot()
		if !reflect.DeepEqual(bot.calls, want) || got.ManagementTopicID != 50 || got.AgentTopics["done"].ThreadID != 51 {
			t.Fatalf("calls=%#v state=%+v", bot.calls, got)
		}
	})
	t.Run("a pending setup of another chat is replaced", func(t *testing.T) {
		state := setupStore(t)
		if err := state.BeginPendingChat(-100999, 50); err != nil {
			t.Fatal(err)
		}
		bot := forumBot()
		if status, code := runChatSetup(t, NewServer(state, bot), setupPayload); status != http.StatusOK {
			t.Fatalf("status=%d code=%q", status, code)
		}
		if got := state.Snapshot(); got.ChatID != setupChatID || got.ManagementTopicID != 101 {
			t.Fatalf("state = %+v", got)
		}
	})
}
