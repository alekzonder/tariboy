package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
)

// The chat endpoints are the read side of the customer's conversations: one
// ranked list for the agent sidebar, one merged feed per agent, and a read mark
// the unread counter is measured against.
func TestChatListFeedAndReadMark(t *testing.T) {
	c, b := ctxWithBus(t)
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []bus.Message{
		{Channel: "agent:worker:inbox", Source: "user:customer", Type: "message", Text: "customer says hi"},
		{Channel: "user:customer", Source: "system:tasks", Type: "task.question",
			Data: map[string]any{"from": "agent:worker"}, Text: "worker asks"},
		{Channel: "agent:worker:inbox", Source: "system:tasks", Type: "task.goal", Text: "own wake"},
	} {
		if _, err := b.Publish(msg); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := h(t, "chat.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	list := listed.(map[string]any)
	if list["customer"] != "user:customer" {
		t.Fatalf("customer = %v", list["customer"])
	}
	chats := list["chats"].([]map[string]any)
	if len(chats) != 3 {
		t.Fatalf("a reconciled agent has a conversation, a tasks and a service chat: %#v", chats)
	}
	if chats[0]["agent"] != "worker" || chats[0]["id"] != "dm:worker" ||
		chats[0]["kind"] != "direct" || chats[0]["unread"] != 1 {
		t.Fatalf("chats = %#v", chats)
	}
	lastTS, _ := chats[0]["last_ts"].(string)
	if lastTS == "" || chats[0]["last_from"] != "agent:worker" {
		t.Fatalf("chat last message = %#v", chats[0])
	}

	fed, err := h(t, "chat.messages")(c, registry.Params{"agent": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	messages := fed.(map[string]any)["messages"].([]map[string]any)
	if len(messages) != 2 || messages[0]["from"] != "user:customer" || messages[1]["from"] != "agent:worker" {
		t.Fatalf("feed = %#v", messages)
	}

	// The feed carries the read mark the chat was opened at, which is what the
	// "New messages" separator is drawn from; before any read it is empty.
	if got := fed.(map[string]any)["read_ts"]; got != "" {
		t.Fatalf("read_ts before the first read = %v, want empty", got)
	}

	if _, err := h(t, "chat.read")(c, registry.Params{"agent": "worker", "ts": lastTS}); err != nil {
		t.Fatal(err)
	}
	fed, err = h(t, "chat.messages")(c, registry.Params{"agent": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if got := fed.(map[string]any)["read_ts"]; got != lastTS {
		t.Fatalf("read_ts after the read = %v, want %v", got, lastTS)
	}
	listed, err = h(t, "chat.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.(map[string]any)["chats"].([]map[string]any)[0]["unread"]; got != 0 {
		t.Fatalf("unread after read = %v, want 0", got)
	}

	// The mark only ever moves forward, so a late or duplicate request from a
	// second window cannot resurrect messages the customer has already seen.
	if _, err := h(t, "chat.read")(c, registry.Params{"agent": "worker", "ts": "2000-01-01T00:00:00.000000000Z"}); err != nil {
		t.Fatal(err)
	}
	parts, err := b.Participants("dm:worker")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		if part.Principal == "user:customer" && part.ReadTS != lastTS {
			t.Fatalf("read mark = %q, want %q", part.ReadTS, lastTS)
		}
	}
}

// An operator message is the customer speaking, and it carries the customer's
// channel as its reply target so an agent's reply lands in the chat rather than
// back in its own inbox.
func TestMessageSendSpeaksAsTheCustomer(t *testing.T) {
	c, b := ctxWithBus(t)
	if _, err := h(t, "message.send")(c, registry.Params{
		"channel": "agent:worker:inbox", "type": "message", "text": "hi", "reply_to": "user:customer",
	}); err != nil {
		t.Fatal(err)
	}
	tail, _ := b.Tail("agent:worker:inbox", 10)
	if len(tail) != 1 || tail[0].Source != "user:customer" || tail[0].ReplyTo != "user:customer" {
		t.Fatalf("tail = %+v", tail)
	}
}

// Marking a chat unread is the one write that has to move the read mark
// backwards, so it asks for that explicitly: without `exact` the mark still
// only moves forward. The feed carries the current mark so the client can place
// its "new messages" divider without fetching the whole chat list.
func TestChatReadExactMovesTheMarkBackAndFeedCarriesIt(t *testing.T) {
	c, b := ctxWithBus(t)
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []bus.Message{
		{Channel: "agent:worker:inbox", Source: "user:customer", Type: "message", Text: "customer says hi"},
		{Channel: "user:customer", Source: "system:tasks", Type: "message",
			Data: map[string]any{"from": "agent:worker"}, Text: "worker answers"},
	} {
		if _, err := b.Publish(msg); err != nil {
			t.Fatal(err)
		}
	}
	fed, err := h(t, "chat.messages")(c, registry.Params{"agent": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := fed.(map[string]any)["read_ts"].(string); !ok || got != "" {
		t.Fatalf("read_ts before any read = %v (present %v), want an empty string", got, ok)
	}
	messages := fed.(map[string]any)["messages"].([]map[string]any)
	first, last := messages[0]["ts"].(string), messages[1]["ts"].(string)

	if _, err := h(t, "chat.read")(c, registry.Params{"agent": "worker", "ts": last}); err != nil {
		t.Fatal(err)
	}
	// Mark unread: the mark goes back to before the newest agent message.
	if _, err := h(t, "chat.read")(c, registry.Params{"agent": "worker", "ts": first, "exact": "true"}); err != nil {
		t.Fatal(err)
	}
	listed, err := h(t, "chat.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.(map[string]any)["chats"].([]map[string]any)[0]["unread"]; got != 1 {
		t.Fatalf("unread after marking unread = %v, want 1", got)
	}
	fed, err = h(t, "chat.messages")(c, registry.Params{"agent": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if got := fed.(map[string]any)["read_ts"]; got != first {
		t.Fatalf("read_ts = %v, want %v", got, first)
	}
}

// The unanswered queue is the answering obligation, and it is not the delivery
// queue: a reply retires a message, a plain send does not.
func TestChatUnansweredAndParticipants(t *testing.T) {
	c, b := ctxWithBus(t)
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	asked, err := b.Publish(bus.Message{
		Channel: "chat:dm:worker", Source: "user:customer", Type: "message", Text: "please answer",
	})
	if err != nil {
		t.Fatal(err)
	}

	listed, err := h(t, "chat.unanswered")(c, registry.Params{"chat": "dm:worker", "principal": "agent:worker"})
	if err != nil {
		t.Fatal(err)
	}
	queue := listed.(map[string]any)["messages"].([]map[string]any)
	if len(queue) != 1 || queue[0]["id"] != asked.ID || queue[0]["from"] != "user:customer" {
		t.Fatalf("queue = %#v", queue)
	}

	if _, err := b.Reply("worker", asked.ID, "answered", nil, ""); err != nil {
		t.Fatal(err)
	}
	listed, err = h(t, "chat.unanswered")(c, registry.Params{"chat": "dm:worker", "principal": "agent:worker"})
	if err != nil {
		t.Fatal(err)
	}
	if got := listed.(map[string]any)["count"]; got != 0 {
		t.Fatalf("count after the reply = %v, want 0", got)
	}

	parts, err := h(t, "chat.participants")(c, registry.Params{"chat": "dm:worker"})
	if err != nil {
		t.Fatal(err)
	}
	rows := parts.(map[string]any)["participants"].([]map[string]any)
	if len(rows) != 2 {
		t.Fatalf("participants = %#v", rows)
	}
}

// The UI opens a tasks or service chat by its id, and a personal chat by the
// agent name the sidebar already knows. Both address the same endpoint, so one
// argument carries either.
func TestChatMessagesAndReadAddressAChatByID(t *testing.T) {
	c, b := ctxWithBus(t)
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Publish(bus.Message{
		Channel: bus.ChatChannelFor(bus.ChatIDTasks("worker")), Source: "system:tasks",
		Type: "task.assigned", Text: "DEV-1 assigned",
	}); err != nil {
		t.Fatal(err)
	}

	fed, err := h(t, "chat.messages")(c, registry.Params{"agent": "tasks:worker"})
	if err != nil {
		t.Fatal(err)
	}
	page := fed.(map[string]any)
	if page["chat"] != "tasks:worker" || page["agent"] != "worker" {
		t.Fatalf("a chat id names its own chat and its agent: %#v", page)
	}
	messages := page["messages"].([]map[string]any)
	if len(messages) != 1 || messages[0]["text"] != "DEV-1 assigned" {
		t.Fatalf("tasks feed = %#v", messages)
	}

	ts, _ := messages[0]["ts"].(string)
	if _, err := h(t, "chat.read")(c, registry.Params{"agent": "tasks:worker", "ts": ts}); err != nil {
		t.Fatal(err)
	}
	mark, err := participantReadTS(b, "tasks:worker", "user:customer")
	if err != nil {
		t.Fatal(err)
	}
	if mark != ts {
		t.Fatalf("read mark = %q, want %q", mark, ts)
	}
}

// A chat id is a channel segment and a namespace at once: the per-agent
// namespaces are provisioned, not claimed by hand, and an id already in use
// must never be silently taken over by a second chat.
func TestCreateChatRejectsReservedAndTakenIDs(t *testing.T) {
	c, _ := ctxWithBus(t)
	create := func(id string) error {
		_, err := h(t, "chat.create")(c, registry.Params{"id": id, "title": "Team Alpha"})
		return err
	}
	for _, id := range []string{
		"", "Team Alpha", "team alpha", "-team",
		"dm:worker", "tasks:worker", "service:worker", "telegram:worker", "group:dev-team",
	} {
		if err := create(id); err == nil {
			t.Fatalf("id %q must be rejected", id)
		}
	}
	if err := create("team-alpha"); err != nil {
		t.Fatalf("an ordinary slug must be accepted: %v", err)
	}
	if err := create("team-alpha"); err == nil {
		t.Fatal("a duplicate id must be rejected, not silently reused")
	}
}

// Creating a chat puts the customer and the named agents in it, and the
// participant commands move principals in and out afterwards. Membership is
// what carries delivery, so an agent added here is subscribed and an agent
// removed is not.
func TestChatCreateAndParticipantCommands(t *testing.T) {
	c, b := ctxWithBus(t)
	created, err := h(t, "chat.create")(c, registry.Params{
		"id": "team-alpha", "title": "Team Alpha", "participants": "agent:worker,agent:reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	page := created.(map[string]any)
	if page["chat"] != "team-alpha" || page["channel"] != "chat:team-alpha" || page["kind"] != "group" {
		t.Fatalf("created chat = %#v", page)
	}
	principals := func() []string {
		t.Helper()
		listed, err := h(t, "chat.participants")(c, registry.Params{"chat": "team-alpha"})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, row := range listed.(map[string]any)["participants"].([]map[string]any) {
			out = append(out, row["principal"].(string))
		}
		return out
	}
	if got := principals(); len(got) != 3 {
		t.Fatalf("the customer and both agents take part: %v", got)
	}

	if _, err := h(t, "chat.leave")(c, registry.Params{
		"chat": "team-alpha", "principal": "agent:reviewer",
	}); err != nil {
		t.Fatal(err)
	}
	if got := principals(); len(got) != 2 {
		t.Fatalf("participants after the removal = %v", got)
	}
	subs, err := b.ListSubscriptions("reviewer")
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range subs {
		if sub.Channel == "chat:team-alpha" {
			t.Fatalf("a removed participant keeps no live subscription: %+v", sub)
		}
	}

	if _, err := h(t, "chat.join")(c, registry.Params{
		"chat": "team-alpha", "principal": "agent:reviewer", "role": "member",
	}); err != nil {
		t.Fatal(err)
	}
	if got := principals(); len(got) != 3 {
		t.Fatalf("participants after the re-add = %v", got)
	}

	// A malformed principal is a user error, not a row with a nonsense name.
	if _, err := h(t, "chat.join")(c, registry.Params{
		"chat": "team-alpha", "principal": "reviewer",
	}); err == nil {
		t.Fatal("a principal without a user:/agent: prefix must be rejected")
	}

	// The chat the customer was put in is one of the customer's chats, and it
	// names who is in it so a reader can tell a group apart from a duplicate.
	listed, err := h(t, "chat.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range listed.(map[string]any)["chats"].([]map[string]any) {
		if row["id"] != "team-alpha" {
			continue
		}
		parts, _ := row["participants"].([]string)
		if len(parts) != 3 || row["agent"] != "" {
			t.Fatalf("a group chat belongs to no single agent and names its participants: %#v", row)
		}
		return
	}
	t.Fatal("the created chat must appear in the customer's chat list")
}
