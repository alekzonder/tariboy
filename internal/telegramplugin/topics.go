package telegramplugin

import (
	"context"
	"fmt"
)

func (s *Server) ReconcileTopics(ctx context.Context) error {
	if s.bot == nil || s.daemon == nil {
		return nil
	}
	state := s.state.Snapshot()
	if state.Token == "" || state.ChatID == 0 || state.ManagementTopicID == 0 {
		return nil
	}
	agents, err := s.daemon.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		topic, ok := state.AgentTopics[agent.Name]
		if !ok {
			created, err := s.bot.CreateForumTopic(ctx, state.Token, state.ChatID, agent.Name)
			if err != nil {
				return fmt.Errorf("create topic for %s: %w", agent.Name, err)
			}
			topic = AgentTopic{Name: agent.Name, ThreadID: created.MessageThreadID}
			if err := s.state.SetAgentTopic(agent.Name, agent.Name, topic.ThreadID); err != nil {
				return err
			}
			state.AgentTopics[agent.Name] = topic
		}
		if err := s.daemon.Subscribe(ctx, agent.Name, "chat:telegram:"+agent.Name); err != nil {
			return err
		}
	}
	return nil
}
