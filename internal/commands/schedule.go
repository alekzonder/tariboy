package commands

import (
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/schedule"
)

func scheduleLs() registry.Command {
	return registry.WithResult[AgentScheduleList](registry.Command{
		Path:    "schedule.ls",
		Summary: "List an agent's schedules (read-only)",
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/schedules"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			list, err := schedule.NewStore(c.Store, nil).List(p.String("name"))
			if err != nil {
				return nil, err
			}
			rows := make([]AgentSchedule, 0, len(list))
			for _, s := range list {
				rows = append(rows, agentScheduleFrom(s))
			}
			return AgentScheduleList{Schedules: rows, Count: len(rows)}, nil
		},
	})
}
