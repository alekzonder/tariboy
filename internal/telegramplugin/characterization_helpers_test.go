package telegramplugin

import (
	"context"
	"testing"
)

// recordingBot is an in-memory BotAPI that logs each call in order. A call
// named in fail returns that error, and beforeTopic runs after a topic is
// accepted and before it is returned.
type recordingBot struct {
	calls       []string
	tokens      map[string]string
	fail        map[string]error
	chat        Chat
	member      ChatMember
	sent        []recordedSend
	beforeTopic func(name string)
	nextThread  int64
}

type recordedSend struct {
	chatID, threadID int64
	text             string
}

func (b *recordingBot) record(name string) error {
	b.calls = append(b.calls, name)
	return b.fail[name]
}

func (b *recordingBot) GetMe(context.Context, string) (BotUser, error) {
	return BotUser{ID: 77}, b.record("getMe")
}

func (b *recordingBot) GetChat(context.Context, string, int64) (Chat, error) {
	return b.chat, b.record("getChat")
}

func (b *recordingBot) GetChatMember(context.Context, string, int64, int64) (ChatMember, error) {
	return b.member, b.record("getChatMember")
}

func (b *recordingBot) CreateForumTopic(_ context.Context, token string, _ int64, name string) (ForumTopic, error) {
	if b.tokens == nil {
		b.tokens = map[string]string{}
	}
	b.tokens[name] = token
	if err := b.record("topic:" + name); err != nil {
		return ForumTopic{}, err
	}
	if b.beforeTopic != nil {
		b.beforeTopic(name)
	}
	b.nextThread++
	return ForumTopic{MessageThreadID: 100 + b.nextThread, Name: name}, nil
}

func (b *recordingBot) GetUpdates(context.Context, string, int64) ([]Update, error) {
	return nil, nil
}

func (b *recordingBot) SendMessage(_ context.Context, _ string, chatID, threadID int64, text string) (SentMessage, error) {
	if err := b.record("send"); err != nil {
		return SentMessage{}, err
	}
	b.sent = append(b.sent, recordedSend{chatID: chatID, threadID: threadID, text: text})
	return SentMessage{MessageID: int64(len(b.sent))}, nil
}

// scriptedDaemon adds failures and a hook to fakeDaemon.
type scriptedDaemon struct {
	*fakeDaemon
	listErr    error
	listCalls  int
	callErr    error
	beforeList func()
}

func (d *scriptedDaemon) ListAgents(ctx context.Context) ([]AgentInfo, error) {
	d.listCalls++
	if d.beforeList != nil {
		d.beforeList()
	}
	if d.listErr != nil {
		return nil, d.listErr
	}
	return d.fakeDaemon.ListAgents(ctx)
}

func (d *scriptedDaemon) Call(ctx context.Context, method, path string, body, result any) error {
	if d.callErr != nil {
		d.calls = append(d.calls, daemonCall{method: method, path: path, body: body})
		return d.callErr
	}
	return d.fakeDaemon.Call(ctx, method, path, body, result)
}

// configuredState is a store with a token, an allowlist and a bound chat with
// management topic 7 and the worker topic 9.
func configuredState(t *testing.T) *StateStore {
	t.Helper()
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Configure("token", []int64{5, 11, 20}); err != nil {
		t.Fatal(err)
	}
	if err := state.BindChat(-100123, 7); err != nil {
		t.Fatal(err)
	}
	if err := state.SetAgentTopic("worker", "worker", 9); err != nil {
		t.Fatal(err)
	}
	return state
}
