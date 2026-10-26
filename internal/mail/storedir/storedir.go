// Package storedir is the mail Directory over the daemon's tables: agents,
// the customer login, installed plugins, task queues and groups. It keeps the
// mail service itself free of those tables.
package storedir

import (
	"database/sql"
	"errors"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/store"
)

// Directory answers the recipient questions of the mail service from the
// daemon's tables: agents, the customer login, installed plugins, task queues
// and groups.
type Directory struct {
	db       *sql.DB
	agents   *agent.Store
	customer string
}

// New returns the directory over st for the customer login customer.
func New(st *store.Store, customer string) Directory {
	return Directory{db: st.DB, agents: agent.NewStore(st), customer: customer}
}

func (d Directory) AgentExists(name string) (bool, error) {
	_, err := d.agents.Get(name)
	if errors.Is(err, agent.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// InboxCap is the agent's messages_max_queue; an unknown agent has none.
func (d Directory) InboxCap(name string) (int, error) {
	a, err := d.agents.Get(name)
	if errors.Is(err, agent.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return a.MessagesMaxQueue, nil
}

func (d Directory) CustomerLogin() string { return d.customer }

func (d Directory) PluginInstalled(name string) (bool, error) {
	return d.exists(`SELECT COUNT(*) FROM plugins WHERE name = ?`, name)
}

func (d Directory) PluginReceivesMail(name string) (bool, error) {
	return d.exists(`SELECT COUNT(*) FROM plugins WHERE name = ? AND enabled = 1
		AND json_valid(mail) AND json_extract(mail, '$.receive') = 1`, name)
}

func (d Directory) QueueExists(prefix string) (bool, error) {
	return d.exists(`SELECT COUNT(*) FROM task_queues WHERE prefix = ?`, prefix)
}

func (d Directory) GroupMembers(group string) ([]string, bool, error) {
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

func (d Directory) exists(query string, arg any) (bool, error) {
	var n int
	err := d.db.QueryRow(query, arg).Scan(&n)
	return n > 0, err
}
