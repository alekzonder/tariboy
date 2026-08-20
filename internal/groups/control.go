package groups

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
)

// Create ensures the group + shared dir, subscribes any existing members and
// wires the lead inbox, and returns a view. Implements registry.GroupControl.
func (p *Provisioner) Create(name, lead string) (map[string]any, error) {
	if err := p.EnsureGroup(name, lead); err != nil {
		return nil, err
	}
	if err := p.Reconcile(name); err != nil {
		return nil, err
	}
	return p.Inspect(name)
}

// List returns one row per group with its lead and member count.
func (p *Provisioner) List() ([]map[string]any, error) {
	gs, err := p.groups.List()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(gs))
	for _, g := range gs {
		members, err := p.agents.ListByGroup(g.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"name": g.Name, "lead": g.Lead, "members": len(members)})
	}
	return out, nil
}

// Inspect returns a group's full view: lead, derived channels, shared dir, and
// member names.
func (p *Provisioner) Inspect(name string) (map[string]any, error) {
	g, ok, err := p.groups.Get(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("group %q not found", name)
	}
	ms, err := p.agents.ListByGroup(name)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ms))
	for _, m := range ms {
		names = append(names, m.Name)
	}
	return map[string]any{
		"name": g.Name, "lead": g.Lead, "members": names,
		"broadcast": bus.GroupBroadcast(name), "inbox": bus.GroupInbox(name),
		"shared_dir": p.SharedDir(name),
	}, nil
}

// Remove implements registry.GroupControl.
func (p *Provisioner) Remove(name string, volumes bool) error {
	return p.RemoveGroup(name, volumes)
}

// Assign implements registry.GroupControl.
func (p *Provisioner) Assign(agent, group string) error {
	return p.AssignAgent(agent, group)
}

func (p *Provisioner) ChangeLead(name, lead string) error {
	if !agent.ValidName(name) || (lead != "" && !agent.ValidName(lead)) {
		return fmt.Errorf("invalid group or lead name")
	}
	if lead != "" {
		member, err := p.agents.Get(lead)
		if err != nil || member.Group != name {
			return fmt.Errorf("lead %s is not a member of %s", lead, name)
		}
	}
	if err := p.groups.SetLead(name, lead); err != nil {
		return err
	}
	return p.Reconcile(name)
}
