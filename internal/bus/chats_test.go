package bus

import "testing"

func TestReconcileChatsProvisionsPersonalChats(t *testing.T) {
	b := newBus(t)
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	chat, err := b.GetChat("dm:worker")
	if err != nil {
		t.Fatalf("personal chat missing: %v", err)
	}
	if chat.Channel != "chat:dm:worker" || chat.Kind != "direct" || chat.LegacyAgent != "worker" {
		t.Fatalf("unexpected chat row: %+v", chat)
	}
	if !ValidChannel(chat.Channel) {
		t.Fatalf("chat channel %q is not a well-formed bus channel", chat.Channel)
	}
	parts, err := b.Participants("dm:worker")
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, p := range parts {
		roles[p.Principal] = p.Role
	}
	if roles["user:customer"] == "" || roles["agent:worker"] == "" {
		t.Fatalf("both principals must be participants, got %v", roles)
	}
	service, err := b.Participants("service:worker")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range service {
		if p.Principal == "user:customer" && p.Role != "observer" {
			t.Fatalf("the customer observes the service chat, got role %q", p.Role)
		}
	}
	if legacy, err := b.GetChat("service:worker"); err != nil || legacy.LegacyAgent != "" {
		t.Fatalf("only the personal chat carries pre-migration history: %+v err=%v", legacy, err)
	}
}

func TestReconcileChatsIsIdempotent(t *testing.T) {
	b := newBus(t)
	for i := range 3 {
		if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	chats, err := b.ListChats()
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 3 {
		t.Fatalf("one agent gets exactly dm/tasks/service, got %d", len(chats))
	}
	parts, err := b.Participants("dm:worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("participants must not duplicate, got %d", len(parts))
	}
}

func TestChatByChannelResolvesAndMisses(t *testing.T) {
	b := newBus(t)
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	chat, ok, err := b.ChatByChannel("chat:dm:worker")
	if err != nil || !ok || chat.ID != "dm:worker" {
		t.Fatalf("ChatByChannel = %+v ok=%v err=%v", chat, ok, err)
	}
	if _, ok, err := b.ChatByChannel("agent:worker:inbox"); err != nil || ok {
		t.Fatalf("a non-chat channel must resolve to no chat, ok=%v err=%v", ok, err)
	}
}

// TestReconcileChatsCarriesLegacyReadMarks pins the migration of the single
// chat_read_v1 config value into per-participant read marks, including the
// forward-only rule: an older stored mark must not pull a newer one back.
func TestReconcileChatsCarriesLegacyReadMarks(t *testing.T) {
	b := newBus(t)
	if _, err := b.db.Exec(`INSERT INTO daemon_config(key, value) VALUES (?, ?)`, legacyReadKey,
		`{"worker":"2026-07-06T10:00:05Z","gone":"2026-07-06T10:00:09Z"}`); err != nil {
		t.Fatal(err)
	}
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	if got := readTS(t, b, "dm:worker", "user:customer"); got != "2026-07-06T10:00:05Z" {
		t.Fatalf("legacy read mark not carried, got %q", got)
	}
	if err := b.SetReadTS("dm:worker", "user:customer", "2026-07-06T10:00:07Z", false); err != nil {
		t.Fatal(err)
	}
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	if got := readTS(t, b, "dm:worker", "user:customer"); got != "2026-07-06T10:00:07Z" {
		t.Fatalf("read mark moved backwards to %q", got)
	}
	if err := b.SetReadTS("dm:worker", "user:customer", "2026-07-06T10:00:06Z", false); err != nil {
		t.Fatal(err)
	}
	if got := readTS(t, b, "dm:worker", "user:customer"); got != "2026-07-06T10:00:07Z" {
		t.Fatalf("SetReadTS is forward-only, got %q", got)
	}
}

func readTS(t *testing.T, b *Bus, chatID, principal string) string {
	t.Helper()
	parts, err := b.Participants(chatID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range parts {
		if p.Principal == principal {
			return p.ReadTS
		}
	}
	t.Fatalf("participant %q not in chat %q", principal, chatID)
	return ""
}

// The requirement the whole change is measured against: a message addressed to
// a chat wakes the agent exactly as an inbox message did. The service chat is
// the strictest case — its publisher is a system component, not a participant.
func TestServiceChatPublishStillWakesTheAgent(t *testing.T) {
	b := newBus(t)
	if err := b.ReconcileChats([]string{"worker"}, "user:customer"); err != nil {
		t.Fatal(err)
	}
	var woken []string
	b.SetPublishHook(func(_ Message, deliveredTo []string) { woken = append(woken, deliveredTo...) })
	if _, err := b.Publish(Message{
		Channel: ChatChannelFor(ChatIDService("worker")), Source: "system:scripts",
		Type: "script.result", Text: "done",
	}); err != nil {
		t.Fatal(err)
	}
	if len(woken) != 1 || woken[0] != "worker" {
		t.Fatalf("a service-chat publish must wake exactly the agent: %v", woken)
	}
	has, err := b.HasPending("worker")
	if err != nil || !has {
		t.Fatalf("HasPending must see the delivery: has=%v err=%v", has, err)
	}
}

// A chat with more than one agent is an ordinary channel with more than one
// subscribed participant: every other agent gets its delivery, and the author
// never receives its own message back into its own queue. This is what keeps a
// multi-agent chat on the one wake path instead of inventing a second.
func TestThreeParticipantChatDeliversToEveryOtherAgent(t *testing.T) {
	b := newBus(t)
	chat, err := b.CreateChat(Chat{ID: "team-alpha", Kind: ChatKindGroup, Title: "Team Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range []string{"user:customer", "agent:worker", "agent:reviewer"} {
		if err := b.AddParticipant(chat.ID, principal, "member"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Publish(Message{
		Channel: chat.Channel, Source: "agent:worker", ProducedByAgent: "worker",
		Type: "message", Text: "ready",
	}); err != nil {
		t.Fatal(err)
	}

	pending, err := b.Pending("reviewer", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Text != "ready" {
		t.Fatalf("reviewer must receive the other agent's message, got %+v", pending)
	}
	own, err := b.Pending("worker", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(own) != 0 {
		t.Fatalf("the author must not receive its own message, got %+v", own)
	}

	// Leaving stops future deliveries; the customer reads over HTTP and never
	// needed a subscription at all.
	if err := b.RemoveParticipant(chat.ID, "agent:reviewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Publish(Message{
		Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "after",
	}); err != nil {
		t.Fatal(err)
	}
	after, err := b.Pending("reviewer", 10)
	if err != nil {
		t.Fatal(err)
	}
	// The message it was already asked to handle stays; nothing new arrives.
	if len(after) != 1 || after[0].Text != "ready" {
		t.Fatalf("a former participant keeps its pending work and gains nothing new: %+v", after)
	}
}
