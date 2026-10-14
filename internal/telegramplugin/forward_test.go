package telegramplugin

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
)

func newForwardTestServer(t *testing.T) (*Server, *fakeDaemon, *StateStore) {
	t.Helper()
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
	return NewServer(state, nil, daemon), daemon, state
}

func decodeUpdate(t *testing.T, raw string) Update {
	t.Helper()
	var update Update
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		t.Fatal(err)
	}
	return update
}

func TestProcessUpdatePublishesForwardedMessages(t *testing.T) {
	for _, tc := range []struct {
		name, message, text string
		forward             map[string]any
	}{{
		name:    "public channel post with caption",
		message: `"caption":"Big news","forward_origin":{"type":"channel","date":1700000000,"chat":{"id":-100555,"type":"channel","title":"Новости","username":"news"},"message_id":123,"author_signature":"Ann"}`,
		text:    "Forwarded from channel \"Новости\" (@news) https://t.me/news/123:\nBig news",
		forward: map[string]any{"type": "channel", "name": "Новости", "username": "news", "chat_id": int64(-100555), "message_id": int64(123), "link": "https://t.me/news/123", "author_signature": "Ann", "date": int64(1700000000)},
	}, {
		name:    "private channel post",
		message: `"text":"secret","forward_origin":{"type":"channel","date":1700000001,"chat":{"id":-100556,"type":"channel","title":"Private"},"message_id":7}`,
		text:    "Forwarded from channel \"Private\":\nsecret",
		forward: map[string]any{"type": "channel", "name": "Private", "chat_id": int64(-100556), "message_id": int64(7), "date": int64(1700000001)},
	}, {
		name:    "user",
		message: `"text":"hi there","forward_origin":{"type":"user","date":1700000002,"sender_user":{"id":42,"first_name":"Ann","last_name":"Lee","username":"ann"}}`,
		text:    "Forwarded from user \"Ann Lee\" (@ann):\nhi there",
		forward: map[string]any{"type": "user", "name": "Ann Lee", "username": "ann", "user_id": int64(42), "date": int64(1700000002)},
	}, {
		name:    "hidden user",
		message: `"text":"psst","forward_origin":{"type":"hidden_user","date":1700000003,"sender_user_name":"Mystery"}`,
		text:    "Forwarded from hidden user \"Mystery\":\npsst",
		forward: map[string]any{"type": "hidden_user", "name": "Mystery", "date": int64(1700000003)},
	}, {
		name:    "chat",
		message: `"text":"from the group","forward_origin":{"type":"chat","date":1700000004,"sender_chat":{"id":-100777,"type":"supergroup","title":"Team","username":"team"}}`,
		text:    "Forwarded from chat \"Team\" (@team):\nfrom the group",
		forward: map[string]any{"type": "chat", "name": "Team", "username": "team", "chat_id": int64(-100777), "date": int64(1700000004)},
	}, {
		name:    "forwarded command is plain text",
		message: `"text":"/stop","forward_origin":{"type":"hidden_user","date":1700000005,"sender_user_name":"Mystery"}`,
		text:    "Forwarded from hidden user \"Mystery\":\n/stop",
		forward: map[string]any{"type": "hidden_user", "name": "Mystery", "date": int64(1700000005)},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			server, daemon, state := newForwardTestServer(t)
			update := decodeUpdate(t, `{"update_id":30,"message":{"message_id":5,"message_thread_id":9,"from":{"id":11},"chat":{"id":-100123,"type":"supergroup"},`+tc.message+`}}`)
			if err := server.ProcessUpdate(context.Background(), update); err != nil {
				t.Fatal(err)
			}
			if len(daemon.calls) != 0 {
				t.Fatalf("forwarded message ran a command: %#v", daemon.calls)
			}
			if len(daemon.published) != 1 || state.Snapshot().Offset != 31 {
				t.Fatalf("published=%#v offset=%d", daemon.published, state.Snapshot().Offset)
			}
			got := daemon.published[0]
			if got.Text != tc.text {
				t.Fatalf("text = %q, want %q", got.Text, tc.text)
			}
			if !reflect.DeepEqual(got.Forward, tc.forward) {
				t.Fatalf("forward = %#v, want %#v", got.Forward, tc.forward)
			}
		})
	}
}

func TestProcessUpdateUsesCaptionAndDropsEmptyMessages(t *testing.T) {
	server, daemon, state := newForwardTestServer(t)
	for id, body := range []string{`"caption":"photo caption"`, `"sticker":{}`} {
		update := decodeUpdate(t, `{"update_id":`+strconv.Itoa(id+1)+`,"message":{"message_thread_id":9,"from":{"id":11},"chat":{"id":-100123,"type":"supergroup"},`+body+`}}`)
		if err := server.ProcessUpdate(context.Background(), update); err != nil {
			t.Fatal(err)
		}
	}
	if len(daemon.published) != 1 || daemon.published[0].Text != "photo caption" || daemon.published[0].Forward != nil {
		t.Fatalf("published = %#v", daemon.published)
	}
	if state.Snapshot().Offset != 3 {
		t.Fatalf("offset = %d", state.Snapshot().Offset)
	}
}
