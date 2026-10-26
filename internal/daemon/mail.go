package daemon

import (
	"database/sql"
	"errors"

	"github.com/alekzonder/tariboy/internal/agent"
)

// mailDirectory answers the recipient questions of the mail service from the
// daemon's tables: agents, the customer login, installed plugins, task queues
// and groups.
type mailDirectory struct {
	db       *sql.DB
	agents   *agent.Store
	customer string
}

func (d mailDirectory) AgentExists(name string) (bool, error) {
	_, err := d.agents.Get(name)
	if errors.Is(err, agent.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// InboxCap is the agent's messages_max_queue; an unknown agent has none.
func (d mailDirectory) InboxCap(name string) (int, error) {
	a, err := d.agents.Get(name)
	if errors.Is(err, agent.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return a.MessagesMaxQueue, nil
}

func (d mailDirectory) CustomerLogin() string { return d.customer }

func (d mailDirectory) PluginInstalled(name string) (bool, error) {
	return d.exists(`SELECT COUNT(*) FROM plugins WHERE name = ?`, name)
}

func (d mailDirectory) QueueExists(prefix string) (bool, error) {
	return d.exists(`SELECT COUNT(*) FROM task_queues WHERE prefix = ?`, prefix)
}

func (d mailDirectory) GroupMembers(group string) ([]string, bool, error) {
	ok, err := d.exists(`SELECT COUNT(*) FROM groups WHERE name = ?`, group)
	if err != nil || !ok {
		return nil, false, err
	}
	members, err := d.agents.ListByGroup(group)
	if err != nil {
		return nil, false, err
	}
	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, m.Name)
	}
	return names, true, nil
}

func (d mailDirectory) exists(query string, arg any) (bool, error) {
	var n int
	err := d.db.QueryRow(query, arg).Scan(&n)
	return n > 0, err
}
