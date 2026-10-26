package loop

import (
	"errors"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
)

// testDirectory knows a fixed set of agents, the customer "customer" and
// nothing else; inboxes have no cap.
type testDirectory map[string]bool

func (d testDirectory) AgentExists(name string) (bool, error)     { return d[name], nil }
func (testDirectory) InboxCap(string) (int, error)                { return 0, nil }
func (testDirectory) CustomerLogin() string                       { return "customer" }
func (testDirectory) PluginInstalled(string) (bool, error)        { return false, nil }
func (testDirectory) QueueExists(string) (bool, error)            { return false, nil }
func (testDirectory) GroupMembers(string) ([]string, bool, error) { return nil, false, nil }

func newTestMail(st *store.Store, agents ...string) *mail.Mail {
	dir := testDirectory{}
	for _, a := range agents {
		dir[a] = true
	}
	return mail.New(st, nil, dir)
}

// inboxOf is the open inbox of agent, oldest first.
func inboxOf(m *mail.Mail, agent string) ([]mail.Letter, error) {
	letters, _, err := m.List(mail.BoxInbox, "agent:"+agent, 100, "")
	return letters, err
}

// agentStoreDirectory knows the agents of an agent store.
type agentStoreDirectory struct{ as *agent.Store }

func (d agentStoreDirectory) AgentExists(name string) (bool, error) {
	_, err := d.as.Get(name)
	if errors.Is(err, agent.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
func (agentStoreDirectory) InboxCap(string) (int, error)                { return 0, nil }
func (agentStoreDirectory) CustomerLogin() string                       { return "customer" }
func (agentStoreDirectory) PluginInstalled(string) (bool, error)        { return false, nil }
func (agentStoreDirectory) QueueExists(string) (bool, error)            { return false, nil }
func (agentStoreDirectory) GroupMembers(string) ([]string, bool, error) { return nil, false, nil }
