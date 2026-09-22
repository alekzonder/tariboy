package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/store"
)

// Startup reconciliation gives every persisted agent its chats, so an agent
// created before the chats migration is not left without a conversation.
func TestReconcileAgentChatsProvisionsPersistedAgents(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	agents := agent.NewStore(st)
	if err := agents.Create(agent.Agent{Name: "worker", ImageRef: "img:latest"}); err != nil {
		t.Fatal(err)
	}
	channelBus := bus.New(st, time.Now)

	if err := reconcileAgentChats(agents, channelBus, "user:customer"); err != nil {
		t.Fatal(err)
	}

	chat, err := channelBus.GetChat(bus.ChatIDDirect("worker"))
	if err != nil {
		t.Fatalf("personal chat missing after reconciliation: %v", err)
	}
	if chat.Channel != bus.ChatChannelFor(bus.ChatIDDirect("worker")) {
		t.Fatalf("unexpected chat channel %q", chat.Channel)
	}
	parts, err := channelBus.Participants(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("personal chat participants = %d, want the agent and the customer", len(parts))
	}
}
