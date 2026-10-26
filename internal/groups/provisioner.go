package groups

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
)

// ProvisionerConfig wires the provisioner to the group store, the agent store
// (membership) and the groups dir root.
type ProvisionerConfig struct {
	Groups    *Store
	Agents    *agent.Store
	GroupsDir string // <base>/groups
	Clock     func() time.Time
}

// Provisioner owns the daemon-side group invariants (spec §4.4): the row and
// the shared dir. Mail to group:<name> expands over the membership, so a
// group has no channels or subscriptions. It is called on the request path
// (agent.run --group, group.* commands, compose reconcile); there is no
// background goroutine.
type Provisioner struct {
	groups    *Store
	agents    *agent.Store
	groupsDir string
	clock     func() time.Time
}

func NewProvisioner(cfg ProvisionerConfig) *Provisioner {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Provisioner{
		groups: cfg.Groups, agents: cfg.Agents,
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

// Reconcile checks the membership of group name after a join, a leave or a
// lead change. It is idempotent; a member with an invalid name is an error.
func (p *Provisioner) Reconcile(name string) error {
	members, err := p.agents.ListByGroup(name)
	if err != nil {
		return err
	}
	for _, m := range members {
		if !agent.ValidName(m.Name) {
			return fmt.Errorf("%w %q", agent.ErrInvalidName, m.Name)
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
	if old != "" && old != group {
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
func (p *Provisioner) Rename(oldName, newName string) (retErr error) {
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
	oldRoot, newRoot := filepath.Join(p.groupsDir, oldName), filepath.Join(p.groupsDir, newName)
	if err := os.Rename(oldRoot, newRoot); err != nil {
		return err
	}
	rollbackDir := true
	defer func() {
		if rollbackDir {
			if err := os.Rename(newRoot, oldRoot); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("roll back group directory %s to %s: %w", newRoot, oldRoot, err))
			}
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
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("roll back group rename %s to %s: %w", oldName, newName, rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollbackDir = false
	return p.Reconcile(newName)
}

// RemoveGroup tears a group down (spec §5.3 `down`): detach every member,
// delete the row, and — only with removeVolumes — delete the shared dir.
func (p *Provisioner) RemoveGroup(name string, removeVolumes bool) error {
	if !agent.ValidName(name) {
		return fmt.Errorf("%w %q", ErrInvalidName, name)
	}
	members, err := p.agents.ListByGroup(name)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := p.agents.SetGroup(m.Name, ""); err != nil {
			return err
		}
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
