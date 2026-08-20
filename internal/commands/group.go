package commands

import (
	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/groups"
	"github.com/alekzonder/tariboy/internal/registry"
)

// compile-time: the daemon's provisioner satisfies the control interface.
var _ registry.GroupControl = (*groups.Provisioner)(nil)

func requireGroups(c *registry.Ctx) (registry.GroupControl, error) {
	if c.Groups == nil {
		return nil, api.UserError{Code: "no_group_control", Msg: "group control is not available"}
	}
	return c.Groups, nil
}

func checkGroupName(name string) error {
	if !agent.ValidName(name) {
		return api.UserError{Code: "bad_name", Msg: "invalid group name " + name}
	}
	return nil
}

func groupCreate() registry.Command {
	return registry.Command{
		Path:    "group.create",
		Summary: "Create (or update) a group with an optional lead",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "group name"},
			{Name: "lead", Flag: "lead", Short: "l", Type: registry.String, Help: "lead member name"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/groups"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkGroupName(name); err != nil {
				return nil, err
			}
			res, err := gc.Create(name, str(p, "lead"))
			if err != nil {
				return nil, api.UserError{Code: "create_failed", Msg: err.Error()}
			}
			return res, nil
		},
	}
}

func groupLs() registry.Command {
	return registry.Command{
		Path:    "group.ls",
		Summary: "List groups (name/lead/member count)",
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/groups"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			list, err := gc.List()
			if err != nil {
				return nil, err
			}
			return map[string]any{"groups": list, "count": len(list)}, nil
		},
	}
}

func groupInspect() registry.Command {
	return registry.Command{
		Path:    "group.inspect",
		Summary: "Show a group's lead, members, channels and shared dir",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "group name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/groups/{name}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkGroupName(name); err != nil {
				return nil, err
			}
			res, err := gc.Inspect(name)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "group " + name + " not found"}
			}
			return res, nil
		},
	}
}

func groupRm() registry.Command {
	return registry.Command{
		Path:    "group.rm",
		Summary: "Remove a group (detach members, delete channels; --volumes drops the shared dir)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "group name"},
			{Name: "volumes", Flag: "volumes", Short: "v", Type: registry.Bool, Help: "also delete the shared dir"},
		},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/groups/{name}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkGroupName(name); err != nil {
				return nil, err
			}
			if err := gc.Remove(name, toBool(p["volumes"])); err != nil {
				return nil, api.UserError{Code: "rm_failed", Msg: err.Error()}
			}
			return map[string]any{"removed": name, "volumes": toBool(p["volumes"])}, nil
		},
	}
}

func groupAssign() registry.Command {
	return registry.Command{
		Path:    "group.assign",
		Summary: "Assign an agent to a group (empty group leaves)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "group name"},
			{Name: "agent", Type: registry.String, Required: true, Help: "agent name"},
		},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/groups/{name}/assign"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			name := str(p, "name")
			if err := checkGroupName(name); err != nil {
				return nil, err
			}
			if err := gc.Assign(str(p, "agent"), name); err != nil {
				return nil, api.UserError{Code: "assign_failed", Msg: err.Error()}
			}
			return map[string]any{"agent": str(p, "agent"), "group": name}, nil
		},
	}
}

func groupRename() registry.Command {
	return registry.Command{
		Path: "group.rename", Summary: "Rename a group without deleting its agents",
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "new_name", Type: registry.String, Required: true}},
		HTTP: &registry.HTTPRoute{Method: "PATCH", Path: "/api/groups/{name}/name"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			oldName, newName := str(p, "name"), str(p, "new_name")
			if err := checkGroupName(oldName); err != nil {
				return nil, err
			}
			if err := checkGroupName(newName); err != nil {
				return nil, err
			}
			if err := gc.Rename(oldName, newName); err != nil {
				return nil, api.UserError{Code: "rename_failed", Msg: err.Error()}
			}
			return gc.Inspect(newName)
		},
	}
}

func groupLeadSet() registry.Command {
	return registry.Command{
		Path: "group.lead.set", Summary: "Set a group's lead to a current member",
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "lead", Type: registry.String, Required: true}},
		HTTP: &registry.HTTPRoute{Method: "PATCH", Path: "/api/groups/{name}/lead"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			name, lead := str(p, "name"), str(p, "lead")
			if err := checkGroupName(name); err != nil {
				return nil, err
			}
			if !agent.ValidName(lead) {
				return nil, api.UserError{Code: "bad_lead", Msg: "invalid lead name"}
			}
			if err := gc.ChangeLead(name, lead); err != nil {
				return nil, api.UserError{Code: "lead_failed", Msg: err.Error()}
			}
			return gc.Inspect(name)
		},
	}
}

func groupMemberRm() registry.Command {
	return registry.Command{
		Path: "group.member.rm", Summary: "Remove an agent from a group without deleting it",
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "agent", Type: registry.String, Required: true}},
		HTTP: &registry.HTTPRoute{Method: "DELETE", Path: "/api/groups/{name}/members/{agent}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			gc, err := requireGroups(c)
			if err != nil {
				return nil, err
			}
			name, agentName := str(p, "name"), str(p, "agent")
			if err := checkGroupName(name); err != nil {
				return nil, err
			}
			view, err := gc.Inspect(name)
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: err.Error()}
			}
			member := false
			if members, ok := view["members"].([]string); ok {
				for _, candidate := range members {
					if candidate == agentName {
						member = true
						break
					}
				}
			}
			if !member {
				return nil, api.UserError{Code: "not_member", Msg: agentName + " is not a member of " + name}
			}
			if err := gc.Assign(agentName, ""); err != nil {
				return nil, api.UserError{Code: "remove_member_failed", Msg: err.Error()}
			}
			return map[string]any{"agent": agentName, "group": ""}, nil
		},
	}
}
