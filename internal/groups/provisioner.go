package groups

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
)

// ProvisionerConfig wires the provisioner to the group store, the agent store
// (membership + env), the bus (subscriptions/channels) and the groups dir root.
type ProvisionerConfig struct {
	Groups    *Store
	Agents    *agent.Store
	Bus       *bus.Bus
	GroupsDir string // <base>/groups
	Clock     func() time.Time
}

// Provisioner owns the daemon-side group invariants (spec §4.4): the row, the
// shared dir, and the broadcast/inbox/DM subscriptions. It is called on the
// request path (agent.run --group, group.* commands, compose reconcile); there
// is no background goroutine.
type Provisioner struct {
	groups    *Store
	agents    *agent.Store
	bus       *bus.Bus
	groupsDir string
	clock     func() time.Time
}

func NewProvisioner(cfg ProvisionerConfig) *Provisioner {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Provisioner{
		groups: cfg.Groups, agents: cfg.Agents, bus: cfg.Bus,
		groupsDir: cfg.GroupsDir, clock: cfg.Clock,
	}
}

// SharedDir is the group's shared workdir (spec §4.3), derived from the group
// name. Callers MUST validate name (agent.ValidName) before trusting this path;
// EnsureGroup does so before any mkdir.
func (p *Provisioner) SharedDir(group string) string {
	return filepath.Join(p.groupsDir, group, "shared")
}

// EnsureGroup idempotently provisions the entity + shared dir. A leaderless call
// (lead == "") never clears an existing lead; a non-empty lead updates it. name
// is validated BEFORE any filesystem path is built (path-traversal guard).
func (p *Provisioner) EnsureGroup(name, lead string) error {
	if !agent.ValidName(name) {
		return fmt.Errorf("%w %q: must match ^[a-z0-9][a-z0-9_-]*$", ErrInvalidName, name)
	}
	g, ok, err := p.groups.Get(name)
	if err != nil {
		return err
	}
	switch {
	case !ok:
		if err := p.groups.Upsert(Group{Name: name, Lead: lead}); err != nil {
			return err
		}
	case lead != "" && g.Lead != lead:
		if err := p.groups.SetLead(name, lead); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(p.SharedDir(name), 0o700); err != nil {
		return fmt.Errorf("group %s shared dir: %w", name, err)
	}
	return nil
}

// subscribe idempotently subscribes agent to channel (empty matcher / no type
// filter). bus.Subscribe is idempotent on (agent, channel, matcher).
func (p *Provisioner) subscribe(agentName, channel string) error {
	_, err := p.bus.Subscribe(agentName, channel, nil, nil)
	return err
}

// unsubscribeFrom removes every subscription agent holds on channel. It is
// scoped to agentName (bus.ListSubscriptions/Unsubscribe are (agent, id)-scoped)
// so one member can never remove another member's subscription.
func (p *Provisioner) unsubscribeFrom(agentName, channel string) error {
	subs, err := p.bus.ListSubscriptions(agentName)
	if err != nil {
		return err
	}
	for _, s := range subs {
		if s.Channel == channel {
			if err := p.bus.Unsubscribe(agentName, s.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Reconcile makes live subscriptions + env match membership (spec §4.4). It is
// idempotent and safe to call after every join/leave/lead-change.
func (p *Provisioner) Reconcile(name string) error {
	g, ok, err := p.groups.Get(name)
	if err != nil {
		return err
	}
	if !ok {
		return nil // nothing to reconcile
	}
	members, err := p.agents.ListByGroup(name)
	if err != nil {
		return err
	}
	broadcast := bus.GroupBroadcast(name)
	inbox := bus.GroupInbox(name)
	for _, m := range members {
		// Defense-in-depth: never build agent:<name>:inbox from an unvalidated
		// name. Membership rows are ValidName-guarded at creation, but a bad
		// name must never reach channel construction (path/channel traversal).
		if !agent.ValidName(m.Name) {
			return fmt.Errorf("%w %q", agent.ErrInvalidName, m.Name)
		}
		if err := p.subscribe(m.Name, broadcast); err != nil {
			return err
		}
		if err := p.subscribe(m.Name, bus.InboxChannel(m.Name)); err != nil {
			return err
		}
		// group:<g>:direct:<member> — where a request from a teammate lands
		// (spec §4.2). group request targets this channel via the Request
		// primitive; the delivery still surfaces in the member's inbox prompt
		// (Inbox aggregates all of an agent's subscriptions), and a member can
		// reply on it because it holds the delivery.
		if err := p.subscribe(m.Name, bus.GroupDirect(name, m.Name)); err != nil {
			return err
		}
		// Lead-only group inbox.
		if g.Lead != "" && m.Name == g.Lead {
			if err := p.subscribe(m.Name, inbox); err != nil {
				return err
			}
		} else if err := p.unsubscribeFrom(m.Name, inbox); err != nil {
			return err
		}
	}
	return nil
}

// AssignAgent (re)assigns an agent's group (group=="" leaves) and reconciles
// both the old and new groups. It validates every name before building paths or
// channels.
func (p *Provisioner) AssignAgent(agentName, group string) error {
	if !agent.ValidName(agentName) {
		return fmt.Errorf("%w %q", agent.ErrInvalidName, agentName)
	}
	cur, err := p.agents.Get(agentName)
	if err != nil {
		return err
	}
	old := cur.Group
	if group != "" {
		if err := p.EnsureGroup(group, ""); err != nil {
			return err
		}
	}
	if err := p.agents.SetGroup(agentName, group); err != nil {
		return err
	}
	// Detach from the old group's channels when leaving/moving.
	if old != "" && old != group {
		if err := p.unsubscribeFrom(agentName, bus.GroupBroadcast(old)); err != nil {
			return err
		}
		if err := p.unsubscribeFrom(agentName, bus.GroupInbox(old)); err != nil {
			return err
		}
		if err := p.unsubscribeFrom(agentName, bus.GroupDirect(old, agentName)); err != nil {
			return err
		}
		if err := p.Reconcile(old); err != nil {
			return err
		}
	}
	if group != "" {
		return p.Reconcile(group)
	}
	return nil
}

// Rename changes group identity and membership together, preserving agents.
func (p *Provisioner) Rename(oldName, newName string) error {
	if !agent.ValidName(oldName) || !agent.ValidName(newName) || oldName == newName {
		return fmt.Errorf("invalid group rename")
	}
	if _, ok, err := p.groups.Get(oldName); err != nil || !ok {
		return fmt.Errorf("group %s not found", oldName)
	}
	if _, ok, err := p.groups.Get(newName); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("group %s already exists", newName)
	}
	members, err := p.agents.ListByGroup(oldName)
	if err != nil {
		return err
	}
	oldRoot, newRoot := filepath.Join(p.groupsDir, oldName), filepath.Join(p.groupsDir, newName)
	if err := os.Rename(oldRoot, newRoot); err != nil {
		return err
	}
	rollbackDir := true
	defer func() {
		if rollbackDir {
			_ = os.Rename(newRoot, oldRoot)
		}
	}()
	tx, err := p.groups.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE groups SET name=? WHERE name=?`, newName, oldName); err == nil {
		_, err = tx.Exec(`UPDATE agents SET "group"=? WHERE "group"=?`, newName, oldName)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollbackDir = false
	for _, member := range members {
		if err := p.unsubscribeFrom(member.Name, bus.GroupBroadcast(oldName)); err != nil {
			return err
		}
		if err := p.unsubscribeFrom(member.Name, bus.GroupInbox(oldName)); err != nil {
			return err
		}
		if err := p.unsubscribeFrom(member.Name, bus.GroupDirect(oldName, member.Name)); err != nil {
			return err
		}
		_ = p.bus.DeleteChannel(bus.GroupDirect(oldName, member.Name))
	}
	_ = p.bus.DeleteChannel(bus.GroupBroadcast(oldName))
	_ = p.bus.DeleteChannel(bus.GroupInbox(oldName))
	return p.Reconcile(newName)
}

// RemoveGroup tears a group down (spec §5.3 `down`): detach every member,
// delete the broadcast + inbox channels, delete the row, and — only with
// removeVolumes — delete the shared dir.
func (p *Provisioner) RemoveGroup(name string, removeVolumes bool) error {
	if !agent.ValidName(name) {
		return fmt.Errorf("%w %q", ErrInvalidName, name)
	}
	members, err := p.agents.ListByGroup(name)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := p.unsubscribeFrom(m.Name, bus.GroupBroadcast(name)); err != nil {
			return err
		}
		if err := p.unsubscribeFrom(m.Name, bus.GroupInbox(name)); err != nil {
			return err
		}
		if err := p.unsubscribeFrom(m.Name, bus.GroupDirect(name, m.Name)); err != nil {
			return err
		}
		if err := p.bus.DeleteChannel(bus.GroupDirect(name, m.Name)); err != nil {
			return err
		}
		if err := p.agents.SetGroup(m.Name, ""); err != nil {
			return err
		}
	}
	if err := p.bus.DeleteChannel(bus.GroupBroadcast(name)); err != nil {
		return err
	}
	if err := p.bus.DeleteChannel(bus.GroupInbox(name)); err != nil {
		return err
	}
	if err := p.groups.Delete(name); err != nil {
		return err
	}
	if removeVolumes {
		if err := os.RemoveAll(filepath.Join(p.groupsDir, name)); err != nil {
			return err
		}
	}
	return nil
}
